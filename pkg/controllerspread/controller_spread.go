// pkg/controllerspread/controller_spread.go
//
// Package controllerspread implements an out-of-tree scheduler plugin.
// The ControllerSpreadFilter plugin prevents pods from the same controller (ReplicaSet, StatefulSet,
// Job, or CronJob) with more than one desired replica/parallelism from being scheduled on a single node.
// It supports an annotation "controller-spread-scheduler/min-hosts" that specifies the minimum
// number of distinct hosts (default: 2).
package controllerspread

import (
	"context"
	"fmt"
	"math"
	"strconv"

	// Core API types.
	v1 "k8s.io/api/core/v1"
	// For label operations.
	"k8s.io/apimachinery/pkg/labels"
	// For runtime conversion.
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	// For managing sets.
	"k8s.io/apimachinery/pkg/util/sets"
	// Listers.
	rsLister "k8s.io/client-go/listers/apps/v1"
	stsLister "k8s.io/client-go/listers/apps/v1"
	cronJobLister "k8s.io/client-go/listers/batch/v1"
	jobLister "k8s.io/client-go/listers/batch/v1"
	podlister "k8s.io/client-go/listers/core/v1"
	// klog for logging.
	"k8s.io/klog/v2"
	// Upstream scheduler framework.
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	// Name is the unique name of this plugin.
	Name = "ControllerSpreadFilter"

	// Annotation key for minimum distinct hosts.
	minHostsAnnotationKey = "controller-spread-scheduler/min-hosts"
)

// ControllerSpreadArgs holds configuration parameters for the plugin.
type ControllerSpreadArgs struct{}

// ControllerType represents a type of controller.
type ControllerType string

const (
	ReplicaSetType  ControllerType = "ReplicaSet"
	StatefulSetType ControllerType = "StatefulSet"
	JobType         ControllerType = "Job"
	CronJobType     ControllerType = "CronJob"
)

// ControllerInfo holds identifying information about a controller.
type ControllerInfo struct {
	Type ControllerType
	UID  string
	Name string
}

// ControllerSpreadFilter implements the framework.Plugin interface.
type ControllerSpreadFilter struct {
	podLister     podlister.PodLister
	rsLister      rsLister.ReplicaSetLister
	stsLister     stsLister.StatefulSetLister
	jobLister     jobLister.JobLister
	cronJobLister cronJobLister.CronJobLister
	args          *ControllerSpreadArgs
}

// getControllerInfo extracts controller information from a pod's owner references.
func getControllerInfo(pod *v1.Pod) (ControllerInfo, bool) {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.UID == "" || ownerRef.Name == "" {
			continue
		}
		switch ownerRef.Kind {
		case string(ReplicaSetType):
			return ControllerInfo{Type: ReplicaSetType, UID: string(ownerRef.UID), Name: ownerRef.Name}, true
		case string(StatefulSetType):
			return ControllerInfo{Type: StatefulSetType, UID: string(ownerRef.UID), Name: ownerRef.Name}, true
		case string(JobType):
			return ControllerInfo{Type: JobType, UID: string(ownerRef.UID), Name: ownerRef.Name}, true
		case string(CronJobType):
			return ControllerInfo{Type: CronJobType, UID: string(ownerRef.UID), Name: ownerRef.Name}, true
		}
	}
	return ControllerInfo{}, false
}

// parseMinHostsAnnotation parses the annotation value into an int32; defaults to 2.
func parseMinHostsAnnotation(val string) int32 {
	if parsed, err := strconv.ParseInt(val, 10, 32); err == nil && parsed >= 2 && parsed <= math.MaxInt32 {
		return int32(parsed)
	}
	return 2
}

// min returns the smaller of two int32 values.
func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// New is the factory for ControllerSpreadFilter.
// It implements the plugin factory interface.
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	args := &ControllerSpreadArgs{}
	if obj != nil {
		uObj, ok := obj.(*unstructured.Unstructured)
		if ok {
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(uObj.Object, args)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ControllerSpreadArgs: %v", err)
			}
		}
	}

	return &ControllerSpreadFilter{
		podLister:     handle.SharedInformerFactory().Core().V1().Pods().Lister(),
		rsLister:      handle.SharedInformerFactory().Apps().V1().ReplicaSets().Lister(),
		stsLister:     handle.SharedInformerFactory().Apps().V1().StatefulSets().Lister(),
		jobLister:     handle.SharedInformerFactory().Batch().V1().Jobs().Lister(),
		cronJobLister: handle.SharedInformerFactory().Batch().V1().CronJobs().Lister(),
		args:          args,
	}, nil
}

