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
	"fmt"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"k8s.io/api/core/v1"

	"github.com/caicloud/kubeflow-controller/pkg/tensorflow"
)

// LocalUpdater is the type for the updater, which updates the TFJob status.
type LocalUpdater struct {
	tfJob               *api.TFJob
	succeededWorkerPods int
	workerPod           *v1.Pod
}

// NewLocal returns a new LocalUpdater.
func NewLocal(tfJob *api.TFJob, succeededWorkerPods int, workerPods []*v1.Pod) (*LocalUpdater, error) {
	var workerPod *v1.Pod
	if len(workerPods) == 0 {
		workerPod = nil
	} else if len(workerPods) == 1 {
		workerPod = workerPods[0]
	} else if len(workerPods) > 1 {
		return nil, fmt.Errorf("Updater for %s: len(workerPods) > 1", tfJob.Name)
	}
	return &LocalUpdater{
		tfJob:               tfJob,
		succeededWorkerPods: succeededWorkerPods,
		workerPod:           workerPod,
	}, nil
}

// ShouldUpdate checks if the status should be updated.
func (lu *LocalUpdater) ShouldUpdate() bool {
	shouldUpdated := false
	if lu.succeededWorkerPods == int(tensorflow.ExpectedLocalWorkerNumber) {
		// TODO(gaocegege): Append conditions.
		// tfJob.Status.Conditions = append(tfJob.Status.Conditions, newCondition(api.TFJobConditionType, "", ""))
		lu.tfJob.Status.Phase = api.TFJobSucceeded
		shouldUpdated = true
	} else if lu.tfJob.Status.Phase != api.TFJobRunning {
		lu.tfJob.Status.Phase = api.TFJobRunning
		shouldUpdated = true
	}

	if lu.workerPod != nil {
		// TODO(gaocegege): Deep equal is needed.
		// We have not the functions so we have to update the status every time.
		replicaStatuses := make([]*api.TFReplicaStatus, 0)
		local := api.TFReplicaLocal
		replicaStatus := &api.TFReplicaStatus{
			Type:             &local,
			TFReplicasStates: make(map[api.TFReplicaState]int),
		}

		replicaStatus.TFReplicasStates[api.TFReplicaState(string(lu.workerPod.Status.Phase))] = 1
		replicaStatuses = append(replicaStatuses, replicaStatus)
		lu.tfJob.Status.TFReplicaStatuses = replicaStatuses
		shouldUpdated = true
	}
	return shouldUpdated
}
