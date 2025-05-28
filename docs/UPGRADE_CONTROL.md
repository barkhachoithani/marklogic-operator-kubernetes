# MarkLogic Kubernetes Operator - Upgrade Control

This document describes how to use custom subresources and API extensions to control the rolling upgrade flow in the MarkLogic Kubernetes operator.

## Overview

The MarkLogic operator provides interactive upgrade control through custom Kubernetes resources that allow you to:

- **Pause/Resume**: Pause an ongoing upgrade and resume it later
- **Cancel**: Cancel an upgrade in progress
- **Retry**: Retry a failed upgrade
- **Force Proceed**: Force an upgrade to proceed despite warnings
- **Rollback**: Roll back to a previous version

## Custom Resources

### MarklogicClusterUpgrade

Controls upgrade operations for a MarkLogic cluster.

```yaml
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-upgrade-pause
  namespace: default
spec:
  clusterName: my-marklogic-cluster
  action: pause
  reason: "Maintenance window - pausing for troubleshooting"
  requestedBy: "admin@company.com"
```

#### Supported Actions

- **pause**: Pause an ongoing upgrade
- **resume**: Resume a paused upgrade  
- **cancel**: Cancel an upgrade in progress
- **retry**: Retry a failed upgrade
- **force-proceed**: Force an upgrade to proceed despite warnings

### MarklogicClusterRollback

Controls rollback operations for a MarkLogic cluster.

```yaml
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterRollback
metadata:
  name: my-cluster-rollback
  namespace: default
spec:
  clusterName: my-marklogic-cluster
  strategy: graceful
  targetImage: "marklogicdb/marklogic-db:11.2.0"
  reason: "Rolling back due to compatibility issues"
  requestedBy: "admin@company.com"
```

#### Rollback Strategies

- **immediate**: Immediate rollback without graceful shutdown
- **graceful**: Graceful rollback with proper shutdown sequence
- **manual**: Manual rollback requiring operator intervention

## Usage Examples

### 1. Starting an Interactive Upgrade

First, trigger an upgrade on your MarkLogic cluster by updating the image:

```bash
kubectl patch marklogiccluster my-cluster -p '{"spec":{"image":"marklogicdb/marklogic-db:11.3.0"}}' --type=merge
```

The operator will start the upgrade process and pause at the approval stage.

### 2. Monitoring Upgrade Progress

Check the upgrade status:

```bash
kubectl get marklogiccluster my-cluster -o jsonpath='{.metadata.annotations.marklogic\.com/upgrade-state}'
```

### 3. Pausing an Ongoing Upgrade

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-pause
  namespace: default
spec:
  clusterName: my-cluster
  action: pause
  reason: "Need to investigate high CPU usage"
  requestedBy: "ops-team@company.com"
EOF
```

### 4. Resuming a Paused Upgrade

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-resume
  namespace: default
spec:
  clusterName: my-cluster
  action: resume
  reason: "Investigation complete, safe to proceed"
  requestedBy: "ops-team@company.com"
EOF
```

### 5. Cancelling an Upgrade

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-cancel
  namespace: default
spec:
  clusterName: my-cluster
  action: cancel
  reason: "Critical security patch needed first"
  requestedBy: "security-team@company.com"
EOF
```

### 6. Retrying a Failed Upgrade

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-retry
  namespace: default
spec:
  clusterName: my-cluster
  action: retry
  reason: "Transient network issue resolved"
  requestedBy: "ops-team@company.com"
EOF
```

### 7. Force Proceeding with an Upgrade

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: my-cluster-force
  namespace: default
spec:
  clusterName: my-cluster
  action: force-proceed
  force: true
  reason: "Override precheck warnings - approved by architecture team"
  requestedBy: "architect@company.com"
EOF
```

### 8. Rolling Back to Previous Version

```bash
cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterRollback
metadata:
  name: my-cluster-rollback
  namespace: default
spec:
  clusterName: my-cluster
  strategy: graceful
  targetImage: "marklogicdb/marklogic-db:11.2.0"
  reason: "New version causing performance issues"
  requestedBy: "ops-team@company.com"
EOF
```

## Checking Status

### Upgrade Status

Check the status of an upgrade control operation:

```bash
kubectl get marklogicclusterupgrade my-cluster-pause -o yaml
```

The status will show:
- Current phase (idle, precheck, waiting-approval, in-progress, completed, failed, cancelled, paused)
- Progress percentage
- Current step being executed
- Any error messages

### Rollback Status

Check the status of a rollback operation:

```bash
kubectl get marklogicclusterrollback my-cluster-rollback -o yaml
```

## Best Practices

### 1. Use Descriptive Names
Always use descriptive names for your upgrade control resources:

```yaml
metadata:
  name: cluster-prod-pause-maintenance-2024-05-28
```

### 2. Provide Clear Reasons
Always include a clear reason for the action:

```yaml
spec:
  reason: "Pausing upgrade during peak business hours to minimize impact"
```

### 3. Track Who Requested Actions
Include the requestor for audit purposes:

```yaml
spec:
  requestedBy: "john.doe@company.com"
```

### 4. Monitor Status Before Taking Actions
Always check the current upgrade state before performing actions:

```bash
kubectl get marklogiccluster my-cluster -o jsonpath='{.metadata.annotations}'
```

### 5. Clean Up Control Resources
Remove upgrade control resources after they've been processed:

```bash
kubectl delete marklogicclusterupgrade my-cluster-pause
```

## Troubleshooting

### Common Issues

1. **Action Not Allowed**: Check that the cluster is in the correct state for the requested action
2. **Resource Not Found**: Ensure the target cluster name is correct
3. **Permission Denied**: Verify RBAC permissions for the custom resources

### Checking Logs

Monitor the operator logs for detailed information:

```bash
kubectl logs -f deployment/marklogic-operator-controller-manager -n marklogic-operator-system
```

## Integration with CI/CD

These custom resources can be easily integrated into CI/CD pipelines:

```bash
# In your deployment script
kubectl apply -f upgrade-control.yaml
kubectl wait --for=condition=Complete marklogicclusterupgrade/prod-upgrade --timeout=300s
```

## API Reference

For complete API reference, see the generated CRD documentation or use:

```bash
kubectl explain marklogicclusterupgrade.spec
kubectl explain marklogicclusterrollback.spec
```
