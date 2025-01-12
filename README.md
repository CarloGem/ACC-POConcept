# Kueue Admission Check Controller with Resource Monitoring

This project is a Kueue Admission Check Controller integrated with resource monitoring capabilities. It leverages [Kueue](https://github.com/kubernetes-sigs/kueue) to manage workloads and includes functionality to log the current cluster resource snapshot whenever a workload is admitted. 

Generally an Admission Check Controller in Kubernetes intercepts and validates incoming workloads before they are admitted to the cluster, applying custom logic to enforce policies or constraints. In this project, the ACC doesn't enforce any policy and the Resource Monitor runs independently but provides real-time resource data, which the controller logs when workloads are admitted, enhancing visibility into cluster resources.

Based on https://github.com/kubernetes-sigs/kueue/pull/3265

## Features

- **Admission Control**: Ensures workloads are admitted after custom checks, such as delaying admission for a minute after creation.
- **Resource Monitoring**: Periodically collects node and pod resource usage and logs snapshots upon workload admission.
- **RBAC Configurations**: Includes necessary permissions to interact with Kubernetes API resources like nodes, pods, workloads, and admission checks.

---

## Project Structure

```plaintext
.
├── cmd                   # Entry point of the controller
│   └── main.go           # Main logic integrating resource monitor and controller manager
├── config                # Kubernetes configurations
│   ├── default           # Default manager configurations
│   ├── rbac              # Role-based access control (RBAC) configurations
│   ├── manager           # Manager-specific Kubernetes manifests
│   └── network-policy    # Network policies for secure communication
├── internal/controller   # Controllers for workloads and admission checks
├── pkg
│   ├── evaluator         # Evaluator component implementation
│   │   └── evaluator.go
│   ├── logger            # To better organize logs
│   │   └── logger.go
│   └── resource_monitor  # Resource monitoring implementation
│       └── resource-monitor.go
├── test                # End-to-end and utility tests
└── Dockerfile          # Dockerfile for building the controller image
```

---

## Prerequisites

1. **Kubernetes Cluster**: A running Kubernetes cluster (e.g., `k3d`).
2. **Docker**: To build the container image.
3. **kubectl**: To interact with the cluster.
4. **k3d**: For local development and testing.

---

## Build and Deploy


### 1. **Build and Load the Docker Image** 
Note: this step needs to be re done if you change the logic fo the controller. 
In case it's NOT your first deployment and you just need to update the image, you can can use the [premade hardcoded script](./update-controller.sh) and skip to testing it.
Otherwise please follow all the directions below.

#### 1.1 **Build the Docker Image**

Build the container image for the Admission Check Controller:

```bash
docker build -t controller:latest .
```

#### 1.2 **Load the Image into `k3d` Cluster**

If you're using a `k3d` cluster (e.g., `my-k3d-cluster`), load the Docker image directly into the cluster:

```bash
k3d image import controller:latest -c my-k3d-cluster
```

if you want to check in which cluster you are in and eventualy switch
```bash
kubectl config get-contexts
kubectl config use-context k3d-my-k3d-cluster
```

#### 1.3 **Verify the Controller Manager Deployment**

```bash
make deploy
```

After deploying, verify that the controller's manager is properly running:

```bash
kubectl get -n acc-3265-system deployments.apps
```

or restart it if you applied any changes
```bash
kubectl rollout restart deployment acc-3265-controller-manager -n acc-3265-system
```

---

### 2. **Install and Configure Kueue**

Install Kueue into the cluster. Note: The second command may fail the first time; retry if needed.

```bash
kubectl apply --server-side -f https://github.com/kubernetes-sigs/kueue/releases/download/v0.9.0/manifests.yaml
kubectl wait deploy/kueue-controller-manager -n kueue-system --for=condition=available --timeout=5m
```

---

### 3. Final Setup of Cluster Queue and Admission Check

#### 3.2 **Set Up a Single Cluster Queue Environment**

Set up the Kueue environment with a single cluster queue:

```bash
kubectl apply -f https://kueue.sigs.k8s.io/examples/admin/single-clusterqueue-setup.yaml
```

#### 3.3 **Create an AdmissionCheck Managed by `demo-acc`**

Create an AdmissionCheck object:

```bash
kubectl apply -f - <<EOF
apiVersion: kueue.x-k8s.io/v1beta1
kind: AdmissionCheck
metadata:
  name: demo-ac
spec:
  controllerName: experimental.kueue.x-k8s.io/demo-acc
EOF
```

Verify that the AdmissionCheck is marked as `Active`:

```bash
kubectl get admissionchecks.kueue.x-k8s.io demo-ac -o=jsonpath='{.status.conditions[?(@.type=="Active")].status}{" -> "}{.status.conditions[?(@.type=="Active")].message}{"\n"}'
```

Expected output:
```plaintext
True -> demo-acc is running
```

#### 3.4 **Add the Admission Check to the Cluster Queue**

Patch the cluster queue to include the AdmissionCheck:

```bash
kubectl patch clusterqueues.kueue.x-k8s.io cluster-queue --type='json' -p='[{"op": "add", "path": "/spec/admissionChecks", "value":["demo-ac"]}]'
```

---

### 4. **Apply Kubernetes Manifests**

Update RBAC permissions, deployment configurations, and apply the default manifests:

```bash
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
kubectl apply -f config/default/
```

---

### Summary of Key Verifications

- **Deployment**: Ensure the controller deployment is running:
  ```bash
  kubectl get deployments -n acc-3265-system
  ```

- **AdmissionCheck Status**:
  ```bash
  kubectl get admissionchecks.kueue.x-k8s.io demo-ac -o=jsonpath='{.status.conditions[?(@.type=="Active")].status}{" -> "}{.status.conditions[?(@.type=="Active")].message}{"\n"}'
  ```

- **Cluster Queue AdmissionCheck**: Confirm the cluster queue is configured with the AdmissionCheck:
  ```bash
  kubectl get clusterqueues.kueue.x-k8s.io cluster-queue -o yaml
  ```

---

## Functionalities

### 1. **Admission Check Controller**

- **Delays workload admission**: Workloads are admitted after one minute from their creation time.
- **Prints resource requests and limits**: Logs workload resource specifications for all containers.

### 2. **Resource Monitoring**

- **Snapshot Logging**: Logs the current cluster resource snapshot whenever a workload is admitted.
- **Node and Pod Monitoring**: Periodically fetches and processes resource usage from nodes and pods.

---

## Testing the Controller

### 1. **Deploy a Sample Workload**

Use the provided `sample-job-limits.yaml` as an example workload:

```bash
kubectl create -f sample-job-limits.yaml
```

### 2. **Monitor Controller Logs**

Check the controller logs to verify admission behavior and resource snapshot logging:

```bash
kubectl logs -n acc-3265-system deployment/acc-3265-controller-manager --since 3m
```

Expected log entries include:
- Workload admission checks.
- Node and pod resource snapshots.

---

## Testing Gang Scheduling

To test the behavior of gang scheduling, you can use the provided script [`gang_test_runner.sh`](test/gang_test_runner.sh). This script creates multiple jobs using the Kubernetes job YAML file `sample-job-limits-gang.yaml`. The job defines a workload that requests **2 CPUs and 1Gi of memory per pod** and runs for 120 seconds. Each job creates 3 pods, which are submitted to the Kubernetes cluster for scheduling.

### How to Run

1. Ensure your cluster has sufficient resources: **more than 30 CPUs and 30Gi of memory**. If your cluster quota is lower, modify the `sample-job-limits-gang.yaml` file to reduce the `cpu` and `memory` requests accordingly.
2. Make the script executable (if not already):
   ```bash
   cd test
   chmod +x gang_test_runner.sh
   ```
3. Run the script:
   ```bash
   ./gang_test_runner.sh
   ```

### What the Script Does
- Clears the `gang_test.txt` file to ensure fresh results.
- Captures the general node resource capacity and appends it to the output file.
- Creates the job (`sample-job-limits-gang.yaml`) 5 times in a loop, waiting 16 seconds between iterations.
- Logs the state of workloads at the end of the test and saves the controller logs from the last 2 minutes.

### Expected Behavior
- The initial job(s) are scheduled and run, consuming resources on the cluster.
- Subsequent job(s) may be suspended due to insufficient resources (if the cluster's available resources are less than required by the job).
- After ~60 seconds, as previous jobs complete, the controller will attempt to admit pending jobs again, leveraging the newly available resources.

```bash
./gang_test_runner.sh
Creating job (iteration 1)...
job.batch/sample-job-gang-jk4qk created
Creating job (iteration 2)...
job.batch/sample-job-gang-bzzr2 created
Creating job (iteration 3)...
job.batch/sample-job-gang-ct56t created
Creating job (iteration 4)...
job.batch/sample-job-gang-gcxft created
Creating job (iteration 5)...
job.batch/sample-job-gang-45knm created
Test completed. Results saved to gang_test.txt.

oc get workloads

---- Workloads state at the end ----

NAME                              QUEUE        RESERVED IN     ADMITTED   FINISHED   AGE
job-sample-job-gang-45knm-54d93   user-queue   cluster-queue                         16s
job-sample-job-gang-bzzr2-4898c   user-queue   cluster-queue   True                  64s
job-sample-job-gang-ct56t-6b2ef   user-queue   cluster-queue   True                  48s
job-sample-job-gang-gcxft-91f87   user-queue   cluster-queue                         32s
job-sample-job-gang-jk4qk-03e37   user-queue   cluster-queue   True                  80s

```

The test is designed to demonstrate the dynamic resource allocation and scheduling of workloads under resource constraints.




---

## Configuration Details

### Controller logic
Controller's logic is defined in [internal/controller/workload_controller.go](internal/controller/workload_controller.go) in the Reconcile function


### RBAC Permissions

- **Nodes**: Required to monitor node resources.
- **Pods**: Required to fetch pod resource details.
- **Workloads and AdmissionChecks**: Core functionality of the admission controller.

RBAC configurations are defined in:
- `config/rbac/role.yaml`
- `config/rbac/role_binding.yaml`


