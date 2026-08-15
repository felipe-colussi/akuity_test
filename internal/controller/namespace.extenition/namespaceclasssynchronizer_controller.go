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
	"encoding/json"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	namespaceextenitionv1 "akuity/api/namespace.extenition/v1"
	"akuity/internal/constants"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	rbacv1 "k8s.io/api/rbac/v1"
)

// NamespaceClassSynchronizerReconciler reconciles a NamespaceClassSynchronizer object
type NamespaceClassSynchronizerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespace.extenition.test.akuity,resources=namespaceclasssynchronizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespace.extenition.test.akuity,resources=namespaceclasssynchronizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespace.extenition.test.akuity,resources=namespaceclasssynchronizers/finalizers,verbs=update
// +kubebuilder:rbac:groups=namespace.extenition.test.akuity,resources=namespaceclasses,verbs=get;list
// TODO - ADD RBAC FOR NEW Fields that we will manage.
// LIST, CREATE, UPDATE, DELETE, PATCH
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceClassSynchronizer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *NamespaceClassSynchronizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("runing Reconcile for NamespaceClassSynchronizerReconciler", "request", req)
	nscs := new(namespaceextenitionv1.NamespaceClassSynchronizer{})
	if err := r.Get(ctx, req.NamespacedName, nscs); err != nil {
		log.Error(err, "unable to find NamespaceClassSynchronizer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// No need for update just return
	if !nscs.Spec.RequireUpdate {
		log.Info("skippeing reconciliation, no update needed")
		return ctrl.Result{}, nil
	}

	nsc := new(namespaceextenitionv1.NamespaceClass{})
	nscObjectKey := client.ObjectKey{
		Namespace: constants.ControllerNamespace,
		Name:      nscs.Spec.TargetNamespaceClassName,
	}
	// TODO - CHECK IF WE NEED TO SUPPORT THE NSC On any namespace. If so this needs to change.
	// We probably need to fetch the reference to the namesapce and save on the syncronizer.
	// Or store it on its spec.
	if err := r.Get(ctx, nscObjectKey, nsc); err != nil {
		if apierrors.IsNotFound(err) {
			// TODO - Define behavior. ATM just stoping execution
			// may consider reconcile and delete all objects. Just not returning should do that
			log.Info("Skipping as no namespaceClass was found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.handleClusterRoles(ctx, nsc.Spec.ClusterRoles, nscs.Spec.TargetNamespaceClassName); err != nil {
		log.Error(err, "failed to update cluster-roles")
	}

	if err := r.handleConfigMaps(ctx, nsc.Spec.ConfigMaps, nscs.Name, nscs.Spec.TargetNamespaceClassName); err != nil {
		log.Error(err, "failed to update config maps")
	}
	if err := r.handleNetworkPolicies(ctx, nsc.Spec.NetworkPolicys, nscs.Name, nscs.Spec.TargetNamespaceClassName); err != nil {
		log.Error(err, "failed to update network policies")
	}

	if err := r.genericImplementation(ctx, nsc.Spec.AnyApproach, nscs); err != nil {
		log.Error(err, "failed to implement generic object")
	}

	nscs.Spec.RequireUpdate = false
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Update(ctx, nscs)
	}); err != nil {
		log.Error(err, "unable to update CRD")
		return ctrl.Result{}, err
	}

	log.Info("returnign - Successfull update")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassSynchronizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&namespaceextenitionv1.NamespaceClassSynchronizer{}).
		Named("namespace.extenition-namespaceclasssynchronizer").
		Complete(r)
}

type GetNameInterface interface {
	GetName() string
}

// behaviorLogic, reads 2 key strings, and check wich objects need to be deleted or "upserted"
func objectsToKeep[T GetNameInterface](requiredObjects []T) map[string]struct{} {
	expectedStatus := make(map[string]struct{}, len(requiredObjects))
	for _, v := range requiredObjects {
		expectedStatus[v.GetName()] = struct{}{}
	}
	return expectedStatus
}

func (r *NamespaceClassSynchronizerReconciler) upsertObject(
	ctx context.Context,
	object client.Object, // Name and namespace should be here
	objectSetupFunc func() error, // Map From our Resource to the desiredOne
	targetNamespaceClass string,
) error {

	mutatingFunction := func() error {
		if objectSetupFunc != nil {
			if err := objectSetupFunc(); err != nil {
				return err
			}
			labels := object.GetLabels()
			if labels == nil {
				labels = make(map[string]string, 2)
			}
			labels[constants.AkuityNamespaceClassManagedResource] = "true"
			labels[constants.AkuityNamespaceClassLabel] = targetNamespaceClass
			object.SetLabels(labels)
		}
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		obj, err := controllerutil.CreateOrUpdate(ctx, r.Client, object, mutatingFunction)
		logf.FromContext(ctx).Info("updated/created_cluster_role", "obj", obj)
		return err
	})

}

