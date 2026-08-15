/*
Copyright 2026 felipe_colussi.

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

package namespaceextenition

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	namespaceextenitionv1 "akuity/api/namespace.extension/v1"
	"akuity/internal/constants"
)

// NamespaceClassReconciler reconciles a NamespaceClass object
type NamespaceClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups=namespace.extension,resources=namespaceclasssynchronizers,verbs=get;update;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *NamespaceClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// TODO - FOR "REAL CASE" We would need to determine 2 pending behaviors:
	// A) What happens if this RUNS after the namespace is created (NS created with anotation)
	// THEN THIS hapens after.  ATM it will "reconcile" and trigger an update.
	// B) TODO - Determine what happens if this is deleted.

	log.Info("runing Reconcile", "request", req)
	nsc := new(namespaceextenitionv1.NamespaceClass{})
	if err := r.Get(ctx, req.NamespacedName, nsc); err != nil {
		log.Error(err, "unabled to get namespaceClass")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nscSynchronizerList := new(namespaceextenitionv1.NamespaceClassSynchronizerList{})

	if err := r.List(ctx,
		nscSynchronizerList,
		client.MatchingLabels{
			constants.AkuityNamespaceClassLabel: nsc.GetName(),
		},
		client.InNamespace(constants.ControllerNamespace),
	); err != nil {
		log.Error(err, "unable to find syncronizers for this object")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, nscSyncronizer := range nscSynchronizerList.Items {
		err := r.updateSynchronizer(ctx, nscSyncronizer)
		if err != nil {
			log.Error(err, "unable to update syncronizer for specific crd")
			// TODO - Add extra though on how to solve when this happens for a single CRD.
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceClassReconciler) updateSynchronizer(
	ctx context.Context,
	nscSyncronizer namespaceextenitionv1.NamespaceClassSynchronizer,
) error {

	nscSyncronizer.Spec.RequireUpdate = true
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Update(ctx, &nscSyncronizer)
	})

	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&namespaceextenitionv1.NamespaceClass{}).
		Named("namespace.extension-namespaceclass").
		Complete(r)
}
