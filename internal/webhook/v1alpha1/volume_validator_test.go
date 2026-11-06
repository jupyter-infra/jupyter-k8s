/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	testVolTeamName     = "team"
	testVolTeamClaim    = "team-pvc"
	testVolTeamMount    = "/home/jovyan/shared"
	testVolScratchName  = "scratch"
	testVolScratchClaim = "scratch-pvc"
	testVolScratchMount = "/home/jovyan/scratch"
	testVolLockedTpl    = "locked-volumes"
	testVolUserVolume   = "mine"
	testVolUserClaim    = "mine-pvc"
	testVolUserMount    = "/home/jovyan/mine"
	testVolSubPath      = "namespaces/ml"
	testVolPrimaryMount = "/home/jovyan"
)

func lockedVolumePolicy() *workspacev1alpha1.VolumeOverridePolicy {
	return &workspacev1alpha1.VolumeOverridePolicy{Allow: boolPtr(false)}
}

func teamVolume() workspacev1alpha1.VolumeSpec {
	return workspacev1alpha1.VolumeSpec{Name: testVolTeamName, MountPath: testVolTeamMount, PersistentVolumeClaimName: testVolTeamClaim, ReadOnly: true}
}

func scratchVolume() workspacev1alpha1.VolumeSpec {
	return workspacev1alpha1.VolumeSpec{Name: testVolScratchName, MountPath: testVolScratchMount, PersistentVolumeClaimName: testVolScratchClaim, SubPath: testVolSubPath}
}

func userVolume() workspacev1alpha1.VolumeSpec {
	return workspacev1alpha1.VolumeSpec{Name: testVolUserVolume, MountPath: testVolUserMount, PersistentVolumeClaimName: testVolUserClaim}
}

func volumeTemplate(name, namespace string, policy *workspacev1alpha1.VolumeOverridePolicy, volumes ...workspacev1alpha1.VolumeSpec) *workspacev1alpha1.WorkspaceTemplate {
	return &workspacev1alpha1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DefaultVolumes: volumes, VolumeOverrides: policy},
	}
}

func volumeWorkspace(templateName string, volumes ...workspacev1alpha1.VolumeSpec) *workspacev1alpha1.Workspace {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testDefaultNamespace},
		Spec:       workspacev1alpha1.WorkspaceSpec{Volumes: volumes},
	}
	if templateName != "" {
		ws.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: templateName}
	}
	return ws
}

