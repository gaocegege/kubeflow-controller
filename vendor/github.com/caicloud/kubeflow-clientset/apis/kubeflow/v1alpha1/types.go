/*
Copyright 2017 Caicloud Inc. All Rights Reserved.

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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=tfjob

// TFJob is a specification for a TFJob resource
type TFJob struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines a specification of a TFJob.
	Spec TFJobSpec `json:"spec"`
	// Status represents the current information/status for the TFJob
	Status TFJobStatus `json:"status"`
}

// TFJobSpec is the spec for a TFJob resource
type TFJobSpec struct {
	// ID of the TFJob in the runtime
	RuntimeID string `json:"runtimeID"`
	// DataDir specify the path of dataset
	DataDir string `json:"dataDir,omitempty"`
	// ModelDir specify the path of checkpoint, graph, etc
	ModelDir string `json:"modelDir,omitempty"`
	// LogDir specify the path of tf.events files
	LogDir string `json:"logDir,omitempty"`
	// ExportDir specify the path of saved model
	ExportDir string `json:"exportDir,omitempty"`
	// An array of working TFReplicas for this TFJob.
	// If empty then this resource can't be scheduled
	TFReplicaSpecs []TFReplicaSpec `json:"tfReplicaSpec"`
}

// TFReplicaSpec is the spec for a TFReplica resource
type TFReplicaSpec struct {
	// Optional. The number of desired replicas. Default 1.
	Replicas *int32 `json:"replicas,omitempty"`
	// Required for Training TFJob. One of `PS`, `Worker` and `Local`.
	TFReplicaType *TFReplicaType `json:"tfReplicaType,omitempty"`
	// PodTemplateSpec describes the data a pod should have when created from a template
	Template *v1.PodTemplateSpec `json:"template,omitempty"`
	// TerminationPolicy specifies the condition that the tfjob should be considered finished.
	TerminationPolicy *TerminationPolicySpec `json:"terminationPolicy,omitempty"`
}

// TFReplicaType is the type for replica.
type TFReplicaType string

const (
	// TFReplicaPS is the type for parameter servers.
	TFReplicaPS TFReplicaType = "PS"
	// TFReplicaWorker is the type for distributed workers.
	TFReplicaWorker TFReplicaType = "Worker"
	// TFReplicaLocal is the type for local running.
	TFReplicaLocal TFReplicaType = "Local"
)

type TerminationPolicySpec struct {
	// Chief policy waits for a particular process (which is the chief) to exit.
	Chief *ChiefSpec `json:"chief,omitempty"`
}

type ChiefSpec struct {
	TFReplicaName  string `json:"tfReplicaName"`
	TFReplicaIndex int    `json:"tfReplicaIndex"`
}

// TFJobStatus define the status of TFJob
type TFJobStatus struct {
	// Phase is the TFJob running phase
	Phase  TFJobPhase `json:"phase"`
	Reason string     `json:"reason"`
	// Condition keeps ten most recent cluster conditions
	Conditions []*TFJobCondition `json:"conditions"`

	// ReplicaStatuses specify the status of each TFReplica.
	TFReplicaStatuses []*TFReplicaStatus `json:"tfReplicaStatuses"`
}

// TFJobPhase is high-level summary of where the TFJob is in its lifecycle
type TFJobPhase string

const (
	// TFJobNone is the None phase for TFJob.
	TFJobNone TFJobPhase = ""

	// For some reason the state of the TFJob could not be obtained,
	// typically due to an error in communicating with the host of the TFJob.
	TFJobUnknown TFJobPhase = "Unknown"

	// The TFJob has been accepted by the Kubernetes system,
	// but one or more of the containers has not been created.
	// This includes time before being scheduled as well as time spent
	// downloading images over the network, which could take a while.
	TFJobPending TFJobPhase = "Pending"

	// The TFJob has been bound to a node, and all of the Containers
	// have been created. At least one container is still running,
	// or is in the process of starting or restarting.
	TFJobRunning TFJobPhase = "Running"

	// All containers in the TFJob have terminated in success,
	// and will not be restarted.
	TFJobSucceeded TFJobPhase = "Succeeded"

	// All containers in the TFJob have terminated, and at least one Container
	// has terminated in failure. That is, the container either exited with
	// non-zero status or was terminated by the system.
	TFJobFailed TFJobPhase = "Failed"
)

// TFJobCondition describes the state of a TFJob at a certain point.
type TFJobCondition struct {
    // Type of TFJob condition.
    Type TFJobConditionType `json:"type"`
    // Status of the condition, one of True, False, Unknown.
    Status v1.ConditionStatus `json:"status"`
    // The reason for the condition's last transition.
    Reason string `json:"reason,omitempty"`
    // A human readable message indicating details about the transition.
    Message string `json:"message,omitempty"`
    // The last time this condition was updated.
    LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
    // Last time the condition transitioned from one status to another.
    LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// TFJobConditionType defines all kinds of types of TFJobStatus.
type TFJobConditionType string

const (
	TFJobScheduled  TFJobConditionType = "Scheduled"
	TFJobReady      TFJobConditionType = "Ready"
	TFJobRecovering TFJobConditionType = "Recovering"
	// All Workers containers in the TFJob have terminated in success,
	// and recycle resource of all PS containers in the TFJob.
	TFJobRecycling TFJobConditionType = "Recycling"
)

type TFReplicaStatus struct {
	Type *TFReplicaType `json:"type"`
	// State is the overall state of the TFReplica.
	State TFReplicaState `json:"state"`
	// PodNames is the name of all pods.
	PodNames []string `json:"podNames"`
	// ReplicasStates provides the number of TFReplicas in each status.
	TFReplicasStates map[TFReplicaState]int `json:"tfReplicasStates,omitempty"`
}

type TFReplicaState string

const (
	TFReplicaUnknown   TFReplicaState = "Unknown"
	TFReplicaWaiting   TFReplicaState = "Waiting"
	TFReplicaRunning   TFReplicaState = "Running"
	TFReplicaSucceeded TFReplicaState = "Succeeded"
	TFReplicaFailed    TFReplicaState = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=tfjobs

// TFJobList is a list of TFJob resources
type TFJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TFJob `json:"items"`
}
