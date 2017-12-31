## Get Started

### Build and run the controller

Before the creation of training jobs, we have to build and run the controller first. See [development.md](developmet.md) to create a Kubernetes single-node cluster and run the Kubeflow controller based on that cluster.

### Create a local training job

We could create a local training job if the Kubernetes cluster and the controller are running successfully:

```bash
export KUBEFLOW_HOSTPATH="${GOPATH}/src/github.com/caicloud/kubeflow-controller/examples/workdir"
envsubst < examples/tfjob/local.yml | kubectl create -f -
```

The local job needs to mount the workdir volume into the container, and we use `envsubst` to hide the abstract path in the YAML configuration file:

```YAML
volumes:
  - name: workdir
    hostPath:
    # TODO: Use https://github.com/kubernetes/helm
    path: $KUBEFLOW_HOSTPATH
    type: Directory
```

But it is not user-friendly so we will use [helm](https://github.com/kubernetes/helm) instead in the future.

We train A simple MNIST classifier in the example just to show the support for local training:

```
step: 1
step: 2
step: 3
...
step: 99999
0.9234
```

### Create a distributed training job

TensorFlow supports the distributed mode, and a distributed job contains several workers and parameter servers (PS). The creation is similar to the local job:

```bash
export KUBEFLOW_HOSTPATH="${GOPATH}/src/github.com/caicloud/kubeflow-controller/examples/workdir"
envsubst < examples/tfjob/dist.yml | kubectl create -f -
```

In this example, the job has 4 workers and 2 PS, and we could get the logs of every worker and PS, e.g. worker-2:

```
Worker 2: Waiting for session to be initialized...
Worker 2: Session initialization complete.
Training begins @ 1514344761.687888
1514344770.659784: Worker 2: training step 1 done (global step: 0)
1514344770.680242: Worker 2: training step 2 done (global step: 2)
1514344770.693580: Worker 2: training step 3 done (global step: 8)
...
1514344771.224505: Worker 2: training step 52 done (global step: 201)
Training ends @ 1514344771.224552
Training elapsed time: 9.536664 s
After 200 training step(s), validation cross entropy = 96112.8
```
