package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	marklogicv1 "github.com/marklogic/marklogic-operator-kubernetes/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpgradeHandler handles the interactive upgrade workflow
type UpgradeHandler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
}

// UpgradeState represents the current state of the upgrade process
type UpgradeState string

const (
	UpgradeStateIdle            UpgradeState = "Idle"
	UpgradeStatePrecheckStart   UpgradeState = "PrecheckStarted"
	UpgradeStatePrecheckDone    UpgradeState = "PrecheckCompleted"
	UpgradeStatePrecheck        UpgradeState = "PrecheckStarted"
	UpgradeStateWaitingUser     UpgradeState = "WaitingForUserApproval"
	UpgradeStateWaitingApproval UpgradeState = "WaitingForUserApproval"
	UpgradeStateInProgress      UpgradeState = "UpgradeInProgress"
	UpgradeStateCompleted       UpgradeState = "UpgradeCompleted"
	UpgradeStateFailed          UpgradeState = "UpgradeFailed"
	UpgradeStateCancelled       UpgradeState = "UpgradeCancelled"
	UpgradeStatePaused          UpgradeState = "UpgradePaused"
)

// Annotation keys for upgrade control
const (
	AnnotationTriggerUpgrade  = "marklogic.com/trigger-upgrade"
	AnnotationProceedUpgrade  = "marklogic.com/proceed-with-upgrade"
	AnnotationCancelUpgrade   = "marklogic.com/cancel-upgrade"
	AnnotationUpgradeState    = "marklogic.com/upgrade-state"
	AnnotationPrecheckResults = "marklogic.com/precheck-results"
	AnnotationSkipForestCheck = "marklogic.com/skip-forest-check"
	AnnotationUpgradePaused   = "marklogic.com/upgrade-paused"

	// Pause/Resume related annotations
	AnnotationUpgradePauseReason = "marklogic.com/upgrade-pause-reason"
	AnnotationUpgradePauseTime   = "marklogic.com/upgrade-pause-time"
	AnnotationUpgradePauseUser   = "marklogic.com/upgrade-pause-user"
	AnnotationUpgradeResumeTime  = "marklogic.com/upgrade-resume-time"
	AnnotationUpgradeResumeUser  = "marklogic.com/upgrade-resume-user"

	// Retry related annotations
	AnnotationUpgradeRetryCount = "marklogic.com/upgrade-retry-count"
	AnnotationUpgradeRetryTime  = "marklogic.com/upgrade-retry-time"
	AnnotationUpgradeRetryUser  = "marklogic.com/upgrade-retry-user"

	// Force proceed related annotations
	AnnotationUpgradeForceTime   = "marklogic.com/upgrade-force-time"
	AnnotationUpgradeForceUser   = "marklogic.com/upgrade-force-user"
	AnnotationUpgradeForceReason = "marklogic.com/upgrade-force-reason"

	// Rollback related annotations
	AnnotationRollbackState       = "marklogic.com/rollback-state"
	AnnotationRollbackStrategy    = "marklogic.com/rollback-strategy"
	AnnotationRollbackTargetImage = "marklogic.com/rollback-target-image"
	AnnotationRollbackTime        = "marklogic.com/rollback-time"
	AnnotationRollbackUser        = "marklogic.com/rollback-user"
	AnnotationRollbackReason      = "marklogic.com/rollback-reason"
	AnnotationRollbackStartTime   = "marklogic.com/rollback-start-time"
	AnnotationRollbackRequestedBy = "marklogic.com/rollback-requested-by"

	// Additional upgrade annotations
	AnnotationUpgradePreviousImage      = "marklogic.com/upgrade-previous-image"
	AnnotationUpgradeNeedsApproval      = "marklogic.com/upgrade-needs-approval"
	AnnotationUpgradeResumeReason       = "marklogic.com/upgrade-resume-reason"
	AnnotationUpgradeCancelReason       = "marklogic.com/upgrade-cancel-reason"
	AnnotationUpgradeCancelTime         = "marklogic.com/upgrade-cancel-time"
	AnnotationUpgradeCancelUser         = "marklogic.com/upgrade-cancel-user"
	AnnotationUpgradeRetryReason        = "marklogic.com/upgrade-retry-reason"
	AnnotationUpgradeForceProceedReason = "marklogic.com/upgrade-force-proceed-reason"
	AnnotationUpgradeForceProceedTime   = "marklogic.com/upgrade-force-proceed-time"
	AnnotationUpgradeForceProceedUser   = "marklogic.com/upgrade-force-proceed-user"
)

