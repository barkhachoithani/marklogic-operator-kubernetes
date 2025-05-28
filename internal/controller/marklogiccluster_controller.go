/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	marklogicv1 "github.com/marklogic/marklogic-operator-kubernetes/api/v1"
	"github.com/marklogic/marklogic-operator-kubernetes/internal/handler"
	"github.com/marklogic/marklogic-operator-kubernetes/pkg/k8sutil"
)

// MarklogicClusterReconciler reconciles a MarklogicCluster object
type MarklogicClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=marklogic.progress.com,resources=marklogicclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=marklogic.progress.com,resources=marklogicclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=marklogic.progress.com,resources=marklogicclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MarklogicCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MarklogicClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info(fmt.Sprintf("Reconciling MarklogicCluster %s", req.NamespacedName))

	cc, err := k8sutil.CreateClusterContext(ctx, &req, r.Client, r.Scheme, r.Recorder)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MarkLogicCluster resource not found. Exiting reconcile loop since there is nothing to do")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get MarkLogicCluster resource")
		return ctrl.Result{}, err
	}

	// Get the MarkLogicCluster resource
	cluster := cc.GetMarkLogicCluster()

	// Handle interactive upgrade workflow using handlers
	upgradeHandler := handler.NewUpgradeHandler(r.Client, r.Log, r.Recorder)
	upgradeResult, err := upgradeHandler.HandleUpgradeWorkflow(ctx, cluster)
	if err != nil {
		logger.Error(err, "Error in upgrade workflow")
		return ctrl.Result{}, err
	}

	// If upgrade workflow is active, prioritize it and skip normal reconciliation
	if upgradeResult.Requeue || upgradeResult.RequeueAfter > 0 {
		logger.V(1).Info("Upgrade workflow active, requeuing")
		return upgradeResult, nil
	}

	// Only proceed with normal reconciliation if no upgrade workflow is active
	annotations := cluster.GetAnnotations()
	upgradeState := ""
	if annotations != nil {
		upgradeState = annotations[handler.AnnotationUpgradeState]
	}

	// Skip normal reconciliation during upgrade states
	if upgradeState != "" && upgradeState != string(handler.UpgradeStateIdle) &&
		upgradeState != string(handler.UpgradeStateCompleted) &&
		upgradeState != string(handler.UpgradeStateFailed) &&
		upgradeState != string(handler.UpgradeStateCancelled) {
		logger.V(1).Info("Skipping normal reconciliation during upgrade", "upgradeState", upgradeState)
		return ctrl.Result{}, nil
	}

	// Continue with normal reconciliation
	result, err := cc.ReconsileMarklogicClusterHandler()

	if err != nil {
		logger.Error(err, "Error reconciling marklogic cluster")
		return ctrl.Result{}, err
	}

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MarklogicClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&marklogicv1.MarklogicCluster{}).
		Owns(&marklogicv1.MarklogicGroup{}).
		Complete(r)
}
