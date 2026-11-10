/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// ErrNamespaceGID reports a namespace whose posix-gid label is missing, invalid or shared.
var ErrNamespaceGID = errors.New("invalid namespace gid")

// NamespaceGID returns the gid of a namespace, from its workspace.jupyter.org/posix-gid label. The
// deployer supplies the label and guarantees it never changes; the operator only checks that it is a
// valid group id outside the user range and that no other namespace carries the same value.
//
// The reader is the manager cache, which holds only labelled namespaces: an unlabelled namespace reads
// as missing.
func NamespaceGID(ctx context.Context, reader client.Reader, namespace string) (int64, error) {
	// A read from the manager cache, not the API server: cheap on every admission, and the only
	// Namespace read in the operator, so the cache's scoping to posix-gid-labelled namespaces
	// (ManagerCacheOptions) cannot hide a namespace from another reader.
	ns := &corev1.Namespace{}
	if err := reader.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return 0, fmt.Errorf("%w: namespace %q has no %s label", ErrNamespaceGID, namespace, workspaceutil.LabelPosixGID)
		}
		return 0, fmt.Errorf("failed to get namespace %q: %w", namespace, err)
	}
	value, ok := ns.Labels[workspaceutil.LabelPosixGID]
	if !ok {
		return 0, fmt.Errorf("%w: namespace %q has no %s label", ErrNamespaceGID, namespace, workspaceutil.LabelPosixGID)
	}
	gid, err := strconv.ParseInt(value, 10, 64)
	if err != nil || !ValidGroupID(gid) {
		return 0, fmt.Errorf("%w: namespace %q label %s=%q must be an id in [%d, %d], not %d, outside the operator range [%d, %d]",
			ErrNamespaceGID, namespace, workspaceutil.LabelPosixGID, value, MinID, MaxID, NobodyID, UserRangeMin, GroupRangeMax)
	}

	// Also from the cache, through the posix-gid field index: only the namespaces sharing this gid.
	list := &corev1.NamespaceList{}
	if err := reader.List(ctx, list, client.MatchingFields{IndexNamespaceGID: value}); err != nil {
		return 0, fmt.Errorf("failed to list namespaces with gid %s: %w", value, err)
	}
	if len(list.Items) > 1 {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		sort.Strings(names)
		return 0, fmt.Errorf("%w: namespaces %s share %s=%s", ErrNamespaceGID, strings.Join(names, ", "), workspaceutil.LabelPosixGID, value)
	}
	return gid, nil
}
