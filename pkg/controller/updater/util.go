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
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
)

// updateTFReplicaStatuses updates TFReplicaStatuses.
func updateTFReplicaStatuses(tfJob *api.TFJob, pods []*v1.Pod, typ api.TFReplicaType) bool {
	if len(pods) != 0 {
		// TODO(gaocegege): Deep equal is needed.
		// We have not the functions so we have to update the status every time.
		index, err := getReplicaStatusesIndex(tfJob.Status.TFReplicaStatuses, typ)
		if err != nil {
			glog.V(4).Info(err)
			// TODO(gaocegege): How to handle the conflict? Will it happen?
			return false
		}

		replicaStatus := &api.TFReplicaStatus{
			Type:             &typ,
			TFReplicasStates: make(map[api.TFReplicaState]int),
		}

		for _, pod := range pods {
			if _, ok := replicaStatus.TFReplicasStates[api.TFReplicaState(string(pod.Status.Phase))]; ok {
				replicaStatus.TFReplicasStates[api.TFReplicaState(string(pod.Status.Phase))]++
			} else {
				replicaStatus.TFReplicasStates[api.TFReplicaState(string(pod.Status.Phase))] = 1
			}
		}

		if index != -1 {
			tfJob.Status.TFReplicaStatuses[index] = replicaStatus
		} else {
			tfJob.Status.TFReplicaStatuses = append(tfJob.Status.TFReplicaStatuses, replicaStatus)
		}

		return true
	}
	return false
}

// dup with github.com/caicloud/kubeflow-controller/pkg/tensorflow/distributed.go
func getTemplateIndex(tfJob *api.TFJob, typ api.TFReplicaType) int {
	if *tfJob.Spec.TFReplicaSpecs[0].TFReplicaType == typ {
		return 0
	} else if *tfJob.Spec.TFReplicaSpecs[1].TFReplicaType == typ {
		return 1
	}
	glog.V(4).Infoln("%s Spec is not in spec[0] or spec[1]", string(typ))
	return -1
}

// getReplicaStatusesIndex returns the index of the replicaStatus which type is typ.
func getReplicaStatusesIndex(replicaStatuses []*api.TFReplicaStatus, typ api.TFReplicaType) (int, error) {
	length := len(replicaStatuses)
	if len(replicaStatuses) > 2 {
		return 0, fmt.Errorf("%s Spec is not in spec[0] or spec[1]", string(typ))
	}

	if length == 0 {
		return -1, nil
	} else if *replicaStatuses[0].Type == typ {
		return 0, nil
	} else if len(replicaStatuses) > 1 && *replicaStatuses[1].Type == typ {
		return 1, nil
	}
	return -1, nil
}
