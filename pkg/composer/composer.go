package composer

import (
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
)

type Composer struct {
	tfJob *api.TFJob
}

// NewLocalJob returns a new LocalJob.
func New(tfJob *api.TFJob) *Composer {
	return &Composer{
		tfJob: tfJob,
	}
}

func (c Composer) Compose() {
	c.tfJob.Spec.RuntimeID = GenerateRuntimeID()
	c.tfJob.Spec.Specs[0].Template.Labels = c.Labels()
	c.tfJob.Spec.Specs[0].Template.Name = GeneratePodName(
		c.tfJob.Spec.Specs[0].Template.Name, string(*c.tfJob.Spec.Specs[0].TFReplicaType), c.tfJob.Spec.RuntimeID)
}

// Labels returns the labels for this replica set.
func (c Composer) Labels() map[string]string {
	return map[string]string{
		"kubeflow.caicloud.io": "",
		"job_type":             string(*c.tfJob.Spec.Specs[0].TFReplicaType),
		"runtime_id":           c.tfJob.Spec.RuntimeID,
		"tf_job_name":          c.tfJob.Name,
	}
}
