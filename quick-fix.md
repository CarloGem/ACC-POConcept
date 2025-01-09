Here's a detailed recap of the changes we made to resolve the issues and ensure the `AdmissionCheck` status is marked as `Active`. This recap will serve as a reproducible checklist for similar scenarios.

---

### 1. **Initial Problem**
The `AdmissionCheck` resource remained in an inactive state because the controller manager lacked proper RBAC permissions to manage the necessary resources (`admissionchecks` and `workloads`) in the `kueue.x-k8s.io` API group. This was evident from the logs showing errors like:

- `"cannot list resource "admissionchecks" in API group "kueue.x-k8s.io" at the cluster scope"`
- `"cannot list resource "workloads" in API group "kueue.x-k8s.io" at the cluster scope"`

---

### 2. **Summary of Fixes**
#### **A. Updated `ClusterRole`**
The `ClusterRole` was updated to include the correct API group and resource permissions. The relevant YAML snippet:

```yaml
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
```

#### **B. Fixed `ClusterRoleBinding`**
The `ClusterRoleBinding` was pointing to an incorrect `ServiceAccount` (`controller-manager` in the `system` namespace). It was updated to reference the correct `ServiceAccount` (`acc-3265-controller-manager`) in the `acc-3265-system` namespace. The updated `ClusterRoleBinding`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: acc-3265
    app.kubernetes.io/managed-by: kustomize
  name: manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: manager-role
subjects:
- kind: ServiceAccount
  name: acc-3265-controller-manager
  namespace: acc-3265-system
```

#### **C. Re-applied RBAC Changes**
We applied the updated RBAC configuration using:
```bash
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
```

#### **D. Restarted Controller Manager Deployment**
After updating RBAC, we restarted the controller manager deployment to ensure it picked up the new permissions:
```bash
kubectl rollout restart deployment acc-3265-controller-manager -n acc-3265-system
```

#### **E. Verified Logs and Status**
We validated the controller manager logs to ensure there were no RBAC-related errors:
```bash
kubectl logs -n acc-3265-system deployment/acc-3265-controller-manager
```

Then, we confirmed the `AdmissionCheck` resource status:
```bash
kubectl get admissionchecks.kueue.x-k8s.io demo-ac -o=jsonpath='{.status.conditions[?(@.type=="Active")].status}{" -> "}{.status.conditions[?(@.type=="Active")].message}{"\n"}'
```

---

### 3. **Best Practices for Future Troubleshooting**
1. **Verify RBAC Permissions:**
   - Ensure that your `ClusterRole` includes the correct API groups, resources, and verbs.
   - Check that your `ClusterRoleBinding` points to the correct `ServiceAccount` in the right namespace.

2. **Check Logs for Errors:**
   - Look for RBAC-related errors in the controller manager logs using:
     ```bash
     kubectl logs -n <namespace> deployment/<controller-manager-deployment>
     ```

3. **Confirm Resource Status:**
   - Use `kubectl get` and `kubectl describe` to inspect the status and events for your custom resources.

4. **Apply and Restart:**
   - After updating RBAC configurations, re-apply them and restart the deployment to ensure changes are applied.

---

### 4. **Checklist for Resolving Similar Issues**
- [ ] Update `ClusterRole` with correct API groups, resources, and verbs.
- [ ] Update `ClusterRoleBinding` with correct `ServiceAccount` and namespace.
- [ ] Apply RBAC changes using `kubectl apply`.
- [ ] Restart the deployment using `kubectl rollout restart`.
- [ ] Check logs for errors.
- [ ] Verify the status of the affected resources.

---

### 5. **Outcome**
By following these steps, we resolved the RBAC-related issues and ensured the controller manager could manage the `AdmissionCheck` resources. The `AdmissionCheck` status is now marked as `Active`, indicating successful operation.