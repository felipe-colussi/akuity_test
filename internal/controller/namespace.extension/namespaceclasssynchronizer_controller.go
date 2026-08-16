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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	machinerymeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	namespaceextenitionv1 "akuity/api/namespace.extension/v1"
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

// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasssynchronizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasssynchronizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasssynchronizers/finalizers,verbs=update
// +kubebuilder:rbac:groups=namespace.extension.test.akuity,resources=namespaceclasses,verbs=get;list
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
	nscsPatchHelper := client.MergeFrom(nscs.DeepCopy())

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

	if err := r.handleClusterRoles(ctx, nsc.Spec.ClusterRoles, nscs); err != nil {
		log.Error(err, "failed to update cluster-roles")
	}

	if err := genericNamespacedRessourceHandler(
		ctx, r, nsc.Spec.ConfigMaps, nscs,
		new(corev1.ConfigMapList),
		configMapObjectFromList, configMapToObject, "ConfigMap",
	); err != nil {
		log.Error(err, "failed to update configMaps")
	}

	if err := genericNamespacedRessourceHandler(
		ctx, r, nsc.Spec.NetworkPolicies, nscs,
		new(networkingv1.NetworkPolicyList),
		networkPolicyObjectFromList, networkPolicyToObject, "NetworkPolicy",
	); err != nil {
		log.Error(err, "failed to update network policies")
	}

	if err := r.genericImplementation(ctx, nsc.Spec.AnyApproach, nscs); err != nil {
		log.Error(err, "failed to implement generic object")
	}

	nscs.Spec.RequireUpdate = false
	if err := r.Patch(ctx, nscs, nscsPatchHelper); err != nil {
		log.Error(err, "unable to update CRD")
		return ctrl.Result{}, err
	}

	// Update all Status on a single go
	if err := r.Status().Patch(ctx, nscs, nscsPatchHelper); err != nil {
		log.Error(err, "unable to update status for CRD, not blocking as it wont break")
	}

	log.Info("returnign - Successfull update")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassSynchronizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&namespaceextenitionv1.NamespaceClassSynchronizer{}).
		Named("namespace.extension-namespaceclasssynchronizer").
		Complete(r)
}

type GetNameInterface interface {
	GetName() string
}

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

// handleClusterRoles this is a global scoped object.
func (r *NamespaceClassSynchronizerReconciler) handleClusterRoles(
	ctx context.Context,
	templates []namespaceextenitionv1.ClusterRoleTemplate,
	nscs *namespaceextenitionv1.NamespaceClassSynchronizer,
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
		if err := r.upsertObject(ctx, object, updateFunc, nscs.Spec.TargetNamespaceClassName); err != nil {
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

func configMapObjectFromList(list *corev1.ConfigMapList) []client.Object {
	if list == nil {
		return nil
	}
	objectSlice := make([]client.Object, len(list.Items))
	for i, _ := range list.Items {
		objectSlice[i] = &list.Items[i]
	}

	return objectSlice
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

// --- NETWORK POLICIES Generic implementation ---
func networkPolicyObjectFromList(list *networkingv1.NetworkPolicyList) []client.Object {
	if list == nil {
		return nil
	}
	objectSlice := make([]client.Object, len(list.Items))
	for i, _ := range list.Items {
		objectSlice[i] = &list.Items[i]
	}

	return objectSlice
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

type K8sResourceManager interface {
	client.Reader
	client.Writer
	upsertObject(ctx context.Context, obj client.Object, updateFunc func() error, targetNamespaceClass string) error
}

// Generic Function if we need multiple implementations.
// Waiting for 1.27 :D
func genericNamespacedRessourceHandler[N GetNameInterface, L client.ObjectList](
	ctx context.Context,
	r K8sResourceManager,
	templates []N,
	nscs *namespaceextenitionv1.NamespaceClassSynchronizer,
	listObj L,
	extractItems func(L) []client.Object,
	toObjectFunc func(N, string) (client.Object, func() error),
	resourceTypeName string, // For Logging
) error {
	log := logf.FromContext(ctx)
	errorList := []string{}
	defer appendCondition(nscs, resourceTypeName, errorList)

	targetNS := nscs.Name
	targetNamespaceClass := nscs.Spec.TargetNamespaceClassName

	if err := r.List(ctx, listObj,
		client.InNamespace(nscs.Name),
		client.MatchingLabels{constants.AkuityNamespaceClassManagedResource: "true"},
	); err != nil && !apierrors.IsNotFound(err) {
		errorList = append(errorList, "unable_to_list")
		return err
	}
	keepMap := objectsToKeep(templates)

	for _, obj := range extractItems(listObj) {
		if _, shouldKeep := keepMap[obj.GetName()]; !shouldKeep {
			log.Info("Deleting orphaned resource", "type", resourceTypeName, "name", obj.GetName(), "namespace", targetNS)
			if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
				errorList = append(errorList, fmt.Sprintf("deleting:%s"), obj.GetName())
				log.Error(err, "unable to delete resource", "type", resourceTypeName, "name", obj.GetName())
			}
		}
	}

	for _, template := range templates {
		object, updateFunc := toObjectFunc(template, targetNS)
		if err := r.upsertObject(ctx, object, updateFunc, targetNamespaceClass); err != nil {
			errorList = append(errorList, fmt.Sprintf("upserting:%s", template.GetName()))
			log.Error(err, "unable to update/create resource", "type", resourceTypeName, "name", template.GetName())
		}
	}

	return nil
}

func appendCondition(
	nscs *namespaceextenitionv1.NamespaceClassSynchronizer,
	handlerObject string,
	failedFields []string) {
	availableCondition := metav1.Condition{
		Type:               handlerObject,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: nscs.Generation,
		Reason:             "Successfull",
		Message:            fmt.Sprintf("All %s components have been successfully deployed.", handlerObject),
	}

	if len(failedFields) != 0 {
		availableCondition.Status = metav1.ConditionFalse
		availableCondition.Reason = "Failure"
		availableCondition.Message = fmt.Sprintf("unable to synchronize %s: %v", handlerObject, failedFields)
	}

	_ = machinerymeta.SetStatusCondition(&nscs.Status.Conditions, availableCondition)
	return

}