// TODO - NEED TO DEFINE HOW TO HANDLE ERRORS.
func (r *NamespaceClassSynchronizerReconciler) handleClusterRoles(
	ctx context.Context,
	templates []namespaceextenitionv1.ClusterRoleTemplate,
	targetNamespaceClass string,
) error {

	clusterRoleBindingList := new(rbacv1.ClusterRoleList{})
	// Get List Of exisitng
	if err := r.List(ctx,
		clusterRoleBindingList,
		client.MatchingLabels{
			constants.AkuityNamespaceClassManagedResource: "true",
		},
	); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	objectsToKeep := objectsToKeep(templates)

	for _, existingObject := range clusterRoleBindingList.Items {
		if _, shouldKeep := objectsToKeep[existingObject.Name]; !shouldKeep {
			logf.FromContext(ctx).Info("ClusterBindingRole should be deleted skipping as is a non-namespace resource")
		}
	}
	for _, clusterRoleTemplate := range templates {
		object, updateFunc := clusterRoleToObject(clusterRoleTemplate, "")
		if err := r.upsertObject(ctx, object, updateFunc, targetNamespaceClass); err != nil {
			logf.FromContext(ctx).Error(err, "unable to update/create clusterRole")
		}
		logf.FromContext(ctx).Info("updated/created_cluster_role")
	}

	return nil
}

func clusterRoleToObject(in namespaceextenitionv1.ClusterRoleTemplate, targetNS string) (client.Object, func() error) {
	objectMeta := metav1.ObjectMeta{
		Name:      in.GetName(),
		Namespace: targetNS,
	}
	object := new(rbacv1.ClusterRole{ObjectMeta: objectMeta})

	mutatingFunction := func() error {
		object.Rules = in.Rules
		object.AggregationRule = in.AggregationRule
		return nil
	}
	return object, mutatingFunction
}

func (r *NamespaceClassSynchronizerReconciler) handleConfigMaps(
	ctx context.Context,
	templates []namespaceextenitionv1.ConfigMapTemplate,
	targetNS string,
	targetNamespaceClass string,
) error {
	log := logf.FromContext(ctx)

	// Fetch Resource
	configMapList := new(corev1.ConfigMapList)
	if err := r.List(ctx, configMapList,
		client.InNamespace(targetNS),
		client.MatchingLabels{constants.AkuityNamespaceClassManagedResource: "true"},
	); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Get Objects to keep
	keepMap := objectsToKeep(templates)

	//Clean up the ones that shouldn't be kept
	for _, existingConfigMap := range configMapList.Items {
		if _, shouldKeep := keepMap[existingConfigMap.Name]; !shouldKeep {
			log.Info("Deleting orphaned ConfigMap", "name", existingConfigMap.Name, "namespace", targetNS)
			if err := r.Delete(ctx, &existingConfigMap); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "unable to delete ConfigMap", "name", existingConfigMap.Name)
			}
		}
	}

	// Update the new tempaltes.
	for _, template := range templates {
		object, updateFunc := configMapToObject(template, targetNS)
		if err := r.upsertObject(ctx, object, updateFunc, targetNamespaceClass); err != nil {
			log.Error(err, "unable to update/create ConfigMap", "name", template.GetName())
			// return err -- TODO CHECK WHAT TO DO ON ERROR
		}
	}
	return nil
}

// --- NETWORK POLICIES HANDLER ---
func (r *NamespaceClassSynchronizerReconciler) handleNetworkPolicies(
	ctx context.Context,
	templates []namespaceextenitionv1.NetworkPoliciyTemplate,
	targetNS string,
	targetNamespaceClass string,
) error {
	log := logf.FromContext(ctx)

	// Fetch Resource
	netPolList := new(networkingv1.NetworkPolicyList)
	if err := r.List(ctx, netPolList,
		client.InNamespace(targetNS),
		client.MatchingLabels{constants.AkuityNamespaceClassManagedResource: "true"},
	); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// DELETE
	keepMap := objectsToKeep(templates)
	for _, existingNetPol := range netPolList.Items {
		if _, shouldKeep := keepMap[existingNetPol.Name]; !shouldKeep {
			log.Info("Deleting orphaned NetworkPolicy", "name", existingNetPol.Name, "namespace", targetNS)
			if err := r.Delete(ctx, &existingNetPol); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "unable to delete NetworkPolicy", "name", existingNetPol.Name)
			}
		}
	}

	// Update and create
	for _, template := range templates {
		object, updateFunc := networkPolicyToObject(template, targetNS)
		if err := r.upsertObject(ctx, object, updateFunc, targetNamespaceClass); err != nil {
			log.Error(err, "unable to update/create NetworkPolicy", "name", template.GetName())
			return err
		}
	}
	return nil
}

