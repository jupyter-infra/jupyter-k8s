/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

var _ = Describe("MetadataDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "production-template"},
			Spec:       workspacev1alpha1.WorkspaceTemplateSpec{},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applyMetadataDefaults", func() {
		It("should add template label when labels is nil", func() {
			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).NotTo(BeNil())
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})

		It("should add template label to existing labels", func() {
			workspace.Labels = map[string]string{
				"existing": "label",
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue("existing", "label"))
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})

		It("should override existing template label", func() {
			workspace.Labels = map[string]string{
				controller.LabelWorkspaceTemplate: "old-template",
			}

			applyMetadataDefaults(workspace, template)

			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "production-template"))
		})
	})
})
