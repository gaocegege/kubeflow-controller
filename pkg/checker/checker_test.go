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
