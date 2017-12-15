# KubeFlow Controller

KubeFlow Controller is the controller for [KubeFlow CRD](https://github.com/caicloud/kubeflow-clientset), which is TensorFlow training job definition on Kubernetes.

KubeFlow Controller is the private implementation of [tensorflow/k8s](https://github.com/tensorflow/k8s), which will eventually replace this repository.

## HOWTO

### Build

```go
make
```

### Run

```
./bin/kubeflow-controller -kubeconfig <PATH TO KUBECONFIG>
```

### Help

```
$ ./bin/kubeflow-controller -h
Usage of ./kubeflow-controller:
  -alsologtostderr
    	log to standard error as well as files
  -kubeconfig string
    	Path to a kubeconfig. Only required if out-of-cluster.
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -logtostderr
    	log to standard error instead of files
  -master string
    	The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.
  -stderrthreshold value
    	logs at or above this threshold go to stderr
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```
