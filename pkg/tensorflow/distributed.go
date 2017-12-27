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
	"fmt"
	"strings"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api/v1"
)

const (
	portName   = "kubeflow-port"
	workerPort = 2222
)

// DistributedJob is the type for distributed TensorFlow training job.
type DistributedJob struct {
	tfJob            *api.TFJob
	activeWorkers    []*v1.Pod
	workerServices   []*v1.Service
	activePSes       []*v1.Pod
	psServices       []*v1.Service
	succeededWorkers int32
}

func NewDistributedJob(tfJob *api.TFJob, activeWorkers []*v1.Pod, activePSes []*v1.Pod, workerServices []*v1.Service, psServices []*v1.Service, succeededWorkers int32) *DistributedJob {
	dj := &DistributedJob{
		tfJob:            tfJob,
		activeWorkers:    activeWorkers,
		workerServices:   workerServices,
		activePSes:       activePSes,
		psServices:       psServices,
		succeededWorkers: succeededWorkers,
	}

	return dj
}

func (dj *DistributedJob) Action() []Event {
	events := make([]Event, 0)

	expected := int(*dj.getWorkerSpec().Replicas - dj.succeededWorkers)
	activeWorkersCount := len(dj.activeWorkers)

	if activeWorkersCount < expected {
		glog.V(4).Infof("Expected workers to be %d but got %d, create %d new workers", expected, activeWorkersCount, expected-activeWorkersCount)

		// TODO: Using once is better.
		dj.compose()
		events = append(events, Event{
			Action: ActionShouldAddWorker,
			Number: int(expected - activeWorkersCount),
		})
	} else if activeWorkersCount == int(expected) {
		events = append(events, Event{
			Action: ActionNothing,
		})
	}

	expected = int(*dj.getPSSpec().Replicas)
	activePSesCount := len(dj.activePSes)

	if activePSesCount < expected {
		glog.V(4).Infof("Expected replicaset to be %d but got %d, create %d new replicasets", expected, activePSesCount, expected-activePSesCount)
		events = append(events, Event{
			Action: ActionShouldAddPS,
			Number: int(expected - activePSesCount),
		})
	}
	return events
}

// GetSpec gets the spec with index.
func (dj *DistributedJob) GetSpec(typ api.TFReplicaType, index int) *api.TFReplicaSpec {
	template := dj.tfJob.Spec.Specs[dj.getTemplateIndex(typ)].Template

	// We need deepcopy
	template.Labels["index"] = fmt.Sprintf("%d", index)
	template.Spec.Containers[0].Args = dj.generateTFClusterSpec(typ, index)

	return &dj.tfJob.Spec.Specs[dj.getTemplateIndex(typ)]
}

func (dj *DistributedJob) generateTFClusterSpec(typ api.TFReplicaType, index int) []string {
	workerHosts := make([]string, 0)
	for i := 0; i < int(*dj.tfJob.Spec.Specs[dj.getTemplateIndex(api.TFReplicaWorker)].Replicas); i++ {
		workerHosts = append(workerHosts, fmt.Sprintf("%s:%d", dj.getServiceName("worker", i), workerPort))
	}
	workerHostsStr := "--worker_hosts="
	for _, workerHost := range workerHosts {
		workerHostsStr += workerHost + ","
	}
	workerHostsStr = workerHostsStr[:len(workerHostsStr)-1]

	psHosts := make([]string, 0)
	for i := 0; i < int(*dj.tfJob.Spec.Specs[dj.getTemplateIndex(api.TFReplicaPS)].Replicas); i++ {
		psHosts = append(psHosts, fmt.Sprintf("%s:%d", dj.getServiceName("ps", i), workerPort))
	}
	psHostsStr := "--ps_hosts="
	for _, psHost := range psHosts {
		psHostsStr += psHost + ","
	}
	psHostsStr = psHostsStr[:len(psHostsStr)-1]

	jobName := fmt.Sprintf("--job_name=%s", strings.ToLower(string(typ)))
	taskIndex := fmt.Sprintf("--task_index=%d", index)

	return []string{
		workerHostsStr,
		psHostsStr,
		jobName,
		taskIndex,
	}
}

func (dj *DistributedJob) getServiceName(typeName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", dj.tfJob.Name, typeName, index)
}

func (dj *DistributedJob) GetService(typ api.TFReplicaType, index int) *v1.Service {
	typeName := strings.ToLower(string(typ))
	labels := dj.getLabels(typ)
	labels["index"] = fmt.Sprintf("%d", index)

	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   dj.getServiceName(typeName, index),
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Ports: []v1.ServicePort{
				{
					Name: portName,
					Port: workerPort,
				},
			},
		},
	}
}

func (dj *DistributedJob) getPSSpec() *api.TFReplicaSpec {
	return &dj.tfJob.Spec.Specs[dj.getTemplateIndex(api.TFReplicaPS)]
}

func (dj *DistributedJob) getWorkerSpec() *api.TFReplicaSpec {
	return &dj.tfJob.Spec.Specs[dj.getTemplateIndex(api.TFReplicaWorker)]
}

func (dj DistributedJob) getTemplateIndex(typ api.TFReplicaType) int {
	if *dj.tfJob.Spec.Specs[0].TFReplicaType == typ {
		return 0
	} else if *dj.tfJob.Spec.Specs[1].TFReplicaType == typ {
		return 1
	}
	glog.V(4).Infoln("%s Spec is not in spec[0] or spec[1]", string(typ))
	return -1
}

// TODO: Find a graceful way to void runtimeID.
// Notice: DO NOT call it multiple times for a tfjob since its runtime ID will be changed.
func (dj *DistributedJob) compose() {
	wokerSpec := dj.getWorkerSpec()
	psSpec := dj.getPSSpec()

	dj.tfJob.Spec.RuntimeID = generateRuntimeID()
	wokerSpec.Template.Labels = dj.getLabels(api.TFReplicaWorker)

	psSpec.Template.Labels = dj.getLabels(api.TFReplicaPS)
}

func (dj DistributedJob) getLabels(typ api.TFReplicaType) map[string]string {
	return map[string]string{
		"kubeflow.caicloud.io": "true",
		"job_type":             string(typ),
		"runtime_id":           dj.tfJob.Spec.RuntimeID,
		"tf_job_name":          dj.tfJob.Name,
	}
}
