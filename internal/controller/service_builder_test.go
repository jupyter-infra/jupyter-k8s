/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("ServiceBuilder", func() {
	var (
		ctx            context.Context
		serviceBuilder *ServiceBuilder
		scheme         *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())

		serviceBuilder = NewServiceBuilder(scheme)
	})

	Context("Service Updates", func() {
		var (
			workspace       *workspacev1alpha1.Workspace
			existingService *corev1.Service
		)

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image: imageBaseNotebook,
				},
			}

			// Create existing service
			var err error
			existingService, err = serviceBuilder.BuildService(workspace, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not detect update when nothing changed", func() {
			needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsUpdate).To(BeFalse())
		})

		It("should update service spec correctly", func() {
			err := serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			// Verify the service spec is still correct
			Expect(existingService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(existingService.Spec.Ports).To(HaveLen(1))
			Expect(existingService.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
		})

		// A Service read back from the API server carries fields the operator never sets.
		// Comparing or overwriting whole specs would report a spurious diff on every
		// reconcile and then clear immutable fields like ClusterIP on update.
		Context("when the existing service has been defaulted by the API server", func() {
			BeforeEach(func() {
				applyAPIServerDefaults(existingService)
			})

			It("should not detect an update", func() {
				needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(needsUpdate).To(BeFalse())
			})

			It("should preserve server-populated fields when updating", func() {
				err := serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(existingService.Spec.ClusterIP).To(Equal(testClusterIP))
				Expect(existingService.Spec.ClusterIPs).To(Equal([]string{testClusterIP}))
				Expect(existingService.Spec.SessionAffinity).To(Equal(corev1.ServiceAffinityNone))
				Expect(existingService.Spec.IPFamilies).To(Equal([]corev1.IPFamily{corev1.IPv4Protocol}))
				Expect(existingService.Spec.IPFamilyPolicy).NotTo(BeNil())
				Expect(existingService.Spec.InternalTrafficPolicy).NotTo(BeNil())

				// Operator-owned fields are still reconciled.
				Expect(existingService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(existingService.Spec.Ports).To(HaveLen(1))
				Expect(existingService.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
			})

			It("should still detect drift in an operator-owned field", func() {
				existingService.Spec.Ports[0].Port = 9999

				needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(needsUpdate).To(BeTrue())

				Expect(serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace, nil)).To(Succeed())
				Expect(existingService.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
				Expect(existingService.Spec.ClusterIP).To(Equal(testClusterIP))
			})
		})
	})
})

