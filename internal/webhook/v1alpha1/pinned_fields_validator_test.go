/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

const (
	testPinPrivateTemplate = "jupyterlab"
	testPinSharedTemplate  = "jupyterlab-shared"
	testPinOwner           = "alice"
)

func pinnedTemplate(name, sharingType string) *workspacev1alpha1.WorkspaceTemplate {
	return &workspacev1alpha1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testDefaultNamespace},
		Spec: workspacev1alpha1.WorkspaceTemplateSpec{
			DefaultImage:             testValidBaseNotebook,
			DefaultAccessType:        sharingType,
			DefaultOwnershipType:     sharingType,
			AllowCustomAccessType:    boolPtr(false),
			AllowCustomOwnershipType: boolPtr(false),
		},
	}
}

func lockedSecurityContextPolicy() *workspacev1alpha1.SecurityContextOverridePolicy {
	return &workspacev1alpha1.SecurityContextOverridePolicy{Allow: boolPtr(false)}
}

var _ = Describe("PinnedFieldsValidator", func() {
	newWorkspace := func(accessType, ownershipType string) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
			Spec:       workspacev1alpha1.WorkspaceSpec{AccessType: accessType, OwnershipType: ownershipType},
		}
	}

	Context("validatePinnedSharingTypes", func() {
		It("accepts any value when the template does not pin", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.AllowCustomAccessType = nil
			template.Spec.AllowCustomOwnershipType = boolPtr(true)
			ws := newWorkspace(testOwnershipPublic, testOwnershipPublic)
			Expect(validatePinnedSharingTypes(ws, template)).To(BeEmpty())
		})

		It("accepts the pinned value", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			ws := newWorkspace(testOwnershipOwnerOnly, testOwnershipOwnerOnly)
			Expect(validatePinnedSharingTypes(ws, template)).To(BeEmpty())
		})

		It("treats empty as Public on both sides", func() {
			template := pinnedTemplate(testPinSharedTemplate, "")
			Expect(validatePinnedSharingTypes(newWorkspace(testOwnershipPublic, ""), template)).To(BeEmpty())
		})

		It("rejects another value, naming the field and the pin", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			violations := validatePinnedSharingTypes(newWorkspace(testOwnershipPublic, testOwnershipPublic), template)
			Expect(violations).To(HaveLen(2))
			Expect(violations[0].Type).To(Equal(ViolationTypeAccessTypeNotAllowed))
			Expect(violations[0].Field).To(Equal("spec.accessType"))
			Expect(violations[0].Message).To(Equal(`spec.accessType: pinned to OwnerOnly by template "jupyterlab"`))
			Expect(violations[1].Type).To(Equal(ViolationTypeOwnershipTypeNotAllowed))
			Expect(violations[1].Field).To(Equal("spec.ownershipType"))
		})
	})

	Context("validatePinnedSecurityContexts", func() {
		privileged := func() *corev1.SecurityContext {
			return &corev1.SecurityContext{Privileged: boolPtr(true)}
		}
		nonRoot := func() *corev1.PodSecurityContext {
			return &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true)}
		}

		It("accepts any security context when overrides are allowed or unset", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.ContainerSecurityContextOverrides = &workspacev1alpha1.SecurityContextOverridePolicy{Allow: boolPtr(true)}
			ws := newWorkspace("", "")
			ws.Spec.ContainerSecurityContext = privileged()
			ws.Spec.PodSecurityContext = nonRoot()
			Expect(validatePinnedSecurityContexts(ws, template)).To(BeEmpty())
		})

		It("accepts the template's default when locked", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.PodSecurityContextOverrides = lockedSecurityContextPolicy()
			template.Spec.DefaultPodSecurityContext = nonRoot()
			ws := newWorkspace("", "")
			ws.Spec.PodSecurityContext = nonRoot()
			Expect(validatePinnedSecurityContexts(ws, template)).To(BeEmpty())
		})

		It("accepts no security context when locked without a default", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.ContainerSecurityContextOverrides = lockedSecurityContextPolicy()
			Expect(validatePinnedSecurityContexts(newWorkspace("", ""), template)).To(BeEmpty())
		})

		It("rejects a custom container security context when locked", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.ContainerSecurityContextOverrides = lockedSecurityContextPolicy()
			ws := newWorkspace("", "")
			ws.Spec.ContainerSecurityContext = privileged()
			violations := validatePinnedSecurityContexts(ws, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSecurityContextOverrideNotAllowed))
			Expect(violations[0].Field).To(Equal("spec.containerSecurityContext"))
			Expect(violations[0].Message).To(ContainSubstring(`by template "jupyterlab"`))
		})

		It("rejects a pod security context that differs from the locked default", func() {
			template := pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
			template.Spec.PodSecurityContextOverrides = lockedSecurityContextPolicy()
			template.Spec.DefaultPodSecurityContext = nonRoot()
			ws := newWorkspace("", "")
			ws.Spec.PodSecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(false)}
			violations := validatePinnedSecurityContexts(ws, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Field).To(Equal("spec.podSecurityContext"))
		})
	})
})

