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
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// ServiceAccountValidator handles service account validation for webhooks
type ServiceAccountValidator struct {
	k8sClient client.Client
}

// NewServiceAccountValidator creates a new ServiceAccountValidator
func NewServiceAccountValidator(k8sClient client.Client) *ServiceAccountValidator {
	return &ServiceAccountValidator{
		k8sClient: k8sClient,
	}
}

// hasServiceAccountAccess checks if the user has access based on ServiceAccount annotations
func (sav *ServiceAccountValidator) hasServiceAccountAccess(userInfo authenticationv1.UserInfo, sa *corev1.ServiceAccount) bool {
	if sa.Annotations == nil {
		return false
	}

	sessionNameRegex := regexp.MustCompile(`^[\w+=,.@-]{2,64}$`)

	matchesTemplate := func(identity, template string) bool {
		if !strings.Contains(template, "{{SessionName}}") {
			return identity == template
		}
		rolePrefix := strings.Replace(template, "{{SessionName}}", "", 1)
		if !strings.HasPrefix(identity, rolePrefix) {
			return false
		}
		sessionName := identity[len(rolePrefix):]
		return sessionNameRegex.MatchString(sessionName)
	}

	// Check service-account-users annotation
	if usersYaml, exists := sa.Annotations[controller.AnnotationServiceAccountUsers]; exists {
		var users []string
		if err := yaml.Unmarshal([]byte(usersYaml), &users); err == nil {
			for _, user := range users {
				if matchesTemplate(userInfo.Username, user) {
					return true
				}
			}
		}
	}

	// Check service-account-groups annotation
	if groupsYaml, exists := sa.Annotations[controller.AnnotationServiceAccountGroups]; exists {
		var groups []string
		if err := yaml.Unmarshal([]byte(groupsYaml), &groups); err == nil {
			for _, group := range groups {
				for _, userGroup := range userInfo.Groups {
					if group == userGroup {
						return true
					}
				}
			}
		}
	}

	return false
}

// ValidateServiceAccountAccess checks if the user has access to the workspace's service account
func (sav *ServiceAccountValidator) ValidateServiceAccountAccess(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.ServiceAccountName == "" {
		return nil
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to extract user information: %w", err)
	}

	sa := &corev1.ServiceAccount{}
	if err := sav.k8sClient.Get(ctx, types.NamespacedName{Name: workspace.Spec.ServiceAccountName, Namespace: workspace.GetNamespace()}, sa); err != nil {
		return fmt.Errorf("failed to get service account %s: %w", workspace.Spec.ServiceAccountName, err)
	}

	if !sav.hasServiceAccountAccess(req.UserInfo, sa) {
		return fmt.Errorf("access denied: user does not have access to service account %s", workspace.Spec.ServiceAccountName)
	}

	return nil
}
