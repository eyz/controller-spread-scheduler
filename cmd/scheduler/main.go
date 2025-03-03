// cmd/scheduler/main.go
package main

import (
	"os"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	// Import our plugin so that its init() function registers it.
	_ "sigs.k8s.io/controller-spread-scheduler/pkg/controllerspread"
)

func main() {
	klog.InitFlags(nil)
	cmd := app.NewSchedulerCommand()
	
	// We need to explicitly parse flags as the options command might not
	// have been updated yet with our custom flags
	if err := cmd.Execute(); err != nil {
		klog.ErrorS(err, "Scheduler command failed")
		os.Exit(1)
	}
}