var _ = Describe("VolumeValidator", func() {
	Context("validateLockedVolumes", func() {
		var template *workspacev1alpha1.WorkspaceTemplate

		BeforeEach(func() {
			template = volumeTemplate(testVolLockedTpl, testDefaultNamespace, lockedVolumePolicy(), teamVolume(), scratchVolume())
		})

		It("accepts a workspace that carries the default volumes alongside its own", func() {
			ws := volumeWorkspace(testVolLockedTpl, userVolume(), teamVolume(), scratchVolume())
			Expect(validateLockedVolumes(ws, template)).To(BeEmpty())
		})

		It("rejects a workspace that declares its own volumes without the default volumes", func() {
			violations := validateLockedVolumes(volumeWorkspace(testVolLockedTpl, userVolume()), template)
			Expect(violations).To(HaveLen(2))
			Expect(violations[0].Field).To(Equal("spec.volumes[team]"))
			Expect(violations[0].Message).To(ContainSubstring("required by template"))
			Expect(violations[1].Field).To(Equal("spec.volumes[scratch]"))
		})

		It("rejects a workspace that drops one default volume", func() {
			violations := validateLockedVolumes(volumeWorkspace(testVolLockedTpl, teamVolume()), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeVolumeOverrideNotAllowed))
			Expect(violations[0].Field).To(Equal("spec.volumes[scratch]"))
		})

		DescribeTable("rejects a workspace that changes a default volume",
			func(mutate func(*workspacev1alpha1.VolumeSpec)) {
				changed := scratchVolume()
				mutate(&changed)
				violations := validateLockedVolumes(volumeWorkspace(testVolLockedTpl, teamVolume(), changed), template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Field).To(Equal("spec.volumes[scratch]"))
				Expect(violations[0].Message).To(ContainSubstring("pinned to its definition"))
			},
			Entry("mount path", func(v *workspacev1alpha1.VolumeSpec) { v.MountPath = "/elsewhere" }),
			Entry("claim", func(v *workspacev1alpha1.VolumeSpec) { v.PersistentVolumeClaimName = "other-pvc" }),
			Entry("subPath removed", func(v *workspacev1alpha1.VolumeSpec) { v.SubPath = "" }),
			Entry("readOnly flipped", func(v *workspacev1alpha1.VolumeSpec) { v.ReadOnly = true }),
		)

		It("accepts anything when the template does not lock its volumes", func() {
			for _, policy := range []*workspacev1alpha1.VolumeOverridePolicy{nil, {}, {Allow: boolPtr(true)}} {
				template.Spec.VolumeOverrides = policy
				Expect(validateLockedVolumes(volumeWorkspace(testVolLockedTpl, userVolume()), template)).To(BeEmpty())
			}
		})

		It("compares emptyDir size limits semantically", func() {
			oneGi := resource.MustParse("1Gi")
			sameSize := resource.MustParse("1024Mi")
			template.Spec.DefaultVolumes = []workspacev1alpha1.VolumeSpec{{Name: "cache", MountPath: "/cache", EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &oneGi}}}
			ws := volumeWorkspace(testVolLockedTpl, workspacev1alpha1.VolumeSpec{Name: "cache", MountPath: "/cache", EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &sameSize}})
			Expect(validateLockedVolumes(ws, template)).To(BeEmpty())
		})
	})

	Context("validateSecondaryStorages", func() {
		var template *workspacev1alpha1.WorkspaceTemplate

		BeforeEach(func() {
			template = volumeTemplate(testVolLockedTpl, testDefaultNamespace, nil, teamVolume())
			template.Spec.AllowSecondaryStorages = boolPtr(false)
		})

		It("accepts the template's own default volumes", func() {
			Expect(validateSecondaryStorages([]workspacev1alpha1.VolumeSpec{teamVolume()}, template)).To(BeNil())
		})

		It("counts only the volumes the workspace adds", func() {
			violation := validateSecondaryStorages([]workspacev1alpha1.VolumeSpec{teamVolume(), userVolume()}, template)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Actual).To(Equal("1 volume(s)"))
			Expect(violation.Message).To(ContainSubstring(testVolUserVolume))
		})

		It("counts a modified default volume as added", func() {
			modified := teamVolume()
			modified.ReadOnly = false
			Expect(validateSecondaryStorages([]workspacev1alpha1.VolumeSpec{modified}, template)).NotTo(BeNil())
		})

		It("accepts any volume when secondary storages are allowed", func() {
			template.Spec.AllowSecondaryStorages = nil
			Expect(validateSecondaryStorages([]workspacev1alpha1.VolumeSpec{userVolume()}, template)).To(BeNil())
		})
	})

	Context("validateUniqueVolumes", func() {
		It("accepts distinct mount paths", func() {
			ws := volumeWorkspace("", teamVolume(), userVolume())
			ws.Spec.Storage = &workspacev1alpha1.StorageSpec{MountPath: testVolPrimaryMount}
			Expect(validateUniqueVolumes(ws)).To(BeNil())
		})

		It("rejects two workspace volumes at one path", func() {
			other := userVolume()
			other.MountPath = testVolTeamMount + "/"
			violation := validateUniqueVolumes(volumeWorkspace("", teamVolume(), other))
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeDuplicateMountPath))
			Expect(violation.Message).To(Equal("mount path " + testVolTeamMount + " is used by both volume 'team' and volume 'mine'"))
		})

		It("rejects two volumes with one name", func() {
			other := userVolume()
			other.Name = testVolTeamName
			violation := validateUniqueVolumes(volumeWorkspace("", teamVolume(), other))
			Expect(violation).NotTo(BeNil())
			Expect(violation.Type).To(Equal(ViolationTypeDuplicateVolumeName))
			Expect(violation.Message).To(Equal("volume name 'team' is used more than once"))
		})

		It("rejects a volume at the primary storage mount path", func() {
			ws := volumeWorkspace("", workspacev1alpha1.VolumeSpec{Name: testVolUserVolume, MountPath: testVolPrimaryMount, PersistentVolumeClaimName: testVolUserClaim})
			ws.Spec.Storage = &workspacev1alpha1.StorageSpec{MountPath: testVolPrimaryMount}
			violation := validateUniqueVolumes(ws)
			Expect(violation).NotTo(BeNil())
			Expect(violation.Message).To(ContainSubstring("primary storage and volume 'mine'"))
		})
	})
})
