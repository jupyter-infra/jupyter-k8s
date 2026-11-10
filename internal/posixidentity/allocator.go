/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// ErrConflict reports an identity an admin must resolve: a user with several PosixIdentity objects,
// several uids claimed by their workspaces, or a claimed uid recorded for another user. The operator
// never picks one silently.
var ErrConflict = errors.New("posix identity conflict")

// ErrExhausted reports that every candidate uid of a user is taken.
var ErrExhausted = errors.New("no free uid among the candidates")

// Allocator finds or allocates the PosixIdentity of a username. Uniqueness comes from the object name:
// creating uid-<n> either succeeds, belongs to this user, or belongs to someone else.
type Allocator struct {
	// client reads through the manager cache, with the indexes of SetupIndexes, and writes
	client client.Client
	// apiReader reads the API server directly, where the cache may lag behind a concurrent create
	apiReader client.Reader
}

// NewAllocator returns an allocator reading through a cached client and an uncached reader.
func NewAllocator(c client.Client, apiReader client.Reader) *Allocator {
	return &Allocator{client: c, apiReader: apiReader}
}

// Lookup returns the PosixIdentity recorded for a username, or nil if there is none.
func (a *Allocator) Lookup(ctx context.Context, username string) (*workspacev1alpha1.PosixIdentity, error) {
	list := &workspacev1alpha1.PosixIdentityList{}
	if err := a.client.List(ctx, list, client.MatchingFields{IndexUsernameHash: UsernameHash(username)}); err != nil {
		return nil, fmt.Errorf("failed to list posix identities: %w", err)
	}
	var found []workspacev1alpha1.PosixIdentity
	for _, identity := range list.Items {
		if identity.Spec.Username == username {
			found = append(found, identity)
		}
	}
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		if _, err := UIDOf(&found[0]); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return &found[0], nil
	default:
		return nil, fmt.Errorf("%w: user %q has %d PosixIdentity objects", ErrConflict, username, len(found))
	}
}

// Ensure returns the PosixIdentity of a username, allocating it on first use. In order: an existing
// object; the uid the user's workspaces already claim (recreating its missing object); the first free
// candidate of the latest strategy. A candidate is free when no object records it and no workspace of
// another user claims it.
func (a *Allocator) Ensure(ctx context.Context, username string) (*workspacev1alpha1.PosixIdentity, error) {
	return a.ensure(ctx, username, false)
}

// Peek returns the PosixIdentity Ensure would return without creating anything, for dry-run requests.
func (a *Allocator) Peek(ctx context.Context, username string) (*workspacev1alpha1.PosixIdentity, error) {
	return a.ensure(ctx, username, true)
}

func (a *Allocator) ensure(ctx context.Context, username string, dryRun bool) (*workspacev1alpha1.PosixIdentity, error) {
	if username == "" {
		return nil, errors.New("workspace has no owner")
	}
	if identity, err := a.Lookup(ctx, username); err != nil || identity != nil {
		return identity, err
	}

	claimed, err := a.claimedUID(ctx, username)
	if err != nil {
		return nil, err
	}
	if claimed != 0 {
		identity, taken, err := a.claim(ctx, username, claimed, dryRun)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, fmt.Errorf("%w: uid %d claimed by the workspaces of %q belongs to another user", ErrConflict, claimed, username)
		}
		return identity, nil
	}

	for _, uid := range v1Candidates(username) {
		if claimedByOther, err := a.claimedByOtherUser(ctx, uid, username); err != nil {
			return nil, err
		} else if claimedByOther {
			continue
		}
		identity, taken, err := a.claim(ctx, username, uid, dryRun)
		if err != nil {
			return nil, err
		}
		if !taken {
			return identity, nil
		}
	}
	return nil, fmt.Errorf("%w for user %q", ErrExhausted, username)
}

