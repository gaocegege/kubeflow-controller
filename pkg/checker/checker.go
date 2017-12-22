package checker

import (
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
)

// IsLocalJob checks if the job is a local job.
func IsLocalJob(tfJob *api.TFJob) bool {
	// Local job should have only one replica, and its type should be local.
	if *tfJob.Spec.Specs[0].TFReplicaType == api.TFReplicaLocal {
		return true
	}
	return false
}
