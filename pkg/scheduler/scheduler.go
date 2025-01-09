package scheduler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	kueueapi "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"sigs.k8s.io/kueue/cmd/experimental/demo-acc/pkg/logger"
	"sigs.k8s.io/kueue/cmd/experimental/demo-acc/pkg/resource_monitor"
)

// As a first implementatipon we decided the following scheduling logic:
// 1. only consider pods which ONLY have containers that include a LIMIT for ALL resource types they use
// 2. the above rule applies to both workloads and the logic for counting the available capacity of nodes
// 		2.1 i.e. if a Node has a running pod that states a resource limit for CPU but only a resource request for Memory, then the available capacity of such Node will remain untouched for both CPU and Memory by not considering the entire pod
// 3. If a workload does not fit the bill for bullet 1, the scheduler prints an info message and accepts it.
// 		3.1 i.e. if a Job has a resource request for 3 CPUs but doesn't specify the CPU resource limit, then it gets accepted by the scheduler (without comparing it with the snapshot) and Kueue will take care of it.
// 4. If a workload lists in its limits a resource that is not present in the limits of ANY node, the scheduler prints an info message and accepts it.

// Schedule checks if a workload can be scheduled on any node in the snapshot.
// Returns true if it can be scheduled, false otherwise, along with any scheduling errors.
func Schedule(wl kueueapi.Workload, snapshot resource_monitor.Snapshot, gang bool) (bool, error) {
	// Initialize the logger for the scheduler
	log := logger.GetLogger().WithField("component", "scheduler")
	log.Logger.SetLevel(logrus.DebugLevel)
	log.Info("---------->> Starting scheduling process for workload ", wl.Name)

	var messages string // Collect messages for nodes that failed
	if gang {           // if gang scheduling is enabled
		log.Info("Gang scheduling set to true: ")

		canFit, nodes, err := gang_fitOnNodes(wl, snapshot)
		if err != nil {
			log.Error("Gang scheduling failed: ", err.Error())
			return false, err
		}
		if canFit {
			log.Info("Workload ", wl.Name, " successfully scheduled with gang scheduling on ", nodes)
			return true, nil
		}
	} else { // if alla the pods need to be on the same node
		log.Info("Gang scheduling set to false: all pods on the same node")
		// Iterate over nodes in the snapshot
		for _, node := range snapshot.Nodes {
			log.Info("Evaluating node for workload", "node", node.NodeName)
			// Check if the workload can run on this node
			if workloadCanRunOnNode(wl, node) {
				canFit, err := nodeCanFitWorkload(wl, node)
				if canFit {
					return true, nil
				}
				// Append the failure message if the node cannot fit the workload
				if err != nil {
					log.Warn("Node cannot fit workload", "node", node.NodeName, "error", err.Error())
					// Append the error message with a newline
					messages += fmt.Sprintf("Node %s: %s\n", node.NodeName, err.Error())
				}
			}
		}

		// Log failure messages if no suitable node is found
		for _, message := range messages {
			log.Warn(message)
		}
	}

	log.Error("Workload cannot be scheduled ", "workload ", wl.Name)
	return false, fmt.Errorf(messages)
}

