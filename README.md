# Controller Spread Scheduler (Out-of-Tree Plugin)

This project implements an out-of-tree Kubernetes scheduler plugin using the scheduler framework.  
The **ControllerSpreadFilter** plugin prevents all pods from the same controller (ReplicaSet, StatefulSet, Job, or CronJob) with more than one desired replica/parallelism from being scheduled on a single node.

## Overview

The Controller Spread Scheduler enforces a minimum level of fault tolerance by ensuring pods belonging to the same controller are distributed across multiple nodes. This is particularly useful for:

- Improving application availability during node failures
- Avoiding resource contention between pods of the same application
- Ensuring that single-node failures don't take down all instances of a service

It examines pod owner references during scheduling to identify the controller, fetches all existing pods from that controller, and ensures they're distributed according to the configured policy.

## How it Works

1. When a pod is being scheduled, the plugin:
   - Identifies the controller (ReplicaSet, StatefulSet, Job, CronJob) from the pod's owner references
   - Determines the desired replica count from the controller specification
   - Checks for any minimum host spread requirement annotation
   - Lists all existing pods belonging to the same controller
   - Counts the number of unique nodes hosting these pods
   - Determines if scheduling on the candidate node would satisfy the spread requirements

2. The plugin will reject a node if placing the pod there would violate the minimum spread requirement

3. The annotation key `controller-spread-scheduler/min-hosts` on the controller resource (not the pod) specifies the minimum required hosts
   - Default value: 2 (if not specified)
   - Effective requirement: min(desired_replicas, annotation_value)

## Installation

### Prerequisites
- Go 1.22+
- Docker or another container builder
- Access to a Kubernetes cluster (1.30.5)
- kubectl configured for your cluster

### Build the Scheduler Image

1. Clone the repository:
   ```
   git clone https://github.com/yourusername/controller-spread-scheduler.git
   cd controller-spread-scheduler
   ```

2. Build the binary:
   ```
   CGO_ENABLED=0 go build -a -o custom-scheduler ./cmd/scheduler
   ```

3. Build and push the container image:
   ```
   docker build -t yourregistry/controller-spread-scheduler:v1.30.5 .
   docker push yourregistry/controller-spread-scheduler:v1.30.5
   ```

4. Update the image reference in `deploy/scheduler-deployment.yaml` to match your registry:
   ```yaml
   image: yourregistry/controller-spread-scheduler:v1.30.5
   ```

### Deploy the Scheduler

1. Apply the ConfigMap:
   ```
   kubectl apply -f deploy/configmap.yaml
   ```

2. Deploy the scheduler:
   ```
   kubectl apply -f deploy/scheduler-deployment.yaml
   ```

3. Verify the scheduler is running:
   ```
   kubectl get pods -n kube-system -l component=controller-spread-scheduler
   ```

## Usage

### Scheduling Pods with the Custom Scheduler

To use the custom scheduler, set the `schedulerName` field in your pod specification:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-application
spec:
  replicas: 3
  template:
    spec:
      schedulerName: controller-spread-scheduler
      containers:
      - name: my-application
        image: my-application:latest
```

### Setting Custom Spread Requirements

To customize the minimum number of hosts, add the annotation to your controller resource:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-application
  annotations:
    controller-spread-scheduler/min-hosts: "3"
spec:
  replicas: 5
  template:
    spec:
      schedulerName: controller-spread-scheduler
      containers:
      - name: my-application
        image: my-application:latest
```

This configuration will ensure that the 5 replicas are distributed across at least 3 different nodes.

### Behavior Summary

The plugin calculates the required minimum hosts as:
```
requiredHosts = min(desired_replicas, annotation_value or 2)
```

#### Desired Count vs. Annotation ("min-hosts") Examples

- **Desired Count = 1:**  
  Regardless of the annotation value (1–5), the requirement is 1. No spread is enforced.

- **Desired Count = 2:**  
  - Annotation = 1 → Required hosts = min(2, 1) = 1 → No spread enforced.
  - Annotation ≥ 2 → Required hosts = 2 → Pods must run on 2 distinct nodes.

- **Desired Count = 3:**  
  - Annotation = 1 → Required hosts = 1 → No spread enforced.
  - Annotation = 2 → Required hosts = 2 → Pods must be spread on at least 2 nodes.
  - Annotation ≥ 3 → Required hosts = 3 → All 3 pods must be on 3 separate nodes.

- **Desired Count = 4:**  
  - Annotation = 1 → Required hosts = 1 → No spread enforced.
  - Annotation = 2 → Required hosts = 2 → Pods must run on at least 2 nodes.
  - Annotation = 3 → Required hosts = 3 → Pods must run on at least 3 nodes.
  - Annotation ≥ 4 → Required hosts = 4 → All 4 pods must be on 4 separate nodes.

- **Desired Count = 5:**  
  - Annotation = 1 → Required hosts = 1 → No spread enforced.
  - Annotation = 2 → Required hosts = 2 → Pods must run on at least 2 nodes.
  - Annotation = 3 → Required hosts = 3 → Pods must run on at least 3 nodes.
  - Annotation = 4 → Required hosts = 4 → Pods must run on at least 4 nodes.
  - Annotation = 5 → Required hosts = 5 → All 5 pods must be on 5 separate nodes.

## Advanced Topics

### Debugging

To enable more verbose logging, you can adjust the klog verbosity level when starting the scheduler by modifying the deployment:

```yaml
command:
- kube-scheduler
- --config=/etc/kubernetes/controller-spread-scheduler.yaml
- --v=4  # Add this line for debug logging
```

### Technical Details

The plugin implements the Filter extension point from the Kubernetes scheduler framework. It:

1. Gets invoked during scheduling decisions as part of the Filter phase
2. Examines the pod being scheduled to determine its controller
3. Lists all pods in the namespace with the same controller label
4. Counts the unique nodes where these pods are running
5. Verifies if adding this pod to the candidate node would maintain the required spread

### Comparison with Built-In Pod Anti-Affinity

While Kubernetes has built-in pod anti-affinity, this plugin provides:

1. **Simplified configuration**: No need for complex affinity rules
2. **Controller-aware spreading**: Automatically identifies pods from the same controller
3. **Dynamic adjustment**: Minimum hosts can be less than total replicas for flexibility
4. **Annotation-based control**: Easy to configure without changing pod templates

## Project Structure

All file paths are relative to the project folder `controller-spread-scheduler/`:

```
controller-spread-scheduler/
├── cmd/
│   └── scheduler/
│       └── main.go                # Main entry point for the custom scheduler.
├── pkg/
│   └── controllerspread/
│       ├── controller_spread.go   # Out-of-tree plugin implementation.
│       └── register.go            # Plugin registration.
├── Dockerfile                     # Dockerfile to build the custom scheduler image.
├── deploy/
│   ├── configmap.yaml             # Scheduler configuration ConfigMap.
│   └── scheduler-deployment.yaml  # Deployment spec for the custom scheduler.
└── README.md                      # Instructions, behavior summary, and usage details.
```

## Compatibility

This scheduler plugin is compatible with:
- Kubernetes 1.30.5
- Go 1.22 (required for building)
- kubescheduler.config.k8s.io/v1 API

## License

[Add your license information here]
