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

package core

import (
	"akuity/internal/constants"
	"context"
	"errors"

	nscextension "akuity/api/namespace.extension/v1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=namespace.extension,resources=namespaceclasssynchronizers,verbs=get;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Namespace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("runing Reconcile", "request", req, "name", req.Name, "ns", req.Namespace)

	ns := new(corev1.Namespace{})
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		log.Info("UNABLE TO GET NAMESPACE, check if CRD is deleted by GC")
		if apierrors.IsNotFound(err) {
			// TODO - Double check if this is necessary in theory it is not.
			// the SetControllerReference Should cause GC to delete
			// errDeleting := r.deleteNamespaceClassSynchronizer(ctx, req.Name)
			// log.Info("deleting CRD", "err", err)
			// return ctrl.Result{}, client.IgnoreNotFound(errDeleting)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ns.GetObjectMeta() == nil || ns.GetObjectMeta().GetLabels() == nil {
		log.Info("skipping reconciliation no labels")
		return ctrl.Result{}, nil
	}

	labels := ns.GetObjectMeta().GetLabels()
	targetNamespaceClass, exists := labels[constants.AkuityNamespaceClassLabel]
	if !exists {
		log.Info("skipping reconciliation no class label")
		return ctrl.Result{}, nil
	}

	NamespaceClassSynchronizerObjectKey := client.ObjectKey{
		Namespace: constants.ControllerNamespace,
		Name:      ns.GetName(),
	}

	namespaceClassSynchronizer := new(nscextension.NamespaceClassSynchronizer{})
	if err := r.Get(ctx, NamespaceClassSynchronizerObjectKey, namespaceClassSynchronizer); err != nil {
		// If it does not exist we just create it. NO need to do the next steps.
		if apierrors.IsNotFound(err) {
			if creatingError := r.createNamespaceClassSynchronizer(ctx, ns, targetNamespaceClass); creatingError != nil {
				log.Error(creatingError, "unable to create nscs")
				return ctrl.Result{}, creatingError
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to find nscs") // Let retry
		return ctrl.Result{}, err
	}
	nscsPatchHelper := client.MergeFrom(namespaceClassSynchronizer.DeepCopy())

	if namespaceClassSynchronizer.Spec.TargetNamespaceClassName == targetNamespaceClass {
		log.Info("already matched class, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if namespaceClassSynchronizer.ObjectMeta.Labels == nil {
		namespaceClassSynchronizer.ObjectMeta.Labels = map[string]string{}
	}
	namespaceClassSynchronizer.ObjectMeta.Labels[constants.AkuityNamespaceClassLabel] = targetNamespaceClass
	namespaceClassSynchronizer.Spec.RequireUpdate = true
	namespaceClassSynchronizer.Spec.TargetNamespaceClassName = targetNamespaceClass

	if err := r.Patch(ctx, namespaceClassSynchronizer, nscsPatchHelper); err != nil {
		log.Error(err, "unable to update CRD")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil

}

func (r *NamespaceReconciler) createNamespaceClassSynchronizer(ctx context.Context, namespace *corev1.Namespace, targetNamespaceClass string) error {
	if namespace == nil {
		return errors.New("invalid namespace")
	}
	metaObject := metav1.ObjectMeta{
		Name:      namespace.GetName(),
		Namespace: constants.ControllerNamespace, // This ones live on the controller namespace.
		Labels:    map[string]string{constants.AkuityNamespaceClassLabel: targetNamespaceClass},
	}

	nsClassSynchronizer := nscextension.NamespaceClassSynchronizer{
		ObjectMeta: metaObject,
		Spec: nscextension.NamespaceClassSynchronizerSpec{
			TargetNamespaceClassName: targetNamespaceClass,
			RequireUpdate:            true,
		},
	}
	if err := controllerutil.SetControllerReference(namespace, &nsClassSynchronizer, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, new(nsClassSynchronizer))
}
func (r *NamespaceReconciler) deleteNamespaceClassSynchronizer(ctx context.Context, namespaceName string) error {
	metaObject := metav1.ObjectMeta{
		Name:      namespaceName,
		Namespace: constants.ControllerNamespace, // This ones live on the controller namespace.
	}
	nsClassSynchronizer := nscextension.NamespaceClassSynchronizer{
		ObjectMeta: metaObject,
	}

	return r.Delete(ctx, &nsClassSynchronizer)

}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Named("core-namespace").
		Complete(r)
}
