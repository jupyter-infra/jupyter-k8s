/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TemplatedPodSecurityContext mirrors corev1.PodSecurityContext, field for field, for a
// WorkspaceTemplate's default. Its identity fields (runAsUser, runAsGroup, fsGroup,
// supplementalGroups) also accept a Go template, resolved per workspace at admission:
// {{ .User.Uid }} and {{ .User.Gid }} (the owner's PosixIdentity, allocated on first use), and
// {{ .Namespace.Gid }} (the workspace namespace's workspace.jupyter.org/posix-gid label). A template
// using a Go template in any identity field must also set runAsNonRoot: true and
// supplementalGroupsPolicy: Strict.
//
// Field docstrings are upstream's, verbatim; identity fields add one paragraph. Keep in sync with
// corev1.PodSecurityContext: TestTemplatedSecurityContextsMirrorCore fails on a missing or retyped
// field, TestTemplatedSecurityContextsMirrorCoreDocs on a docstring that differs from upstream's.
type TemplatedPodSecurityContext struct {
	// The SELinux context to be applied to all containers.
	// If unspecified, the container runtime will allocate a random SELinux context for each
	// container.  May also be set in SecurityContext.  If set in
	// both SecurityContext and PodSecurityContext, the value specified in SecurityContext
	// takes precedence for that container.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	SELinuxOptions *corev1.SELinuxOptions `json:"seLinuxOptions,omitempty"`
	// The Windows specific settings applied to all containers.
	// If unspecified, the options within a container's SecurityContext will be used.
	// If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.
	// Note that this field cannot be set when spec.os.name is linux.
	// +optional
	WindowsOptions *corev1.WindowsSecurityContextOptions `json:"windowsOptions,omitempty"`
	// The UID to run the entrypoint of the container process.
	// Defaults to user specified in image metadata if unspecified.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence
	// for that container.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// An integer, or a Go template resolved for each workspace at admission: {{ .User.Uid }} (the
	// owner's uid, allocated on first use) or {{ .Namespace.Gid }} (the workspace namespace's
	// workspace.jupyter.org/posix-gid label).
	// +optional
	RunAsUser *intstr.IntOrString `json:"runAsUser,omitempty"`
	// The GID to run the entrypoint of the container process.
	// Uses runtime default if unset.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence
	// for that container.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// An integer, or a Go template resolved for each workspace at admission: {{ .User.Gid }} (the
	// owner's primary gid, equal to their uid) or {{ .Namespace.Gid }}.
	// +optional
	RunAsGroup *intstr.IntOrString `json:"runAsGroup,omitempty"`
	// Indicates that the container must run as a non-root user.
	// If true, the Kubelet will validate the image at runtime to ensure that it
	// does not run as UID 0 (root) and fail to start the container if it does.
	// If unset or false, no such validation will be performed.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty"`
	// A list of groups applied to the first process run in each container, in
	// addition to the container's primary GID and fsGroup (if specified).  If
	// the SupplementalGroupsPolicy feature is enabled, the
	// supplementalGroupsPolicy field determines whether these are in addition
	// to or instead of any group memberships defined in the container image.
	// If unspecified, no additional groups are added, though group memberships
	// defined in the container image may still be used, depending on the
	// supplementalGroupsPolicy field.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// Each entry is an integer, or a Go template resolved for each workspace at admission:
	// {{ .User.Gid }} or {{ .Namespace.Gid }}, e.g. ["{{ .Namespace.Gid }}", 2147000123].
	// +kubebuilder:validation:MaxItems=16
	// +listType=atomic
	// +optional
	SupplementalGroups []intstr.IntOrString `json:"supplementalGroups,omitempty"`
	// Defines how supplemental groups of the first container processes are calculated.
	// Valid values are "Merge" and "Strict". If not specified, "Merge" is used.
	// (Alpha) Using the field requires the SupplementalGroupsPolicy feature gate to be enabled
	// and the container runtime must implement support for this feature.
	// Note that this field cannot be set when spec.os.name is windows.
	// TODO: update the default value to "Merge" when spec.os.name is not windows in v1.34
	// +optional
	SupplementalGroupsPolicy *corev1.SupplementalGroupsPolicy `json:"supplementalGroupsPolicy,omitempty"`
	// A special supplemental group that applies to all containers in a pod.
	// Some volume types allow the Kubelet to change the ownership of that volume
	// to be owned by the pod:
	//
	// 1. The owning GID will be the FSGroup
	// 2. The setgid bit is set (new files created in the volume will be owned by FSGroup)
	// 3. The permission bits are OR'd with rw-rw----
	//
	// If unset, the Kubelet will not modify the ownership and permissions of any volume.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// An integer, or a Go template resolved for each workspace at admission: {{ .User.Gid }} (the
	// owner's primary gid, equal to their uid) or {{ .Namespace.Gid }}.
	// +optional
	FSGroup *intstr.IntOrString `json:"fsGroup,omitempty"`
	// Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported
	// sysctls (by the container runtime) might fail to launch.
	// Note that this field cannot be set when spec.os.name is windows.
	// +listType=atomic
	// +optional
	Sysctls []corev1.Sysctl `json:"sysctls,omitempty"`
	// fsGroupChangePolicy defines behavior of changing ownership and permission of the volume
	// before being exposed inside Pod. This field will only apply to
	// volume types which support fsGroup based ownership(and permissions).
	// It will have no effect on ephemeral volume types such as: secret, configmaps
	// and emptydir.
	// Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	FSGroupChangePolicy *corev1.PodFSGroupChangePolicy `json:"fsGroupChangePolicy,omitempty"`
	// The seccomp options to use by the containers in this pod.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	SeccompProfile *corev1.SeccompProfile `json:"seccompProfile,omitempty"`
	// appArmorProfile is the AppArmor options to use by the containers in this pod.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	AppArmorProfile *corev1.AppArmorProfile `json:"appArmorProfile,omitempty"`
	// seLinuxChangePolicy defines how the container's SELinux label is applied to all volumes used by the Pod.
	// It has no effect on nodes that do not support SELinux or to volumes does not support SELinux.
	// Valid values are "MountOption" and "Recursive".
	//
	// "Recursive" means relabeling of all files on all Pod volumes by the container runtime.
	// This may be slow for large volumes, but allows mixing privileged and unprivileged Pods sharing the same volume on the same node.
	//
	// "MountOption" mounts all eligible Pod volumes with `-o context` mount option.
	// This requires all Pods that share the same volume to use the same SELinux label.
	// It is not possible to share the same volume among privileged and unprivileged Pods.
	// Eligible volumes are in-tree FibreChannel and iSCSI volumes, and all CSI volumes
	// whose CSI driver announces SELinux support by setting spec.seLinuxMount: true in their
	// CSIDriver instance. Other volumes are always re-labelled recursively.
	// "MountOption" value is allowed only when SELinuxMount feature gate is enabled.
	//
	// If not specified and SELinuxMount feature gate is enabled, "MountOption" is used.
	// If not specified and SELinuxMount feature gate is disabled, "MountOption" is used for ReadWriteOncePod volumes
	// and "Recursive" for all other volumes.
	//
	// This field affects only Pods that have SELinux label set, either in PodSecurityContext or in SecurityContext of all containers.
	//
	// All Pods that use the same volume should use the same seLinuxChangePolicy, otherwise some pods can get stuck in ContainerCreating state.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	SELinuxChangePolicy *corev1.PodSELinuxChangePolicy `json:"seLinuxChangePolicy,omitempty"`
}

