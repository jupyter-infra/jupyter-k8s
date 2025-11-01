package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("AccessResourcesBuilder", func() {
	var (
		accessBuilder      *AccessResourcesBuilder
		testAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		testWorkspace      *workspacev1alpha1.Workspace
		testService        *corev1.Service
	)

	// Setup test objects
	BeforeEach(func() {
		accessBuilder = NewAccessResourcesBuilder()

		// Define test objects based on config/samples_routing
		testAccessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-access-strategy",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DisplayName: "JupyterLab Routing Strategy",
				AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{
					{
						Kind:       "IngressRoute",
						ApiVersion: "traefik.io/v1alpha1",
						NamePrefix: "test-route",
						Template:   "spec:\n  entryPoints:\n    - websecure\n  routes:\n    - match: \"Host(`example.com`) && PathPrefix(`/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}`)\"\n      kind: Rule\n      services:\n        - name: \"{{ .Service.Name }}\"\n          namespace: \"{{ .Service.Namespace }}\"\n          port: 8888",
					},
				},
				AccessURLTemplate: "https://example.com/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/",
			},
		}

		testWorkspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName:    "Test Workspace",
				Image:          "jupyter/minimal-notebook:latest",
				DesiredStatus:  "Running",
				AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "sample-access-strategy"},
			},
		}

		// Create test service for each test
		testService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "test-namespace",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Port:     8888,
						Protocol: "TCP",
					},
				},
			},
		}
	})

	Context("BuildUnstructuredResource", func() {
		It("Should set the name of resource based on the prefix", func() {
			template := testAccessStrategy.Spec.AccessResourceTemplates[0]

			resource, err := accessBuilder.BuildUnstructuredResource(
				template,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resource).NotTo(BeNil())
			Expect(resource.GetName()).To(Equal(fmt.Sprintf("%s-%s", "test-route", testWorkspace.Name)))
		})

		It("Should use the namespace of the Workspace", func() {
			// Create a copy of the access strategy without a namespace specified
			strategyWithoutNamespace := testAccessStrategy.DeepCopy()

			template := strategyWithoutNamespace.Spec.AccessResourceTemplates[0]

			resource, err := accessBuilder.BuildUnstructuredResource(
				template,
				testWorkspace,
				strategyWithoutNamespace,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resource).NotTo(BeNil())
			Expect(resource.GetNamespace()).To(Equal(testWorkspace.Namespace))
		})

		It("Should set the Group-Version-Kind", func() {
			// Test with a normal API group/version format
			template := testAccessStrategy.Spec.AccessResourceTemplates[0]

			resource, err := accessBuilder.BuildUnstructuredResource(
				template,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resource).NotTo(BeNil())

			// Check the GVK is set correctly (traefik.io/v1alpha1)
			gvk := resource.GroupVersionKind()
			Expect(gvk.Group).To(Equal("traefik.io"))
			Expect(gvk.Version).To(Equal("v1alpha1"))
			Expect(gvk.Kind).To(Equal("IngressRoute"))

			// Test with a core API version (no group)
			coreApiTemplate := workspacev1alpha1.AccessResourceTemplate{
				Kind:       "ConfigMap",
				ApiVersion: "v1", // Core API version without group
				NamePrefix: "test-config",
				Template:   "data:\n  key: value",
			}

			coreResource, err := accessBuilder.BuildUnstructuredResource(
				coreApiTemplate,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(coreResource).NotTo(BeNil())

			// Check the GVK is set correctly for core API (empty group, v1 version)
			coreGvk := coreResource.GroupVersionKind()
			Expect(coreGvk.Group).To(Equal(""))
			Expect(coreGvk.Version).To(Equal("v1"))
			Expect(coreGvk.Kind).To(Equal("ConfigMap"))
		})

		It("Should substitute data from Workspace, AccessStrategy and Service", func() {
			template := testAccessStrategy.Spec.AccessResourceTemplates[0]

			resource, err := accessBuilder.BuildUnstructuredResource(
				template,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resource).NotTo(BeNil())

			// Check that the template substitutions were made
			routes, found, err := unstructured.NestedSlice(resource.Object, "spec", "routes")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(routes).NotTo(BeEmpty())

			firstRoute, ok := routes[0].(map[string]interface{})
			Expect(ok).To(BeTrue())

			services, found, err := unstructured.NestedSlice(firstRoute, "services")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(services).NotTo(BeEmpty())

			firstService, ok := services[0].(map[string]interface{})
			Expect(ok).To(BeTrue())

			// Check substituted values
			Expect(firstService["name"]).To(Equal(testService.Name))
			Expect(firstService["namespace"]).To(Equal(testService.Namespace))

			// Check the match field contains the workspace name and namespace
			match, found, err := unstructured.NestedString(firstRoute, "match")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(match).To(ContainSubstring(testWorkspace.Namespace))
			Expect(match).To(ContainSubstring(testWorkspace.Name))
		})

		It("Should add all the labels", func() {
			template := testAccessStrategy.Spec.AccessResourceTemplates[0]

			resource, err := accessBuilder.BuildUnstructuredResource(
				template,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resource).NotTo(BeNil())

			labels := resource.GetLabels()
			Expect(labels).NotTo(BeNil())

			// Check that all expected labels are present
			Expect(labels[LabelWorkspaceName]).To(Equal(testWorkspace.Name))
			Expect(labels[LabelWorkspaceNamespace]).To(Equal(testWorkspace.Namespace))
			Expect(labels[LabelAccessStrategyName]).To(Equal(testAccessStrategy.Name))
			Expect(labels[LabelAccessStrategyNamespace]).To(Equal(testAccessStrategy.Namespace))
		})

		It("Should return en error if the resource template is not parsable", func() {
			invalidTemplate := testAccessStrategy.Spec.AccessResourceTemplates[0]
			invalidTemplate.Template = "{{ .InvalidSyntax }"

			_, err := accessBuilder.BuildUnstructuredResource(
				invalidTemplate,
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse resource template"))
		})

		It("Should return en error if substitutions cannot be made", func() {
			// FIX this test to evaluate the case where a {{ .Service.UnknownAttribute }} is referenced
			// in the template
		})
	})

	Context("ResolveAccessURL", func() {
		It("Should return the empty string if the strategy does not define an accessUrl", func() {
			// Create a copy of the access strategy without an URL template
			strategyWithoutURL := testAccessStrategy.DeepCopy()
			strategyWithoutURL.Spec.AccessURLTemplate = ""

			url, err := accessBuilder.ResolveAccessURL(
				testWorkspace,
				strategyWithoutURL,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal(""))
		})

		It("Should return resolved URL with data from Workspace, AccessStrategy and Service", func() {
			url, err := accessBuilder.ResolveAccessURL(
				testWorkspace,
				testAccessStrategy,
				testService,
			)

			Expect(err).NotTo(HaveOccurred())
			expectedURL := "https://example.com/workspaces/test-namespace/test-workspace/"
			Expect(url).To(Equal(expectedURL))
		})

		It("Should return en error if the accessUrl is not parsable", func() {
			// Create a copy of the access strategy with an invalid URL template
			strategyWithInvalidURL := testAccessStrategy.DeepCopy()
			strategyWithInvalidURL.Spec.AccessURLTemplate = "{{ .InvalidSyntax }"

			_, err := accessBuilder.ResolveAccessURL(
				testWorkspace,
				strategyWithInvalidURL,
				testService,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse AccessURLTemplate"))
		})

		It("Should return en error if substitutions cannot be made", func() {
			// Create a copy of the access strategy with a template referencing non-existent fields
			strategyWithBadTemplate := testAccessStrategy.DeepCopy()
			strategyWithBadTemplate.Spec.AccessURLTemplate = "https://{{ .NonExistentField }}"

			_, err := accessBuilder.ResolveAccessURL(
				testWorkspace,
				strategyWithBadTemplate,
				testService,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute AccessURLTemplate"))
		})
	})

	Context("ResolveAccessResourceSelector", func() {
		It("Should return the empty string if the strategy does not define accessResources", func() {
			// Create a copy of the access strategy without access resources
			strategyWithoutResources := testAccessStrategy.DeepCopy()
			strategyWithoutResources.Spec.AccessResourceTemplates = []workspacev1alpha1.AccessResourceTemplate{}

			selector := accessBuilder.ResolveAccessResourceSelector(
				testWorkspace,
				strategyWithoutResources,
			)

			Expect(selector).To(Equal(""))
		})

		It("Should return the correct label selector", func() {
			selector := accessBuilder.ResolveAccessResourceSelector(
				testWorkspace,
				testAccessStrategy,
			)

			expectedSelector := fmt.Sprintf("%s=%s", LabelWorkspaceName, testWorkspace.Name)
			Expect(selector).To(Equal(expectedSelector))
		})
	})
})