func networkPolicyToObject(in namespaceextenitionv1.NetworkPoliciyTemplate, targetNS string) (client.Object, func() error) {
	objectMeta := metav1.ObjectMeta{
		Name:      in.GetName(),
		Namespace: targetNS,
	}
	object := new(networkingv1.NetworkPolicy{ObjectMeta: objectMeta})

	mutatingFunction := func() error {
		object.Spec = in.Spec
		return nil
	}
	return object, mutatingFunction
}

func configMapToObject(in namespaceextenitionv1.ConfigMapTemplate, targetNS string) (client.Object, func() error) {
	objectMeta := metav1.ObjectMeta{
		Name:      in.GetName(),
		Namespace: targetNS,
	}
	object := new(corev1.ConfigMap{ObjectMeta: objectMeta})

	mutatingFunction := func() error {
		object.Immutable = in.Immutable
		object.Data = in.Data
		object.BinaryData = in.BinaryData
		return nil
	}
	return object, mutatingFunction
}

// genericImplementation that I DO NOT LIKE.
// on the current status this will:
// A) Not delete on reconcile:  I would move (or duplicate the CRD to namespace) And have GC to remove it.
// This would need some extra testing to ensure it works.
// ON CURRENT IMPLEMENTATION GC WILL NOT DELETE THOSE OBJECTS AS THEY ARE ON ANOTHER NS BY DESIGN !!!!!
// B) Only work for RBCA set (so only for the objects that are configured here)
// C) Limit to spec / metadata so A think like a ConfigMap will not be created. (I could do a single any{})
func (r *NamespaceClassSynchronizerReconciler) genericImplementation(
	ctx context.Context,
	templates []namespaceextenitionv1.TargetResource,
	nscs *namespaceextenitionv1.NamespaceClassSynchronizer,
) error {
	//
	log := logf.FromContext(ctx)
	for _, resource := range templates {
		// Local copy of loop variable for safety inside closure
		targetResouce := resource

		log = log.WithValues("targe_name", targetResouce.Name, "target_kind", targetResouce.Kind,
			"target_group", targetResouce.Group, "target_version", targetResouce.Version)
		// 3. Unmarshal the raw JSON metadata payload into a generic map

		rawMetadata := map[string]interface{}{}
		if len(targetResouce.Metadata.Raw) > 0 {
			if err := json.Unmarshal(targetResouce.Metadata.Raw, &rawMetadata); err != nil {
				log.Error(err, "Failed to unmarshal raw metadata payload")
				return err
			}
		}

		rawSpec := map[string]interface{}{}
		if len(targetResouce.Spec.Raw) > 0 {
			if err := json.Unmarshal(targetResouce.Spec.Raw, &rawSpec); err != nil {
				log.Error(err, "Failed to unmarshal raw spec payload")
				return err
			}
		}

		// 6. Build a fresh dynamic unstructured target resource
		obj := new(unstructured.Unstructured{})
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   targetResouce.Group,
			Version: targetResouce.Version,
			Kind:    targetResouce.Kind,
		})
		obj.SetName(targetResouce.Name)
		obj.SetNamespace(nscs.Name)

		objectUpdateFunc := func() error {
			unstructured.SetNestedMap(obj.Object, rawSpec, "spec")
			unstructured.SetNestedMap(obj.Object, rawMetadata, "metadata")
			obj.SetName(targetResouce.Name)
			obj.SetNamespace(nscs.Name)
			/*
				if err := controllerutil.SetControllerReference(nscs, obj, r.Scheme); err != nil {
					return err
			}*/
			return nil
		}

		if err := r.upsertObject(ctx, client.Object(obj), objectUpdateFunc, nscs.Spec.TargetNamespaceClassName); err != nil {

			log.Info("unable to upsert_object")
			return err
		}
	}

	return nil
}
