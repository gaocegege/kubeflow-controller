/*
Copyright 2018 Caicloud Inc. All Rights Reserved.

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
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getStatus returns no of succeeded and failed pods running a job
func getStatus(pods []*v1.Pod) (succeeded, failed int32) {
	succeeded = int32(filterPods(pods, v1.PodSucceeded))
	failed = int32(filterPods(pods, v1.PodFailed))
	return
}

// filterPods returns pods based on their phase.
func filterPods(pods []*v1.Pod, phase v1.PodPhase) int {
	result := 0
	for i := range pods {
		if phase == pods[i].Status.Phase {
			result++
		}
	}
	return result
}

func newControllerRef(tfJob *api.TFJob) *metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return &metav1.OwnerReference{
		APIVersion:         groupVersionKind.GroupVersion().String(),
		Kind:               groupVersionKind.Kind,
		Name:               tfJob.Name,
		UID:                tfJob.UID,
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}
