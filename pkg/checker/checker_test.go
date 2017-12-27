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

package checker

import (
	"testing"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestIsLocalJob(t *testing.T) {
	testCase := &api.TFJob{
		Spec: api.TFJobSpec{
			Specs: []api.TFReplicaSpec{
				api.TFReplicaSpec{},
				api.TFReplicaSpec{},
			},
		},
	}

	testTypes := []api.TFReplicaType{
		api.TFReplicaLocal,
		api.TFReplicaPS,
	}

	testResults := []bool{
		true,
		false,
	}

	for index, testType := range testTypes {
		testCase.Spec.Specs[0].TFReplicaType = &testType
		for _, nestedTestType := range testTypes {
			testCase.Spec.Specs[1].TFReplicaType = &nestedTestType
			isLocalJob := IsLocalJob(testCase)
			assert.Equal(t, testResults[index], isLocalJob, "expected IsLocalJob to be `%t` but got `%t`", testResults[index], isLocalJob)
		}
	}
}
