# MarkLogic Kubernetes Operator - Custom Subresources for Upgrade Control

## Overview

The MarkLogic Kubernetes operator now supports custom subresources and API extensions that provide interactive control over rolling upgrade flows. This implementation allows for pause/resume functionality, rollback capabilities, and interactive upgrade workflows using standard Kubernetes custom resources.

## Architecture

The upgrade control system consists of:

1. **Custom Resource Definitions (CRDs)**:
   - `MarklogicClusterUpgrade` - Controls upgrade operations
   - `MarklogicClusterRollback` - Controls rollback operations

2. **Controllers**:
   - `MarklogicClusterUpgradeReconciler` - Handles upgrade control actions
   - `MarklogicClusterRollbackReconciler` - Handles rollback operations

3. **Status Subresources**: Provide real-time status and progress information

## Custom Resources

### MarklogicClusterUpgrade

This resource provides interactive control over upgrade operations. The cluster name is extracted from the resource name using the convention `{cluster-name}-upgrade`.

**Example:**
```yaml
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-upgrade  # Targets cluster named "my-cluster"
  namespace: default
spec:
  action: pause
  reason: "Maintenance window - pausing for troubleshooting"
  requestedBy: "admin@company.com"
  force: false
```

**Supported Actions:**
- `pause` - Pause an ongoing upgrade
- `resume` - Resume a paused upgrade
- `cancel` - Cancel an upgrade in progress
- `retry` - Retry a failed upgrade
- `force-proceed` - Force an upgrade to proceed despite warnings

### MarklogicClusterRollback

This resource provides rollback capabilities with different strategies.

**Example:**
```yaml
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterRollback
metadata:
  name: my-cluster-rollback  # Targets cluster named "my-cluster"
  namespace: default
spec:
  strategy: graceful
  targetImage: "marklogicdb/marklogic-db:11.2.0"
  reason: "Rolling back due to performance issues"
  requestedBy: "ops-team@company.com"
```

**Rollback Strategies:**
- `immediate` - Immediate rollback without graceful shutdown
- `graceful` - Graceful rollback with proper shutdown sequence  
- `manual` - Manual rollback requiring operator intervention

## Implementation Details

### Controllers

The controllers use annotations on the target MarklogicCluster to communicate state:

**Upgrade State Annotations:**
- `marklogic.com/upgrade-state` - Current upgrade state
- `marklogic.com/upgrade-paused` - Indicates if upgrade is paused
- `marklogic.com/upgrade-pause-reason` - Reason for pause
- `marklogic.com/upgrade-pause-time` - When upgrade was paused
- `marklogic.com/upgrade-resume-time` - When upgrade was resumed

**Rollback State Annotations:**
- `marklogic.com/rollback-state` - Current rollback state
- `marklogic.com/rollback-strategy` - Rollback strategy being used
- `marklogic.com/rollback-target-image` - Target image for rollback

### Status Tracking

Both resources provide comprehensive status information through their status subresources:

**UpgradeStatus fields:**
- `progress.phase` - Current upgrade phase (idle, precheck, waiting-approval, in-progress, completed, failed, cancelled, paused)
- `progress.progress` - Progress percentage (0-100)
- `progress.message` - Human-readable status message
- `progress.currentStep` - Current operation being performed
- `retryCount` - Number of retry attempts
- `canPause`, `canCancel`, `canRollback` - Available actions

**RollbackStatus fields:**
- `phase` - Current rollback phase
- `progress` - Progress percentage
- `message` - Status information
- `startTime`, `completionTime` - Timing information
- `error` - Error message if rollback failed

## Usage Workflow

### 1. Start an Interactive Upgrade

```bash
# Update cluster image to trigger upgrade
kubectl patch marklogiccluster my-cluster -p '{"spec":{"image":"marklogicdb/marklogic-db:11.3.0"}}' --type=merge

# Monitor upgrade state
kubectl get marklogiccluster my-cluster -o jsonpath='{.metadata.annotations.marklogic\.com/upgrade-state}'
```

### 2. Pause an Ongoing Upgrade

```bash
kubectl apply -f - <<EOF
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-upgrade
  namespace: default
spec:
  action: pause
  reason: "Need to investigate resource usage"
  requestedBy: "ops-team@company.com"
EOF
```

### 3. Resume a Paused Upgrade

```bash
kubectl apply -f - <<EOF
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-upgrade-resume
  namespace: default
spec:
  action: resume
  reason: "Investigation complete"
  requestedBy: "ops-team@company.com"
EOF
```

### 4. Rollback to Previous Version

```bash
kubectl apply -f - <<EOF
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterRollback
metadata:
  name: my-cluster-rollback
  namespace: default
spec:
  strategy: graceful
  targetImage: "marklogicdb/marklogic-db:11.2.0"
  reason: "Performance regression detected"
  requestedBy: "ops-team@company.com"
EOF
```

### 5. Monitor Status

```bash
# Check upgrade status
kubectl get marklogicclusterupgrade my-cluster-upgrade -o yaml

# Check rollback status  
kubectl get marklogicclusterrollback my-cluster-rollback -o yaml

# Watch cluster annotations
kubectl get marklogiccluster my-cluster -o jsonpath='{.metadata.annotations}' | jq
```

## Integration Points

### Existing Upgrade Handler

The custom resources integrate with the existing `UpgradeHandler` in `/internal/handler/upgrade_handler.go` by:

1. **Setting annotations** on the target cluster to control upgrade flow
2. **Reading upgrade state** from cluster annotations
3. **Updating status** based on cluster state changes

### Main Cluster Controller

The `MarklogicClusterReconciler` checks for these annotations during reconciliation and adjusts its upgrade behavior accordingly.

## Error Handling

The controllers include comprehensive error handling:

- **Invalid actions** for current state are rejected with clear error messages
- **Missing target clusters** result in failed status with appropriate message
- **State validation** ensures actions are only performed when safe
- **Retry logic** handles transient failures
- **Status updates** provide detailed error information

## Testing

Integration tests verify:
- Pause/resume workflow
- Cancel operations
- Rollback functionality
- Status updates
- Error conditions

Run tests with:
```bash
make test
```

## Future Enhancements

Potential improvements include:
- **Webhook validation** for upgrade control resources
- **Advanced scheduling** for maintenance windows
- **Multi-cluster coordination** for complex topologies
- **Automated rollback triggers** based on health metrics
- **Approval workflows** with RBAC integration

## Files Modified/Created

- `/api/v1/upgrade_types.go` - Custom resource definitions
- `/internal/controller/marklogicclusterupgrade_controller.go` - Upgrade controller
- `/internal/controller/marklogicclusterrollback_controller.go` - Rollback controller
- `/internal/handler/upgrade_handler.go` - Updated annotation constants
- `/cmd/main.go` - Controller registration
- `/config/crd/bases/` - Generated CRD manifests
- `/config/samples/upgrade-control-examples.yaml` - Usage examples
- `/docs/UPGRADE_CONTROL.md` - User documentation

This implementation provides a robust foundation for interactive upgrade control that integrates seamlessly with existing Kubernetes workflows and tooling.
