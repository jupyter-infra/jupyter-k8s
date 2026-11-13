/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// Identity is a template's security context defaults resolved for one workspace.
type Identity struct {
	// Pod is the template's defaultPodSecurityContext with every identity field resolved; nil if the
	// template has none
	Pod *corev1.PodSecurityContext
	// Container is the template's defaultContainerSecurityContext with every identity field resolved;
	// nil if the template has none
	Container *corev1.SecurityContext
	// UID is the owner's PosixIdentity uid when a template references .User, 0 otherwise
	UID int64
}

// Resolver renders the templated identity fields of a WorkspaceTemplate for one workspace.
type Resolver struct {
	allocator *Allocator
	reader    client.Reader
}

// NewResolver returns a resolver allocating through allocator and reading namespaces through reader,
// the manager cache.
func NewResolver(allocator *Allocator, reader client.Reader) *Resolver {
	return &Resolver{allocator: allocator, reader: reader}
}

// Resolve renders the template's security context defaults for a workspace owned by owner in
// namespace. Inputs are evaluated only when referenced: a template referencing .User allocates the
// owner's uid on first use (or only peeks at it when dryRun), one referencing .Namespace reads the
// namespace's gid label. Every value rendered from a template must be the owner's uid or a valid group
// id outside the operator ranges, and every integer the template sets beside them a valid id outside
// the operator ranges (see CheckIntegerIDs).
func (r *Resolver) Resolve(
	ctx context.Context, spec *workspacev1alpha1.WorkspaceTemplateSpec, owner, namespace string, dryRun bool,
) (*Identity, error) {
	// Checked before any input is read, so a template the template webhook missed never allocates.
	if err := RequireHardening(spec); err != nil {
		return nil, err
	}
	inputs := &liveInputs{
		User:      &liveUser{ctx: ctx, allocator: r.allocator, owner: owner, dryRun: dryRun},
		Namespace: &liveNamespace{ctx: ctx, reader: r.reader, name: namespace},
	}
	identity := &Identity{}
	var rendered []int64
	resolver := func(field string, value intstr.IntOrString) (int64, error) {
		if value.Type == intstr.Int {
			return checkIntegerID(field, value.IntVal)
		}
		id, err := render(field, value.StrVal, inputs)
		if err != nil {
			return 0, err
		}
		rendered = append(rendered, id)
		return id, nil
	}

	var err error
	if identity.Pod, err = spec.DefaultPodSecurityContext.Resolve(resolver); err != nil {
		return nil, fmt.Errorf("defaultPodSecurityContext.%w", err)
	}
	if identity.Container, err = spec.DefaultContainerSecurityContext.Resolve(resolver); err != nil {
		return nil, fmt.Errorf("defaultContainerSecurityContext.%w", err)
	}
	if inputs.User.identity != nil {
		if identity.UID, err = UIDOf(inputs.User.identity); err != nil {
			return nil, err
		}
	}
	for _, value := range rendered {
		if (identity.UID != 0 && value == identity.UID) || ValidGroupID(value) {
			continue
		}
		return nil, fmt.Errorf("resolved id %d is neither the owner's uid nor a group id in [%d, %d] outside the operator range [%d, %d] (and not %d)",
			value, MinID, MaxID, UserRangeMin, GroupRangeMax, NobodyID)
	}
	return identity, nil
}

// RequireHardening rejects a template whose security context defaults hold identity templates unless
// its defaultPodSecurityContext also sets runAsNonRoot: true and supplementalGroupsPolicy: Strict, so
// the template states what pods get: no root, and no groups added by the image's /etc/group.
func RequireHardening(spec *workspacev1alpha1.WorkspaceTemplateSpec) error {
	if !spec.HasIdentityTemplates() {
		return nil
	}
	pod := spec.DefaultPodSecurityContext
	var missing []string
	if pod == nil || pod.RunAsNonRoot == nil || !*pod.RunAsNonRoot {
		missing = append(missing, "runAsNonRoot: true")
	}
	if pod == nil || pod.SupplementalGroupsPolicy == nil || *pod.SupplementalGroupsPolicy != corev1.SupplementalGroupsPolicyStrict {
		missing = append(missing, "supplementalGroupsPolicy: Strict")
	}
	if len(missing) > 0 {
		return fmt.Errorf("defaultPodSecurityContext must set %s when an identity field holds a template",
			strings.Join(missing, " and "))
	}
	return nil
}

// ValidateTemplates checks a WorkspaceTemplate's identity templates at the template author's write:
// each parses, references only .User.Uid, .User.Gid and .Namespace.Gid, and renders to an integer.
// A template without identity templates is not checked: its integers keep their meaning from before
// identity templates existed.
func ValidateTemplates(spec *workspacev1alpha1.WorkspaceTemplateSpec) error {
	if !spec.HasIdentityTemplates() {
		return nil
	}
	sample := staticInputs{User: staticUser{Uid: UserRangeMin, Gid: UserRangeMin}, Namespace: staticNamespace{Gid: MaxID}}
	check := func(field string, value intstr.IntOrString) (int64, error) {
		if value.Type == intstr.Int {
			return checkIntegerID(field, value.IntVal)
		}
		return render(field, value.StrVal, sample)
	}
	if _, err := spec.DefaultPodSecurityContext.Resolve(check); err != nil {
		return fmt.Errorf("defaultPodSecurityContext.%w", err)
	}
	if _, err := spec.DefaultContainerSecurityContext.Resolve(check); err != nil {
		return fmt.Errorf("defaultContainerSecurityContext.%w", err)
	}
	return RequireHardening(spec)
}