var _ = Describe("Pinned fields through the workspace webhook", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		defaulter  WorkspaceCustomDefaulter
		validator  WorkspaceCustomValidator
		privateTpl *workspacev1alpha1.WorkspaceTemplate
		sharedTpl  *workspacev1alpha1.WorkspaceTemplate
	)

	updateContext := func() context.Context {
		return createUserContext(ctx, "UPDATE", testPinOwner)
	}

	// createWorkspace runs the workspace through the defaulter on CREATE, as the API server would.
	createWorkspace := func(templateName string) *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testWorkspaceDisplayName},
		}
		if templateName != "" {
			ws.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: templateName}
		}
		Expect(defaulter.Default(createUserContext(ctx, "CREATE", testPinOwner), ws)).To(Succeed())
		return ws
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		privateTpl = pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
		privateTpl.Spec.ContainerSecurityContextOverrides = lockedSecurityContextPolicy()
		privateTpl.Spec.DefaultContainerSecurityContext = &corev1.SecurityContext{AllowPrivilegeEscalation: boolPtr(false)}
		sharedTpl = pinnedTemplate(testPinSharedTemplate, testOwnershipPublic)
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(privateTpl, sharedTpl,
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testDefaultNamespace}},
		).Build()
		defaulter = WorkspaceCustomDefaulter{
			templateDefaulter:       NewTemplateDefaulter(k8sClient, ""),
			serviceAccountDefaulter: NewServiceAccountDefaulter(k8sClient),
			templateGetter:          NewTemplateGetter(k8sClient, ""),
			templateValidator:       NewTemplateValidator(k8sClient, ""),
			accessStrategyValidator: NewAccessStrategyValidator(""),
			client:                  k8sClient,
		}
		validator = WorkspaceCustomValidator{
			templateValidator:       NewTemplateValidator(k8sClient, ""),
			accessStrategyValidator: NewAccessStrategyValidator(""),
			serviceAccountValidator: NewServiceAccountValidator(k8sClient),
			volumeValidator:         NewVolumeValidator(k8sClient),
			storageValidator:        NewStorageValidator(k8sClient),
		}
	})

	It("rejects a create that sets a pinned field to another value", func() {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: testPinPrivateTemplate},
				AccessType:  testOwnershipPublic,
			},
		}
		userCtx := createUserContext(ctx, "CREATE", testPinOwner)
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err := validator.ValidateCreate(userCtx, ws)
		Expect(err).To(MatchError(ContainSubstring(`spec.accessType: pinned to OwnerOnly by template "jupyterlab"`)))
	})

	It("rejects a create with a privileged container under a locked template", func() {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				TemplateRef:              &workspacev1alpha1.TemplateRef{Name: testPinPrivateTemplate},
				ContainerSecurityContext: &corev1.SecurityContext{Privileged: boolPtr(true)},
			},
		}
		userCtx := createUserContext(ctx, "CREATE", testPinOwner)
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err := validator.ValidateCreate(userCtx, ws)
		Expect(err).To(MatchError(ContainSubstring("spec.containerSecurityContext: pinned")))
	})

	It("applies the template defaults to a create that sets nothing", func() {
		ws := createWorkspace(testPinPrivateTemplate)
		Expect(ws.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
		Expect(ws.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		Expect(ws.Spec.ContainerSecurityContext).To(Equal(privateTpl.Spec.DefaultContainerSecurityContext))
		_, err := validator.ValidateCreate(createUserContext(ctx, "CREATE", testPinOwner), ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects an update that sets a pinned field to another value", func() {
		oldWorkspace := createWorkspace(testPinPrivateTemplate)
		ws := oldWorkspace.DeepCopy()
		ws.Spec.AccessType = testOwnershipPublic
		userCtx := updateContext()
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		Expect(ws.Spec.AccessType).To(Equal(testOwnershipPublic))
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).To(MatchError(ContainSubstring(`spec.accessType: pinned to OwnerOnly`)))
	})

	It("keeps the workspace's values on a templateRef switch, and rejects the ones the new template pins differently", func() {
		oldWorkspace := createWorkspace(testPinPrivateTemplate)
		ws := oldWorkspace.DeepCopy()
		ws.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: testPinSharedTemplate}
		userCtx := updateContext()
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		Expect(ws.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
		Expect(ws.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).To(MatchError(And(
			ContainSubstring(`spec.accessType: pinned to Public by template "jupyterlab-shared"`),
			ContainSubstring(`spec.ownershipType: pinned to Public by template "jupyterlab-shared"`),
		)))
	})

	It("accepts a templateRef switch that sets the new template's pinned values", func() {
		oldWorkspace := createWorkspace(testPinPrivateTemplate)
		ws := oldWorkspace.DeepCopy()
		ws.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: testPinSharedTemplate}
		ws.Spec.AccessType = testOwnershipPublic
		ws.Spec.OwnershipType = testOwnershipPublic
		userCtx := updateContext()
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		// The shared template does not lock the container security context, so the workspace's own stays
		Expect(ws.Spec.ContainerSecurityContext).To(Equal(privateTpl.Spec.DefaultContainerSecurityContext))
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a templateRef switch that keeps a security context the new template locks out", func() {
		oldWorkspace := createWorkspace(testPinSharedTemplate)
		oldWorkspace.Spec.ContainerSecurityContext = &corev1.SecurityContext{RunAsUser: int64Ptr(0)}
		ws := oldWorkspace.DeepCopy()
		ws.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: testPinPrivateTemplate}
		ws.Spec.AccessType = testOwnershipOwnerOnly
		ws.Spec.OwnershipType = testOwnershipOwnerOnly
		userCtx := updateContext()
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).To(MatchError(ContainSubstring("spec.containerSecurityContext: pinned")))
	})

	// With a default template in the namespace, removing templateRef is a switch to that template: the
	// defaulter assigns it to any workspace without a templateRef, and the workspace keeps its own access
	// and ownership types. The shared template is the default here, so its Public pins conflict with the
	// OwnerOnly values the workspace got from the private template.
	It("holds a workspace that removes its templateRef to the default template's pins", func() {
		sharedTpl.Labels = map[string]string{webhookconst.DefaultTemplateLabel: labelValueTrue}
		Expect(k8sClient.Update(ctx, sharedTpl)).To(Succeed())
		oldWorkspace := createWorkspace(testPinPrivateTemplate)
		userCtx := updateContext()

		By("removing templateRef alone: the defaulter assigns the shared default template, whose pins it breaks")
		ws := oldWorkspace.DeepCopy()
		ws.Spec.TemplateRef = nil
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		Expect(ws.Spec.TemplateRef).NotTo(BeNil())
		Expect(ws.Spec.TemplateRef.Name).To(Equal(testPinSharedTemplate))
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).To(MatchError(ContainSubstring(`spec.accessType: pinned to Public by template "jupyterlab-shared"`)))

		By("removing templateRef and setting the default template's pinned values")
		ws = oldWorkspace.DeepCopy()
		ws.Spec.TemplateRef = nil
		ws.Spec.AccessType = testOwnershipPublic
		ws.Spec.OwnershipType = testOwnershipPublic
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err = validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).NotTo(HaveOccurred())
	})

	It("lets a non-compliant existing workspace stop without complying", func() {
		oldWorkspace := createWorkspace(testPinSharedTemplate)
		oldWorkspace.Spec.AccessType = testOwnershipOwnerOnly
		ws := oldWorkspace.DeepCopy()
		ws.Spec.DesiredStatus = controller.DesiredStateStopped
		userCtx := updateContext()
		Expect(defaulter.Default(userCtx, ws)).To(Succeed())
		_, err := validator.ValidateUpdate(userCtx, oldWorkspace, ws)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("WorkspaceTemplate Validator pin warnings", func() {
	var (
		ctx       context.Context
		validator WorkspaceTemplateCustomValidator
		oldTpl    *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = WorkspaceTemplateCustomValidator{accessStrategyValidator: NewAccessStrategyValidator("")}
		oldTpl = pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly)
		oldTpl.Spec.AllowCustomAccessType = nil
		oldTpl.Spec.AllowCustomOwnershipType = nil
	})

	It("flags that a pin change may break workspaces on their next update", func() {
		warnings, err := validator.ValidateUpdate(ctx, oldTpl, pinnedTemplate(testPinPrivateTemplate, testOwnershipOwnerOnly))
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(ContainElement(pinsChangedWarning))
	})

	It("does not flag pins when no pin changes", func() {
		newTpl := oldTpl.DeepCopy()
		newTpl.Spec.AllowedImages = []string{testValidBaseNotebook}
		warnings, err := validator.ValidateUpdate(ctx, oldTpl, newTpl)
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).NotTo(ContainElement(pinsChangedWarning))
	})
})