// NewUpgradeHandler creates a new UpgradeHandler
func NewUpgradeHandler(client client.Client, log logr.Logger, recorder record.EventRecorder) *UpgradeHandler {
	return &UpgradeHandler{
		Client:   client,
		Log:      log,
		Recorder: recorder,
	}
}

// HandleUpgradeWorkflow manages the interactive upgrade workflow based on image changes
func (h *UpgradeHandler) HandleUpgradeWorkflow(ctx context.Context, cluster *marklogicv1.MarklogicCluster) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	annotations := cluster.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	currentState := UpgradeState(annotations[AnnotationUpgradeState])
	if currentState == "" {
		currentState = UpgradeStateIdle
	}

	// For new clusters that aren't deployed yet, update status with current image
	if currentState == UpgradeStateIdle && !h.isClusterDeployed(cluster) {
		log.Info("New cluster detected, updating status with current image")
		if err := h.updateStatusAfterDeployment(ctx, cluster); err != nil {
			log.Error(err, "Failed to update status for new cluster")
			return ctrl.Result{}, err
		}
		// Don't trigger upgrade workflow for new clusters
		return ctrl.Result{}, nil
	}

	// Check for image changes and trigger upgrade automatically
	// Only check for image changes if the cluster is already deployed (has current image in status)
	if currentState == UpgradeStateIdle && h.isClusterDeployed(cluster) {
		imageChanged := h.detectImageChanges(cluster)
		if imageChanged {
			log.Info("Image change detected, triggering upgrade", "currentImage", cluster.Status.CurrentImage, "newImage", cluster.Spec.Image)
			// Set trigger annotation automatically
			annotations[AnnotationTriggerUpgrade] = "true"
			cluster.SetAnnotations(annotations)
			if err := h.Update(ctx, cluster); err != nil {
				log.Error(err, "Failed to set trigger annotation for image change")
				return ctrl.Result{}, err
			}
			// Continue to process the trigger
		}
	}

	// Early return for idle state with no trigger
	if currentState == UpgradeStateIdle && annotations[AnnotationTriggerUpgrade] != "true" {
		// No upgrade activity, don't requeue
		return ctrl.Result{}, nil
	}

	log.Info("Processing upgrade workflow", "currentState", currentState)

	// Check for cancellation first
	if annotations[AnnotationCancelUpgrade] == "true" {
		return h.handleCancellation(ctx, cluster, annotations)
	}

	switch currentState {
	case UpgradeStateIdle:
		return h.handleIdleState(ctx, cluster, annotations)
	case UpgradeStatePrecheckStart:
		return h.handlePrecheckStartState(ctx, cluster, annotations)
	case UpgradeStatePrecheckDone:
		return h.handlePrecheckDoneState(ctx, cluster, annotations)
	case UpgradeStateWaitingUser:
		return h.handleWaitingUserState(ctx, cluster, annotations)
	case UpgradeStateInProgress:
		return h.handleInProgressState(ctx, cluster, annotations)
	case UpgradeStateCompleted, UpgradeStateFailed, UpgradeStateCancelled:
		// These are terminal states, clean up and return to idle
		return h.cleanupUpgradeAnnotations(ctx, cluster, UpgradeStateIdle)
	default:
		log.Info("Unknown upgrade state, resetting to idle", "state", currentState)
		return h.updateUpgradeState(ctx, cluster, UpgradeStateIdle)
	}
}

