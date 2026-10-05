/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// testUserShmVolume is the name a workspace gives its own /dev/shm volume in these tests.
const testUserShmVolume = "shm"

// The /dev/shm volume contract (#484, #486): the primary container of a workspace that asks for it,
// with sharedMemory set by itself or by its template, gets a memory-backed emptyDir named workspace-shm
// at /dev/shm sized to its memory limit, unless the workspace disables it or something mounts another
// volume there. A workspace that does not ask keeps the container default.
var _ = Describe("DeploymentBuilder shared memory", func() {
	var (
		ctx     context.Context
		builder *DeploymentBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		builder = NewDeploymentBuilder(scheme, WorkspaceControllerOptions{
			ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			ApplicationImagesRegistry:   testImageRegistry,
		})
	})

	memoryLimited := func() *corev1.ResourceRequirements {
		return &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
		}
	}

	newWorkspace := func(resources *corev1.ResourceRequirements) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "shm-workspace", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Storage:      &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")},
				Resources:    resources,
				SharedMemory: &workspacev1alpha1.SharedMemorySpec{},
			},
		}
	}

	build := func(workspace *workspacev1alpha1.Workspace) *appsv1.Deployment {
		deployment, err := builder.BuildDeployment(ctx, workspace)
		Expect(err).NotTo(HaveOccurred())
		return deployment
	}

	findVolume := func(deployment *appsv1.Deployment, name string) *corev1.Volume {
		for i := range deployment.Spec.Template.Spec.Volumes {
			if deployment.Spec.Template.Spec.Volumes[i].Name == name {
				return &deployment.Spec.Template.Spec.Volumes[i]
			}
		}
		return nil
	}

	findMount := func(container corev1.Container, path string) *corev1.VolumeMount {
		for i := range container.VolumeMounts {
			if container.VolumeMounts[i].MountPath == path {
				return &container.VolumeMounts[i]
			}
		}
		return nil
	}

	expectShm := func(deployment *appsv1.Deployment, size string) {
		GinkgoHelper()
		volume := findVolume(deployment, volumeNameWorkspaceSharedMemory)
		Expect(volume).NotTo(BeNil())
		Expect(volume.EmptyDir).NotTo(BeNil())
		Expect(volume.EmptyDir.Medium).To(Equal(corev1.StorageMediumMemory))
		Expect(volume.EmptyDir.SizeLimit).NotTo(BeNil())
		Expect(volume.EmptyDir.SizeLimit.Cmp(resource.MustParse(size))).To(BeZero())
		mount := findMount(deployment.Spec.Template.Spec.Containers[0], SharedMemoryMountPath)
		Expect(mount).NotTo(BeNil())
		Expect(mount.Name).To(Equal(volumeNameWorkspaceSharedMemory))
	}

	expectNoShm := func(deployment *appsv1.Deployment) {
		GinkgoHelper()
		Expect(findVolume(deployment, volumeNameWorkspaceSharedMemory)).To(BeNil())
		for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
			Expect(mount.Name).NotTo(Equal(volumeNameWorkspaceSharedMemory))
		}
	}

	It("mounts a memory-backed volume at /dev/shm sized to the memory limit", func() {
		deployment := build(newWorkspace(memoryLimited()))
		expectShm(deployment, "2Gi")
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(2))
	})

	It("sizes the volume to the memory request when there is no limit", func() {
		workspace := newWorkspace(&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
		})
		expectShm(build(workspace), "512Mi")
	})

	It("mounts no volume when the workspace declares no memory", func() {
		workspace := newWorkspace(&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
		})
		expectNoShm(build(workspace))
	})

	It("sizes the volume to the default memory when the workspace sets no resources", func() {
		expectShm(build(newWorkspace(nil)), DefaultMemoryRequest)
	})

	It("mounts no volume when the workspace does not ask for one", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.SharedMemory = nil
		expectNoShm(build(workspace))
	})

	It("omits the volume when the workspace disables it", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.SharedMemory = &workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}
		expectNoShm(build(workspace))
	})

	It("treats an empty sharedMemory as a request for the volume", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.SharedMemory = &workspacev1alpha1.SharedMemorySpec{}
		expectShm(build(workspace), "2Gi")
	})

	It("keeps a volume the workspace declares at /dev/shm", func() {
		workspace := newWorkspace(memoryLimited())
		userSize := resource.MustParse("1Gi")
		workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{{
			Name:      testUserShmVolume,
			MountPath: SharedMemoryMountPath,
			EmptyDir:  &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory, SizeLimit: &userSize},
		}}
		deployment := build(workspace)
		expectNoShm(deployment)
		userVolume := findVolume(deployment, testUserShmVolume)
		Expect(userVolume).NotTo(BeNil())
		Expect(userVolume.EmptyDir.SizeLimit).NotTo(BeNil())
		Expect(userVolume.EmptyDir.SizeLimit.Cmp(userSize)).To(BeZero())
		Expect(findMount(deployment.Spec.Template.Spec.Containers[0], SharedMemoryMountPath).Name).To(Equal(testUserShmVolume))
	})

	It("treats a user volume at /dev/shm/ as a volume at /dev/shm", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{{
			Name:      testUserShmVolume,
			MountPath: SharedMemoryMountPath + "/",
			EmptyDir:  &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
		}}
		expectNoShm(build(workspace))
	})

	It("drops its volume when an access strategy mounts /dev/shm into the primary container", func() {
		workspace := newWorkspace(memoryLimited())
		accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{Name: "shm-strategy", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						Volumes: []corev1.Volume{{
							Name:         "strategy-shm",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}},
						}},
						PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
							VolumeMounts: []corev1.VolumeMount{{Name: "strategy-shm", MountPath: SharedMemoryMountPath}},
						},
					},
				},
			},
		}
		deployment, err := builder.BuildWorkspaceDeployment(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		expectNoShm(deployment)
		mounts := 0
		for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
			if mount.MountPath == SharedMemoryMountPath {
				mounts++
				Expect(mount.Name).To(Equal("strategy-shm"))
			}
		}
		Expect(mounts).To(Equal(1))
	})

	It("yields to an integration that mounts its own /dev/shm into the workspace and its sidecar", func() {
		// The Ray integration in jupyter-k8s-aws shares one memory-backed volume between the Ray
		// sidecar and the workspace container this way.
		const integrationShm = "ray-dshm"
		integrationSize := resource.MustParse("1Gi")
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.IntegrationTemplateRefs = []workspacev1alpha1.IntegrationTemplateRef{{Name: rayIntegrationName}}
		workspace.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{{
			Name:                               rayIntegrationName,
			ParametersHash:                     getIntegrationParametersHash(&workspace.Spec.IntegrationTemplateRefs[0]),
			ObservedIntegrationTemplateVersion: testIntegrationTemplateVersion,
			Values:                             map[string]string{},
		}}
		integration := &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: rayIntegrationName, Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						Volumes: []corev1.Volume{{
							Name: integrationShm,
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
								Medium: corev1.StorageMediumMemory, SizeLimit: &integrationSize,
							}},
						}},
						AdditionalContainers: []corev1.Container{{
							Name:         raySidecarName,
							Image:        "ray:latest",
							VolumeMounts: []corev1.VolumeMount{{Name: integrationShm, MountPath: SharedMemoryMountPath}},
						}},
						PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
							VolumeMounts: []corev1.VolumeMount{{Name: integrationShm, MountPath: SharedMemoryMountPath}},
						},
					},
				},
			},
		}

		deployment, err := builder.BuildWorkspaceDeployment(ctx, workspace, nil, integrationTemplatesFor(integration))
		Expect(err).NotTo(HaveOccurred())

		expectNoShm(deployment)
		Expect(findVolume(deployment, integrationShm)).NotTo(BeNil())
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
		for _, container := range deployment.Spec.Template.Spec.Containers {
			mounts := 0
			for _, mount := range container.VolumeMounts {
				if mount.MountPath == SharedMemoryMountPath {
					mounts++
					Expect(mount.Name).To(Equal(integrationShm), container.Name)
				}
			}
			Expect(mounts).To(Equal(1), container.Name)
		}
	})

	It("skips a user volume that takes the reserved name", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{{
			Name:                      volumeNameWorkspaceSharedMemory,
			MountPath:                 testScratchMountPath,
			PersistentVolumeClaimName: testScratchPVC,
		}}
		deployment := build(workspace)
		expectShm(deployment, "2Gi")
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
		Expect(findMount(deployment.Spec.Template.Spec.Containers[0], testScratchMountPath)).To(BeNil())
	})

	It("does not let a reserved-name user volume at /dev/shm suppress the operator's volume", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{{
			Name:      volumeNameWorkspaceSharedMemory,
			MountPath: SharedMemoryMountPath,
			EmptyDir:  &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
		}}
		deployment := build(workspace)
		expectShm(deployment, "2Gi")
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
	})

	It("mounts alongside a user PVC volume", func() {
		workspace := newWorkspace(memoryLimited())
		workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{{
			Name:                      volumeValidationNameData,
			MountPath:                 volumeValidationMountData,
			PersistentVolumeClaimName: volumeValidationPVCData,
		}}
		deployment := build(workspace)
		expectShm(deployment, "2Gi")
		Expect(findVolume(deployment, volumeValidationNameData)).NotTo(BeNil())
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(3))
	})

	It("mounts the volume in the primary container only and sizes it to that container", func() {
		workspace := newWorkspace(memoryLimited())
		accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{Name: "sidecar-strategy", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						AdditionalContainers: []corev1.Container{{
							Name:  "sidecar",
							Image: "sidecar:latest",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
							},
						}},
					},
				},
			},
		}
		deployment, err := builder.BuildWorkspaceDeployment(ctx, workspace, accessStrategy, nil)
		Expect(err).NotTo(HaveOccurred())
		expectShm(deployment, "2Gi")
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
		Expect(findMount(deployment.Spec.Template.Spec.Containers[1], SharedMemoryMountPath)).To(BeNil())
	})

	It("reports an existing Deployment without the volume as needing an update", func() {
		workspace := newWorkspace(memoryLimited())
		desired, err := builder.BuildWorkspaceDeployment(ctx, workspace, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		needsUpdate, err := builder.NeedsUpdate(ctx, desired.DeepCopy(), workspace, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(needsUpdate).To(BeFalse())

		existing := desired.DeepCopy()
		podSpec := &existing.Spec.Template.Spec
		var volumes []corev1.Volume
		for _, volume := range podSpec.Volumes {
			if volume.Name != volumeNameWorkspaceSharedMemory {
				volumes = append(volumes, volume)
			}
		}
		podSpec.Volumes = volumes
		var mounts []corev1.VolumeMount
		for _, mount := range podSpec.Containers[0].VolumeMounts {
			if mount.Name != volumeNameWorkspaceSharedMemory {
				mounts = append(mounts, mount)
			}
		}
		podSpec.Containers[0].VolumeMounts = mounts

		needsUpdate, err = builder.NeedsUpdate(ctx, existing, workspace, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(needsUpdate).To(BeTrue())
	})
})
