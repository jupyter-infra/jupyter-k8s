/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
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