// workloadCanRunOnNode checks if the workload's tolerations allow it to run on a node.
// It also checks for nodeSelector labels
func workloadCanRunOnNode(wl kueueapi.Workload, node resource_monitor.NodeUsage) bool {
	//log := logger.GetLogger().WithField("component", "scheduler")
	workloadTolerations := getWorkloadTolerations(wl)

	// Check if the node matches the workload's nodeSelector
	if !nodeSelectorMatches(wl, node) {
		///log.Info("doesn't match the workload's nodeSelector constraints")
		return false
	}

	// Check each taint on the node to ensure it is tolerated by the workload
	for _, taint := range node.Taints {
		tolerated := false
		for _, toleration := range workloadTolerations {
			if toleration.ToleratesTaint(&taint) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}

// nodeSelectorMatches checks if the node satisfies the workload's nodeSelector constraints.
// A nodeSelector defines hard constraints that must match for a workload to run on a node.
func nodeSelectorMatches(wl kueueapi.Workload, node resource_monitor.NodeUsage) bool {
	log := logger.GetLogger().WithField("component", "scheduler")
	//log.Info("Starting to check NodeSelector")
	// Ensure the workload has at least one PodSet
	if len(wl.Spec.PodSets) == 0 {
		// If no PodSet is defined, assume it matches all nodes
		logger.Info("len(wl.Spec.PodSets) == 0. return True")
		return true
	}

	// Extract the PodSpec from the first PodSet
	podSpec := wl.Spec.PodSets[0].Template.Spec
	nodeSelector := podSpec.NodeSelector
	if nodeSelector == nil || len(nodeSelector) == 0 {
		// log.Info("podSpec.NodeSelector is empty. return True")
		// If no nodeSelector is defined, assume it matches all nodes
		return true
	}

	// Compare each key-value pair in the nodeSelector with the node's labels
	for key, requiredValue := range nodeSelector {
		// Check if the node has the required label
		nodeValue, exists := node.Labels[key]
		if !exists {
			// Node is missing the required label
			log.Warn("Node is missing the required label for NodeSelector ", key)
			return false
		}
		// Check if the node's label value matches the required value
		if nodeValue != requiredValue {
			log.Warn("Node has the NodeSelector key ", key, " but not the required value ", requiredValue)
			return false
		}
	}

	// All nodeSelector constraints are satisfied
	return true
}

// getWorkloadTolerations extracts tolerations from a workload object.
func getWorkloadTolerations(wl kueueapi.Workload) []corev1.Toleration {
	var tolerations []corev1.Toleration

	// Loop through the PodSets in the workload spec
	for _, podSet := range wl.Spec.PodSets {
		// Append the tolerations from the Pod template spec
		tolerations = append(tolerations, podSet.Template.Spec.Tolerations...)
	}
	return tolerations
}

// nodeCanFitWorkload checks if a node has sufficient resources to accommodate the workload.
func nodeCanFitWorkload(wl kueueapi.Workload, node resource_monitor.NodeUsage) (bool, error) {
	// Aggregate resource limits for the workload
	aggregatedLimits, status := aggregateResources(wl)
	if !status {
		return false, fmt.Errorf("workload has resource requetest but no resource limit")
	}

	// Check each required resource against the node's available resources
	for resourceName, requiredQuantity := range aggregatedLimits {
		nodeRemaining, exists := node.Remaining[resourceName]
		if !exists {
			return false, fmt.Errorf("resource %s required but not available on node %s", resourceName, node.NodeName)
		}
		if nodeRemaining.Cmp(requiredQuantity) < 0 {
			return false, fmt.Errorf("resource %s required (%s) exceeds node %s remaining (%s)",
				resourceName, requiredQuantity.String(), node.NodeName, nodeRemaining.String())
		}
	}

	return true, nil
}

// aggregateResources aggregates the resource requirements of all containers in the workload.
func aggregateResources(wl kueueapi.Workload) (corev1.ResourceList, bool) {
	aggregatedLimits := corev1.ResourceList{}
	validWorkload := true

	// Iterate over all PodSets in the workload
	for _, podSet := range wl.Spec.PodSets {
		// Iterate over all containers in the PodSet
		for _, container := range podSet.Template.Spec.Containers {
			for resourceName, _ := range container.Resources.Requests {
				if _, hasLimit := container.Resources.Limits[resourceName]; !hasLimit {
					// add log to psecify missing limit?
					validWorkload = false
					return nil, false
				}
			}
			for resourceName, quantity := range container.Resources.Limits {
				if existing, exists := aggregatedLimits[resourceName]; exists {
					existing.Add(quantity)
					aggregatedLimits[resourceName] = existing
				} else {
					aggregatedLimits[resourceName] = quantity.DeepCopy()
				}
			}
		}
	}
	return aggregatedLimits, validWorkload
}

// Gang scheduling: If there are enough resources to host all the pods across a subset of the available nodes, then the workload should be accepted.

// gang_fitOnNodes checks if all pods in a workload can fit across multiple nodes simultaneously.
// It returns true and the list of node names if the workload can be scheduled; otherwise, it returns false and an error.
func gang_fitOnNodes(wl kueueapi.Workload, snapshot resource_monitor.Snapshot) (bool, []string, error) {
	// Clone the current snapshot to simulate resource consumption during scheduling
	simulatedSnapshot := cloneSnapshot(snapshot)

	// Track the nodes where the pods are scheduled
	nodeNames := []string{}

	// Iterate over each PodSet in the workload
	for _, podSet := range wl.Spec.PodSets {
		// Determine how many replicas we need to schedule
		replicas := podSet.Count

		// Attempt to fit each replica across available nodes
		for i := 0; i < int(replicas); i++ {
			fit := false

			for idx, node := range simulatedSnapshot.Nodes {
				// Check if the workload can run on this node
				if !workloadCanRunOnNode(wl, node) {
					// skip this node
					continue
				}
				// Check if the node can fit the current pod replica
				canFit, err := nodeCanFitWorkloadPod(podSet.Template.Spec, node)
				if err != nil {
					continue // Move to the next node if this one cannot fit
				}

				if canFit {
					// Simulate resource consumption on the node
					simulatedSnapshot.Nodes[idx] = allocatePodResources(podSet.Template.Spec, node)
					fit = true
					nodeNames = append(nodeNames, node.NodeName) // Record the node name
					break                                        // Break as we have successfully placed this pod
				}
			}

			if !fit {
				return false, nil, fmt.Errorf("workload %s cannot fit on available nodes", wl.Name)
			}
		}
	}

	return true, nodeNames, nil
}

// nodeCanFitWorkloadPod checks if a single pod can fit on a specific node.
func nodeCanFitWorkloadPod(podSpec corev1.PodSpec, node resource_monitor.NodeUsage) (bool, error) {
	aggregatedLimits := corev1.ResourceList{}

	for _, container := range podSpec.Containers {
		for resourceName, quantity := range container.Resources.Limits {
			if existing, exists := aggregatedLimits[resourceName]; exists {
				existing.Add(quantity)
				aggregatedLimits[resourceName] = existing
			} else {
				aggregatedLimits[resourceName] = quantity.DeepCopy()
			}
		}
	}

	for resourceName, requiredQuantity := range aggregatedLimits {
		nodeRemaining, exists := node.Remaining[resourceName]
		if !exists {
			return false, fmt.Errorf("resource %s required but not available on node %s", resourceName, node.NodeName)
		}
		if nodeRemaining.Cmp(requiredQuantity) < 0 {
			return false, fmt.Errorf("resource %s required (%s) exceeds node %s remaining (%s)",
				resourceName, requiredQuantity.String(), node.NodeName, nodeRemaining.String())
		}
	}

	return true, nil
}

// allocatePodResources updates the node's available resources to simulate the allocation of a pod.
func allocatePodResources(podSpec corev1.PodSpec, node resource_monitor.NodeUsage) resource_monitor.NodeUsage {
	for _, container := range podSpec.Containers {
		for resourceName, quantity := range container.Resources.Limits {
			if remaining, exists := node.Remaining[resourceName]; exists {
				remaining.Sub(quantity)
				node.Remaining[resourceName] = remaining
			}
		}
	}
	return node
}

// cloneSnapshot creates a deep copy of a Snapshot to simulate resource consumption.
func cloneSnapshot(snapshot resource_monitor.Snapshot) resource_monitor.Snapshot {
	clonedNodes := make([]resource_monitor.NodeUsage, len(snapshot.Nodes))
	for i, node := range snapshot.Nodes {
		clonedNode := resource_monitor.NodeUsage{
			NodeName:  node.NodeName,
			UUID:      node.UUID,
			Taints:    append([]corev1.Taint{}, node.Taints...),
			Labels:    map[string]string{},
			Remaining: corev1.ResourceList{},
			Pods:      append([]resource_monitor.PodInfo{}, node.Pods...),
		}
		for k, v := range node.Labels {
			clonedNode.Labels[k] = v
		}
		for k, v := range node.Remaining {
			clonedNode.Remaining[k] = v.DeepCopy()
		}
		clonedNodes[i] = clonedNode
	}

	return resource_monitor.Snapshot{
		LastCheck: snapshot.LastCheck,
		Nodes:     clonedNodes,
	}
}
