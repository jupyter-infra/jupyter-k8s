/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

func TestEnsureAllocatesThenReuses(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)
	a := NewAllocator(c, c)

	first, err := a.Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, first) != v1Candidates(alice)[0] {
		t.Fatalf("unexpected identity %+v", first)
	}
	if first.Spec.UnixName != "github-alice" || first.Spec.UsernameHash != UsernameHash(alice) || len(first.Finalizers) != 1 {
		t.Fatalf("unexpected identity fields %+v", first)
	}

	again, err := a.Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, again) != mustUID(t, first) {
		t.Fatalf("uid changed: %d then %d", mustUID(t, first), mustUID(t, again))
	}
	list := &workspacev1alpha1.PosixIdentityList{}
	if err := c.List(ctx, list); err != nil || len(list.Items) != 1 {
		t.Fatalf("want one object, got %d (%v)", len(list.Items), err)
	}
}

func TestEnsureProbesPastAnotherUsersUID(t *testing.T) {
	ctx := context.Background()
	taken := newIdentity(bob, v1Candidates(alice)[0])
	c := newFakeClient(t, taken)

	identity, err := NewAllocator(c, c).Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, identity) != v1Candidates(alice)[1] {
		t.Fatalf("want second candidate, got %d", mustUID(t, identity))
	}
}

func TestEnsureReusesObjectMissingFromCache(t *testing.T) {
	// A concurrent request created the user's object; Create hits AlreadyExists and the object is reused.
	ctx := context.Background()
	existing := newIdentity(alice, v1Candidates(alice)[0])
	c := newFakeClient(t, existing)
	// An empty cache: Lookup finds nothing, the API reader holds the object.
	cache := newFakeClient(t)
	a := &Allocator{client: &createThrough{Client: cache, target: c}, apiReader: c}

	identity, err := a.Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, identity) != mustUID(t, existing) {
		t.Fatalf("got %d, want %d", mustUID(t, identity), mustUID(t, existing))
	}
}

// createThrough reads from one client and creates through another.
type createThrough struct {
	client.Client
	target client.Client
}

func (c *createThrough) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.target.Create(ctx, obj, opts...)
}

func TestLookupRejectsAnInvalidName(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"alice", "uid-", "uid-0123", "uid-999", "uid-65534", "uid-x"} {
		misnamed := newIdentity(alice, 2000000)
		misnamed.Name = name
		c := newFakeClient(t, misnamed)
		if _, err := NewAllocator(c, c).Lookup(ctx, alice); !errors.Is(err, ErrConflict) {
			t.Errorf("%s: want ErrConflict, got %v", name, err)
		}
	}
}

func TestEnsureSkipsUIDClaimedByAnotherUsersWorkspace(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t, claimingWorkspace("bob-ws", bob, v1Candidates(alice)[0]))

	identity, err := NewAllocator(c, c).Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, identity) != v1Candidates(alice)[1] {
		t.Fatalf("want second candidate, got %d", mustUID(t, identity))
	}
}

func TestEnsureRecreatesFromWorkspaceClaim(t *testing.T) {
	ctx := context.Background()
	const claimed int64 = 1234567
	c := newFakeClient(t, claimingWorkspace("alice-ws", alice, claimed))

	identity, err := NewAllocator(c, c).Ensure(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, identity) != claimed {
		t.Fatalf("got %d, want the claimed uid %d", mustUID(t, identity), claimed)
	}
}

func TestEnsureConflicts(t *testing.T) {
	ctx := context.Background()
	cases := map[string][]client.Object{
		"two objects for one user": {
			newIdentity(alice, 2000000),
			newIdentity(alice, 2000001),
		},
		"workspaces claim two uids": {
			claimingWorkspace("ws1", alice, 2000000),
			claimingWorkspace("ws2", alice, 2000001),
		},
		"claimed uid recorded for another user": {
			claimingWorkspace("ws1", alice, 2000000),
			newIdentity(bob, 2000000),
		},
	}
	for name, objs := range cases {
		t.Run(name, func(t *testing.T) {
			c := newFakeClient(t, objs...)
			if _, err := NewAllocator(c, c).Ensure(ctx, alice); !errors.Is(err, ErrConflict) {
				t.Fatalf("want ErrConflict, got %v", err)
			}
		})
	}
}

func TestPeekCreatesNothing(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient(t)

	identity, err := NewAllocator(c, c).Peek(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if mustUID(t, identity) != v1Candidates(alice)[0] {
		t.Fatalf("unexpected uid %d", mustUID(t, identity))
	}
	list := &workspacev1alpha1.PosixIdentityList{}
	if err := c.List(ctx, list); err != nil || len(list.Items) != 0 {
		t.Fatalf("peek created %d objects (%v)", len(list.Items), err)
	}
}

func TestUserInputsFor(t *testing.T) {
	ctx := context.Background()
	seeded := newIdentity(alice, 2000000)
	seeded.Spec.UnixName = "adalice"
	c := newFakeClient(t, seeded)
	a := NewAllocator(c, c)

	got := a.UserInputsFor(ctx, alice)
	if got.UnixName != "adalice" || got.UsernameHash != UsernameHash(alice) {
		t.Errorf("an owner with a PosixIdentity gets its recorded unix name: %+v", got)
	}
	got = a.UserInputsFor(ctx, bob)
	if got.UnixName != "github-bob" || got.UsernameHash != UsernameHash(bob) {
		t.Errorf("an owner without a PosixIdentity gets the derived unix name: %+v", got)
	}
}

func TestEnsureRejectsAnInvalidClaim(t *testing.T) {
	ws := claimingWorkspace("alice-ws", alice, 0)
	ws.Labels[workspaceutil.LabelPosixUID] = "not-a-uid"
	c := newFakeClient(t, ws)
	if _, err := NewAllocator(c, c).Ensure(context.Background(), alice); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

// failingClient fails every Create and Get with err.
type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return f.err
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

func TestEnsureSurfacesAPIErrors(t *testing.T) {
	ctx := context.Background()
	apiErr := errors.New("api unavailable")

	c := newFakeClient(t)
	if _, err := (&Allocator{client: &failingClient{Client: c, err: apiErr}, apiReader: c}).Ensure(ctx, alice); !errors.Is(err, apiErr) {
		t.Errorf("a failed create: want the API error, got %v", err)
	}

	taken := newFakeClient(t, newIdentity(alice, v1Candidates(alice)[0]))
	cache := newFakeClient(t)
	a := &Allocator{client: &createThrough{Client: cache, target: taken}, apiReader: &failingClient{Client: taken, err: apiErr}}
	if _, err := a.Ensure(ctx, alice); !errors.Is(err, apiErr) {
		t.Errorf("a failed read-back after AlreadyExists: want the API error, got %v", err)
	}
	if _, err := a.Peek(ctx, alice); !errors.Is(err, apiErr) {
		t.Errorf("a failed dry-run read: want the API error, got %v", err)
	}
}
