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

package updater

import (
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"k8s.io/api/core/v1"
)

// DistributedUpdater is the type for distributed job updater, which updates the distributed TFJob status.
type DistributedUpdater struct {
	tfJob               *api.TFJob
	succeededworkerPods int
	workerPods          []*v1.Pod
	psPods              []*v1.Pod
}

// NewDistributed returns a new DistributedUpdater.
func NewDistributed(tfJob *api.TFJob, succeededworkerPods int, workerPods []*v1.Pod, psPods []*v1.Pod) *DistributedUpdater {
	return &DistributedUpdater{
		tfJob:               tfJob,
		succeededworkerPods: succeededworkerPods,
		workerPods:          workerPods,
		psPods:              psPods,
	}
}

// ShouldUpdate checks if the status should be updated.
// TODO(gaocegege): Notify controller to clean.
func (du *DistributedUpdater) ShouldUpdate() bool {
	shouldUpdated := false

	// Get the expected number of workerPods.
	workerPodSpec := du.tfJob.Spec.TFReplicaSpecs[getTemplateIndex(du.tfJob, api.TFReplicaWorker)]
	expected := int(*workerPodSpec.Replicas)

	if du.succeededworkerPods == expected {
		// TODO(gaocegege): Append conditions.
		// tfJob.Status.Conditions = append(tfJob.Status.Conditions, newCondition(api.TFJobConditionType, "", ""))
		du.tfJob.Status.Phase = api.TFJobSucceeded
		shouldUpdated = true
	} else if du.tfJob.Status.Phase != api.TFJobRunning {
		du.tfJob.Status.Phase = api.TFJobRunning
		shouldUpdated = true
	}

	shouldUpdateWrokerStatus := updateTFReplicaStatuses(du.tfJob, du.workerPods, api.TFReplicaWorker)
	shouldUpdatePSStatus := updateTFReplicaStatuses(du.tfJob, du.psPods, api.TFReplicaPS)

	if shouldUpdatePSStatus || shouldUpdateWrokerStatus {
		shouldUpdated = true
	}

	return shouldUpdated
}
