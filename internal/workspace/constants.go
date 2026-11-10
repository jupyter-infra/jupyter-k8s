/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

const (
	// LabelWorkspaceName is the label key for workspace name
	LabelWorkspaceName = "workspace.jupyter.org/workspace-name"

	// LabelWorkspaceTemplate is the label key for template name in the Workspace labels
	LabelWorkspaceTemplate = "workspace.jupyter.org/template-name"

	// LabelWorkspaceTemplateNamespace is the label key for template namespace in the Workspace labels
	LabelWorkspaceTemplateNamespace = "workspace.jupyter.org/template-namespace"

	// LabelAccessStrategyName is the label key for access strategy name in the Workspace labels
	LabelAccessStrategyName = "workspace.jupyter.org/access-strategy-name"

	// LabelAccessStrategyNamespace is the label key for access strategy namespace in the Workspace labels
	LabelAccessStrategyNamespace = "workspace.jupyter.org/access-strategy-namespace"

	// TemplateFinalizerName is the name of the finalizer placed on a template that is referenced by workspaces
	TemplateFinalizerName = "workspace.jupyter.org/template-protection"

	// AccessStrategyFinalizerName is the name of the finalizer placed on an accessStrategy that is referenced by workspaces
	AccessStrategyFinalizerName = "workspace.jupyter.org/accessstrategy-protection"

	// AccessStrategyTemplateFinalizerName is the name of the finalizer placed on an accessStrategy that is
	// referenced by templates (via spec.defaultAccessStrategy). It is intentionally distinct from
	// AccessStrategyFinalizerName so workspace and template references protect the access strategy
	// independently: each finalizer is managed solely by its own referrer type, and Kubernetes keeps the
	// access strategy alive until BOTH are removed.
	AccessStrategyTemplateFinalizerName = "workspace.jupyter.org/accessstrategy-template-protection"

	// AnnotationCreatedBy is the annotation key recording the workspace's owner (its creator's username)
	AnnotationCreatedBy = "workspace.jupyter.org/created-by"

	// LabelPosixUID is the reserved Workspace label recording the uid of the owner's PosixIdentity when
	// its template resolves identity from it; written by the workspace defaulter only. A label so admins
	// can select the workspaces (and, through label propagation, the pods) running as a uid.
	LabelPosixUID = "workspace.jupyter.org/posix-uid"

	// LabelPosixGID is the deployer-supplied Namespace label carrying the team's gid ({{ .Namespace.Gid }})
	LabelPosixGID = "workspace.jupyter.org/posix-gid"

	// ConflictRetryDelayMilliseconds is the base delay in milliseconds before retrying after a conflict
	ConflictRetryDelayMilliseconds = 100

	// ConflictRetryJitterMilliseconds is the maximum random jitter in milliseconds added to the retry delay
	ConflictRetryJitterMilliseconds = 50

	// WorkspacePageLimit defines the maximum number of Workspace returned by List(Workspace) call
	WorkspacePageLimit int64 = 100
)
