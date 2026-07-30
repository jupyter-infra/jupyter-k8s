/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Shared nil-combination case names, kept as constants so goconst does not flag
// the repeated literals across the *Changed table tests.
const (
	caseBothNil     = "both nil"
	caseOldNilNew   = "old nil new set"
	caseOldSetNewer = "old set new nil"
)

func intPtr(i int) *int { return &i }

func qtyPtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func templateWithSpec(spec workspacev1alpha1.WorkspaceTemplateSpec) *workspacev1alpha1.WorkspaceTemplate {
	return &workspacev1alpha1.WorkspaceTemplate{Spec: spec}
}

func TestConstraintsChanged(t *testing.T) {
	tests := []struct {
		name string
		old  workspacev1alpha1.WorkspaceTemplateSpec
		new  workspacev1alpha1.WorkspaceTemplateSpec
		want bool
	}{
		{
			name: "identical specs report no change",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{AllowedImages: []string{"a", "b"}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{AllowedImages: []string{"a", "b"}},
			want: false,
		},
		{
			name: "allowed images changed",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{AllowedImages: []string{"a"}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{AllowedImages: []string{"a", "b"}},
			want: true,
		},
		{
			name: "resource bounds changed",
			old: workspacev1alpha1.WorkspaceTemplateSpec{ResourceBounds: &workspacev1alpha1.ResourceBounds{
				Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
					corev1.ResourceCPU: {Min: resource.MustParse("100m"), Max: resource.MustParse("1")},
				},
			}},
			new: workspacev1alpha1.WorkspaceTemplateSpec{ResourceBounds: &workspacev1alpha1.ResourceBounds{
				Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
					corev1.ResourceCPU: {Min: resource.MustParse("100m"), Max: resource.MustParse("2")},
				},
			}},
			want: true,
		},
		{
			name: "max storage size changed",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{PrimaryStorage: &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("10Gi")}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{PrimaryStorage: &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("20Gi")}},
			want: true,
		},
		{
			name: "idle shutdown allow changed",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(false)}},
			want: true,
		},
		{
			name: "idle shutdown timeout bounds changed",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(5)}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(10)}},
			want: true,
		},
		{
			name: "env requirements changed",
			old:  workspacev1alpha1.WorkspaceTemplateSpec{EnvRequirements: []workspacev1alpha1.EnvRequirement{{Name: "FOO"}}},
			new:  workspacev1alpha1.WorkspaceTemplateSpec{EnvRequirements: []workspacev1alpha1.EnvRequirement{{Name: "BAR"}}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constraintsChanged(templateWithSpec(tt.old), templateWithSpec(tt.new))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResourceBoundsChanged(t *testing.T) {
	bounds := func(maxCPU string) *workspacev1alpha1.ResourceBounds {
		return &workspacev1alpha1.ResourceBounds{
			Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
				corev1.ResourceCPU: {Min: resource.MustParse("100m"), Max: resource.MustParse(maxCPU)},
			},
		}
	}

	tests := []struct {
		name     string
		old, new *workspacev1alpha1.ResourceBounds
		want     bool
	}{
		{name: caseBothNil, old: nil, new: nil, want: false},
		{name: caseOldNilNew, old: nil, new: bounds("1"), want: true},
		{name: caseOldSetNewer, old: bounds("1"), new: nil, want: true},
		{name: "equal values", old: bounds("1"), new: bounds("1"), want: false},
		{name: "different values", old: bounds("1"), new: bounds("2"), want: true},
		{
			name: "semantic equality: 1000m equals 1",
			old:  bounds("1000m"),
			new:  bounds("1"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceBoundsChanged(tt.old, tt.new))
		})
	}
}

func TestMaxStorageSizeChanged(t *testing.T) {
	tests := []struct {
		name     string
		old, new *workspacev1alpha1.StorageConfig
		want     bool
	}{
		{name: caseBothNil, old: nil, new: nil, want: false},
		{name: caseOldNilNew, old: nil, new: &workspacev1alpha1.StorageConfig{}, want: true},
		{name: caseOldSetNewer, old: &workspacev1alpha1.StorageConfig{}, new: nil, want: true},
		{
			name: "max size added",
			old:  &workspacev1alpha1.StorageConfig{},
			new:  &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("10Gi")},
			want: true,
		},
		{
			name: "max size changed",
			old:  &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("10Gi")},
			new:  &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("20Gi")},
			want: true,
		},
		{
			name: "max size unchanged",
			old:  &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("10Gi")},
			new:  &workspacev1alpha1.StorageConfig{MaxSize: qtyPtr("10Gi")},
			want: false,
		},
		{
			name: "min size added",
			old:  &workspacev1alpha1.StorageConfig{},
			new:  &workspacev1alpha1.StorageConfig{MinSize: qtyPtr("1Gi")},
			want: true,
		},
		{
			name: "min size changed",
			old:  &workspacev1alpha1.StorageConfig{MinSize: qtyPtr("1Gi")},
			new:  &workspacev1alpha1.StorageConfig{MinSize: qtyPtr("2Gi")},
			want: true,
		},
		{
			name: "min and max unchanged",
			old:  &workspacev1alpha1.StorageConfig{MinSize: qtyPtr("1Gi"), MaxSize: qtyPtr("10Gi")},
			new:  &workspacev1alpha1.StorageConfig{MinSize: qtyPtr("1Gi"), MaxSize: qtyPtr("10Gi")},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maxStorageSizeChanged(tt.old, tt.new))
		})
	}
}

func TestIdleShutdownAllowOverrideChanged(t *testing.T) {
	tests := []struct {
		name     string
		old, new *workspacev1alpha1.IdleShutdownOverridePolicy
		want     bool
	}{
		{name: caseBothNil, old: nil, new: nil, want: false},
		{name: caseOldNilNew, old: nil, new: &workspacev1alpha1.IdleShutdownOverridePolicy{}, want: true},
		{name: caseOldSetNewer, old: &workspacev1alpha1.IdleShutdownOverridePolicy{}, new: nil, want: true},
		{
			name: "allow added",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)},
			want: true,
		},
		{
			name: "allow flipped",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(false)},
			want: true,
		},
		{
			name: "allow unchanged",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, idleShutdownAllowOverrideChanged(tt.old, tt.new))
		})
	}
}

func TestIdleShutdownTimeoutBoundsChanged(t *testing.T) {
	tests := []struct {
		name     string
		old, new *workspacev1alpha1.IdleShutdownOverridePolicy
		want     bool
	}{
		{name: caseBothNil, old: nil, new: nil, want: false},
		{name: caseOldNilNew, old: nil, new: &workspacev1alpha1.IdleShutdownOverridePolicy{}, want: true},
		{
			name: "min added",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(5)},
			want: true,
		},
		{
			name: "min changed",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(5)},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(10)},
			want: true,
		},
		{
			name: "max added",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{MaxIdleTimeoutInMinutes: intPtr(60)},
			want: true,
		},
		{
			name: "max changed",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{MaxIdleTimeoutInMinutes: intPtr(60)},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{MaxIdleTimeoutInMinutes: intPtr(120)},
			want: true,
		},
		{
			name: "min and max unchanged",
			old:  &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(5), MaxIdleTimeoutInMinutes: intPtr(60)},
			new:  &workspacev1alpha1.IdleShutdownOverridePolicy{MinIdleTimeoutInMinutes: intPtr(5), MaxIdleTimeoutInMinutes: intPtr(60)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, idleShutdownTimeoutBoundsChanged(tt.old, tt.new))
		})
	}
}
