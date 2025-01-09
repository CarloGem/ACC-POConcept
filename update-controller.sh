#!/bin/bash

# Script to build, load, deploy, and verify the Admission Check Controller.

set -e

echo "### Step 1: Build the Docker Image for the Controller ###"
docker build -t controller:latest .

echo "### Step 2: Load the Image into the k3d Cluster ###"
k3d image import controller:latest -c my-k3d-cluster
kubectl config use-context k3d-my-k3d-cluster

echo "### Step 3: Delete old deployment and Deploy with Updated Image ###"
kubectl delete deployment acc-3265-controller-manager -n acc-3265-system
make deploy

echo "### Step 3.1: Update Deployment Image ###"
kubectl set image deployment/acc-3265-controller-manager manager=controller:latest -n acc-3265-system


echo "### Step 4: Restart the Deployment to Apply Changes ###"
kubectl rollout restart deployment acc-3265-controller-manager -n acc-3265-system

echo "### Step 4.1: Overwrite role.yaml with updated permissions ###"
cat <<EOF > config/rbac/role.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: manager-role
rules:
- apiGroups:
  - kueue.x-k8s.io
  resources:
  - admissionchecks
  - workloads
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - kueue.x-k8s.io
  resources:
  - admissionchecks/finalizers
  - workloads/finalizers
  verbs:
  - update
- apiGroups:
  - kueue.x-k8s.io
  resources:
  - admissionchecks/status
  - workloads/status
  verbs:
  - get
  - patch
  - update
# Add permissions for nodes and pods monitoring
- apiGroups:
  - "" # Core API group
  resources:
  - nodes
  - pods
  verbs:
  - list
  - watch
EOF


echo "### Step 4.1 Patch the cluster queue to include the AdmissionCheck: ###"
kubectl patch clusterqueues.kueue.x-k8s.io cluster-queue --type='json' -p='[{"op": "add", "path": "/spec/admissionChecks", "value":["demo-ac"]}]'

echo "### Step 5: Apply permissions for monitoring and scheduling ###"
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

echo "### Step 6: Verify the Deployment Status. It may indicate it si not Ready, in that case check manually with: ###"
echo "\t kubectl get -n acc-3265-system deployments.apps"

kubectl get -n acc-3265-system deployments.apps

echo "### Step 7: Verify the AdmissionCheck Status ###"
admission_check_status=$(kubectl get admissionchecks.kueue.x-k8s.io demo-ac -o=jsonpath='{.status.conditions[?(@.type=="Active")].status}{" -> "}{.status.conditions[?(@.type=="Active")].message}{"\n"}')

if [[ "$admission_check_status" == "True -> demo-acc is running" ]]; then
  echo "AdmissionCheck is Active: $admission_check_status"
else
  echo "AdmissionCheck Status Check Failed: $admission_check_status"
  exit 1
fi

echo "### Update Completed Successfully ###"