// TemplatedSecurityContext mirrors corev1.SecurityContext, field for field, for a WorkspaceTemplate's
// default container security context. Its identity fields (runAsUser, runAsGroup) also accept a Go
// template, with the inputs of TemplatedPodSecurityContext.
//
// Field docstrings are upstream's, verbatim; identity fields add one paragraph. Keep in sync with
// corev1.SecurityContext as TemplatedPodSecurityContext is.
type TemplatedSecurityContext struct {
	// The capabilities to add/drop when running containers.
	// Defaults to the default set of capabilities granted by the container runtime.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	Capabilities *corev1.Capabilities `json:"capabilities,omitempty"`
	// Run container in privileged mode.
	// Processes in privileged containers are essentially equivalent to root on the host.
	// Defaults to false.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	Privileged *bool `json:"privileged,omitempty"`
	// The SELinux context to be applied to the container.
	// If unspecified, the container runtime will allocate a random SELinux context for each
	// container.  May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	SELinuxOptions *corev1.SELinuxOptions `json:"seLinuxOptions,omitempty"`
	// The Windows specific settings applied to all containers.
	// If unspecified, the options from the PodSecurityContext will be used.
	// If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.
	// Note that this field cannot be set when spec.os.name is linux.
	// +optional
	WindowsOptions *corev1.WindowsSecurityContextOptions `json:"windowsOptions,omitempty"`
	// The UID to run the entrypoint of the container process.
	// Defaults to user specified in image metadata if unspecified.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// An integer, or a Go template resolved for each workspace at admission: {{ .User.Uid }} (the
	// owner's uid, allocated on first use) or {{ .Namespace.Gid }} (the workspace namespace's
	// workspace.jupyter.org/posix-gid label).
	// +optional
	RunAsUser *intstr.IntOrString `json:"runAsUser,omitempty"`
	// The GID to run the entrypoint of the container process.
	// Uses runtime default if unset.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// Note that this field cannot be set when spec.os.name is windows.
	//
	// An integer, or a Go template resolved for each workspace at admission: {{ .User.Gid }} (the
	// owner's primary gid, equal to their uid) or {{ .Namespace.Gid }}.
	// +optional
	RunAsGroup *intstr.IntOrString `json:"runAsGroup,omitempty"`
	// Indicates that the container must run as a non-root user.
	// If true, the Kubelet will validate the image at runtime to ensure that it
	// does not run as UID 0 (root) and fail to start the container if it does.
	// If unset or false, no such validation will be performed.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty"`
	// Whether this container has a read-only root filesystem.
	// Default is false.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty"`
	// AllowPrivilegeEscalation controls whether a process can gain more
	// privileges than its parent process. This bool directly controls if
	// the no_new_privs flag will be set on the container process.
	// AllowPrivilegeEscalation is true always when the container is:
	// 1) run as Privileged
	// 2) has CAP_SYS_ADMIN
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	AllowPrivilegeEscalation *bool `json:"allowPrivilegeEscalation,omitempty"`
	// procMount denotes the type of proc mount to use for the containers.
	// The default value is Default which uses the container runtime defaults for
	// readonly paths and masked paths.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	ProcMount *corev1.ProcMountType `json:"procMount,omitempty"`
	// The seccomp options to use by this container. If seccomp options are
	// provided at both the pod & container level, the container options
	// override the pod options.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	SeccompProfile *corev1.SeccompProfile `json:"seccompProfile,omitempty"`
	// appArmorProfile is the AppArmor options to use by this container. If set, this profile
	// overrides the pod's appArmorProfile.
	// Note that this field cannot be set when spec.os.name is windows.
	// +optional
	AppArmorProfile *corev1.AppArmorProfile `json:"appArmorProfile,omitempty"`
}

