/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// transportPodExecValue is the non-default detection transport used to represent a divergence
// from the template default in idle shutdown override tests.
const transportPodExecValue = "podExec"

var _ = Describe("Idle Shutdown Validator", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }

	// defaultIdleShutdown returns a fresh idle shutdown config matching the template default.
	defaultIdleShutdown := func() *workspacev1alpha1.IdleShutdownSpec {
		return &workspacev1alpha1.IdleShutdownSpec{
			Enabled:              true,
			IdleTimeoutInMinutes: 30,
			Detection: workspacev1alpha1.IdleDetectionSpec{
				HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{
					Transport: "network",
				},
			},
		}
	}

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testTemplateName,
				Namespace: testDefaultNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultIdleShutdown: defaultIdleShutdown(),
				IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{
					Allow:                   boolPtr(false),
					MinIdleTimeoutInMinutes: intPtr(15),
					MaxIdleTimeoutInMinutes: intPtr(60),
				},
			},
		}
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ws", Namespace: testDefaultNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IdleShutdown: defaultIdleShutdown(),
			},
		}
	})

	Context("when the policy is absent", func() {
		It("should not enforce anything when the policy is nil", func() {
			template.Spec.IdleShutdownOverrides = nil
			workspace.Spec.IdleShutdown.Enabled = false
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 999
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})
	})

	Context("when overrides are permitted (Allow true or unset)", func() {
		BeforeEach(func() {
			template.Spec.IdleShutdownOverrides.Allow = boolPtr(true)
		})

		It("should not enforce structure: disabling and diverging is allowed", func() {
			workspace.Spec.IdleShutdown.Enabled = false
			workspace.Spec.IdleShutdown.Detection.HTTPGet.Transport = transportPodExecValue
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 999
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should treat unset Allow the same as true", func() {
			template.Spec.IdleShutdownOverrides.Allow = nil
			workspace.Spec.IdleShutdown.Enabled = false
			workspace.Spec.IdleShutdown.Detection.HTTPGet.Transport = transportPodExecValue
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should still enforce declared timeout bounds when enabled", func() {
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 45
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 5
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 120
			violations = validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))
		})

		It("should skip timeout bounds when idle shutdown is disabled", func() {
			workspace.Spec.IdleShutdown.Enabled = false
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 999
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should leave a side unbounded when its bound is unset", func() {
			template.Spec.IdleShutdownOverrides.MinIdleTimeoutInMinutes = nil
			// No lower bound: a tiny timeout is fine; the upper bound still applies.
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 1
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 120
			Expect(validateIdleShutdownOverrides(workspace, template)).To(HaveLen(1))
		})

		It("should enforce nothing on the timeout when no bounds are declared", func() {
			template.Spec.IdleShutdownOverrides.MinIdleTimeoutInMinutes = nil
			template.Spec.IdleShutdownOverrides.MaxIdleTimeoutInMinutes = nil
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 99999
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})
	})

	Context("when overrides are not allowed but there is no template default", func() {
		It("should skip the structural lock but still enforce timeout bounds", func() {
			template.Spec.DefaultIdleShutdown = nil
			// Structure can't be checked without a baseline, but declared bounds still apply.
			workspace.Spec.IdleShutdown.Enabled = false
			workspace.Spec.IdleShutdown.Detection.HTTPGet.Transport = transportPodExecValue
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 45
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())

			workspace.Spec.IdleShutdown.Enabled = true
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 120
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))
		})
	})

	Context("when overrides are not allowed", func() {
		It("should allow a workspace matching the default with an in-bounds timeout", func() {
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 45
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should allow a timeout equal to the default when it also equals bounds edges", func() {
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 15
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 60
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should reject a workspace with no idle shutdown config", func() {
			workspace.Spec.IdleShutdown = nil
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownOverrideNotAllowed))
			Expect(violations[0].Field).To(Equal("spec.idleShutdown"))
		})

		It("should reject disabling idle shutdown", func() {
			workspace.Spec.IdleShutdown.Enabled = false
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownOverrideNotAllowed))
		})

		It("should reject changing a non-timeout field like detection transport", func() {
			workspace.Spec.IdleShutdown.Detection.HTTPGet.Transport = transportPodExecValue
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownOverrideNotAllowed))
		})

		It("should reject a timeout below the minimum", func() {
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 5
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))
			Expect(violations[0].Field).To(Equal("spec.idleShutdown.idleTimeoutInMinutes"))
		})

		It("should reject a timeout above the maximum", func() {
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 120
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))
		})

		It("should report both an override and an out-of-bounds violation", func() {
			// Enabled (so bounds apply) but a non-timeout field diverges and the timeout is
			// out of range: both the structural lock and the bounds check fire.
			workspace.Spec.IdleShutdown.Detection.HTTPGet.Transport = transportPodExecValue
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 120
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(2))
		})
	})

	Context("bound fallbacks when min or max is unset", func() {
		It("should use the default timeout as the min when min is unset", func() {
			template.Spec.IdleShutdownOverrides.MinIdleTimeoutInMinutes = nil
			// default timeout is 30, so anything below 30 must be rejected
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 20
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 45
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should use the default timeout as the max when max is unset", func() {
			template.Spec.IdleShutdownOverrides.MaxIdleTimeoutInMinutes = nil
			// default timeout is 30, so anything above 30 must be rejected
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 45
			violations := validateIdleShutdownOverrides(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeIdleShutdownTimeoutOutOfBounds))

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 20
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())
		})

		It("should pin the timeout to the default when both bounds are unset", func() {
			template.Spec.IdleShutdownOverrides.MinIdleTimeoutInMinutes = nil
			template.Spec.IdleShutdownOverrides.MaxIdleTimeoutInMinutes = nil
			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 30
			Expect(validateIdleShutdownOverrides(workspace, template)).To(BeEmpty())

			workspace.Spec.IdleShutdown.IdleTimeoutInMinutes = 31
			Expect(validateIdleShutdownOverrides(workspace, template)).To(HaveLen(1))
		})
	})
})