// handleIdleState processes the idle state - waiting for trigger
func (h *UpgradeHandler) handleIdleState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	// Check if upgrade is triggered
	if annotations[AnnotationTriggerUpgrade] == "true" {
		log.Info("Upgrade triggered, starting prechecks")

		// Start prechecks
		precheckHandler := NewPrecheckHandler(h.Client, h.Log, h.Recorder)
		if err := precheckHandler.StartPrechecks(ctx, cluster); err != nil {
			log.Error(err, "Failed to start prechecks")
			h.Recorder.Event(cluster, corev1.EventTypeWarning, "PrecheckFailed", fmt.Sprintf("Failed to start prechecks: %v", err))
			return h.updateUpgradeState(ctx, cluster, UpgradeStateFailed)
		}

		h.Recorder.Event(cluster, corev1.EventTypeNormal, "PrecheckStarted", "Upgrade prechecks started")
		return h.updateUpgradeState(ctx, cluster, UpgradeStatePrecheckStart)
	}

	return ctrl.Result{}, nil
}

// handlePrecheckStartState monitors precheck progress
func (h *UpgradeHandler) handlePrecheckStartState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	precheckHandler := NewPrecheckHandler(h.Client, h.Log, h.Recorder)
	completed, results, err := precheckHandler.CheckPrecheckStatus(ctx, cluster)

	if err != nil {
		log.Error(err, "Error checking precheck status")
		h.Recorder.Event(cluster, corev1.EventTypeWarning, "PrecheckError", fmt.Sprintf("Error during prechecks: %v", err))
		return h.updateUpgradeState(ctx, cluster, UpgradeStateFailed)
	}

	if !completed {
		log.V(1).Info("Prechecks still in progress")
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil // Check again in 2 minutes
	}

	// Prechecks completed - store results and transition to done state
	log.Info("Prechecks completed", "results", results)
	h.Recorder.Event(cluster, corev1.EventTypeNormal, "PrecheckCompleted", fmt.Sprintf("Prechecks completed with %d checks", len(results.Results)))

	return h.updateUpgradeStateWithResults(ctx, cluster, UpgradeStatePrecheckDone, results)
}

// handlePrecheckDoneState presents results and waits for user decision
func (h *UpgradeHandler) handlePrecheckDoneState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	// Display precheck results in event
	resultsJson := annotations[AnnotationPrecheckResults]
	log.Info("Precheck results available for review", "results", resultsJson)

	h.Recorder.Event(cluster, corev1.EventTypeNormal, "AwaitingApproval",
		fmt.Sprintf("Prechecks completed. Review results and set annotation %s=true to proceed or %s=true to cancel",
			AnnotationProceedUpgrade, AnnotationCancelUpgrade))

	return h.updateUpgradeState(ctx, cluster, UpgradeStateWaitingUser)
}

// handleWaitingUserState waits for user approval or cancellation
func (h *UpgradeHandler) handleWaitingUserState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	if annotations[AnnotationProceedUpgrade] == "true" {
		log.Info("User approved upgrade, starting rolling upgrade")
		h.Recorder.Event(cluster, corev1.EventTypeNormal, "UpgradeApproved", "User approved upgrade, starting rolling upgrade")

		// Start rolling upgrade
		rollingHandler := NewRollingUpgradeHandler(h.Client, h.Log, h.Recorder)
		if err := rollingHandler.StartRollingUpgrade(ctx, cluster); err != nil {
			log.Error(err, "Failed to start rolling upgrade")
			h.Recorder.Event(cluster, corev1.EventTypeWarning, "UpgradeFailed", fmt.Sprintf("Failed to start rolling upgrade: %v", err))
			return h.updateUpgradeState(ctx, cluster, UpgradeStateFailed)
		}

		return h.updateUpgradeState(ctx, cluster, UpgradeStateInProgress)
	}

	// Still waiting for user input
	log.V(1).Info("Waiting for user approval or cancellation")
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil // Check again in 5 minutes
}

