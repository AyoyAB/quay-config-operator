/*
Copyright 2024 ayoy.

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
	"net/url"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	quayv1alpha1 "github.com/ayoy/quay-config-operator/api/v1alpha1"
	"github.com/ayoy/quay-config-operator/internal/quay"
)

const (
	finalizerName    = "quay.ayoy.se/finalizer"
	conditionSuccess = "Successful"
	conditionRunning = "Running"
	requeueDelay     = 30 * time.Second
)

// QuayClientFunc is a function type for creating Quay API clients.
type QuayClientFunc func(baseURL, token string, opts ...quay.ClientOption) *quay.Client

// RepositoryMirrorReconciler reconciles a RepositoryMirror object.
type RepositoryMirrorReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	QuayClientFunc QuayClientFunc
}

// +kubebuilder:rbac:groups=quay.ayoy.se,resources=repositorymirrors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=quay.ayoy.se,resources=repositorymirrors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=quay.ayoy.se,resources=repositorymirrors/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile handles reconciliation of RepositoryMirror custom resources.
func (r *RepositoryMirrorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var mirror quayv1alpha1.RepositoryMirror
	if err := r.Get(ctx, req.NamespacedName, &mirror); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get RepositoryMirror: %w", err)
	}

	// Build Quay client from connSecretRef
	quayClient, err := r.buildQuayClient(ctx, &mirror)
	if err != nil {
		logger.Error(err, "failed to build Quay client")
		r.setCondition(&mirror, conditionSuccess, metav1.ConditionFalse, "SecretError", err.Error())
		if updateErr := r.Status().Update(ctx, &mirror); updateErr != nil {
			logger.Error(updateErr, "failed to update status")
		}
		return ctrl.Result{RequeueAfter: requeueDelay}, nil
	}

	// Parse repo name into namespace + shortname
	namespace, repoName, err := parseRepoName(mirror.Spec.Name)
	if err != nil {
		logger.Error(err, "invalid repository name")
		r.setCondition(&mirror, conditionSuccess, metav1.ConditionFalse, "InvalidName", err.Error())
		if updateErr := r.Status().Update(ctx, &mirror); updateErr != nil {
			logger.Error(updateErr, "failed to update status")
		}
		return ctrl.Result{}, nil // Don't requeue for invalid name
	}

	// Handle deletion
	if !mirror.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &mirror, quayClient, namespace, repoName)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(&mirror, finalizerName) {
		controllerutil.AddFinalizer(&mirror, finalizerName)
		if err := r.Update(ctx, &mirror); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Reconcile mirror in Quay
	return r.reconcileMirror(ctx, &mirror, quayClient, namespace, repoName)
}

func (r *RepositoryMirrorReconciler) handleDeletion(
	ctx context.Context,
	mirror *quayv1alpha1.RepositoryMirror,
	quayClient *quay.Client,
	namespace, repoName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(mirror, finalizerName) {
		preserveInQuay := mirror.Spec.PreserveInQuayOnDeletion != nil && *mirror.Spec.PreserveInQuayOnDeletion

		if !preserveInQuay {
			// Disable mirror in Quay (mirrors can't be deleted, only disabled)
			isEnabled := false
			updateReq := &quay.MirrorUpdateRequest{
				IsEnabled: &isEnabled,
			}
			if err := quayClient.UpdateMirror(ctx, namespace, repoName, updateReq); err != nil {
				logger.Error(err, "failed to disable mirror in Quay")
				return ctrl.Result{RequeueAfter: requeueDelay}, nil
			}
			logger.Info("disabled mirror in Quay", "repository", mirror.Spec.Name)
		}

		controllerutil.RemoveFinalizer(mirror, finalizerName)
		if err := r.Update(ctx, mirror); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *RepositoryMirrorReconciler) reconcileMirror(
	ctx context.Context,
	mirror *quayv1alpha1.RepositoryMirror,
	quayClient *quay.Client,
	namespace, repoName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.setCondition(mirror, conditionRunning, metav1.ConditionTrue, "Reconciling", "Reconciling mirror configuration")

	// Get existing mirror from Quay
	existing, err := quayClient.GetMirror(ctx, namespace, repoName)
	if err != nil {
		logger.Error(err, "failed to get mirror from Quay")
		r.setCondition(mirror, conditionSuccess, metav1.ConditionFalse, "QuayAPIError", err.Error())
		mirror.Status.Message = err.Error()
		if updateErr := r.Status().Update(ctx, mirror); updateErr != nil {
			logger.Error(updateErr, "failed to update status")
		}
		return ctrl.Result{RequeueAfter: requeueDelay}, nil
	}

	if existing == nil {
		// Create mirror
		if err := r.createMirror(ctx, mirror, quayClient, namespace, repoName); err != nil {
			logger.Error(err, "failed to create mirror in Quay")
			r.setCondition(mirror, conditionSuccess, metav1.ConditionFalse, "CreateError", err.Error())
			mirror.Status.Message = err.Error()
			if updateErr := r.Status().Update(ctx, mirror); updateErr != nil {
				logger.Error(updateErr, "failed to update status")
			}
			return ctrl.Result{RequeueAfter: requeueDelay}, nil
		}
		logger.Info("created mirror in Quay", "repository", mirror.Spec.Name)
	} else {
		// Update mirror
		if err := r.updateMirror(ctx, mirror, quayClient, namespace, repoName); err != nil {
			logger.Error(err, "failed to update mirror in Quay")
			r.setCondition(mirror, conditionSuccess, metav1.ConditionFalse, "UpdateError", err.Error())
			mirror.Status.Message = err.Error()
			if updateErr := r.Status().Update(ctx, mirror); updateErr != nil {
				logger.Error(updateErr, "failed to update status")
			}
			return ctrl.Result{RequeueAfter: requeueDelay}, nil
		}
		logger.Info("updated mirror in Quay", "repository", mirror.Spec.Name)
	}

	// Handle forceSync
	if mirror.Spec.ForceSync != nil && *mirror.Spec.ForceSync {
		if err := quayClient.SyncNow(ctx, namespace, repoName); err != nil {
			logger.Error(err, "failed to trigger sync")
			r.setCondition(mirror, conditionSuccess, metav1.ConditionFalse, "SyncError", err.Error())
			mirror.Status.Message = err.Error()
			if updateErr := r.Status().Update(ctx, mirror); updateErr != nil {
				logger.Error(updateErr, "failed to update status")
			}
			return ctrl.Result{RequeueAfter: requeueDelay}, nil
		}
		logger.Info("triggered immediate sync", "repository", mirror.Spec.Name)
	}

	// Success
	mirror.Status.ExistInQuay = true
	mirror.Status.Message = "Mirror configuration reconciled successfully"
	r.setCondition(mirror, conditionSuccess, metav1.ConditionTrue, "Reconciled", "Mirror configuration reconciled successfully")
	r.setCondition(mirror, conditionRunning, metav1.ConditionFalse, "Idle", "Reconciliation complete")
	if err := r.Status().Update(ctx, mirror); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{RequeueAfter: requeueDelay}, nil
	}

	return ctrl.Result{}, nil
}

func (r *RepositoryMirrorReconciler) createMirror(
	ctx context.Context,
	mirror *quayv1alpha1.RepositoryMirror,
	quayClient *quay.Client,
	namespace, repoName string,
) error {
	syncInterval, err := quay.ParseSyncInterval(mirror.Spec.SyncInterval)
	if err != nil {
		return fmt.Errorf("invalid sync interval: %w", err)
	}

	isEnabled := false
	if mirror.Spec.IsEnabled != nil {
		isEnabled = *mirror.Spec.IsEnabled
	}

	syncStartDate := mirror.Spec.SyncStartDate
	if syncStartDate == "" {
		syncStartDate = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}

	extUsername, extPassword, err := r.getExternalRegistryCredentials(ctx, mirror)
	if err != nil {
		return fmt.Errorf("external registry credentials: %w", err)
	}

	createReq := &quay.MirrorCreateRequest{
		IsEnabled:                isEnabled,
		ExternalReference:        mirror.Spec.ExternalReference,
		SyncInterval:             syncInterval,
		SyncStartDate:            syncStartDate,
		RobotUsername:            mirror.Spec.RobotUsername,
		ExternalRegistryUsername: extUsername,
		ExternalRegistryPassword: extPassword,
	}

	// Build external registry config
	extConfig := r.buildExternalRegistryConfig(mirror)
	if extConfig != nil {
		createReq.ExternalRegistryConfig = extConfig
	}

	// Build root rule for image tags
	if len(mirror.Spec.ImageTags) > 0 {
		createReq.RootRule = &quay.RootRule{
			RuleKind:  "tag_glob_csv",
			RuleValue: mirror.Spec.ImageTags,
		}
	}

	return quayClient.CreateMirror(ctx, namespace, repoName, createReq)
}

func (r *RepositoryMirrorReconciler) updateMirror(
	ctx context.Context,
	mirror *quayv1alpha1.RepositoryMirror,
	quayClient *quay.Client,
	namespace, repoName string,
) error {
	extUsername, extPassword, err := r.getExternalRegistryCredentials(ctx, mirror)
	if err != nil {
		return fmt.Errorf("external registry credentials: %w", err)
	}

	updateReq := &quay.MirrorUpdateRequest{
		IsEnabled:                mirror.Spec.IsEnabled,
		ExternalReference:        mirror.Spec.ExternalReference,
		RobotUsername:            mirror.Spec.RobotUsername,
		SyncStartDate:            mirror.Spec.SyncStartDate,
		ExternalRegistryUsername: extUsername,
		ExternalRegistryPassword: extPassword,
	}

	if mirror.Spec.SyncInterval != "" {
		syncInterval, err := quay.ParseSyncInterval(mirror.Spec.SyncInterval)
		if err != nil {
			return fmt.Errorf("invalid sync interval: %w", err)
		}
		updateReq.SyncInterval = &syncInterval
	}

	// Build external registry config
	extConfig := r.buildExternalRegistryConfig(mirror)
	if extConfig != nil {
		updateReq.ExternalRegistryConfig = extConfig
	}

	// Build root rule for image tags
	if len(mirror.Spec.ImageTags) > 0 {
		updateReq.RootRule = &quay.RootRule{
			RuleKind:  "tag_glob_csv",
			RuleValue: mirror.Spec.ImageTags,
		}
	}

	return quayClient.UpdateMirror(ctx, namespace, repoName, updateReq)
}

func (r *RepositoryMirrorReconciler) buildExternalRegistryConfig(mirror *quayv1alpha1.RepositoryMirror) *quay.ExternalRegistryConfig {
	config := &quay.ExternalRegistryConfig{}
	hasConfig := false

	if mirror.Spec.VerifyTls != nil {
		config.VerifyTLS = mirror.Spec.VerifyTls
		hasConfig = true
	}

	if mirror.Spec.UnsignedImages != nil {
		config.UnsignedImages = mirror.Spec.UnsignedImages
		hasConfig = true
	}

	if mirror.Spec.HttpProxy != "" || mirror.Spec.HttpsProxy != "" || mirror.Spec.NoProxy != "" {
		config.Proxy = &quay.Proxy{
			HTTPProxy:  mirror.Spec.HttpProxy,
			HTTPSProxy: mirror.Spec.HttpsProxy,
			NoProxy:    mirror.Spec.NoProxy,
		}
		hasConfig = true
	}

	if hasConfig {
		return config
	}
	return nil
}

func (r *RepositoryMirrorReconciler) getExternalRegistryCredentials(ctx context.Context, mirror *quayv1alpha1.RepositoryMirror) (string, string, error) {
	if mirror.Spec.ExternalRegistryCredentialsRef == nil {
		return "", "", nil
	}

	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Name:      mirror.Spec.ExternalRegistryCredentialsRef.Name,
		Namespace: mirror.Namespace,
	}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		return "", "", fmt.Errorf("failed to get external registry credentials secret %s: %w", mirror.Spec.ExternalRegistryCredentialsRef.Name, err)
	}

	return string(secret.Data["username"]), string(secret.Data["password"]), nil
}

func (r *RepositoryMirrorReconciler) buildQuayClient(ctx context.Context, mirror *quayv1alpha1.RepositoryMirror) (*quay.Client, error) {
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Name:      mirror.Spec.ConnSecretRef.Name,
		Namespace: mirror.Namespace,
	}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("failed to get connection secret %s: %w", mirror.Spec.ConnSecretRef.Name, err)
	}

	host := string(secret.Data["host"])
	if host == "" {
		return nil, fmt.Errorf("connection secret %s is missing 'host' key", mirror.Spec.ConnSecretRef.Name)
	}

	parsedURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("connection secret %s has invalid 'host' URL: %w", mirror.Spec.ConnSecretRef.Name, err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return nil, fmt.Errorf("connection secret %s 'host' must use http or https scheme, got %q", mirror.Spec.ConnSecretRef.Name, parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("connection secret %s 'host' is missing hostname", mirror.Spec.ConnSecretRef.Name)
	}

	token := string(secret.Data["token"])
	if token == "" {
		return nil, fmt.Errorf("connection secret %s is missing 'token' key", mirror.Spec.ConnSecretRef.Name)
	}

	var opts []quay.ClientOption
	validateCerts := string(secret.Data["validateCerts"])
	if strings.EqualFold(validateCerts, "false") {
		opts = append(opts, quay.WithInsecureSkipVerify())
	}

	return r.QuayClientFunc(host, token, opts...), nil
}

func (r *RepositoryMirrorReconciler) setCondition(mirror *quayv1alpha1.RepositoryMirror, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&mirror.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: mirror.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// validRepoComponent matches safe Quay namespace/repo names (alphanumeric, hyphens, underscores, dots).
var validRepoComponent = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// parseRepoName splits "namespace/shortname" into its components.
func parseRepoName(name string) (string, string, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository name must be in 'namespace/shortname' format, got %q", name)
	}
	if !validRepoComponent.MatchString(parts[0]) || !validRepoComponent.MatchString(parts[1]) {
		return "", "", fmt.Errorf("repository name contains invalid characters, got %q", name)
	}
	return parts[0], parts[1], nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RepositoryMirrorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quayv1alpha1.RepositoryMirror{}).
		Complete(r)
}
