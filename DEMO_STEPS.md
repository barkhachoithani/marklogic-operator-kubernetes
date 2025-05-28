# Demo: Simplified Rolling Upgrade Workflow
## MarkLogic Kubernetes Operator (MLE-17936)

### **Prerequisites**
- Kubernetes cluster running (Rancher Desktop/minikube/kind)
- kubectl configured and working
- MarkLogic Operator source code ready
- Terminal access

---

## **Phase 1: Environment Setup & Initial Deployment**

### **Step 1: Start the Operator Locally**
```bash
cd /Users/choithan/Library/CloudStorage/OneDrive-ProgressSoftwareCorporation/Desktop/MyMac/k8s/operator/MLE-17936/dev/marklogic-operator-kubernetes
make run ENABLE_WEBHOOKS=false
```

**What happens:**
- Operator starts in development mode on localhost:8080
- Watches for MarklogicCluster custom resources
- Begins reconciliation loop
- Logs show operator startup and readiness

---

### **Step 2: Deploy Test Cluster**
```bash
# In a new terminal
kubectl apply -f - <<EOF
apiVersion: marklogic.progress.com/v1
kind: MarklogicCluster
metadata:
  name: test-cluster
  namespace: default
spec:
  image: progressofficial/marklogic-db:11.3.0-ubi-rootless
  markLogicGroups:
  - name: node
    replicas: 1
    isBootstrap: true
    groupConfig:
      name: node
  persistence:
    enabled: true
    size: 10Gi
EOF
```

**What happens:**
- Creates a single-node MarkLogic cluster
- Uses MarkLogic 11.3.0 image as baseline
- Operator detects new cluster and begins deployment
- StatefulSet, Services, and Secrets are created
- Pod starts and MarkLogic initializes

---

### **Step 3: Verify Initial Deployment**
```bash
# Check cluster status
kubectl get marklogiccluster test-cluster -o yaml

# Check pods
kubectl get pods -l app.kubernetes.io/instance=test-cluster

# Check StatefulSet
kubectl get statefulset

# Verify current image
kubectl get statefulset node -o jsonpath='{.spec.template.spec.containers[0].image}'
```

**What happens:**
- Shows cluster is in Ready state
- Pod is running with 11.3.0 image
- No upgrade annotations present
- Baseline state established

---

## **Phase 2: Trigger Simplified Rolling Upgrade**

### **Step 4: Initiate Image Change**
```bash
# Update cluster to new image version
kubectl patch marklogiccluster test-cluster --type='merge' -p='{"spec":{"image":"progressofficial/marklogic-db:11.4.0-ubi-rootless"}}'
```

**What happens:**
- Changes cluster spec from 11.3.0 to 11.4.0
- Operator detects image change automatically
- Upgrade workflow is triggered
- `marklogic.com/trigger-upgrade=true` annotation is added automatically

---

### **Step 5: Monitor Precheck Execution**
```bash
# Watch cluster annotations for upgrade progress
kubectl get marklogiccluster test-cluster -o yaml | grep -A 10 "annotations:"

# Monitor operator logs (in operator terminal)
# Look for precheck execution logs
```

**What happens:**
- Upgrade state transitions to `PrecheckStarted`
- 8 comprehensive prechecks execute:
  - Image Change Validation
  - Cluster Health Check  
  - Database Connectivity
  - Forest Health Check
  - Resource Availability
  - Backup Status
  - License Validation
  - Network Connectivity
- Results stored in `marklogic.com/precheck-results` annotation
- State transitions to `WaitingForUserApproval`

---

### **Step 6: Review Precheck Results**
```bash
# View detailed precheck results
kubectl get marklogiccluster test-cluster -o jsonpath='{.metadata.annotations.marklogic\.com/precheck-results}' | jq

# Check current upgrade state
kubectl get marklogiccluster test-cluster -o jsonpath='{.metadata.annotations.marklogic\.com/upgrade-state}'
```

**What happens:**
- Shows detailed results of all 8 prechecks
- Typically 6 passes, 2 warnings (acceptable)
- Cluster waits for user approval
- No further progress until approval granted

---

## **Phase 3: User Approval & Upgrade Execution**

### **Step 7: Approve the Upgrade**
```bash
# Add approval annotation
kubectl annotate marklogiccluster test-cluster marklogic.com/proceed-with-upgrade=true
```

**What happens:**
- Operator detects approval annotation
- State transitions from `WaitingForUserApproval` to `UpgradeInProgress`
- Rolling upgrade handler is invoked
- StatefulSet spec is updated with new image