// claimedUID returns the uid the user's workspaces claim through their posix-uid label, 0 if none.
func (a *Allocator) claimedUID(ctx context.Context, username string) (int64, error) {
	list := &workspacev1alpha1.WorkspaceList{}
	if err := a.client.List(ctx, list, client.MatchingFields{IndexWorkspaceClaimOwner: UsernameHash(username)}); err != nil {
		return 0, fmt.Errorf("failed to list workspaces claiming a uid: %w", err)
	}
	var claimed int64
	for _, ws := range list.Items {
		if ws.Annotations[workspaceutil.AnnotationCreatedBy] != username {
			continue
		}
		uid, err := strconv.ParseInt(ws.Labels[workspaceutil.LabelPosixUID], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: workspace %s/%s has an invalid %s label", ErrConflict, ws.Namespace, ws.Name, workspaceutil.LabelPosixUID)
		}
		if claimed != 0 && uid != claimed {
			return 0, fmt.Errorf("%w: the workspaces of %q claim uids %d and %d", ErrConflict, username, claimed, uid)
		}
		claimed = uid
	}
	return claimed, nil
}

// claimedByOtherUser reports whether a workspace of another user claims a uid; an object recording the
// uid is found by name when claiming it.
func (a *Allocator) claimedByOtherUser(ctx context.Context, uid int64, username string) (bool, error) {
	list := &workspacev1alpha1.WorkspaceList{}
	if err := a.client.List(ctx, list, client.MatchingFields{IndexWorkspaceClaimedUID: strconv.FormatInt(uid, 10)}); err != nil {
		return false, fmt.Errorf("failed to list workspaces claiming uid %d: %w", uid, err)
	}
	for _, ws := range list.Items {
		if ws.Annotations[workspaceutil.AnnotationCreatedBy] != username {
			return true, nil
		}
	}
	return false, nil
}

// claim records uid for username. It reports taken when the uid belongs to another user.
func (a *Allocator) claim(
	ctx context.Context, username string, uid int64, dryRun bool,
) (*workspacev1alpha1.PosixIdentity, bool, error) {
	identity := newIdentity(username, uid)
	if dryRun {
		existing := &workspacev1alpha1.PosixIdentity{}
		err := a.apiReader.Get(ctx, client.ObjectKey{Name: identity.Name}, existing)
		switch {
		case apierrors.IsNotFound(err):
			return identity, false, nil
		case err != nil:
			return nil, false, fmt.Errorf("failed to get PosixIdentity %s: %w", identity.Name, err)
		default:
			return existing, existing.Spec.Username != username, nil
		}
	}

	err := a.client.Create(ctx, identity)
	if err == nil {
		return identity, false, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return nil, false, fmt.Errorf("failed to create PosixIdentity %s: %w", identity.Name, err)
	}
	// The cache may not hold an object a concurrent request just created: read the API server.
	existing := &workspacev1alpha1.PosixIdentity{}
	if err := a.apiReader.Get(ctx, client.ObjectKey{Name: identity.Name}, existing); err != nil {
		return nil, false, fmt.Errorf("failed to get PosixIdentity %s: %w", identity.Name, err)
	}
	return existing, existing.Spec.Username != username, nil
}

func newIdentity(username string, uid int64) *workspacev1alpha1.PosixIdentity {
	return &workspacev1alpha1.PosixIdentity{
		ObjectMeta: metav1.ObjectMeta{Name: ObjectName(uid), Finalizers: []string{FinalizerName}},
		Spec: workspacev1alpha1.PosixIdentitySpec{
			Username:     username,
			UsernameHash: UsernameHash(username),
			UnixName:     UnixName(username),
		},
	}
}

// UserInputs are the per-user template inputs of access-strategy env and integration templates:
// {{ .User.UnixName }} and {{ .User.UsernameHash }}.
type UserInputs struct {
	UnixName     string
	UsernameHash string
}

// UserInputsFor returns the template inputs of an owner: the unix name recorded on their PosixIdentity, or
// derived from the username when they have none (or it cannot be read). It never allocates.
func (a *Allocator) UserInputsFor(ctx context.Context, owner string) UserInputs {
	inputs := UserInputs{UnixName: UnixName(owner), UsernameHash: UsernameHash(owner)}
	if identity, err := a.Lookup(ctx, owner); err == nil && identity != nil {
		inputs.UnixName = identity.Spec.UnixName
	}
	return inputs
}
