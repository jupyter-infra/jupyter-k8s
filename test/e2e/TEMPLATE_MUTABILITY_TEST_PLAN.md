# Template Mutability E2E Test Plan

## Test Suite Goals

Comprehensive E2E validation of template mutability feature following Kubernetes and industry best practices:
- **Separation of concerns**: Webhooks enforce, controllers reconcile
- **Status observability**: Compliance checking via status conditions
- **Lazy application**: Existing workspaces grandfathered
- **API compatibility**: TemplateRef type change doesn't break existing behavior

---

## Test Categories

### 1. Template Spec Mutability

#### Test 1.1: Modify Template DisplayName
**Goal**: Verify basic template mutability
**Steps**:
1. Create template with displayName="Original"
2. Modify displayName to "Modified"
3. Verify change succeeded
**Expected**: Template spec.displayName updates successfully
**Static Files**: `static/template-mutability/base-template.yaml`, `static/template-mutability/modified-displayname.yaml`

#### Test 1.2: Modify Template Resource Bounds
**Goal**: Verify resource bounds can be changed
**Steps**:
1. Create template with CPU max=2
2. Modify CPU max to 4
3. Create new workspace, verify new bounds apply
**Expected**: New workspaces use new bounds
**Static Files**: `static/template-mutability/template-bounds-v1.yaml`, `static/template-mutability/template-bounds-v2.yaml`

#### Test 1.3: Modify Template Image Allowlist
**Goal**: Verify image restrictions can be changed
**Steps**:
1. Create template with allowedImages=["image-a"]
2. Modify to allowedImages=["image-a", "image-b"]
3. Create workspace with image-b, verify accepted
**Expected**: New image allowed after template change
**Static Files**: `static/template-mutability/template-restrictive.yaml`, `static/template-mutability/template-permissive.yaml`

#### Test 1.4: Modify Multiple Template Fields
**Goal**: Verify multiple simultaneous changes
**Steps**:
1. Create template
2. Modify displayName, defaultImage, resourceBounds in one update
3. Verify all changes applied
**Expected**: All fields update atomically
**Static Files**: `static/template-mutability/template-v1.yaml`, `static/template-mutability/template-v2.yaml`

---

### 2. Lazy Application Behavior

#### Test 2.1: Existing Workspace Unchanged After Template Modification
**Goal**: Verify lazy application - existing workspaces don't auto-update
**Steps**:
1. Create template with CPU max=2
2. Create workspace-A with CPU=1
3. Modify template CPU max to 0.5
4. Verify workspace-A still has CPU=1 (not rejected)
**Expected**: Workspace-A continues running, spec unchanged
**Static Files**: `static/lazy-application/template-before.yaml`, `static/lazy-application/template-after.yaml`, `static/lazy-application/workspace-grandfathered.yaml`

#### Test 2.2: New Workspace Uses New Template Defaults
**Goal**: Verify new workspaces get new defaults
**Steps**:
1. Create template with defaultImage="image-v1"
2. Create workspace-A (gets image-v1)
3. Modify template defaultImage to "image-v2"
4. Create workspace-B without specifying image
5. Verify workspace-A has image-v1, workspace-B has image-v2
**Expected**: Lazy application - old workspace unaffected, new workspace uses new defaults
**Static Files**: `static/lazy-application/template-initial.yaml`, `static/lazy-application/template-updated.yaml`, `static/lazy-application/workspace-old.yaml`, `static/lazy-application/workspace-new.yaml`

#### Test 2.3: Grandfathered Workspace Shows Compliance Status
**Goal**: Verify compliance checking detects drift
**Steps**:
1. Create template with CPU max=2
2. Create workspace-A with CPU=1.5
3. Modify template CPU max to 1
4. Wait for compliance check
5. Verify workspace-A status shows TemplateCompliant=False
**Expected**: Status condition reflects non-compliance, but workspace continues running
**Static Files**: `static/lazy-application/template-permissive.yaml`, `static/lazy-application/template-strict.yaml`, `static/lazy-application/workspace-becomes-noncompliant.yaml`

---

### 3. TemplateRef Mutability