---

### **Step 8: Monitor Upgrade Execution**
```bash
# Watch upgrade progress in operator logs
# Monitor cluster status
kubectl get marklogiccluster test-cluster -o yaml | grep -E "(state|image)"

# Check StatefulSet update
kubectl get statefulset node -o jsonpath='{.spec.template.spec.containers[0].image}'

# Monitor cluster status
kubectl get marklogiccluster test-cluster -o jsonpath='{.status.currentImage}'
```

**What happens:**
- StatefulSet is updated with new image (11.4.0)
- Cluster status `currentImage` is updated
- Upgrade state transitions to `UpgradeCompleted`
- Annotations are cleaned up
- State returns to `Idle`

---

## **Phase 4: Verification & Pod Update**

### **Step 9: Verify Upgrade Completion**
```bash
# Check final cluster state
kubectl get marklogiccluster test-cluster -o yaml | grep -A 20 "annotations:\|status:"

# Verify StatefulSet image
kubectl get statefulset node -o jsonpath='{.spec.template.spec.containers[0].image}'

# Check current pod image (still old)
kubectl describe pod node-0 | grep "Image:"
```

**What happens:**
- Cluster status shows upgrade completed
- StatefulSet has new image configuration
- Pod still runs old image (OnDelete strategy)
- Upgrade workflow completed successfully

---

### **Step 10: Complete Pod Update**
```bash
# Delete pod to trigger recreation with new image
kubectl delete pod node-0

# Monitor pod recreation
kubectl get pods -w

# Note: This will show ImagePullBackOff because 11.4.0 image doesn't exist
# This demonstrates the workflow works correctly
```

**What happens:**
- Pod deletion triggers recreation
- StatefulSet attempts to use new image
- ImagePullBackOff occurs (expected - 11.4.0 doesn't exist)
- Proves upgrade workflow updated StatefulSet correctly

---

### **Step 11: Restore Working State**
```bash
# Revert to working image
kubectl patch marklogiccluster test-cluster --type='merge' -p='{"spec":{"image":"progressofficial/marklogic-db:11.3.0-ubi-rootless"}}'

# Clean up annotations to allow normal reconciliation
kubectl annotate marklogiccluster test-cluster marklogic.com/approve-upgrade- marklogic.com/precheck-results- marklogic.com/upgrade-state-

# Manually update StatefulSet (normal reconciliation would handle this)
kubectl patch statefulset node --type='merge' -p='{"spec":{"template":{"spec":{"containers":[{"name":"marklogic-server","image":"progressofficial/marklogic-db:11.3.0-ubi-rootless"}]}}}}'

# Delete pod to recreate with working image
kubectl delete pod node-0

# Verify restoration
kubectl get pods
```

**What happens:**
- Reverts to working 11.3.0 image
- Cleans up upgrade annotations
- Pod recreates successfully
- System returns to stable state

---

## **Demo Summary & Key Points**

### **✅ Successfully Demonstrated:**

1. **Automatic Trigger Detection**: Image changes automatically trigger upgrade workflow
2. **Comprehensive Prechecks**: 8 validation checks ensure upgrade safety
3. **User Control**: Manual approval required before execution
4. **Simplified State Management**: Reduced state complexity from original design
5. **Proper StatefulSet Updates**: Specs updated correctly with new configurations
6. **Status Tracking**: Cluster status properly reflects upgrade progress
7. **Cleanup**: Annotations and state properly managed throughout lifecycle

### **🔧 Technical Details:**

- **Update Strategy**: OnDelete requires manual pod deletion (by design)
- **State Transitions**: Idle → PrecheckStarted → PrecheckCompleted → WaitingForUserApproval → UpgradeInProgress → UpgradeCompleted → Idle
- **Annotations Used**: 
  - `marklogic.com/trigger-upgrade`
  - `marklogic.com/proceed-with-upgrade` 
  - `marklogic.com/upgrade-state`
  - `marklogic.com/precheck-results`

### **🎯 Business Value:**

- **Safety**: Comprehensive prechecks prevent dangerous upgrades
- **Control**: User approval ensures planned upgrade execution
- **Transparency**: Detailed logging and status tracking
- **Reliability**: Simplified workflow reduces complexity and failure points
- **Automation**: Minimal manual intervention required

---

## **Cleanup**
```bash
# Remove test cluster
kubectl delete marklogiccluster test-cluster

# Stop operator (Ctrl+C in operator terminal)
```

**Total Demo Time: ~10-15 minutes**
