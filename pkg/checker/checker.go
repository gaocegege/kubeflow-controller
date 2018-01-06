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

package checker

import (
	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
)

// IsLocalJob checks if the job is a local job.
func IsLocalJob(tfJob *api.TFJob) bool {
	// Local job should have only one replica, and its type should be local.
	return *tfJob.Spec.Specs[0].TFReplicaType == api.TFReplicaLocal
}
