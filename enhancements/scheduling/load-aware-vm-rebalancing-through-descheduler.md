---
title: load-aware-vm-rebalancing-through-descheduler
authors:
  - "@ingvagabund"
  - "@ricardomaraschini"
  - "@tiraboschi"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2025-02-17
last-updated: 2025-02-26
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CNV-55593
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Load-aware VM rebalancing through descheduler

## Summary

Descheduling and re-scheduling of VM pods based on requested resources alone is not sufficient for achieving effective resource rebalancing. The existing load-aware descheduling and scheduling plugins provided by the upstream community are still not fully applicable. To address this, an additional mechanism must be implemented to ensure VM-friendly considerations.

This proposal outlines an approach to extend the current descheduler with an additional Prometheus-based plugin, adjusting the KubeVirt stack to collect PSI-based metrics from each host. Additionally, introducing a new taint controller to assist the requested resource oriented Kubernetes scheduler by assigning lower scores to nodes that are soft-tainted based on actual resource utilization.

Load-aware rebalancing is a random process influenced by multiple factors that affect the speed and final placement of pods. To introduce a level of control over these variables, the descheduler operator will be enhanced with a set of experimental tuning options.

## Motivation

By default, VM pods [overcommit CPU](https://kubevirt.io/user-guide/compute/node_overcommit/#node-cpu-allocation-ratio) by a factor of 10 (10x). A node is considered overcommitted when the total sum of CPU limits exceeds its allocatable resources. This high overcommitment ratio results in a broad range of actual resource utilization, making CPU request-based scheduling ineffective.

The default Kubernetes scheduler in OpenShift does not account for actual resource utilization when placing new pods. Instead, it relies solely on requested resources. While a community-built load-aware plugin called [Trimaran](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/pkg/trimaran) exists, it has not been adopted as a default plugin and lacks flexibility for easy extension to consume arbitrary metrics.

Additionally, the Kubernetes Descheduler has recently been improved to consider actual CPU and memory utilization using Kubernetes metrics. However, its current implementation remains simplistic and does not fully align with the needs of VMs, where a pod’s actual resource usage may not accurately reflect the appropriate eviction threshold.

This creates an asymmetry with the scheduler, particularly the default kube-scheduler, which schedules pods based solely on resource requests. As a result, this mismatch could break the assumption that a pod descheduled from an overutilized node will be placed on an underutilized one, since the scheduler lacks awareness of how the descheduler defines overutilized and underutilized nodes. More specifically, during a live migration of a busy VM, the target pod receiving the VM will be identical to the source pod. From the scheduler’s perspective, this means the new pod is treated the same as the original, regardless of the workload that triggered the migration.

### User Stories

* As an administrator, I want to periodically rebalance virtual machines across nodes so I can minimize the number of VMs experiencing resource pressure.
* As an administrator, I want to enhance the default Kubernetes scheduler's sensitivity to load by implementing node-friendly tainting so overly saturated nodes (in terms of resource pressure) are less attractive for scheduling new workloads, helping to balance resource utilization more effectively.
* As an OpenShift engineer, I want to understand the boundaries and limitations between descheduling, scheduling, and virtualization requirements so I can clearly identify which functionalities are supported and feasible within Kubernetes/OpenShift design principles.
* As an administrator, I want to monitor cluster rebalancing and receive alerts when the process takes too long or stops yielding effective improvements so I can prevent unnecessary VM migrations and ensure more efficient resource utilization.

### Goals

* **Load-aware eviction**: VM pods are evicted based on their actual resource consumption, such as high PSI (Pressure Stall Information) resource pressure. The descheduler will be enhanced to incorporate Prometheus metrics for classifying node utilization.
* **Guided scheduling**: While keeping the default Kubernetes scheduler unchanged, nodes are soft-tainted when their actual resource utilization exceeds a predefined threshold. Soft taints help prevent overutilized nodes from becoming completely unschedulable while making them less desirable for scheduling. A soft taint (`effect: PreferNoSchedule`) acts as a preference rather than a strict rule, meaning the scheduler will attempt to avoid placing pods on nodes flagged as overutilized by the descheduler, but it is not guaranteed.
* **Keeping rbac rules restricted**: The descheduler should have read-only permissions, except for accessing the eviction API. Any expansion of permitted rules must be kept to a minimum.
* **Limited tuning**: Eviction, scheduling, and node-tainting are independent actions, which makes VM pod rebalancing not fully predictable. Therefore, experimental parameter tuning will be introduced to accommodate various scenarios:
  * **Multi soft tainting**: Apply one or more soft-taints based on actual utilization—higher utilization results in a greater number of soft-taints.
  * **Adaptive thresholds**: Instead of using static thresholds, dynamically compute the average utilization and define lower and upper thresholds based on deviation from the average.
  * **Multi evictions**: Instead of evicting a single pod per node within a single descheduling cycle, evict more.

### Non-Goals

* **Load-aware scheduling**: It is highly unlikely that the default Kubernetes scheduler will be extended with load-aware plugins in the near future. Therefore, the current set of scheduling capabilities will remain unchanged.
* **Collection of metrics**: Different virtualization use cases may require various metrics for selecting and rebalancing VMs. The descheduler is expected to read a metric that reflects node utilization on a scale from 0 to 1. However, the method of generating these metrics is outside the scope. Future enhancements may introduce mechanisms to produce different types of metrics within this interval.
* **Limited eviction abilities**: Not all resource utilization metrics can determine which pod is causing resource pressure or predict the impact of an eviction. As a result, only the lowest-priority pod is evicted during each descheduling cycle, relying on the cluster to adapt to the change and reflect it back through the metrics. To ensure a quick response to cluster changes, the next descheduling cycle should run within minutes. As a consequence, strategies for evicting multiple pods while achieving more efficient rebalancing are beyond the current scope. With one exception of blindly evicting multiple pods when tuned.
* **Control-plane nodes are excluded from VM scheduling**: VMs are intended to be scheduled only to worker nodes. While an OpenShift cluster can enable scheduling to control-plane nodes, this functionality is not recommended, as it could unexpectedly affect the scheduling of control plane pods due to soft-tainting.
* **Distributive nature of eviction, scheduling and node soft-tainting**: Eviction, scheduling, and node tainting are independent processes. When a VM pod is evicted and a live migration is initiated, its replacement pod is scheduled to a new node based on past node utilization (as the evicted pod still exists in the system). Similarly, node soft-tainting responds to the cluster state recorded before the VM pod eviction was started. This inherent nature of the process cannot be fully resolved but can be regulated by fine-tuning various parameters.
* **Hard-tainting**: hard taints (`effect: NoSchedule`/`effect: NoExecute`)  can restrict scheduling decisions but may also reduce overall cluster flexibility. The node controller already applies certain condition-based hard taints for critical node states such as `MemoryPressure`, `DiskPressure`, and `PIDPressure`. However, these hard taints are only set when a node reaches a serious or critical condition. The goal of this proposal is to adopt a similar approach using a soft taint when a node is overutilized but not yet in a critical state. This ensures that the scheduler is aware of resource pressure without making the node entirely unschedulable.

## Proposal

The enhancements will include the following functionality changes:
1. Updating the descheduler to be load-aware to evict pods based on actual resource utilization.
2. Soft-tainting over-utilized nodes so the requested resources are less effective during scheduling.
3. Extending the descheduler operator customization to enable the load-aware eviction with tuning options.

### Update LowNodeUtilization plugin to consume Prometheus metrics

Currently, the descheduler's `LowNodeUtilization` plugin classifies node utilization based on Kubernetes metrics. However, these metrics primarily capture CPU and memory usage at the node and pod levels, which are not well-suited for VMs. By consuming Prometheus metrics, the plugin can leverage a broader range of data that better aligns with VM-specific needs.

The existing plugin code is already structured to support additional resource clients, making integration straightforward. To simplify implementation, the plugin will accept a single metric entry per node, with values in the <0;1> interval—where higher value indicates higher node utilization. Since resource thresholds are expressed as percentages by default, this classification remains intuitive and easy to interpret.

In the current iteration of the design, PSI pressure metrics were selected. Due to their nature, it is not possible to predict how evicting a single VM will impact a node's reported PSI pressure. Therefore, the plugin will be designed to evict only one VM per over-utilized node per cycle by default with option to evict more. Since it is safe to assume that each evicted VM will undergo live migration and the number of evictions per node is limited, running the descheduling cycle more frequently (e.g., every 3 minutes) will approximate the effect of evicting multiple VMs within a single cycle.

### Soft-taint overutilized nodes to make them less attractive for scheduling

The default Kubernetes scheduler in OpenShift cannot currently be extended with load-aware capabilities. However, applying soft-taints (taints with the `PreferNoSchedule` effect) to over-utilized nodes can guide the scheduler to avoid placing VM pods on these nodes, which would otherwise be selected as targets.

A new taint controller applying the soft taints requires permission to update a node by adding or removing taints. Typically, node update RBAC allows modifications to any part of a node object. However, this permission must be restricted to allow manipulation only for specific taints, ensuring more granular control over node updates. The CEL `ValidatingAdmissionPolicy` can be used to enforce this restriction.

At present, only a single soft-taint is expected to be applied. However, in certain scenarios, applying multiple soft-taints could enhance overall rebalancing by providing more granular scheduling guidance.

The taint controller needs to be allowed to switch to a mode where it only removes the soft-taint(s) from all affected nodes. In case an administrator decides to disable the load-aware functionality.

In general, eviction, scheduling, and tainting steps can occur at any time, making VM rebalancing inherently non-deterministic. As a result, the process may not always converge to an optimal state or may take a long time to do so.

### Both LowNodeUtilization and taint controller consume the same metrics

Both the descheduling plugin and the taint controller consume the same data, though not necessarily identical. If their reconciliation loops are triggered close together, the queried samples may be collected around the same time. However, unless the taint controller always runs before each descheduling cycle, the data they use will differ, varying based on how frequently and when each component runs.

Over time, these reconciliation loops will inevitably desynchronize due to factors such as transient outages, the number of pods and nodes to process, container restarts, and other operational randomness. So, while it may be tempting to share the same snapshot of metrics between both controllers, the benefit would be minimal unless explicit synchronization is required.

The following is enumeration of some approaches for sharing the node clasification:
  1. **Shared volume** mounted in both descheduler and taint controller containers assuming that they are part of the same pod:
     - Pros:
       - Data can be shared through a local file. Transparent outside the pod.
       - The local file can store thousands of nodes without reaching the etcd data entry limit.
     - Cons:
       - The service account is configured at pod level: having the descheduler and the taint controller will not solve the issue of additional privileges needed by that service account in order to be able to taint nodes.
       - Both components need to access and reconcile a file that requires non-standard logic.

  1. **ConfigMap/CR** updated by the descheduler each time it classifies nodes.
     - Pros:
       - Descheduler and taint controller are two separate components, eventually on different nodes, indirectly communicating via the API server.
       - The two separate components are going to be executed by two different service accounts with different privileges.
       - Watch operation by the taint controller is pretty efficient and supposed to get triggered with a low latency.
     - Cons:
       - On clusters with thousands of nodes (especially using a dedicated custom resource) the cost of an update operation on etcd could be not negligible.
       - Malicious user can substitude the content of the file to falsely classify nodes are over-utilized and have the descheduler evict from incorrect nodes.

  1. **Events** generated by the descheduler each time it classifies nodes.
      - Pros
          - Descheduler and taint controller are two separate components, eventually on different nodes, indirectly communicating via the API server.
          - The two separate components are going to be executed by two different service accounts with different privileges.
          - Watch operation by the taint controller is pretty efficient and supposed to get triggered with a low latency.
      - Cons
          - On clusters with thousands of nodes (especially using a dedicated custom resource) the cost of an update operation on etcd could be not negligible.
          - Events are not supposed to be a reliable inter-process communication mechanism.
  1. **Metrics** updated by the descheduler each time it classifies nodes.
      - Pros
          - Descheduler and taint controller are two separate components, eventually on different nodes, indirectly communicating via the API server.
          - The two separate components are going to be executed by two different service accounts with different privileges.
          - Node classification metrics increases observability of the descheduling process.
      - Cons
          - The metric endpoint exposed by the descheduler is usually asynchronously scraped by Prometheus every 30 seconds and this can introduce an additional random latency.
          - The taint controller can only asynchronously read fresh metrics introducing a second source of latency.

### Extend the descheduler operator customization with load aware descheduling

Load-aware descheduling is not enabled by default, as it requires a Prometheus query to identify the appropriate metrics to use. Given the proposal identified tunning options that are VM focused it's more suitable to create a new profile with only `LowNodeUtilization` plugin enabled. Alongside  recommended defaults. Enabling the new profile will instruct the operator to deploy the new taint controller and enabled the descheduler upstream's `EvictionsInBackground` feature. With the following tunning options:
- *metrics data profile*: predefined list of queries
- *utilization thresholds*: predefined list
- *dynamic thresholds*: enabling computation of thresholds based on the average utilization
- *multi soft tainting*: applying more soft-taints based on growing high utilization
- *multi evictions*: evict more pods per node within a single descheduling cycle

For example:

```yaml
apiVersion: operator.openshift.io/v1
kind: KubeDescheduler
metadata:
  name: cluster
  namespace: openshift-kube-descheduler-operator
spec:
  ...
  profileCustomizations:
    # utilization thresholds
    devLowNodeUtilizationThresholds: Medium
    # enable load-aware descheduling
    devActualUtilizationProfile: PrometheusCPUPSIPressure
    # have the thresholds be based on the average utilization
    devEnableDeviationThresholds: false
    # for applying multiple soft-taints instead of a single one
    devMultiSoftTainting: false
    # evict multiple pods per node during a descheduling cycle
    devMultiEvictions: Simple
```

Given the new profile is VM oriented we can expose or extend `devLowNodeUtilizationThresholds` with a different/new predefined thresholds.

To reduce the risk of excessive exposure, the new `devActualUtilizationProfile` field will accept only a predefined set of profiles, each corresponding to a specific Prometheus query. After the initial integration testing and customer feedback, the field can be updated to support a broader range of values.

Additionally, the descheduler already includes the `useDeviationThresholds` withing the [LowNodeUtilization](https://github.com/kubernetes-sigs/descheduler/?tab=readme-ov-file#lownodeutilization) plugin, which alters how utilization thresholds are calculated. Exposing this option through the operator could enhance overall performance during VM rebalancing. This option adjusts the values of `devLowNodeUtilizationThresholds`, as the thresholds are interpreted as the distance from the average utilization, either lower or higher.

To increase the leverage against over-utilized nodes, the taint controller can be configured to apply multiple soft taints based on overall resource pressure through `devMultiSoftTainting` option. For example, when a node is classified as over-utilized and its utilization exceeds 50% of the remaining capacity, a second soft taint could be applied. The scaling of taints can follow different models, such as linear or exponential, depending on the desired behavior.

In certain situations, it may be beneficial to evict multiple pods per node within a single descheduling cycle. By exposing `devMultiEvictions` with predefied values of `Simple` (1), `Modest` (2) and `Rapid` (5), one can explore this direction further.

Currently, there is no dependency between the descheduler and the scheduler, so no additional validation of either configuration is required. However, when the scheduler configuration is extended with load-aware elements, the descheduler operator is expected to scale the descheduler down to zero if an invalid combination of configurations is detected.

### Workflow Description

On the descheduler side:
1. The node classification thresholds gets configured as static (using `thresholds` and `targetThresholds`, as configured in the current `LowNodeUtilization` plugin) or dynamic, based on the state of other nodes in the cluster (using the `useDeviationThresholds` option within the plugin).
1. The `LowNodeUtilization` plugin classifies nodes into three categories: under-utilized, appropriately-utilized, and over-utilized, as it does today. The metric used for classification will be derived from a PromQL query executed against a Prometheus instance. Prometheus will handle the aggregation of historical data and time series processing. The query is expected to return a scalar [0-1] value for each node.
1. The `LowNodeUtilization` plugin will keep identifying pods for eviction. A single or multiple pods with the lowest priority are evicted during each descheduling cycle.

On the soft-tainter controller side:
1. The soft-tainter controller reads the same Prometheus metrics as the descheduler to generate the updated node classification. This ensures that both components are aligned in their understanding of node utilization and can act accordingly based on the latest metrics.
1. Looping through the node list:
    1. The soft-tainter controller will apply a soft taint (`effect: PreferNoSchedule`) to the node if it is not already present and the node is classified as overutilized.
    1. The soft-tainter controller will remove the soft taint from the node if it is present and the node is now classified as either low or appropriately utilized.

### API Extensions

New (TechPreview) descheduler operator profile with a list of (TechPreview) tuning options.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The KubeVirt provider for HCP is using `TopologySpreadConstraints` with a policy of `whenUnsatisfiable: ScheduleAnyway` which is just a soft constraint so it could eventually get ignored.
In order to get it statisfied again when possible, the kube-descheduler-operator should be configured with `SoftTopologyAndDuplicates` profile that will try to evict pods that are not respecting "soft" topology constraints.

*How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?*

Descheduler and the taint controller are expected to be installed within a guest cluster. Never within a management cluster.

#### Standalone Clusters

Assuming a standalone cluster is a self-managed cluster without any external control plane, this case is the standard case.

#### Single-node Deployments or MicroShift

A soft-taint will not provide any benefit on SNO clusters, on the other side being just a soft taint it will never make it unschedulable.

### Implementation Details/Notes/Constraints

#### Pods sorting

Currently, predicting how evicting a pod will impact a node's overall PSI pressure is not feasible. As a result, a straightforward sorting method based on priority classes is used. The descheduler prioritizes evicting as many low-priority pods as possible, assuming they can be relocated without significantly affecting overall workload performance. Potentially allowing other VMs to operate under lower PSI pressure. However, it may not always lead to optimal rebalancing. Alternative pod eviction strategies could yield better results, and this area remains open for exploration in future releases. For example, by leveraging additional metrics and formulating queries more efficiently, metrics providers could identify the most suitable pods for eviction. This approach may enhance decision-making and improve overall rebalancing effectiveness.

#### Multiple evictions within a single cycle

In some situations, evicting multiple pods from the same node within a single descheduling cycle can speed up rebalancing. The effectiveness of this approach depends on which pods are chosen for eviction. For example, removing several low-priority pods may achieve faster rebalancing, or selecting pods from different priority classes could be beneficial. However, the descheduler lacks the necessary expertise to make these decisions effectively. Therefore, the assistance of an external arbiter is required.

#### Factors slowing down the rebalancing

By default, metrics are collected every 30 seconds. Calculating the rate of change over aggregated data takes additional time, potentially delaying the retrieval of Prometheus metrics by several minutes. As a result, the actual utilization data may become outdated. Consequently, both the descheduler and the taint controller make decisions based on previous states. Furthermore, VM live migration, limits on the number of evictions per node or descheduling cycle, and fluctuations in VM resource utilization due to ongoing computations further complicate the rebalancing process. While some of these factors can be mitigated through configuration tuning, others are inherently uncontrollable.

### Risks and Mitigations

* **Node tainting requires additional node permissions.** In order to be able to taint nodes, the service account used by the soft-tainter controller should be allowed to update/patch the spec of nodes. This could be considered too broad.
We can mitigate this risk, regardless of the fact that we use one service account for the descheduler and another one for the soft-tainter, defining also a `ValidatingAdmissionPolicy` (GA since k8s 1.30) to restrict the service account to be able only to apply taints containing a specific prefix in the key (eventually parameterizable with an external object) and also enforcing `PreferNoSchedule` for the taint `effect`.

  For example:
  ```yaml
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: descheduler-cluster-role-node-update
    rules:
      - apiGroups:
          - ""
        resources:
          - nodes
        verbs:
          - patch
          - update
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: descheduler-cluster-role-binding-node-update
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: descheduler-cluster-role-node-update
    subjects:
      - kind: ServiceAccount
        name: descheduler-sa
        namespace: kube-system
    ---
    apiVersion: admissionregistration.k8s.io/v1
    kind: ValidatingAdmissionPolicy
    metadata:
      name: "descheduler-sa-update-nodes"
    spec:
      failurePolicy: Fail
      matchConstraints:
        matchPolicy: Equivalent
        namespaceSelector: {}
        objectSelector: {}
        resourceRules:
          - apiGroups:   [""]
            apiVersions: ["*"]
            operations:  ["UPDATE"]
            resources:   ["nodes"]
            scope: "*"
      matchConditions:
        - name: 'descheduler-sa'
          expression: "request.userInfo.username=='system:serviceaccount:kube-system:descheduler-sa'"
      variables:
        - name: "oldNonDeschedulerTaints"
          expression: "has(oldObject.spec.taints) ? oldObject.spec.taints.filter(t, !t.key.contains('descheduler.kubernetes.io')) : []"
        - name: "oldTaints"
          expression: "has(oldObject.spec.taints) ? oldObject.spec.taints : []"
        - name: "newNonDeschedulerTaints"
          expression: "has(object.spec.taints) ? object.spec.taints.filter(t, !t.key.contains('descheduler.kubernetes.io')) : []"
        - name: "newTaints"
          expression: "has(object.spec.taints) ? object.spec.taints : []"
        - name: "newDeschedulerTaints"
          expression: "has(object.spec.taints) ? object.spec.taints.filter(t, t.key.contains('descheduler.kubernetes.io')) : []"
      validations:
        - expression: |
            oldObject.metadata.filter(k, k != "resourceVersion" && k != "generation" && k != "managedFields").all(k, k in object.metadata) &&
            object.metadata.filter(k, k != "resourceVersion" && k != "generation" && k != "managedFields").all(k, k in oldObject.metadata && oldObject.metadata[k] == object.metadata[k])
          messageExpression: "'User ' + string(request.userInfo.username) + ' is only allowed to update taints'"
          reason: Forbidden
        - expression: |
            oldObject.spec.filter(k, k != "taints").all(k, k in object.spec) &&
            object.spec.filter(k, k != "taints").all(k, k in oldObject.spec && oldObject.spec[k] == object.spec[k])
          messageExpression: "'User ' + string(request.userInfo.username) + ' is only allowed to update taints'"
          reason: Forbidden
        - expression: "size(variables.newNonDeschedulerTaints) == size(variables.oldNonDeschedulerTaints)"
          messageExpression: "'User ' + string(request.userInfo.username) + ' is not allowed to create/delete non descheduler taints'"
          reason: Forbidden
        - expression: "variables.newNonDeschedulerTaints.all(nt, size(variables.oldNonDeschedulerTaints.filter(ot, nt.key==ot.key)) > 0 ? variables.oldNonDeschedulerTaints.filter(ot, nt.key==ot.key)[0].value == nt.value && variables.oldNonDeschedulerTaints.filter(ot, nt.key==ot.key)[0].effect == nt.effect : true)"
          messageExpression: "'User ' + string(request.userInfo.username) + ' is not allowed to update non descheduler taints'"
          reason: Forbidden
        - expression: "variables.newDeschedulerTaints.all(t, t.effect == 'PreferNoSchedule')"
          messageExpression: "'User ' + string(request.userInfo.username) + ' is only allowed to set taints with PreferNoSchedule effect'"
          reason: Forbidden
    ---
    apiVersion: admissionregistration.k8s.io/v1
    kind: ValidatingAdmissionPolicyBinding
    metadata:
      name: "descheduler-sa-update-nodes"
    spec:
      policyName: "descheduler-sa-update-nodes"
      validationActions: [Deny]
    ```

    It's enough to prevent unwanted updates from the `descheduler-sa`:
    ```bash
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa patch node kind-worker -p '{"spec":{"unschedulable":true}}'
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is only allowed to update taints
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa label node kind-worker key=value
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is only allowed to update taints
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa annotate node kind-worker key=value
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is only allowed to update taints
    ```

    It's enough to block the `descheduler-sa` from creating/updating/deleting other taints:
    ```bash
    $ kubectl taint node kind-worker key1=value1:NoSchedule
    node/kind-worker tainted
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint node kind-worker key2=value2:NoSchedule
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is not allowed to create/delete non descheduler taints
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint --overwrite node kind-worker key1=value3:NoSchedule
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is not allowed to update non descheduler taints
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint  node kind-worker key1=value3:NoSchedule-
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is not allowed to create/delete non descheduler taints
    ```

    The `descheduler-sa` can create/update/delete its taints if with `PreferNoSchedule` effect:
    ```bash
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint node kind-worker nodeutilization.descheduler.kubernetes.io/overutilized=level1:PreferNoSchedule
    node/kind-worker tainted
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint --overwrite node kind-worker nodeutilization.descheduler.kubernetes.io/overutilized=level2:PreferNoSchedule
    node/kind-worker modified
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint --overwrite node kind-worker nodeutilization.descheduler.kubernetes.io/overutilized=level2:PreferNoSchedule-
    node/kind-worker modified
    ```
    but it's not able to set and hard taint (effect != `PreferNoSchedule`):
    ```bash
    $ kubectl --as=system:serviceaccount:kube-system:descheduler-sa taint --overwrite node kind-worker nodeutilization.descheduler.kubernetes.io/overutilized=level3:NoSchedule
    Error from server (Forbidden): nodes "kind-worker" is forbidden: ValidatingAdmissionPolicy 'descheduler-sa-update-nodes' with binding 'descheduler-sa-update-nodes' denied request: User system:serviceaccount:kube-system:descheduler-sa is only allowed to set taints with PreferNoSchedule effect
    ```
* **Evicting the lowest priority pod does not necesarilly reduces a PSI pressure.** Currently, there is no straightforward way to predict how evicting a specific pod will impact the overall PSI pressure. Many factors influence the overall resource utilization, such as how CPU and memory resources are allocated on a host, how the container runtime operates, and how system processes function, among others. These factors are complex and difficult to translate into a simple model that can accurately predict the next resource utilization. As a result, instead of attempting to model the eviction's impact, the descheduler plugin evicts only a limited number of pods during each descheduling cycle, allowing the host to facilitate the actual PSI pressure recomputation.
* **Rebalancing may produce unnecessary VM migration or perform poorly.** There are multiple components that participate in the rebalancing process. Each working independently of each other. In addition, VM pods over commit the CPU resource. Which creates a non-deterministic behavior that is impossible to fully predict. The process may converge or alternate. In some cases take siginificantly more time than expected. There's currently no procedure for guaranteeing fast and optional rebalancing other than tunning the provided experimental customization.

### Drawbacks

If we strictly adhere to the *separation of concerns* principle, we might argue that the descheduler should focus solely on the task of descheduling without influencing the resulting scheduling decisions. This is a technically valid point. However, on the other hand, if we accept making the descheduler load-aware (by consuming real utilization metrics), we are implicitly introducing an information imbalance. In this case, the descheduler could evict a pod based on actual node utilization, while the scheduler, working only with static resource reservations, might later make suboptimal decisions. These resource reservations can be significantly different from the real metrics used by the descheduler.

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.
 > 1. **Synchronization:** Should the taint controller be synchronized with the descheduler? In other words, should pods be evicted only after an overutilized node has been soft-tainted?
 > **Answer**: The initial implementation will proceed without synchronization. Based on customer feedback, future iterations may explore the possibility of adding scheduling gates for newly created VM target pods, with the taint controller responsible for removing the gate once all nodes have been properly soft-tainted.
 > 1. **Scheduling:**
    - Will soft-tainting be a sufficient mechanism for making the Kubernetes scheduler repel VM pods from over-utilized nodes? Any other scheduling guidance/constraints we can provide?
    - Will the suggested soft-tainting prevent oscillations and sufficiently accelerate rebalancing?
 > **Answer**: The descheduler operator will provide a set of tuning options to help administrators achieve the optimal balance. Additional tuning options or enhancements may be considered based on customer feedback.
 > 1. **Cost:** Each VM live migration comes with a cost, both in terms of resource consumption and potential performance impact. This raises important questions:
    - Should we impose a limit on the overall rebalancing cost or the number of live migrations per unit of time?
    - Is the current chaotic nature of rebalancing acceptable from a cost perspective?
  Uncontrolled rebalancing could lead to excessive migrations, reducing overall efficiency and potentially degrading workload performance. Introducing constraints—such as rate-limiting migrations or evaluating cost thresholds—could help strike a balance between achieving a well-balanced cluster and minimizing unnecessary disruptions.
 > **Answer**: Currently, there is no SLA defining the expected duration for rebalancing or the maximum number of allowed live migrations. However, an SLA may be introduced in the future based on customer feedback.
 > 1. **Descheduler overstepping:** Should the descheduler provide scheduling instructions by soft-tainting over-utilized nodes?
 > **Answer**: While this could be an interesting avenue to explore, there are currently no plans to extend the descheduler with the ability to provide scheduling hints.
 > 1. **Utilization thresholds:** How to set the thresholds for practical rebalancing? What guidedance/tips to share with administrators to help them set the thresholds right?
 > **Answer**: The descheduler operator will provide a set of tuning options to help administrators achieve the optimal balance. Additional tuning options or enhancements may be considered based on customer feedback.

## Test Plan

* Unit tests are added to verify the integration of `LowNodeUtilization` with Prometheus metrics.
* Unit tests are added to validate the functionality of the new taint controller.
* End-to-end (e2e) tests are added to ensure the taint controller can only add or remove a specific soft taint when updating a node.
* The integration of the descheduler, taint controller, and a CNV cluster is tested to verify overall functionality during VM rebalancing.

## Graduation Criteria

Currently targeted as a Tech Preview, with graduation to GA dependent on customer feedback from the available experimental customization.

Rebalancing is an inherently chaotic process. Multiple iterations of customer feedback may be necessary to refine the solution, ensuring it performs well and meets the standards required for a GA release.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The upgrade/downgrade does not require any external changes. The descheduler operator will either render the additional taint controller or not. The load-aware descheduling will either get enabled or not.

In the worst case, if the taint controller is no longer present, some nodes may retain soft-taints indefinitely. These taints would need to be removed manually. Alternatively, if the taint controller is still present, it can be configured to automatically remove the soft-taints as needed.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

- Use the same debugging procedure as for the descheduler when it is not functioning correctly, such as reviewing the descheduler logs.
- Check the taint controller logs if nodes are incorrectly soft-tainted.
- Review the operator logs or its status to identify potential configuration issues or problems with the descheduler/taint controller not running properly.

## Alternatives

### Extend LowNodeUtilization plugin with ability to soft-taint highly-utilized nodes

The descheduler could potentially apply soft-taints to nodes before performing descheduling, ensuring that the subsequent scheduling decisions consider this information. However, this approach might be viewed as deviating from the descheduler's original purpose, which is focused on evicting pods rather than directly influencing scheduling.

### Have node controller soft-taint node and descheduler evict based on soft-taints presence only

The descheduler does not necessarily need to classify overutilized nodes. Instead, a soft-taint controller can consume Prometheus metrics (or another source of truth) about VM well-being and use the data to identify the most suitable nodes for eviction. This approach makes it easy to replace the controller with a different implementation while keeping the descheduler unaware of resource considerations.

However, the soft-taint controller introduces a single point of failure. If soft-taints are not removed when a node's utilization drops below the configured threshold, the descheduler may continue evicting pods until none are left. To mitigate this risk, a watchdog mechanism should be implemented to help the descheduler recognize nodes whose soft-taints have not been updated recently. This would allow the system to distinguish between nodes that remain overutilized due to slow VM migrations and those where a single eviction has successfully reduced utilization below the threshold.

One key benefit of this approach is that nodes can be soft-tainted before any VM pods are evicted, allowing the Kubernetes scheduler to make more informed scheduling decisions. This is more effective than applying soft-taints at random points and evicting VM pods before tainting, which could lead to suboptimal scheduling outcomes.

The descheduler's `RemovePodsViolatingNodeTaints` plugin (with `includePreferNoSchedule=true`) is well-suited for this approach, as it only needs to consider whether a node has been soft-tainted with the expected taint. Since the impact of evicting a single VM pod on overall node resource utilization or pressure is uncertain, each descheduling cycle is limited to evicting only one pod. This can be enforced by setting `maxNoOfPodsToEvictPerNode` to `1`. However, the plugin currently does not sort pods based on their priority, which needs to be addressed upstream before this approach can be fully effective.


### Scheduling gating for newly created VM pods during live migration

Due to the current lack of synchronization between the descheduler and the taint controller, newly created pods might set a single [scheduging gate](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-scheduling-readiness/) and wait until the taint controller applies soft taints to the nodes and removes the scheduling gate. Whether this functionality can be incorporated depends on potential updates to the [VM migrator](https://github.com/kubevirt/kubevirt/blob/2fb7868063bf55b3d507d5abacdf4f583ad5941f/pkg/virt-controller/watch/migration/migration.go#L787) can be updated to provide such functionality. The expected workflow is as follows:
1. A VM pod is evicted.
1. The VM migrator creates a target pod with a scheduling gate, e.g., `kubevirt.io/schedulinggated`.
1. The taint controller detects the creation of the new pod with the scheduling gate.
1. The taint controller ensures that all nodes are properly soft-tainted.
1. The taint controller removes the scheduling gate from the new VM pods.

The presence of a scheduling gate introduces an additional point of failure if the taint controller stops running. To mitigate this, the migrator could implement a timeout mechanism for each scheduling gate, automatically removing the gate if it times out to prevent blocking the live migration.

## Infrastructure Needed [optional]

N/A
