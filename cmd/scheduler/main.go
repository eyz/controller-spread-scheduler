package main

import (
	"os"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	// Import our plugin so that its PluginRegistry (or init function, if used) is included.
	_ "sigs.k8s.io/controller-spread-scheduler/pkg/controllerspread"
)

func main() {
	klog.InitFlags(nil)
	// Create the scheduler command without using the undefined WithPluginRegistry option.
	cmd := app.NewSchedulerCommand()
	if err := cmd.Execute(); err != nil {
		klog.ErrorS(err, "Scheduler command failed")
		os.Exit(1)
	}
}
