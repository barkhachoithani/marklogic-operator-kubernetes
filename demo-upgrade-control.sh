#!/bin/bash

# MarkLogic Kubernetes Operator - Upgrade Control Demo
# This script demonstrates how to use custom subresources for upgrade control

set -e

CLUSTER_NAME=${1:-"demo-cluster"}
NAMESPACE=${2:-"default"}

echo "🚀 MarkLogic Upgrade Control Demo"
echo "Cluster: test-cluster"
echo "Namespace: default"
echo ""

# Function to wait for user input
wait_for_input() {
    echo "Press Enter to continue..."
    read
}

# Function to check cluster status
check_cluster_status() {
    echo "📊 Current cluster status:"
    kubectl get marklogiccluster test-cluster -n default -o jsonpath='{.metadata.annotations}' | jq '.' 2>/dev/null || echo "Cluster not found or no annotations"
    echo ""
}

# Function to check upgrade status
check_upgrade_status() {
    local upgrade_name="$1"
    echo "📈 Checking upgrade status for: $upgrade_name"
    kubectl get marklogicclusterupgrade $upgrade_name -n default -o yaml 2>/dev/null || echo "Upgrade resource not found"
    echo ""
}

echo "1️⃣  First, let's create a sample MarkLogic cluster"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicCluster
metadata:
  name: test-cluster
  namespace: default
spec:
  image: "marklogicdb/marklogic-db:11.2.0"
  auth:
    adminUsername: "admin"
    adminPassword: "admin"
  persistence:
    enabled: true
    size: "10Gi"
EOF

echo "✅ Cluster created"
check_cluster_status

echo "2️⃣  Now let's trigger an upgrade by changing the image"
wait_for_input

kubectl patch marklogiccluster test-cluster -n default -p '{"spec":{"image":"marklogicdb/marklogic-db:11.4.0"}}' --type=merge

echo "✅ Upgrade triggered"
check_cluster_status

echo "3️⃣  Let's pause the upgrade using a custom resource"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: test-cluster-upgrade
  namespace: default
spec:
  action: pause
  reason: "Demo: Pausing upgrade for inspection"
  requestedBy: "demo-user@company.com"
EOF

echo "✅ Pause request submitted"
sleep 3
check_cluster_status
check_upgrade_status "${CLUSTER_NAME}-upgrade"

echo "4️⃣  Let's resume the upgrade"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: ${CLUSTER_NAME}-upgrade-resume
  namespace: default
spec:
  action: resume
  reason: "Demo: Resuming upgrade after inspection"
  requestedBy: "demo-user@company.com"
EOF

echo "✅ Resume request submitted"
sleep 3
check_cluster_status
check_upgrade_status "${CLUSTER_NAME}-upgrade-resume"

echo "5️⃣  Let's cancel the upgrade instead"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: ${CLUSTER_NAME}-upgrade-cancel
  namespace: default
spec:
  action: cancel
  reason: "Demo: Cancelling upgrade for testing"
  requestedBy: "demo-user@company.com"
EOF

echo "✅ Cancel request submitted"
sleep 3
check_cluster_status
check_upgrade_status "${CLUSTER_NAME}-upgrade-cancel"

echo "6️⃣  Now let's demonstrate rollback functionality"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterRollback
metadata:
  name: ${CLUSTER_NAME}-rollback
  namespace: default
spec:
  strategy: graceful
  targetImage: "marklogicdb/marklogic-db:11.1.0"
  reason: "Demo: Rolling back to previous version"
  requestedBy: "demo-user@company.com"
EOF

echo "✅ Rollback request submitted"
sleep 3
check_cluster_status

echo "📊 Checking rollback status:"
kubectl get marklogicclusterrollback ${CLUSTER_NAME}-rollback -n default -o yaml 2>/dev/null || echo "Rollback resource not found"
echo ""

echo "7️⃣  Let's see all our upgrade control resources"
wait_for_input

echo "📋 All upgrade control resources:"
kubectl get marklogicclusterupgrade -n default
echo ""
kubectl get marklogicclusterrollback -n default
echo ""

echo "8️⃣  Let's force proceed with an upgrade (demonstration)"
wait_for_input

cat <<EOF | kubectl apply -f -
apiVersion: marklogic.progress.com/v1
kind: MarklogicClusterUpgrade
metadata:
  name: ${CLUSTER_NAME}-upgrade-force
  namespace: default
spec:
  action: force-proceed
  force: true
  reason: "Demo: Force proceeding despite warnings"
  requestedBy: "demo-user@company.com"
EOF

echo "✅ Force proceed request submitted"
sleep 3
check_upgrade_status "${CLUSTER_NAME}-upgrade-force"

echo "9️⃣  Cleanup demo resources"
wait_for_input

echo "🧹 Cleaning up upgrade control resources..."
kubectl delete marklogicclusterupgrade --all -n default 2>/dev/null || true
kubectl delete marklogicclusterrollback --all -n default 2>/dev/null || true

echo "🧹 Cleaning up cluster (optional - comment out if you want to keep it)..."
# kubectl delete marklogiccluster test-cluster -n default 2>/dev/null || true

echo ""
echo "🎉 Demo completed!"
echo ""
echo "📚 Key takeaways:"
echo "  • Use MarklogicClusterUpgrade resources to control upgrade flow"
echo "  • Use MarklogicClusterRollback resources for rollback operations"
echo "  • Resource names follow convention: {cluster-name}-{action}"
echo "  • Check status subresources for progress information"
echo "  • Monitor cluster annotations for real-time state"
echo ""
echo "📖 For more information, see:"
echo "  • docs/UPGRADE_CONTROL.md"
echo "  • README_UPGRADE_CONTROL.md"
echo "  • config/samples/upgrade-control-examples.yaml"
