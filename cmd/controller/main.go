package main

import (
	"flag"
	"os"
	"runtime"
	"time"

	clientset "github.com/caicloud/kubeflow-clientset/clientset"
	kubeflowinformers "github.com/caicloud/kubeflow-clientset/informers"
	"github.com/coreos/etcd/version"
	"github.com/golang/glog"
	"k8s.io/client-go/tools/clientcmd"
	kubernetes "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	kubeinformers "k8s.io/kubernetes/pkg/client/informers/informers_generated/externalversions"

	"github.com/caicloud/kubeflow-controller/pkg/controller"
	"github.com/caicloud/kubeflow-controller/pkg/util/signals"
)

var (
	masterURL    string
	kubeconfig   string
	printVersion bool
)

func run() {
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	tfJobClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building TFJob clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	tfJobInformerFactory := kubeflowinformers.NewSharedInformerFactory(tfJobClient, time.Second*30)

	controller := controller.NewController(kubeClient, tfJobClient, kubeInformerFactory, tfJobInformerFactory)

	go kubeInformerFactory.Start(stopCh)
	go tfJobInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")

	glog.Infof("kubeflow-controller Version: %v", version.Version)
	glog.Infof("Git SHA: %s", version.GitSHA)
	glog.Infof("Go Version: %s", runtime.Version())
	glog.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func main() {
	// This is to solve https://github.com/golang/glog/commit/65d674618f712aa808a7d0104131b9206fc3d5ad, which is definitely NOT cool.
	flag.Parse()
	if printVersion {
		os.Exit(0)
	}
	run()
}
