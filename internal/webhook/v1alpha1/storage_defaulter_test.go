package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("StorageDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		storageClassName := "fast-ssd"
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				PrimaryStorage: &workspacev1alpha1.StorageConfig{
					DefaultSize:             resource.MustParse("5Gi"),
					DefaultStorageClassName: &storageClassName,
					DefaultMountPath:        "/workspace",
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applyStorageDefaults", func() {
		It("should create storage spec and apply all defaults", func() {
			applyStorageDefaults(workspace, template)

			Expect(workspace.Spec.Storage).NotTo(BeNil())
			Expect(workspace.Spec.Storage.Size).To(Equal(resource.MustParse("5Gi")))
			Expect(*workspace.Spec.Storage.StorageClassName).To(Equal("fast-ssd"))
			Expect(workspace.Spec.Storage.MountPath).To(Equal("/workspace"))
		})

		It("should apply defaults to existing storage spec", func() {
			workspace.Spec.Storage = &workspacev1alpha1.StorageSpec{}

			applyStorageDefaults(workspace, template)

			Expect(workspace.Spec.Storage.Size).To(Equal(resource.MustParse("5Gi")))
			Expect(*workspace.Spec.Storage.StorageClassName).To(Equal("fast-ssd"))
			Expect(workspace.Spec.Storage.MountPath).To(Equal("/workspace"))
		})

		It("should not override existing storage values", func() {
			existingClassName := "existing-storage"
			workspace.Spec.Storage = &workspacev1alpha1.StorageSpec{
				Size:             resource.MustParse("10Gi"),
				StorageClassName: &existingClassName,
				MountPath:        "/existing",
			}

			applyStorageDefaults(workspace, template)

			Expect(workspace.Spec.Storage.Size).To(Equal(resource.MustParse("10Gi")))
			Expect(*workspace.Spec.Storage.StorageClassName).To(Equal("existing-storage"))
			Expect(workspace.Spec.Storage.MountPath).To(Equal("/existing"))
		})

		It("should do nothing when template has no primary storage", func() {
			template.Spec.PrimaryStorage = nil

			applyStorageDefaults(workspace, template)

			Expect(workspace.Spec.Storage).To(BeNil())
		})

		It("should not create storage when no default size", func() {
			template.Spec.PrimaryStorage.DefaultSize = resource.Quantity{}

			applyStorageDefaults(workspace, template)

			Expect(workspace.Spec.Storage).To(BeNil())
		})
	})
})
