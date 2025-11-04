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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

var _ = Describe("ServiceAccount Validator", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("hasServiceAccountAccess", func() {
		var sa *corev1.ServiceAccount

		BeforeEach(func() {
			sa = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "default",
				},
			}
		})

		It("should allow access when username is in service-account-users list", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountUsers: "- user1\n- user2@example.com",
			}
			userInfo := authenticationv1.UserInfo{Username: "user1"}
			sav := NewServiceAccountValidator(nil)
			Expect(sav.hasServiceAccountAccess(userInfo, sa)).To(BeTrue())
		})

		It("should allow access when user group is in service-account-groups list", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountGroups: "- data_scientists\n- admins",
			}
			userInfo := authenticationv1.UserInfo{Groups: []string{"data_scientists", "other"}}
			sav := NewServiceAccountValidator(nil)
			Expect(sav.hasServiceAccountAccess(userInfo, sa)).To(BeTrue())
		})

		It("should deny access when user is not in either list", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountUsers:  "- user1\n- user2",
				controller.AnnotationServiceAccountGroups: "- group1\n- group2",
			}
			userInfo := authenticationv1.UserInfo{Username: "user3", Groups: []string{"group3"}}
			sav := NewServiceAccountValidator(nil)
			Expect(sav.hasServiceAccountAccess(userInfo, sa)).To(BeFalse())
		})
	})

	Context("checkUsernamePatternAccess", func() {
		var sa *corev1.ServiceAccount
		var sav *ServiceAccountValidator

		BeforeEach(func() {
			sa = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "default",
				},
			}
			sav = NewServiceAccountValidator(nil)
		})

		It("should match username with asterisk wildcard", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountUsersPattern: "- arn:aws:iam::123456789012:role/MyRole/*",
			}
			Expect(sav.checkUsernamePatternAccess("arn:aws:iam::123456789012:role/MyRole/user123", sa)).To(BeTrue())
		})

		It("should match username with question mark wildcard", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountUsersPattern: "- user?",
			}
			Expect(sav.checkUsernamePatternAccess("user1", sa)).To(BeTrue())
		})

		It("should not match when pattern does not match", func() {
			sa.Annotations = map[string]string{
				controller.AnnotationServiceAccountUsersPattern: "- arn:aws:iam::123456789012:role/MyRole/*",
			}
			Expect(sav.checkUsernamePatternAccess("arn:aws:iam::123456789012:role/OtherRole/user123", sa)).To(BeFalse())
		})
	})

	Context("validateServiceAccountAccess", func() {
		var mockClient *MockClient
		var workspace *workspacev1alpha1.Workspace

		BeforeEach(func() {
			mockClient = &MockClient{}
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					ServiceAccountName: "test-sa",
				},
			}
		})

		It("should return nil when ServiceAccountName is empty", func() {
			workspace.Spec.ServiceAccountName = ""
			sav := NewServiceAccountValidator(mockClient)
			err := sav.ValidateServiceAccountAccess(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when no request context", func() {
			sav := NewServiceAccountValidator(mockClient)
			err := sav.ValidateServiceAccountAccess(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract user information"))
		})

		It("should return error when service account not found", func() {
			userCtx := createUserContext(ctx, "CREATE", "test-user")
			mockClient.GetError = fmt.Errorf("not found")
			sav := NewServiceAccountValidator(mockClient)
			err := sav.ValidateServiceAccountAccess(userCtx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get service account"))
		})

		It("should pass validation when user has access to service account", func() {
			userCtx := createUserContext(ctx, "CREATE", "allowed-user")
			mockClient.ServiceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "default",
					Annotations: map[string]string{
						controller.AnnotationServiceAccountUsers: "- allowed-user\n- other-user",
					},
				},
			}
			sav := NewServiceAccountValidator(mockClient)
			err := sav.ValidateServiceAccountAccess(userCtx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