// handleInProgressState monitors rolling upgrade progress
func (h *UpgradeHandler) handleInProgressState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	rollingHandler := NewRollingUpgradeHandler(h.Client, h.Log, h.Recorder)
	completed, err := rollingHandler.CheckUpgradeStatus(ctx, cluster)

	if err != nil {
		log.Error(err, "Error during rolling upgrade")
		h.Recorder.Event(cluster, corev1.EventTypeWarning, "UpgradeError", fmt.Sprintf("Error during rolling upgrade: %v", err))
		return h.updateUpgradeState(ctx, cluster, UpgradeStateFailed)
	}

	if !completed {
		log.V(1).Info("Rolling upgrade still in progress")
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil // Check again in 2 minutes
	}

	log.Info("Rolling upgrade completed successfully")
	h.Recorder.Event(cluster, corev1.EventTypeNormal, "UpgradeCompleted", "Rolling upgrade completed successfully")

	// Update current images in status to reflect successful upgrade
	if err := h.updateCurrentImages(ctx, cluster); err != nil {
		log.Error(err, "Failed to update current images after upgrade completion")
		// Don't fail the upgrade completion for this error
	}

	return h.cleanupUpgradeAnnotations(ctx, cluster, UpgradeStateCompleted)
}

// handleCancellation processes upgrade cancellation
func (h *UpgradeHandler) handleCancellation(ctx context.Context, cluster *marklogicv1.MarklogicCluster, annotations map[string]string) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	currentState := UpgradeState(annotations[AnnotationUpgradeState])
	log.Info("Upgrade cancellation requested", "currentState", currentState)

	if currentState == UpgradeStateInProgress {
		// Cannot cancel in-progress upgrade safely
		h.Recorder.Event(cluster, corev1.EventTypeWarning, "CancellationDenied",
			"Cannot cancel upgrade while rolling upgrade is in progress")
		return ctrl.Result{}, nil
	}

	h.Recorder.Event(cluster, corev1.EventTypeNormal, "UpgradeCancelled", "Upgrade cancelled by user")
	return h.cleanupUpgradeAnnotations(ctx, cluster, UpgradeStateCancelled)
}

// updateUpgradeState updates the upgrade state annotation and cluster status
func (h *UpgradeHandler) updateUpgradeState(ctx context.Context, cluster *marklogicv1.MarklogicCluster, state UpgradeState) (ctrl.Result, error) {
	return h.updateUpgradeStateWithResults(ctx, cluster, state, nil)
}

// updateUpgradeStateWithResults updates state and optionally stores precheck results
func (h *UpgradeHandler) updateUpgradeStateWithResults(ctx context.Context, cluster *marklogicv1.MarklogicCluster, state UpgradeState, results *PrecheckResults) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	annotations := cluster.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[AnnotationUpgradeState] = string(state)

	if results != nil {
		// Store precheck results as JSON annotation
		resultsJson, err := results.ToJSON()
		if err != nil {
			log.Error(err, "Failed to serialize precheck results")
		} else {
			annotations[AnnotationPrecheckResults] = resultsJson
		}
	}

	cluster.SetAnnotations(annotations)

	// Update status fields
	cluster.Status.UpgradePaused = (state == UpgradeStateWaitingUser)
	cluster.Status.UpgradeState = string(state)

	// Update condition
	condition := metav1.Condition{
		Type:               "UpgradeInProgress",
		Status:             metav1.ConditionTrue,
		Reason:             string(state),
		Message:            fmt.Sprintf("Upgrade workflow in state: %s", state),
		LastTransitionTime: metav1.Now(),
	}

	if state == UpgradeStateCompleted || state == UpgradeStateFailed || state == UpgradeStateCancelled {
		condition.Status = metav1.ConditionFalse
		// Set last upgrade time for completed states
		if state == UpgradeStateCompleted {
			now := metav1.Now()
			cluster.Status.LastUpgradeTime = &now
		}
	}

	// Update or add condition
	cluster.Status.Conditions = updateCondition(cluster.Status.Conditions, condition)

	if err := h.Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update cluster annotations")
		return ctrl.Result{}, err
	}

	if err := h.Status().Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update cluster status")
		return ctrl.Result{}, err
	}

	log.Info("Updated upgrade state", "newState", state)
	return ctrl.Result{}, nil
}