#### Test 3.1: Switch TemplateRef Between Templates
**Goal**: Verify workspace can change template reference
**Steps**:
1. Create template-A and template-B
2. Create workspace with templateRef=template-A
3. Verify workspace.spec.templateRef.name=="template-A"
4. Patch workspace templateRef to template-B
5. Verify switch succeeded
**Expected**: TemplateRef changes, webhook validates against new template
**Static Files**: `static/templateref-mutability/template-a.yaml`, `static/templateref-mutability/template-b.yaml`, `static/templateref-mutability/workspace-switch.yaml`

#### Test 3.2: Switch Fails If New Template Rejects Current Config
**Goal**: Verify validation against new template
**Steps**:
1. Create permissive-template (allows CPU=4)
2. Create strict-template (max CPU=1)
3. Create workspace with templateRef=permissive, CPU=2
4. Attempt to switch to strict-template
5. Verify webhook rejects (CPU=2 exceeds strict max)
**Expected**: Webhook blocks invalid switch
**Static Files**: `static/templateref-mutability/template-permissive.yaml`, `static/templateref-mutability/template-strict.yaml`, `static/templateref-mutability/workspace-rejected-switch.yaml`

#### Test 3.3: Remove TemplateRef (Set to Null)
**Goal**: Verify workspace can become template-less
**Steps**:
1. Create workspace with templateRef=template-A
2. Patch workspace to remove templateRef (set to null)
3. Verify workspace continues, must specify image directly
**Expected**: Workspace validates without template
**Static Files**: `static/templateref-mutability/workspace-with-template.yaml`

---

### 4. Compliance Checking Status Conditions

#### Test 4.1: Compliant Workspace Shows Positive Status
**Goal**: Verify TemplateCompliant condition when compliant
**Steps**:
1. Create template with CPU max=2
2. Create workspace with CPU=1
3. Wait for compliance check (≤10s)
4. Verify status.conditions shows TemplateCompliant=True
**Expected**: Status updated by WorkspaceTemplate controller
**Static Files**: `static/compliance-checking/template.yaml`, `static/compliance-checking/workspace-compliant.yaml`

#### Test 4.2: Non-Compliant Workspace Shows Negative Status
**Goal**: Verify TemplateCompliant condition when non-compliant
**Steps**:
1. Create template-v1 with CPU max=2
2. Create workspace with CPU=1.5
3. Modify template to CPU max=1
4. Wait for compliance check
5. Verify status shows TemplateCompliant=False with violation message
**Expected**: Status includes human-readable violation details
**Static Files**: `static/compliance-checking/template-v1.yaml`, `static/compliance-checking/template-v2.yaml`, `static/compliance-checking/workspace-drifts.yaml`

#### Test 4.3: Status Condition Updates on Template Change
**Goal**: Verify compliance checking runs on template update
**Steps**:
1. Create template with CPU max=2
2. Create workspace with CPU=1.5 (compliant initially)
3. Verify TemplateCompliant=True
4. Modify template to CPU max=1
5. Wait for WorkspaceTemplate controller reconcile
6. Verify TemplateCompliant=False
**Expected**: Status automatically updates within reconciliation period
**Static Files**: `static/compliance-checking/template-before.yaml`, `static/compliance-checking/template-after.yaml`, `static/compliance-checking/workspace-status-updates.yaml`

#### Test 4.4: Multiple Violations Reported in Status
**Goal**: Verify comprehensive violation reporting
**Steps**:
1. Create restrictive template
2. Create workspace violating CPU, memory, and image constraints (via lazy application)
3. Verify status message lists all violations
**Expected**: Status.message contains all constraint violations
**Static Files**: `static/compliance-checking/template-restrictive.yaml`, `static/compliance-checking/workspace-multiple-violations.yaml`

---

### 5. Validation Architecture

#### Test 5.1: Webhook Rejects Invalid New Workspace
**Goal**: Verify admission control works
**Steps**:
1. Create template with CPU max=1
2. Attempt to create workspace with CPU=2
3. Verify webhook rejects at admission
**Expected**: kubectl apply fails, workspace not created
**Static Files**: `static/validation/template-strict.yaml`, `static/validation/workspace-invalid.yaml`

#### Test 5.2: Controller Does Not Reject Running Workspace
**Goal**: Verify controller doesn't enforce validation
**Steps**:
1. Create template with CPU max=2
2. Create workspace with CPU=1 (valid)
3. Modify template to CPU max=0.5
4. Verify workspace continues running
5. Verify no Degraded condition from validation failure
**Expected**: Controller updates status (TemplateCompliant=False) but doesn't stop workspace
**Static Files**: `static/validation/template-permissive.yaml`, `static/validation/template-strict.yaml`, `static/validation/workspace-keeps-running.yaml`