// IDResolver turns an identity field into an integer: an int value as is, a string by rendering it.
// +kubebuilder:object:generate=false
type IDResolver func(field string, value intstr.IntOrString) (int64, error)

// HasTemplates reports whether any identity field holds a template rather than an integer.
func (t *TemplatedPodSecurityContext) HasTemplates() bool {
	if t == nil {
		return false
	}
	for _, v := range t.SupplementalGroups {
		if v.Type == intstr.String {
			return true
		}
	}
	return isTemplate(t.RunAsUser) || isTemplate(t.RunAsGroup) || isTemplate(t.FSGroup)
}

// HasTemplates reports whether any identity field holds a template rather than an integer.
func (t *TemplatedSecurityContext) HasTemplates() bool {
	return t != nil && (isTemplate(t.RunAsUser) || isTemplate(t.RunAsGroup))
}

// Resolve returns the corev1.PodSecurityContext with every identity field resolved; nil for nil.
func (t *TemplatedPodSecurityContext) Resolve(resolve IDResolver) (*corev1.PodSecurityContext, error) {
	if t == nil {
		return nil, nil
	}
	out := &corev1.PodSecurityContext{
		SELinuxOptions:           t.SELinuxOptions.DeepCopy(),
		WindowsOptions:           t.WindowsOptions.DeepCopy(),
		RunAsNonRoot:             copyPtr(t.RunAsNonRoot),
		SupplementalGroupsPolicy: copyPtr(t.SupplementalGroupsPolicy),
		FSGroupChangePolicy:      copyPtr(t.FSGroupChangePolicy),
		SeccompProfile:           t.SeccompProfile.DeepCopy(),
		AppArmorProfile:          t.AppArmorProfile.DeepCopy(),
		SELinuxChangePolicy:      copyPtr(t.SELinuxChangePolicy),
	}
	if t.Sysctls != nil {
		out.Sysctls = append([]corev1.Sysctl{}, t.Sysctls...)
	}
	var err error
	if out.RunAsUser, err = resolveID("runAsUser", t.RunAsUser, resolve); err != nil {
		return nil, err
	}
	if out.RunAsGroup, err = resolveID("runAsGroup", t.RunAsGroup, resolve); err != nil {
		return nil, err
	}
	if out.FSGroup, err = resolveID("fsGroup", t.FSGroup, resolve); err != nil {
		return nil, err
	}
	for i := range t.SupplementalGroups {
		gid, err := resolveID(fmt.Sprintf("supplementalGroups[%d]", i), &t.SupplementalGroups[i], resolve)
		if err != nil {
			return nil, err
		}
		out.SupplementalGroups = append(out.SupplementalGroups, *gid)
	}
	return out, nil
}

