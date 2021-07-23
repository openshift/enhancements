# Manifest Annotation For Object Deletion

Developers can remove any of the currently managed CVO objects by modifying an existing manifest and adding the delete annotation `.metadata.annotations["release.openshift.io/delete"]="true"`. This manifest annotation is a request for the CVO to delete the in-cluster object instead of creating/updating it.

Actual object deletion and subsequent deletion monitoring and status checking will only occur during an upgrade. During initial installation the delete annotation prevents further processing of the manifest since the given object should not be created.

## Implementation Details

When the following annotation appears in a CVO supported manifest and is set to `true` the associated object will be removed from the cluster by the CVO.
Values other than `true` will result in a CVO failure and should therefore result in CVO CI failure.
```yaml
apiVersion: apps/v1
...
metadata:
...
  annotations:
    release.openshift.io/delete: "true"
```
The existing CVO ordering scheme defined [here](operators.md) is also used for object removal. This provides a simple and familiar method of deleting multiple objects by reversing the order in which they were created. It is the developer's responsibility to ensure proper deletion ordering and to ensure that all items originally created by an object are deleted when that object is deleted. For example, an operator may have to be modified, or a new operator created, to take explicit actions to remove external resources. The modified or new operator would then be removed in a subsequent update.

Similar to how CVO handles create/update requests, deletion requests are implemented in a non-blocking manner whereby CVO issues the initial request to delete an object kicking off resource finalization and after which resource removal. CVO does not wait for actual resource removal but instead continues. CVO logs when a delete is initiated, that the delete is ongoing when a manifest is processed again and found to have a deletion time stamp, and delete completion upon resource finalization.

If an object cannot be successfully removed CVO will set `Upgradeable=False` which in turn blocks cluster update to the next minor release.

## Examples

The following examples provide guidance to OpenShift developers on how resources may be removed but this will vary depending on the component.
Ultimately it is the developer's responsibility to ensure the removal works by thoroughly testing.
In all cases, and as general guidance, an operator should never allow itself to be removed if the operator's operand has not been removed.

### The autoscaler operator

Remove the cluster-autoscaler-operator deployment.
The existing cluster-autoscaler-operator deployment manifest 0000_50_cluster-autoscaler-operator_07_deployment.yaml is modified to contain the delete annotation:
```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-autoscaler-operator
  namespace: openshift-machine-api
  annotations:
    release.openshift.io/delete: "true"
...
```
Additional manifest properties such as `spec` may be set if convenient (e.g. because you are looking to make a minimal change vs. a previous version of the manifest), but those properties have no affect on manifests with the delete annotation.

### The service-catalog operators

In release 4.5 two jobs were created to remove the Service Catalog - openshift-service-catalog-controller-manager-remover and openshift-service-catalog-apiserver-remover.
In release 4.6, these jobs and all their supporting cluster objects also needed to be removed.
The following example shows how to do Service Catalog removal using the object deletion manifest annotation.

The Service Catalog is composed of two components, the cluster-svcat-apiserver-operator and the cluster-svcat-controller-manager-operator.
Each of these components use manifests for creation/update of the component's required resources: namespace, roles, operator deployment, etc.
The cluster-svcat-apiserver-operator had [these associated manifests][svcat-apiserver-4.4-manifests].

The deletion annotation would be added to these manifests:

* `0000_50_cluster-svcat-apiserver-operator_00_namespace.yaml` containing the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_50_cluster-svcat-apiserver-operator_02_config.crd.yaml` containing a cluster-scoped CRD.
* `0000_50_cluster-svcat-apiserver-operator_03_config.cr.yaml` containing a cluster-scoped, create-only ServiceCatalogAPIServer.
* `0000_50_cluster-svcat-apiserver-operator_04_roles.yaml` containing a cluster-scoped ClusterRoleBinding.
* `0000_50_cluster-svcat-apiserver-operator_08_cluster-operator.yaml` containing a cluster-scoped ClusterOperator.

These manifests would be dropped because their removal would occur as part of one of the above resource deletions:

* `0000_50_cluster-svcat-apiserver-operator_03_configmap.yaml` containing a ConfigMap in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_50_cluster-svcat-apiserver-operator_03_version-configmap.yaml` containing another ConfigMap in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_50_cluster-svcat-apiserver-operator_05_serviceaccount.yaml` containing a ServiceAccount in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_50_cluster-svcat-apiserver-operator_06_service.yaml` containing a Service in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_50_cluster-svcat-apiserver-operator_07_deployment.yaml` containing a Deployment in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_90_cluster-svcat-apiserver-operator_00_prometheusrole.yaml` containing a Role in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_90_cluster-svcat-apiserver-operator_01_prometheusrolebinding.yaml` containing a RoleBinding in the `openshift-service-catalog-apiserver-operator` namespace.
* `0000_90_cluster-svcat-apiserver-operator_02-operator-servicemonitor.yaml` containing a ServiceMonitor in the `openshift-service-catalog-apiserver-operator` namespace.

So the remaining manifests with deletion annotations would be the namespace and the cluster-scoped CRD, ServiceCatalogAPIServer, ClusterRoleBinding, and ClusterOperator.
The ordering of the surviving manifests would not be particularly important, although keeping the namespace first would avoid removing the ClusterRoleBinding while the consuming Deployment was still running.
If multiple deletions are required, it is up to the developer to name the manifests such that deletions occur in the correct order.

Similar handling would be required for the svcat-controller-manager operator.

If resources external to kubernetes must be removed the developer must provide the means to do so.
This is expected to be done through modification of an operator to do the removals during it's finalization.
If operator modification for object removal is necessary that operator would be deleted in a subsequent update.

The deletion manifests described above would have been preserved through 4.5.z release and removed in 4.6.
See the [Subsequent Releases Strategy](#subsequent-releases-strategy) section for details.

## Removing functionality that users might notice

Below is the flow for removing functionality that users might notice, like the web console.

* The first step is deprecating the functionality.
    During the deprecation release 4.y, the functionality should remain available, with the operator setting Upgradeable=False and linking release notes like [these][deprecated-marketplace-apis].
* Cluster Administrators must follow the linked release notes to opt in to the removal before updating to the next minor release in 4.(y+1).
    When the administrator opts in to the removal, the operator should stop setting Upgradeable=False.
* Depending on how the engineering team that owns the functionality implements its removal, the operand components may be removed when Cluster Administrators opt-in to the removal, or they may be left running and be removed during the transition to the next minor release 4.(y+1).
* During the update to the next minor release 4.(y+1) the manifest delete annotation would be used to remove the operator, and, if they have not already been removed as part of the opt-in in release 4.y, any remaining operand components.

## Subsequent Releases Strategy

Special consideration must be given to subsequent updates which may still contain manifests with the delete annotation.
These manifests will result in `object no longer exists` errors assuming the current release had properly and fully removed the given objects.
It is acceptable for subsequent z-level updates to still contain the delete manifests but minor level updates should not and therefore the handling of the delete error will differ between these update levels.
A z-level update will be allowed to proceed while a minor level update will be blocked.
This will be accomplished through the existing CVO precondition mechanism which already behaves in this manner with regard to z-level and minor updates.

[svcat-apiserver-4.4-manifests]: https://github.com/openshift/cluster-svcat-apiserver-operator/tree/aa7927fbfe8bf165c5b84167b7c3f5d9cb394e14/manifests
[deprecated-marketplace-apis]: https://docs.openshift.com/container-platform/4.4/release_notes/ocp-4-4-release-notes.html#ocp-4-4-marketplace-apis-deprecated
