/*
Copyright 2017 The Caicloud Authors.
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

package tensorflow

import (
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api/v1"
)

const (
	ExpectedLocalWorkerNumber int32 = 1
)

// LocalJob is the type for local training TensorFlow job.
type LocalJob struct {
	tfJob     *api.TFJob
	pod       *v1.Pod
	succeeded int32
}

// NewLocalJob creates a local job.
func NewLocalJob(tfJob *api.TFJob, activePods []*v1.Pod, succeeded int32) *LocalJob {
	var pod *v1.Pod
	if len(activePods) == 0 {
		pod = nil
	} else if len(activePods) == 1 {
		pod = activePods[0]
	} else {
		glog.V(4).Infof("Local job has more than one spec: %v", tfJob)
	}
	return &LocalJob{
		tfJob:     tfJob,
		pod:       pod,
		succeeded: succeeded,
	}
}

func (lj *LocalJob) Action() Event {
	// Had succeeded result, not need to create new pod.
	if lj.succeeded > 0 {
		return Event{
			Action: ActionNothing,
		}
	}

	var active = int32(0)
	if lj.pod != nil {
		active = int32(1)
	}

	if active < ExpectedLocalWorkerNumber {
		lj.compose()
		return Event{
			Action: ActionShouldAddWorker,
			Number: int(ExpectedLocalWorkerNumber - active),
		}
	}
	return Event{
		Action: ActionNothing,
	}
}

// TODO: Find a graceful way to avoid runtimeID.
// Notice: DO NOT call it multiple times for a tfjob since its runtime ID will be changed.
func (lj *LocalJob) compose() {
	lj.tfJob.Spec.RuntimeID = generateRuntimeID()
	lj.tfJob.Spec.Specs[0].Template.Labels = lj.labels()
}

// labels returns the labels for this replica set.
func (lj LocalJob) labels() map[string]string {
	return map[string]string{
		"kubeflow.caicloud.io": "true",
		"job_type":             string(*lj.tfJob.Spec.Specs[0].TFReplicaType),
		"runtime_id":           lj.tfJob.Spec.RuntimeID,
		"tf_job_name":          lj.tfJob.Name,
	}
}
