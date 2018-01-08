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

package controller

import (
	"fmt"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	clientset "github.com/caicloud/kubeflow-clientset/clientset/versioned"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/caicloud/kubeflow-controller/pkg/controller/control"
	"github.com/caicloud/kubeflow-controller/pkg/controller/ref"
)

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   api.GroupName,
		Version: api.GroupVersion,
		Kind:    api.TFJobResourceKind,
	}
)

// HelperInterface is the interface for helper.
type HelperInterface interface {
	CreateService(tfJob *api.TFJob, service *v1.Service) error
	CreatePod(tfJob *api.TFJob, template *v1.PodTemplateSpec) error
	GetPodsForTFJob(tfJob *api.TFJob, typ api.TFReplicaType) ([]*v1.Pod, error)
	GetServicesForTFJob(tfJob *api.TFJob, typ api.TFReplicaType) ([]*v1.Service, error)
	DeleteServicesForTFJob(tfJob *api.TFJob) error
	DeletePodsForTFJob(tfJob *api.TFJob) error
}

// Helper is the type to manage internal resources in Kubernetes.
type Helper struct {
	tfJobClientset clientset.Interface

	podLister  kubelisters.PodLister
	podControl controller.PodControlInterface

	serviceLister  kubelisters.ServiceLister
	serviceControl control.ServiceControlInterface
}

// NewHelper creates a new Helper.
func NewHelper(tfJobClientset clientset.Interface, podLister kubelisters.PodLister, podControl controller.PodControlInterface, serviceLister kubelisters.ServiceLister, serviceControl control.ServiceControlInterface) *Helper {
	return &Helper{
		tfJobClientset: tfJobClientset,
		podLister:      podLister,
		podControl:     podControl,
		serviceLister:  serviceLister,
		serviceControl: serviceControl,
	}
}

func (h *Helper) CreateService(tfJob *api.TFJob, service *v1.Service) error {
	err := h.serviceControl.CreateServicesWithControllerRef(
		tfJob.Namespace, service, tfJob, newControllerRef(tfJob))
	if err != nil && errors.IsTimeout(err) {
		// Service is created but its initialization has timed out.
		// If the initialization is successful eventually, the
		// controller will observe the creation via the informer.
		// If the initialization fails, or if the service keeps
		// uninitialized for a long time, the informer will not
		// receive any update, and the controller will create a new
		// service when the expectation expires.
		glog.V(4).Info("Service is created but its initialization has timed out.")
	}
	if err != nil {
		return err
	}
	return nil
}

// createPod create a pod which is contolled by the tfjob.
func (h *Helper) CreatePod(tfJob *api.TFJob, template *v1.PodTemplateSpec) error {
	err := h.podControl.CreatePodsWithControllerRef(
		tfJob.Namespace, template, tfJob,
		newControllerRef(tfJob))
	if err != nil && errors.IsTimeout(err) {
		// Pod is created but its initialization has timed out.
		// If the initialization is successful eventually, the
		// controller will observe the creation via the informer.
		// If the initialization fails, or if the pod keeps
		// uninitialized for a long time, the informer will not
		// receive any update, and the controller will create a new
		// pod when the expectation expires.
		glog.V(4).Info("Pod is created but its initialization has timed out.")
	}
	if err != nil {
		return err
	}
	return nil
}

// GetPodsForTFJob gets all pods whose type is typ for the tfjob.
func (h *Helper) GetPodsForTFJob(tfJob *api.TFJob, typ api.TFReplicaType) ([]*v1.Pod, error) {
	// TODO(gaocegege): It is a hack, we definitely should replace it with a graceful way.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubeflow.caicloud.io": "true",
			"job_type":             string(typ),
			"runtime_id":           tfJob.Spec.RuntimeID,
			"tf_job_name":          tfJob.Name,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't convert Job selector: %v", err)
	}
	// List all pods to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.
	pods, err := h.podLister.Pods(tfJob.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	// If any adoptions are attempted, we should first recheck for deletion
	// with an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := h.tfJobClientset.KubeflowV1alpha1().TFJobs(tfJob.Namespace).Get(tfJob.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != tfJob.UID {
			return nil, fmt.Errorf("original Job %v/%v is gone: got uid %v, wanted %v", tfJob.Namespace, tfJob.Name, fresh.UID, tfJob.UID)
		}
		return fresh, nil
	})
	cm := controller.NewPodControllerRefManager(h.podControl, tfJob, selector, groupVersionKind, canAdoptFunc)
	return cm.ClaimPods(pods)
}

// GetServicesForTFJob gets all services whose type is typ for the tfjob.
func (h *Helper) GetServicesForTFJob(tfJob *api.TFJob, typ api.TFReplicaType) ([]*v1.Service, error) {
	// TODO(gaocegege): It is a hack, we definitely should replace it with a graceful way.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubeflow.caicloud.io": "true",
			"job_type":             string(typ),
			"runtime_id":           tfJob.Spec.RuntimeID,
			"tf_job_name":          tfJob.Name,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't convert Job selector: %v", err)
	}

	// List all services to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.
	services, err := h.serviceLister.Services(tfJob.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	// If any adoptions are attempted, we should first recheck for deletion
	// with an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := h.tfJobClientset.KubeflowV1alpha1().TFJobs(tfJob.Namespace).Get(tfJob.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != tfJob.UID {
			return nil, fmt.Errorf("original Job %v/%v is gone: got uid %v, wanted %v", tfJob.Namespace, tfJob.Name, fresh.UID, tfJob.UID)
		}
		return fresh, nil
	})
	cm := ref.NewServiceControllerRefManager(h.serviceControl, tfJob, selector, groupVersionKind, canAdoptFunc)
	return cm.ClaimServices(services)
}

func (h *Helper) DeleteServicesForTFJob(tfJob *api.TFJob) error {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubeflow.caicloud.io": "true",
			"runtime_id":           tfJob.Spec.RuntimeID,
			"tf_job_name":          tfJob.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("couldn't convert Job selector: %v", err)
	}

	services, err := h.serviceLister.Services(tfJob.Namespace).List(selector)
	if err != nil {
		return err
	}

	for _, service := range services {
		if err := h.serviceControl.DeleteService(service.Namespace, service.Name); err != nil {
			return err
		}
	}
	return nil
}

func (h *Helper) DeletePodsForTFJob(tfJob *api.TFJob) error {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubeflow.caicloud.io": "true",
			"runtime_id":           tfJob.Spec.RuntimeID,
			"tf_job_name":          tfJob.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("couldn't convert Job selector: %v", err)
	}

	pods, err := h.podLister.Pods(tfJob.Namespace).List(selector)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if err := h.podControl.DeletePod(tfJob.Namespace, pod.Name, tfJob); err != nil {
			return err
		}
	}
	return nil
}