// Resolve returns the corev1.SecurityContext with every identity field resolved; nil for nil.
func (t *TemplatedSecurityContext) Resolve(resolve IDResolver) (*corev1.SecurityContext, error) {
	if t == nil {
		return nil, nil
	}
	out := &corev1.SecurityContext{
		Capabilities:             t.Capabilities.DeepCopy(),
		Privileged:               copyPtr(t.Privileged),
		SELinuxOptions:           t.SELinuxOptions.DeepCopy(),
		WindowsOptions:           t.WindowsOptions.DeepCopy(),
		RunAsNonRoot:             copyPtr(t.RunAsNonRoot),
		ReadOnlyRootFilesystem:   copyPtr(t.ReadOnlyRootFilesystem),
		AllowPrivilegeEscalation: copyPtr(t.AllowPrivilegeEscalation),
		ProcMount:                copyPtr(t.ProcMount),
		SeccompProfile:           t.SeccompProfile.DeepCopy(),
		AppArmorProfile:          t.AppArmorProfile.DeepCopy(),
	}
	var err error
	if out.RunAsUser, err = resolveID("runAsUser", t.RunAsUser, resolve); err != nil {
		return nil, err
	}
	if out.RunAsGroup, err = resolveID("runAsGroup", t.RunAsGroup, resolve); err != nil {
		return nil, err
	}
	return out, nil
}

// StaticID resolves integer identity fields only, failing on a template: for callers that cannot
// resolve templates.
func StaticID(field string, value intstr.IntOrString) (int64, error) {
	if value.Type == intstr.String {
		return 0, fmt.Errorf("%s: template %q needs to be resolved", field, value.StrVal)
	}
	return int64(value.IntVal), nil
}

func resolveID(field string, value *intstr.IntOrString, resolve IDResolver) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	id, err := resolve(field, *value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func isTemplate(v *intstr.IntOrString) bool {
	return v != nil && v.Type == intstr.String
}

func copyPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