#### Test 5.3: Webhook Re-validates on Workspace Update
**Goal**: Verify webhook validates all updates
**Steps**:
1. Create template with CPU max=1
2. Create workspace with CPU=0.5 (valid)
3. Attempt to patch workspace CPU to 2
4. Verify webhook rejects update
**Expected**: Webhook blocks invalid modification
**Static Files**: `static/validation/template.yaml`, `static/validation/workspace-valid.yaml`

---

### 6. Deletion Protection (Regression Test)

#### Test 6.1: Template With Active Workspaces Cannot Delete
**Goal**: Verify finalizer pattern unchanged
**Steps**:
1. Create template
2. Create workspace using template
3. Attempt to delete template
4. Verify template has deletionTimestamp but still exists
5. Delete workspace
6. Verify template deletes automatically
**Expected**: Lazy finalizer protection works
**Static Files**: `static/deletion-protection/template.yaml`, `static/deletion-protection/workspace.yaml`

---

### 7. TemplateRef Type Structure

#### Test 7.1: TemplateRef With Name Field Works
**Goal**: Verify new type structure
**Steps**:
1. Create template
2. Create workspace with templateRef.name="template-name"
3. Verify workspace created successfully
4. Verify workspace.spec.templateRef.name accessible
**Expected**: New TemplateRef{Name} structure works
**Static Files**: `static/templateref-type/template.yaml`, `static/templateref-type/workspace-new-type.yaml`

#### Test 7.2: Template Label Set Correctly
**Goal**: Verify label-based lookup works with new type
**Steps**:
1. Create workspace with templateRef.name="my-template"
2. Verify label workspace.jupyter.org/template="my-template" set
3. Query workspaces by label, verify found
**Expected**: Label-based queries work (needed for compliance checking)
**Static Files**: `static/templateref-type/template.yaml`, `static/templateref-type/workspace-labels.yaml`

---

### 8. Backward Compatibility

#### Test 8.1: Existing Validation Rules Unchanged
**Goal**: Verify image allowlist still enforced
**Steps**:
1. Create template with allowedImages=["image-a"]
2. Attempt to create workspace with image="image-b"
3. Verify webhook rejects
**Expected**: Validation unchanged despite mutability
**Static Files**: `static/backward-compat/template-restrictive.yaml`, `static/backward-compat/workspace-invalid-image.yaml`

#### Test 8.2: Resource Bounds Still Enforced
**Goal**: Verify bounds checking unchanged
**Steps**:
1. Create template with CPU max=1
2. Attempt to create workspace with CPU=2
3. Verify webhook rejects
**Expected**: Bounds enforcement unchanged
**Static Files**: `static/backward-compat/template-bounded.yaml`, `static/backward-compat/workspace-exceeds-bounds.yaml`

---

## Test Execution Order

1. **Setup**: Deploy controller, install CRDs, wait for webhooks
2. **Smoke tests**: Template mutability, TemplateRef mutability (fast validation)
3. **Core behavior**: Lazy application, compliance checking (state transitions)
4. **Edge cases**: Validation architecture, deletion protection
5. **Regression**: Backward compatibility
6. **Teardown**: Clean up resources

---

## Performance Considerations

- **Compliance checking**: Should complete within 10-15 seconds per template update
- **Minimal API calls**: Verify controller uses 1 GET + 1 LIST pattern
- **No thundering herd**: Modifying 1 template shouldn't cause N workspace reconciles

---

## Test Data Philosophy

- **Static YAML files**: All test resources defined in `test/e2e/static/`
- **Descriptive names**: Files indicate purpose (e.g., `workspace-becomes-noncompliant.yaml`)
- **Minimal**: Each file contains only fields needed for test
- **Reusable**: Common templates shared across related tests
- **Versioned**: Templates with v1/v2 naming for modification tests

---

## Success Criteria

✅ All 25+ tests pass
✅ No flaky tests (100% pass rate on 3 consecutive runs)
✅ Tests complete in <10 minutes total
✅ Clear failure messages for debugging
✅ Static files cover all code paths
