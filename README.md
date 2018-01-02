[tensorflow/k8s]: https://github.com/tensorflow/k8s
[caicloud/kubeflow-clientset]: https://github.com/caicloud/kubeflow-clientset

# KubeFlow Controller

Kubeflow controller is the open source re-implementation of part of the internal tool, ml-executor, in CaiCloud. This project is temporary and we hope we could use [tensorflow/k8s][] powered by the Kubeflow community eventually to replace this controller.

## Background

Recently Google open sources a new project [Kubeflow](https://github.com/google/kubeflow), which is dedicated to making using ML stacks on Kubernetes easy, fast and extensible.

We have a similar project in CaiCloud and it serves for several years. In that project we took different approaches but similar design decisions to Kubeflow. Then we re-implement our internal tool and open source it to accelerate the development of the community powered controller [tensorflow/k8s][].

## Overview

We separate Kubeflow into two parts:

- Kubeflow controller, which contains the controller for Kuberflow CRD.
- [Kubeflow clientset][caicloud/kubeflow-clientset], which contains the clientset and CRD specification for Kubeflow.

There are some differences between [tensorflow/k8s][] and this controller:

- caicloud/kubeflow-controller follows the design pattern of controller, which does not keep a state machine in the code and just moves from the current state to the desired state. tensorflow/k8s follows the design pattern of operator.
- caicloud/kubeflow-controller uses pods to run the workers and PS, while tensorflow/k8s uses jobs.

Besides these, there are also some differences between [tensorflow/k8s CRD][tensorflow/k8s] and [caicloud/kubeflow-clientset CRD][caicloud/kubeflow-clientset]:

- We do not support TensorBoard in the CRD, since the TensorBoard and Serving support will be in other CRDs.

These three points are the biggest differences, others could be ignored or unified.

## Get Started

The controller is easy to use, see [docs/get_started.md](docs/get_started.md) for more details.

## CONTRIBUTING

Feel free to hack on the controller! [docs/development.md](docs/development.md) will help you to get involved into the development.

## Acknowledgments

- Thanks to [Kubeflow community](https://github.com/google/kubeflow) for the awesome open source project.
- Thanks to [tensorflow/k8s][] authors for the operator.
