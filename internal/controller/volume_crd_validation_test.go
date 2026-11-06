/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	volumeValidationNameData  = "data"
	volumeValidationMountData = "/data"
	volumeValidationPVCData   = "data-pvc"
)

var _ = Describe("Volume CRD validation", func() {
	It("should reject volumes with both PVC and emptyDir sources", func() {
		workspace := workspaceWithVolume("both-sources", workspacev1alpha1.VolumeSpec{
			Name:                      volumeValidationNameData,
			MountPath:                 volumeValidationMountData,
			PersistentVolumeClaimName: volumeValidationPVCData,
			EmptyDir:                  &corev1.EmptyDirVolumeSource{},
		})

		err := k8sClient.Create(ctx, workspace)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of persistentVolumeClaimName or emptyDir must be specified"))
	})

	It("should reject volumes with neither PVC nor emptyDir source", func() {
		workspace := workspaceWithVolume("no-source", workspacev1alpha1.VolumeSpec{
			Name:      volumeValidationNameData,
			MountPath: volumeValidationMountData,
		})

		err := k8sClient.Create(ctx, workspace)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of persistentVolumeClaimName or emptyDir must be specified"))
	})

	It("should reject volumes with an empty PVC name", func() {
		workspace := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "workspace.jupyter.org/v1alpha1",
				"kind":       "Workspace",
				"metadata": map[string]interface{}{
					fieldName:   "empty-pvc-name",
					"namespace": testNamespace,
				},
				"spec": map[string]interface{}{
					"displayName": testWorkspaceDisplayName,
					"volumes": []interface{}{
						map[string]interface{}{
							fieldName:                   volumeValidationNameData,
							"mountPath":                 volumeValidationMountData,
							"persistentVolumeClaimName": "",
						},
					},
				},
			},
		}

		err := k8sClient.Create(ctx, workspace)

		Expect(err).To(HaveOccurred())
		Expect(strings.ToLower(err.Error())).To(ContainSubstring("persistentvolumeclaimname"))
	})

	DescribeTable("should accept valid subPaths",
		func(subPath string) {
			workspace := workspaceWithVolume("valid-subpath", workspacev1alpha1.VolumeSpec{
				Name:                      volumeValidationNameData,
				MountPath:                 volumeValidationMountData,
				PersistentVolumeClaimName: volumeValidationPVCData,
				SubPath:                   subPath,
				ReadOnly:                  true,
			})
			Expect(k8sClient.Create(ctx, workspace, client.DryRunAll)).To(Succeed())
		},
		Entry("one segment", "ml"),
		Entry("nested", "namespaces/ml"),
		Entry("dot segment", "./ml"),
		Entry("hidden directory", ".cache/ml"),
		Entry("name starting with two dots", "..ml"),
	)

	DescribeTable("should reject invalid subPaths",
		func(subPath string) {
			workspace := workspaceWithVolume("invalid-subpath", workspacev1alpha1.VolumeSpec{
				Name:                      volumeValidationNameData,
				MountPath:                 volumeValidationMountData,
				PersistentVolumeClaimName: volumeValidationPVCData,
				SubPath:                   subPath,
			})
			err := k8sClient.Create(ctx, workspace, client.DryRunAll)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subPath"))
		},
		Entry("absolute", "/ml"),
		Entry("parent segment", ".."),
		Entry("parent segment inside", "namespaces/../ml"),
		Entry("trailing parent segment", "namespaces/.."),
		Entry("empty segment", "namespaces//ml"),
		Entry("trailing slash", "namespaces/"),
	)

	It("should reject subPath on an emptyDir volume", func() {
		workspace := workspaceWithVolume("emptydir-subpath", workspacev1alpha1.VolumeSpec{
			Name:      volumeValidationNameData,
			MountPath: volumeValidationMountData,
			EmptyDir:  &corev1.EmptyDirVolumeSource{},
			SubPath:   "ml",
		})

		err := k8sClient.Create(ctx, workspace, client.DryRunAll)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("subPath applies to persistentVolumeClaimName volumes only, not to emptyDir"))
	})
})

func workspaceWithVolume(name string, volume workspacev1alpha1.VolumeSpec) *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: testWorkspaceDisplayName,
			Volumes:     []workspacev1alpha1.VolumeSpec{volume},
		},
	}
}
