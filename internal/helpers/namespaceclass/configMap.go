package namespaceclass

import (
	nscextension "akuity/api/namespace.extension/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ConfigMapObjectFromList(list *corev1.ConfigMapList) []client.Object {
	if list == nil {
		return nil
	}
	objectSlice := make([]client.Object, len(list.Items))
	for i, _ := range list.Items {
		objectSlice[i] = &list.Items[i]
	}

	return objectSlice
}

func ConfigMapTemplateToObject(in nscextension.ConfigMapTemplate, targetNS string) (client.Object, func() error) {
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
