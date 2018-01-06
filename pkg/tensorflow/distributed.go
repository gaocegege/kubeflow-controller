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

package tensorflow

import (
	"fmt"
	"strings"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	portName   = "kubeflow-port"
	workerPort = 2222
)

// DistributedJob is the type for distributed TensorFlow training job.
type DistributedJob struct {
	tfJob               *api.TFJob
	activeWorkerPods    []*v1.Pod
	workerServices      []*v1.Service
	activePSPods        []*v1.Pod
	psServices          []*v1.Service
	serviceNames        map[string]string
	succeededWorkerPods int32
}

func NewDistributedJob(tfJob *api.TFJob, activeWorkerPods []*v1.Pod, activePSPods []*v1.Pod, workerServices []*v1.Service, psServices []*v1.Service, succeededWorkerPods int32) *DistributedJob {
	dj := &DistributedJob{
		tfJob:               tfJob,
		activeWorkerPods:    activeWorkerPods,
		workerServices:      workerServices,
		activePSPods:        activePSPods,
		psServices:          psServices,
		succeededWorkerPods: succeededWorkerPods,
		serviceNames:        make(map[string]string),
	}

	return dj
}

func (dj *DistributedJob) Action() []Event {
	events := make([]Event, 0)

	// Create services first.
	expectedWorker := int(*dj.getWorkerSpec().Replicas - dj.succeededWorkerPods)
	workerServicesCount := len(dj.workerServices)
	activeWorkerPodsCount := len(dj.activeWorkerPods)

	// TODO(gaocegege): Check succeeded.
	expectedPS := int(*dj.getPSSpec().Replicas)
	psServicesCount := len(dj.psServices)
	activePSPodsCount := len(dj.activePSPods)

	if workerServicesCount == 0 {
		glog.V(4).Infof("Expected worker services to be %d but got %d, create %d new workers", expectedWorker, workerServicesCount, expectedWorker-workerServicesCount)
		events = append(events, Event{
			Action: ActionShouldAddWorkerService,
			Number: expectedWorker - workerServicesCount,
		})
	} else if workerServicesCount < expectedWorker {
		// TODO(gaocegege): deal with the situation.
		glog.V(4).Infof("Expected worker services to be %d but got %d, create %d new workers", expectedWorker, workerServicesCount, expectedWorker-workerServicesCount)
	}

	if psServicesCount == 0 {
		glog.V(4).Infof("Expected ps services to be %d but got %d, create %d new workers", expectedPS, psServicesCount, expectedPS-psServicesCount)
		events = append(events, Event{
			Action: ActionShouldAddPSService,
			Number: expectedPS - psServicesCount,
		})
	} else if psServicesCount < expectedPS {
		// TODO(gaocegege): deal with the situation.
		glog.V(4).Infof("Expected ps services to be %d but got %d, create %d new workers", expectedPS, psServicesCount, expectedPS-psServicesCount)
	}

	if activeWorkerPodsCount < expectedWorker {
		glog.V(4).Infof("Expected workers to be %d but got %d, create %d new workers", expectedWorker, activeWorkerPodsCount, expectedWorker-activeWorkerPodsCount)

		// TODO(gaocegege): Using once is better.
		dj.compose()
		events = append(events, Event{
			Action: ActionShouldAddWorker,
			Number: expectedWorker - activeWorkerPodsCount,
		})
	} else if activeWorkerPodsCount == expectedWorker {
		events = append(events, Event{
			Action: ActionNothing,
		})
	}

	if activePSPodsCount < expectedPS {
		glog.V(4).Infof("Expected PS pods to be %d but got %d, create %d new PS pods", expectedPS, activePSPodsCount, expectedPS-activePSPodsCount)
		events = append(events, Event{
			Action: ActionShouldAddPS,
			Number: expectedPS - activePSPodsCount,
		})
	}
	return events
}

// GetSpec gets the spec with index.
func (dj *DistributedJob) GetSpec(typ api.TFReplicaType, index int) *api.TFReplicaSpec {
	template := dj.tfJob.Spec.Specs[dj.getTemplateIndex(typ)].Template

	// TODO(gaocegege): We need deepcopy
	template.Labels["index"] = fmt.Sprintf("%d", index)
	template.Spec.Containers[0].Args = dj.generateTFClusterSpec(typ, index)

	return &dj.tfJob.Spec.Specs[dj.getTemplateIndex(typ)]
}

func (dj *DistributedJob) generateTFClusterSpec(typ api.TFReplicaType, index int) []string {
	workerHosts := make([]string, 0)
	workerReplicas := int(*dj.getWorkerSpec().Replicas)
	for i := 0; i < workerReplicas; i++ {
		workerHosts = append(workerHosts, fmt.Sprintf("%s:%d", dj.serviceNames[dj.getServiceName(api.TFReplicaWorker, i)], workerPort))
	}
	workerHostsStr := "--worker_hosts="
	for _, workerHost := range workerHosts {
		workerHostsStr += workerHost + ","
	}
	workerHostsStr = workerHostsStr[:len(workerHostsStr)-1]

	psHosts := make([]string, 0)
	psReplicas := int(*dj.getPSSpec().Replicas)
	for i := 0; i < psReplicas; i++ {
		psHosts = append(psHosts, fmt.Sprintf("%s:%d", dj.serviceNames[dj.getServiceName(api.TFReplicaPS, i)], workerPort))
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

func (dj *DistributedJob) getServiceName(typ api.TFReplicaType, index int) string {
	return fmt.Sprintf("%s-%s-%d", dj.tfJob.Name, strings.ToLower(string(typ)), index)
}

func (dj *DistributedJob) GetService(typ api.TFReplicaType, index int) *v1.Service {
	labels := dj.getLabels(typ)
	labels["index"] = fmt.Sprintf("%d", index)

	name := dj.getServiceName(typ, index)
	generateName := generateName(fmt.Sprintf("%s-", dj.getServiceName(typ, index)))
	dj.serviceNames[name] = generateName

	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   generateName,
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

// TODO(gaocegege): Find a graceful way to void runtimeID.
// Notice: DO NOT call it multiple times for a tfjob since its runtime ID will be changed.
func (dj *DistributedJob) compose() {
	workerPodSpec := dj.getWorkerSpec()
	psPodSpec := dj.getPSSpec()

	dj.tfJob.Spec.RuntimeID = generateRuntimeID()
	workerPodSpec.Template.Labels = dj.getLabels(api.TFReplicaWorker)

	psPodSpec.Template.Labels = dj.getLabels(api.TFReplicaPS)
	// TODO(gaocegege): Compose the serviceNames.
}

func (dj DistributedJob) getLabels(typ api.TFReplicaType) map[string]string {
	return map[string]string{
		"kubeflow.caicloud.io": "true",
		"job_type":             string(typ),
		"runtime_id":           dj.tfJob.Spec.RuntimeID,
		"tf_job_name":          dj.tfJob.Name,
	}
}