// Name returns the name of the plugin.
func (csf *ControllerSpreadFilter) Name() string {
	return Name
}

// Filter is invoked during scheduling.
func (csf *ControllerSpreadFilter) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	controller, ok := getControllerInfo(pod)
	if !ok {
		return framework.NewStatus(framework.Success)
	}

	var desired int32
	minHostsVal := int32(2)
	annotations := map[string]string{}

	switch controller.Type {
	case ReplicaSetType:
		rs, err := csf.rsLister.ReplicaSets(pod.Namespace).Get(controller.Name)
		if err != nil {
			klog.ErrorS(err, "Could not retrieve ReplicaSet", "controller", controller.Name, "namespace", pod.Namespace)
			return framework.NewStatus(framework.Success)
		}
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		} else {
			desired = 1
		}
		annotations = rs.Annotations
	case StatefulSetType:
		sts, err := csf.stsLister.StatefulSets(pod.Namespace).Get(controller.Name)
		if err != nil {
			klog.ErrorS(err, "Could not retrieve StatefulSet", "controller", controller.Name, "namespace", pod.Namespace)
			return framework.NewStatus(framework.Success)
		}
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		} else {
			desired = 1
		}
		annotations = sts.Annotations
	case JobType:
		job, err := csf.jobLister.Jobs(pod.Namespace).Get(controller.Name)
		if err != nil {
			klog.ErrorS(err, "Could not retrieve Job", "controller", controller.Name, "namespace", pod.Namespace)
			return framework.NewStatus(framework.Success)
		}
		if job.Spec.Parallelism != nil {
			desired = *job.Spec.Parallelism
		} else {
			desired = 1
		}
		annotations = job.Annotations
	case CronJobType:
		cj, err := csf.cronJobLister.CronJobs(pod.Namespace).Get(controller.Name)
		if err != nil {
			klog.ErrorS(err, "Could not retrieve CronJob", "controller", controller.Name, "namespace", pod.Namespace)
			return framework.NewStatus(framework.Success)
		}
		if cj.Spec.JobTemplate.Spec.Parallelism != nil {
			desired = *cj.Spec.JobTemplate.Spec.Parallelism
		} else {
			desired = 1
		}
		annotations = cj.Annotations
	default:
		return framework.NewStatus(framework.Success)
	}

	if val, exists := annotations[minHostsAnnotationKey]; exists {
		minHostsVal = parseMinHostsAnnotation(val)
	}

	requiredHosts := min(desired, minHostsVal)
	if desired <= 1 {
		return framework.NewStatus(framework.Success)
	}

	allPods, err := csf.podLister.Pods(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "Error listing pods", "namespace", pod.Namespace)
		return framework.NewStatus(framework.Error, fmt.Sprintf("error listing pods: %v", err))
	}

	var controllerPods []v1.Pod
	for _, p := range allPods {
		if isOwnedByController(p, controller) && (p.Status.Phase == v1.PodRunning || p.Status.Phase == v1.PodPending) {
			controllerPods = append(controllerPods, *p)
		}
	}
	if len(controllerPods) <= 1 {
		return framework.NewStatus(framework.Success)
	}

	nodeSet := sets.NewString()
	for _, p := range controllerPods {
		if p.Spec.NodeName != "" {
			nodeSet.Insert(p.Spec.NodeName)
		}
	}

	effectiveSpread := nodeSet.Len()
	if !nodeSet.Has(nodeInfo.Node().Name) {
		effectiveSpread++
	}

	if effectiveSpread < int(requiredHosts) {
		klog.V(4).InfoS("Rejecting scheduling due to minimum host spread constraint",
			"candidateNode", nodeInfo.Node().Name,
			"currentSpread", nodeSet.Len(),
			"requiredHosts", requiredHosts,
			"controllerUID", controller.UID,
			"controllerName", controller.Name)
		return framework.NewStatus(framework.Unschedulable,
			fmt.Sprintf("must schedule across at least %d distinct nodes", requiredHosts))
	}

	return framework.NewStatus(framework.Success)
}

func isOwnedByController(pod *v1.Pod, controller ControllerInfo) bool {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == string(controller.Type) && string(ownerRef.UID) == controller.UID {
			return true
		}
	}
	return false
}

// Export the plugin registry so it can be merged with the scheduler’s built-in registry.
var PluginRegistry = map[string]func(runtime.Object, framework.Handle) (framework.Plugin, error){
	Name: New,
}