var _ = Describe("ServiceBuilder sidecar ports", func() {
	var (
		ctx            context.Context
		serviceBuilder *ServiceBuilder
		workspace      *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		serviceBuilder = NewServiceBuilder(scheme)

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testWorkspaceName,
				Namespace: testNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: imageBaseNotebook,
			},
		}
	})

	// accessStrategy builds an access strategy that injects the given sidecar containers and
	// opts the named ports into the workspace Service via exposedPorts.
	accessStrategy := func(exposedPorts []string, containers ...corev1.Container) *workspacev1alpha1.WorkspaceAccessStrategy {
		return &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{Name: "websocket-access-strategy", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						AdditionalContainers: containers,
						ExposedPorts:         exposedPorts,
					},
				},
			},
		}
	}

	wsProxyContainer := func() corev1.Container {
		return corev1.Container{
			Name:  nameWSProxy,
			Image: "ghcr.io/jupyter-infra/workspace-websocket-proxy:v0.1.0-rc.1",
			Ports: []corev1.ContainerPort{{Name: nameWSProxy, ContainerPort: 8080}},
		}
	}

	metricsContainer := func() corev1.Container {
		return corev1.Container{
			Name:  nameMetrics,
			Ports: []corev1.ContainerPort{{Name: nameMetrics, ContainerPort: 9090}},
		}
	}

	Context("when nothing is exposed", func() {
		It("exposes only the application port with a nil access strategy", func() {
			service, err := serviceBuilder.BuildService(workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Name).To(Equal(httpScheme))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
		})

		It("exposes only the application port when the access strategy has no deployment modifications", func() {
			service, err := serviceBuilder.BuildService(workspace, &workspacev1alpha1.WorkspaceAccessStrategy{})
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports).To(HaveLen(1))
		})

		// A declared port is not exposed unless it is opted in: exposure is explicit, so a
		// sidecar port left off exposedPorts stays internal to the pod.
		It("exposes only the application port when a sidecar declares a port but exposes none", func() {
			service, err := serviceBuilder.BuildService(workspace, accessStrategy(nil, wsProxyContainer()))
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
		})

		// The SSM sidecar is this shape: it dials outbound, declares no ports, exposes none.
		It("exposes only the application port when a sidecar declares no ports", func() {
			service, err := serviceBuilder.BuildService(workspace, accessStrategy(nil, corev1.Container{
				Name:  "ssm-agent-sidecar",
				Image: "example.com/ssm-agent:latest",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
		})
	})

	Context("when ports are exposed", func() {
		It("exposes a named sidecar port alongside the application port", func() {
			service, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{nameWSProxy}, wsProxyContainer()))
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(2))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))

			wsPort := service.Spec.Ports[1]
			Expect(wsPort.Name).To(Equal(nameWSProxy))
			Expect(wsPort.Port).To(Equal(int32(8080)))
			Expect(wsPort.TargetPort).To(Equal(intstr.FromInt32(8080)))
			Expect(wsPort.Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("exposes only the ports listed, not every declared port", func() {
			service, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{nameWSProxy}, wsProxyContainer(), metricsContainer()))
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(2))
			Expect(service.Spec.Ports[1].Name).To(Equal(nameWSProxy))
		})

		It("exposes ports drawn from several sidecars", func() {
			service, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{nameWSProxy, nameMetrics}, wsProxyContainer(), metricsContainer()))
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(3))
			Expect(service.Spec.Ports[1].Name).To(Equal(nameWSProxy))
			Expect(service.Spec.Ports[2].Name).To(Equal(nameMetrics))
			Expect(service.Spec.Ports[2].Port).To(Equal(int32(9090)))
		})

		It("preserves the order of exposedPorts", func() {
			service, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{nameMetrics, nameWSProxy}, wsProxyContainer(), metricsContainer()))
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports[1].Name).To(Equal(nameMetrics))
			Expect(service.Spec.Ports[2].Name).To(Equal(nameWSProxy))
		})

		It("preserves a non-TCP protocol", func() {
			service, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{"dns"}, corev1.Container{
					Name:  "dns-sidecar",
					Ports: []corev1.ContainerPort{{Name: "dns", ContainerPort: 5353, Protocol: corev1.ProtocolUDP}},
				}))
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports[1].Protocol).To(Equal(corev1.ProtocolUDP))
		})
	})

	Context("when exposedPorts is invalid", func() {
		It("errors when a name matches no declared port", func() {
			_, err := serviceBuilder.BuildService(workspace,
				accessStrategy([]string{"nonexistent"}, wsProxyContainer()))
			Expect(err).To(MatchError(ContainSubstring("matches no port")))
		})

		It("errors when a name is declared by more than one container", func() {
			_, err := serviceBuilder.BuildService(workspace, accessStrategy([]string{nameShared},
				corev1.Container{Name: "proxy", Ports: []corev1.ContainerPort{{Name: nameShared, ContainerPort: 8080}}},
				corev1.Container{Name: "other", Ports: []corev1.ContainerPort{{Name: nameShared, ContainerPort: 9090}}},
			))
			Expect(err).To(MatchError(ContainSubstring("more than one")))
		})

		It("errors when two exposed ports target the same number", func() {
			_, err := serviceBuilder.BuildService(workspace, accessStrategy([]string{literalFirst, literalSecond},
				corev1.Container{Name: "a", Ports: []corev1.ContainerPort{{Name: literalFirst, ContainerPort: 8080}}},
				corev1.Container{Name: "b", Ports: []corev1.ContainerPort{{Name: literalSecond, ContainerPort: 8080}}},
			))
			Expect(err).To(MatchError(ContainSubstring("both target port 8080")))
		})

		It("errors when an exposed port targets the application port number", func() {
			_, err := serviceBuilder.BuildService(workspace, accessStrategy([]string{nameShadow},
				corev1.Container{Name: nameShadow, Ports: []corev1.ContainerPort{{Name: nameShadow, ContainerPort: JupyterPort}}},
			))
			Expect(err).To(MatchError(ContainSubstring("reserved for the application")))
		})

		It("errors when an exposed port reuses the application port name", func() {
			_, err := serviceBuilder.BuildService(workspace, accessStrategy([]string{httpScheme},
				corev1.Container{Name: "sidecar", Ports: []corev1.ContainerPort{{Name: httpScheme, ContainerPort: 8080}}},
			))
			Expect(err).To(MatchError(ContainSubstring("reserved application port name")))
		})

		// A name collision among ports nobody exposes is a legal pod config and must not fail.
		It("ignores a name collision among ports that are not exposed", func() {
			service, err := serviceBuilder.BuildService(workspace, accessStrategy([]string{nameWSProxy},
				wsProxyContainer(),
				corev1.Container{Name: "a", Ports: []corev1.ContainerPort{{Name: "dup", ContainerPort: 7000}}},
				corev1.Container{Name: "b", Ports: []corev1.ContainerPort{{Name: "dup", ContainerPort: 7001}}},
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports).To(HaveLen(2))
			Expect(service.Spec.Ports[1].Name).To(Equal(nameWSProxy))
		})
	})

	Context("reconciling an existing service", func() {
		It("detects that a single-port service must gain the exposed port", func() {
			existingService, err := serviceBuilder.BuildService(workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			applyAPIServerDefaults(existingService)

			strategy := accessStrategy([]string{nameWSProxy}, wsProxyContainer())

			needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, strategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsUpdate).To(BeTrue())

			Expect(serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace, strategy)).To(Succeed())
			Expect(existingService.Spec.Ports).To(HaveLen(2))
			Expect(existingService.Spec.Ports[1].Port).To(Equal(int32(8080)))
			// The update must not clobber server-populated immutable fields.
			Expect(existingService.Spec.ClusterIP).To(Equal(testClusterIP))
		})

		It("detects no update once the exposed port is already present", func() {
			strategy := accessStrategy([]string{nameWSProxy}, wsProxyContainer())
			existingService, err := serviceBuilder.BuildService(workspace, strategy)
			Expect(err).NotTo(HaveOccurred())
			applyAPIServerDefaults(existingService)

			needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, strategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsUpdate).To(BeFalse())
		})

		It("removes the exposed port when the access strategy no longer opts into it", func() {
			strategy := accessStrategy([]string{nameWSProxy}, wsProxyContainer())
			existingService, err := serviceBuilder.BuildService(workspace, strategy)
			Expect(err).NotTo(HaveOccurred())
			applyAPIServerDefaults(existingService)

			needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsUpdate).To(BeTrue())

			Expect(serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace, nil)).To(Succeed())
			Expect(existingService.Spec.Ports).To(HaveLen(1))
		})
	})
})

const (
	// testClusterIP is the ClusterIP the fake API server "assigns" in tests.
	testClusterIP = "10.100.42.7"
	// nameWSProxy is the WebSocket proxy sidecar container / port name used across tests.
	nameWSProxy = "ws-proxy"
	// nameMetrics is a second sidecar container / port name used across tests.
	nameMetrics = "metrics"
	// nameShared is a port name two sidecars both declare, making it ambiguous to expose.
	nameShared = "shared"
	// nameShadow is a port name whose container port shadows the application port.
	nameShadow = "shadow"
)

// applyAPIServerDefaults mutates a freshly built Service to look like one read back from
// the API server, which populates these fields on create.
func applyAPIServerDefaults(service *corev1.Service) {
	ipFamilyPolicy := corev1.IPFamilyPolicySingleStack
	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyCluster

	service.Spec.ClusterIP = testClusterIP
	service.Spec.ClusterIPs = []string{testClusterIP}
	service.Spec.SessionAffinity = corev1.ServiceAffinityNone
	service.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
	service.Spec.IPFamilyPolicy = &ipFamilyPolicy
	service.Spec.InternalTrafficPolicy = &internalTrafficPolicy
}
