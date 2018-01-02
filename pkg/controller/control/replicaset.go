/*
Copyright 2018 Caicloud Authors
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

package control

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

type ReplicaSetControlInterface interface {
	// CreateReplicaSets creates new replicasets according to the spec.
	CreateReplicaSets(namespace string, replicaSet *extensions.ReplicaSet, object runtime.Object) error
	// CreateReplicaSetsWithControllerRef creates new replicaSets according to the spec, and sets object as the replicaSet's controller.
	CreateReplicaSetsWithControllerRef(namespace string, replicaSet *extensions.ReplicaSet, object runtime.Object, controllerRef *metav1.OwnerReference) error
	// PatchReplicaSet patches the replicaset.
	PatchReplicaSet(namespace, name string, data []byte) error
}

// RealReplicaSetControl is the default implementation of ReplicaSetControlInterface.
type RealReplicaSetControl struct {
	KubeClient clientset.Interface
	Recorder   record.EventRecorder
}

func (r RealReplicaSetControl) CreateReplicaSets(namespace string, replicaSet *extensions.ReplicaSet, object runtime.Object) error {
	return r.createReplicaSets(namespace, replicaSet, object, nil)
}

func (r RealReplicaSetControl) CreateReplicaSetsWithControllerRef(namespace string, replicaSet *extensions.ReplicaSet, controllerObject runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createReplicaSets(namespace, replicaSet, controllerObject, controllerRef)
}

func (r RealReplicaSetControl) PatchReplicaSet(namespace, name string, data []byte) error {
	_, err := r.KubeClient.Extensions().ReplicaSets(namespace).Patch(name, types.StrategicMergePatchType, data)
	return err
}

func (r RealReplicaSetControl) createReplicaSets(namespace string, replicaSet *extensions.ReplicaSet, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	if labels.Set(replicaSet.Labels).AsSelectorPreValidated().Empty() {
		return fmt.Errorf("unable to create ReplicaSets, no labels")
	}
	if newReplicaSet, err := r.KubeClient.Extensions().ReplicaSets(namespace).Create(replicaSet); err != nil {
		r.Recorder.Eventf(object, v1.EventTypeWarning, FailedCreateReplicaSetReason, "Error creating: %v", err)
		return fmt.Errorf("unable to create replicaSets: %v", err)
	} else {
		accessor, err := meta.Accessor(object)
		if err != nil {
			glog.Errorf("parentObject does not have ObjectMeta, %v", err)
			return nil
		}
		glog.V(4).Infof("Controller %v created replicaSet %v", accessor.GetName(), newReplicaSet.Name)
		r.Recorder.Eventf(object, v1.EventTypeNormal, SuccessfulCreateReplicaSetReason, "Created replicaSet: %v", newReplicaSet.Name)
	}
	return nil
}