// ReferencesUser reports whether a template's identity templates reference .User, so that its
// workspaces run as their owner's uid and carry the posix-uid label. Templates are rendered against
// inputs that record what they read, so a reference in an unused branch is not counted. A template
// that does not render (ValidateTemplates rejects it) counts as referencing .User.
func ReferencesUser(spec *workspacev1alpha1.WorkspaceTemplateSpec) bool {
	if !spec.HasIdentityTemplates() {
		return false
	}
	inputs := &recordingInputs{User: &recordingUser{}}
	references := false
	probe := func(field string, value intstr.IntOrString) (int64, error) {
		if value.Type == intstr.String {
			if _, err := render(field, value.StrVal, inputs); err != nil {
				references = true
			}
		}
		return 0, nil
	}
	_, _ = spec.DefaultPodSecurityContext.Resolve(probe)
	_, _ = spec.DefaultContainerSecurityContext.Resolve(probe)
	return references || inputs.User.read
}

// checkIntegerID checks an integer identity field set beside identity templates, for example the gid
// of a volume shared by several teams. It must be a valid id outside the operator ranges, as a namespace
// gid must: in the user range it would be some user's uid, and so their private group, and 0 is root.
func checkIntegerID(field string, value int32) (int64, error) {
	id := int64(value)
	if !ValidGroupID(id) {
		return 0, fmt.Errorf("%s: %d must be an id in [%d, %d] outside the operator range [%d, %d] (and not %d)",
			field, id, MinID, MaxID, UserRangeMin, GroupRangeMax, NobodyID)
	}
	return id, nil
}

// render executes one identity template, which must render to a decimal integer.
func render(field, text string, data any) (int64, error) {
	tmpl, err := template.New(field).Option("missingkey=error").Parse(text)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid template %q: %w", field, text, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	value, err := strconv.ParseInt(strings.TrimSpace(out.String()), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: template %q renders to %q, not an integer", field, text, out.String())
	}
	return value, nil
}

// Static inputs: typed structs, so an unknown field such as {{ .User.Name }} fails to render.
type staticInputs struct {
	User      staticUser
	Namespace staticNamespace
}

type staticUser struct {
	Uid int64 //nolint:revive // template input name {{ .User.Uid }}
	Gid int64
}

type staticNamespace struct {
	Gid int64
}

// Recording inputs: like the static ones, recording whether a template read .User.
type recordingInputs struct {
	User      *recordingUser
	Namespace staticNamespace
}

type recordingUser struct {
	read bool
}

// Uid records the read, {{ .User.Uid }}.
func (u *recordingUser) Uid() int64 { //nolint:revive // template input name {{ .User.Uid }}
	u.read = true
	return UserRangeMin
}

// Gid records the read, {{ .User.Gid }}.
func (u *recordingUser) Gid() int64 {
	u.read = true
	return UserRangeMin
}

// Live inputs: methods, so an input is read only when a template references it.
type liveInputs struct {
	User      *liveUser
	Namespace *liveNamespace
}

type liveUser struct {
	ctx       context.Context
	allocator *Allocator
	owner     string
	dryRun    bool
	identity  *workspacev1alpha1.PosixIdentity
}

func (u *liveUser) get() (*workspacev1alpha1.PosixIdentity, error) {
	if u.identity != nil {
		return u.identity, nil
	}
	var identity *workspacev1alpha1.PosixIdentity
	var err error
	if u.dryRun {
		identity, err = u.allocator.Peek(u.ctx, u.owner)
	} else {
		identity, err = u.allocator.Ensure(u.ctx, u.owner)
	}
	if err != nil {
		return nil, err
	}
	u.identity = identity
	return identity, nil
}

// Uid returns the owner's uid, {{ .User.Uid }}.
func (u *liveUser) Uid() (int64, error) { //nolint:revive // template input name {{ .User.Uid }}
	identity, err := u.get()
	if err != nil {
		return 0, err
	}
	return UIDOf(identity)
}

// Gid returns the owner's primary gid, equal to the uid, {{ .User.Gid }}.
func (u *liveUser) Gid() (int64, error) {
	return u.Uid()
}

type liveNamespace struct {
	ctx    context.Context
	reader client.Reader
	name   string
	gid    *int64
}

// Gid returns the workspace namespace's gid, {{ .Namespace.Gid }}.
func (n *liveNamespace) Gid() (int64, error) {
	if n.gid != nil {
		return *n.gid, nil
	}
	gid, err := NamespaceGID(n.ctx, n.reader, n.name)
	if err != nil {
		return 0, err
	}
	n.gid = &gid
	return gid, nil
}
