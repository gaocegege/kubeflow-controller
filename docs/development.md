## Development Documentation

### Clone

We host the project in GitHub, so you could clone the source code using git.

```bash
mkdir -p ${GOPATH}/src/github.com/caicloud
cd ${GOPATH}/src/github.com/caicloud
git clone git@github.com:caicloud/kubeflow-controller.git
git checkout release-1.7
```

### Build

```
make
```

### Run locally

The controller needs to communicate with Kubernetes so it is necessary to run a Kubernetes cluster first and create the corresponding CRD on the cluster.

#### Run a Kubernetes cluster

There are lots of choices to run a Kubernetes cluster locally:

- [local-up-cluster.sh in Kubernetes](https://github.com/kubernetes/kubernetes/blob/master/hack/local-up-cluster.sh)
- [minikube](https://github.com/kubernetes/minikube)

`local-up-cluster.sh` runs a single-node Kubernetes cluster locally, but Minikube runs a single-node Kubernetes cluster inside a VM. It is all compilable with the controller, but the Kubernetes version should be `1.7`. We have not tested on other versions but it should works too, theoretically.

Notice: If you use `local-up-cluster.sh`, please make sure that the kube-dns is up, see [kubernetes/kubernetes#47739](https://github.com/kubernetes/kubernetes/issues/47739) for more details.

#### Create CRD

After the cluster is up, the [Kubeflow CRD](https://github.com/caicloud/kubeflow-clientset) should be created on the cluster.

```bash
kubectl create -f ./examples/crd/crd.yml
```

#### Run the controller

The controller needs the kubeconfig or masterurl to communicate with Kubernetes apiserver:

```bash
./bin/kubeflow-controller -kubeconfig <PATH_TO_KUBECONFIG>
```

or

```bash
./bin/kubeflow-controller -kubeconfig <PATH_TO_KUBECONFIG> -master string
```

`-logtostderr -v 4` could be added to get all the logs.
