# Idle Shutdown Examples

This directory contains **4 essential examples** for testing idle shutdown functionality.

## Examples

### 1. Simple Workspace (No Template)
**File**: `workspaces/01-simple-workspace.yaml`
- Complete idle shutdown configuration in workspace
- No template dependency
- Uses Code Editor with 3-minute timeout

### 2. Jupyter Template + Inherit
**Files**: `templates/02-jupyter-template.yaml` + `workspaces/02-jupyter-workspace.yaml`
- Template provides complete idle configuration
- Workspace inherits everything from template
- Uses Jupyter Lab with 3-minute timeout

### 3. Code Editor Template + Override
**Files**: `templates/03-code-editor-template.yaml` + `workspaces/03-code-editor-workspace.yaml`
- Template provides base configuration
- Workspace overrides timeout (5 minutes) but keeps template's detection config
- Uses Code Editor

### 4. Override Denied (Validation Test)
**Files**: `templates/04-locked-template.yaml` + `workspaces/04-override-denied.yaml`
- Template denies idle shutdown overrides (`allow: false`)
- Workspace attempts to override timeout (should FAIL validation)
- Tests template validation enforcement

## Quick Test

```bash
# Test Case 1: Simple workspace
kubectl apply -f workspaces/01-simple-workspace.yaml

# Test Case 2: Template + inherit
kubectl apply -f templates/02-jupyter-template.yaml
kubectl apply -f workspaces/02-jupyter-workspace.yaml

# Test Case 3: Template + override
kubectl apply -f templates/03-code-editor-template.yaml
kubectl apply -f workspaces/03-code-editor-workspace.yaml

# Test Case 4: Override denied (should fail)
kubectl apply -f templates/04-locked-template.yaml
kubectl apply -f workspaces/04-override-denied.yaml  # This should be rejected

# Check all workspaces
kubectl get workspaces
```

## Testing Configuration

**⚠️ For Testing**: The idle check interval is set to **5 minutes** for production. For faster testing, update the constant in `internal/controller/constants.go`:

```go
// Change from:
IdleCheckInterval = 5 * time.Minute

// To (for testing):
IdleCheckInterval = 1 * time.Minute  // or 30 * time.Second
```

Then rebuild and redeploy the controller.

## Expected Behavior

**Cases 1-3** should:
- ✅ Create workspace successfully
- ✅ Start pod and reach Running status
- ✅ Begin idle checking (check controller logs)
- ✅ Shut down after configured timeout of inactivity

**Case 4** should:
- ❌ **FAIL** during workspace creation/update
- ❌ Validation error: "idle shutdown overrides not allowed by template"
- ❌ Workspace creation should be rejected by webhook with validation error

## Debugging

```bash
# Watch workspace status
kubectl get workspace <workspace-name> -w

# Check controller logs for idle checking
kubectl logs -n jupyter-k8s-system deployment/jupyter-k8s-controller-manager -f | grep -E "(idle|Resolved idle config)"

# Test endpoint manually
kubectl exec -it <pod-name> -- curl -s http://localhost:8888/api/idle
```

## Cleanup

```bash
kubectl delete workspace --all
kubectl delete workspacetemplate --all
```