// cleanupUpgradeAnnotations removes upgrade-related annotations after completion
func (h *UpgradeHandler) cleanupUpgradeAnnotations(ctx context.Context, cluster *marklogicv1.MarklogicCluster, finalState UpgradeState) (ctrl.Result, error) {
	log := h.Log.WithValues("cluster", cluster.Name)

	annotations := cluster.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Set final state but remove control annotations
	annotations[AnnotationUpgradeState] = string(finalState)
	delete(annotations, AnnotationTriggerUpgrade)
	delete(annotations, AnnotationProceedUpgrade)
	delete(annotations, AnnotationCancelUpgrade)
	// Keep precheck results for reference

	cluster.SetAnnotations(annotations)
	cluster.Status.UpgradePaused = false
	cluster.Status.UpgradeState = string(finalState)

	// Set last upgrade time for completed state
	if finalState == UpgradeStateCompleted {
		now := metav1.Now()
		cluster.Status.LastUpgradeTime = &now
	}

	// Update final condition
	condition := metav1.Condition{
		Type:               "UpgradeInProgress",
		Status:             metav1.ConditionFalse,
		Reason:             string(finalState),
		Message:            fmt.Sprintf("Upgrade workflow completed with state: %s", finalState),
		LastTransitionTime: metav1.Now(),
	}

	cluster.Status.Conditions = updateCondition(cluster.Status.Conditions, condition)

	if err := h.Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to cleanup annotations")
		return ctrl.Result{}, err
	}

	if err := h.Status().Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update final status")
		return ctrl.Result{}, err
	}

	log.Info("Cleaned up upgrade workflow", "finalState", finalState)
	return ctrl.Result{}, nil
}

// updateCondition updates or adds a condition to the conditions slice
func updateCondition(conditions []metav1.Condition, newCondition metav1.Condition) []metav1.Condition {
	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			conditions[i] = newCondition
			return conditions
		}
	}
	return append(conditions, newCondition)
}

// detectImageChanges checks if the cluster image has changed compared to the current status
func (h *UpgradeHandler) detectImageChanges(cluster *marklogicv1.MarklogicCluster) bool {
	// Only check cluster-level image, not group images
	currentImage := cluster.Status.CurrentImage
	desiredImage := cluster.Spec.Image

	// If current image is empty, cluster is not deployed yet
	if currentImage == "" {
		return false
	}

	// Return true if images are different
	return currentImage != desiredImage
}

// updateCurrentImages updates the status with the current deployed images
func (h *UpgradeHandler) updateCurrentImages(ctx context.Context, cluster *marklogicv1.MarklogicCluster) error {
	log := h.Log.WithValues("cluster", cluster.Name)

	// Update current image in status to reflect successful upgrade
	cluster.Status.CurrentImage = cluster.Spec.Image

	// Update the status
	if err := h.Status().Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update current image in status")
		return err
	}

	log.Info("Updated current image in status", "currentImage", cluster.Status.CurrentImage)
	return nil
}

// isClusterDeployed checks if the cluster is already deployed by looking for existing status
func (h *UpgradeHandler) isClusterDeployed(cluster *marklogicv1.MarklogicCluster) bool {
	// Check if CurrentImage exists in status (indicates initial deployment completed)
	if cluster.Status.CurrentImage != "" {
		return true
	}

	// Check if cluster is older than a minimum age (e.g., 5 minutes)
	// This prevents triggering upgrades on very fresh clusters
	if time.Since(cluster.CreationTimestamp.Time) < 5*time.Minute {
		return false
	}

	// Check for deployment-related conditions
	for _, condition := range cluster.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return true
		}
		if condition.Type == "Deployed" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}

	// If cluster is older than 5 minutes but no deployment indicators,
	// assume it might be deployed (for backward compatibility)
	return time.Since(cluster.CreationTimestamp.Time) > 5*time.Minute
}

// updateStatusAfterDeployment updates the status with current deployed images
func (h *UpgradeHandler) updateStatusAfterDeployment(ctx context.Context, cluster *marklogicv1.MarklogicCluster) error {
	log := h.Log.WithValues("cluster", cluster.Name)

	// Update current image in status to reflect what's deployed
	cluster.Status.CurrentImage = cluster.Spec.Image

	if err := h.Status().Update(ctx, cluster); err != nil {
		log.Error(err, "Failed to update current image in status")
		return err
	}

	log.Info("Updated current image in status", "currentImage", cluster.Status.CurrentImage)
	return nil
}
