/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"fmt"

	v1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createBearerTokenReview calls create:BearerTokenReview API to verify a bearer token.
// This delegates token verification to the extensionapi server, which holds the signing key.
// The authmiddleware treats the bearer token as opaque — it does not need the signing key.
func (s *Server) createBearerTokenReview(
	ctx context.Context,
	token string,
	namespace string,
) (*v1alpha1.BearerTokenReviewStatus, error) {
	if s.restClient == nil {
		return nil, fmt.Errorf("kubernetes REST client not initialized")
	}

	reviewRequest := &v1alpha1.BearerTokenReview{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha1.BearerTokenReviewSpec{
			Token: token,
		},
	}

	url := fmt.Sprintf("/apis/%s/namespaces/%s/bearertokenreviews",
		v1alpha1.SchemeGroupVersion.String(), namespace)

	var result v1alpha1.BearerTokenReview
	err := s.restClient.Post().
		AbsPath(url).
		Body(reviewRequest).
		Do(ctx).
		Into(&result)

	if err != nil {
		s.logger.Error("create BearerTokenReview failed",
			"namespace", namespace,
			"error", err.Error())
		return nil, fmt.Errorf("failed to create BearerTokenReview: %w", err)
	}

	s.logger.Info("BearerTokenReview completed",
		"namespace", namespace,
		"authenticated", result.Status.Authenticated,
		"user", result.Status.User.Username)

	return &result.Status, nil
}
