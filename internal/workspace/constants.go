/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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

	// AccessStrategyFinalizerName is the name of the finalizer place on an accessStrategy that is referenced by workspaces
	AccessStrategyFinalizerName = "workspace.jupyter.org/accessstrategy-protection"

	// ConflictRetryDelayMilliseconds is the base delay in milliseconds before retrying after a conflict
	ConflictRetryDelayMilliseconds = 100

	// ConflictRetryJitterMilliseconds is the maximum random jitter in milliseconds added to the retry delay
	ConflictRetryJitterMilliseconds = 50

	// WorkspacePageLimit defines the maximum number of Workspace returned by List(Workspace) call
	WorkspacePageLimit int64 = 100
)
