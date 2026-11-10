/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespaceGID(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t,
		labelledNamespace("team-a", "2130706500"),
		labelledNamespace("team-b", "2130706501"),
		labelledNamespace("team-c", "2130706501"),
		labelledNamespace("in-user-range", "1048576"),
		labelledNamespace("not-a-number", "x"),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "unlabelled"}},
	)
	if gid, err := NamespaceGID(ctx, c, "team-a"); err != nil || gid != 2130706500 {
		t.Fatalf("team-a: %d, %v", gid, err)
	}
	for _, ns := range []string{"team-b", "in-user-range", "not-a-number", "unlabelled", "absent"} {
		if _, err := NamespaceGID(ctx, c, ns); !errors.Is(err, ErrNamespaceGID) {
			t.Errorf("%s: want ErrNamespaceGID, got %v", ns, err)
		}
	}
}
