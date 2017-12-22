/*
Copyright 2017 The Caicloud KubeFlow Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/package clientset

import (
	kubeflowv1alpha1 "github.com/caicloud/kubeflow-clientset/clientset/typed/kubeflow/v1alpha1"
	glog "github.com/golang/glog"
	kubernetes "k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	kubernetes.Interface
	KubeflowV1alpha1() kubeflowv1alpha1.KubeflowV1alpha1Interface
	// Deprecated: please explicitly pick a version if possible.
	Kubeflow() kubeflowv1alpha1.KubeflowV1alpha1Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*kubernetes.Clientset
	*kubeflowv1alpha1.KubeflowV1alpha1Client
}

// KubeflowV1alpha1 retrieves the KubeflowV1alpha1Client
func (c *Clientset) KubeflowV1alpha1() kubeflowv1alpha1.KubeflowV1alpha1Interface {
	if c == nil {
		return nil
	}
	return c.KubeflowV1alpha1Client
}

// Deprecated: Kubeflow retrieves the default version of KubeflowClient.
// Please explicitly pick a version.
func (c *Clientset) Kubeflow() kubeflowv1alpha1.KubeflowV1alpha1Interface {
	if c == nil {
		return nil
	}
	return c.KubeflowV1alpha1Client
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.KubeflowV1alpha1Client, err = kubeflowv1alpha1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.Clientset, err = kubernetes.NewForConfig(&configShallowCopy)
	if err != nil {
		glog.Errorf("failed to create the client-go Clientset: %v", err)
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.KubeflowV1alpha1Client = kubeflowv1alpha1.NewForConfigOrDie(c)

	cs.Clientset = kubernetes.NewForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.KubeflowV1alpha1Client = kubeflowv1alpha1.New(c)

	cs.Clientset = kubernetes.New(c)
	return &cs
}
