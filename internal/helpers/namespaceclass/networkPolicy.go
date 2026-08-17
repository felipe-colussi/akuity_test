package namespaceclass

import (
	nscextension "akuity/api/namespace.extension/v1"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// --- NETWORK POLICIES Generic implementation ---
func NetworkPolicyObjectFromList(list *networkingv1.NetworkPolicyList) []client.Object {
	if list == nil {
		return nil
	}
	objectSlice := make([]client.Object, len(list.Items))
	for i, _ := range list.Items {
		objectSlice[i] = &list.Items[i]
	}

	return objectSlice
}
func NetworkPolicyTemplateToObject(in nscextension.NetworkPolicyTemplate, targetNS string) (client.Object, func() error) {
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
