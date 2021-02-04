---
title: automated-cpu-resource-request-scaling
authors:
  - "@dhellmann"
  - "@csrwng"
  - "@wking"
  - "@browsell"
reviewers:
  - TBD
approvers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@markmc"
creation-date: 2021-02-04
last-updated: 2021-02-04
status: provisional
see-also:
  - "/enhancements/single-node-production-deployment-approach.md"
---

# Automated CPU Resource Request Scaling for Control Plane

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes an approach to allow us to scale the
resource requests for the control plane services to reduce consumption
for constrained environments. This will be especially useful for
single-node production deployments, where the user wants to reserve
most of the CPU resources for their own workloads and needs to
configure OpenShift to run on a fixed number of CPUs within the host.

One example of this use case is seen in telecommunication service
providers implementation of a Radio Access Network (RAN). This use case
is discussed in more detail below.

## Motivation

The resource requests for cluster operators and their operands are
based on performance analysis on multiple cloud VMs using the
end-to-end test suite to gather data. While the resulting numbers work
well for similar cloud environments and even multi-node bare metal
deployments, they result in over-provisioning resources for
single-node deployments.

### Goals

* This enhancement describes an approach for configuring OpenShift
  clusters to run with lower CPU resource requests than its default
  configuration. The current resource requirements for an OpenShift
  control plane exceed the size desired by some customers running on
  bare metal.
* Clusters built in this way should pass the same Kubernetes and
  OpenShift conformance and functional end-to-end tests as single-node
  deployments that have not been scaled down.
* We have a goal of 4 hyperthreads today, but we do not know what
  constraints we might be given in the future so we need a solution
  that is not tied to the current known limit.
* We want a general approach, that can be applied to all OpenShift
  control plane components.

### Non-Goals

* This enhancement is focused on CPU resources. Other compressible
  resource types may need to be managed in the future, and those are
  likely to need different approaches.
* This enhancement does not address non-compressible resource
  requests, such as for memory.
* This enhancement does not attempt to describe application-specific
  APIs for tuning the configuration of individual operators or their
  workloads.
* This enhancement does not address ways to disable operators or
  operands entirely.
* Although the results of this enhancement may be useful for
  single-node developer deployments, multi-node production
  deployments, and Internet-of-things devices, those use cases are not
  addressed directly in this document.
* This enhancement describes a way to limit the CPU resource
  requests. It does not address reducing actual utilization. There is
  no expectation that a cluster configured to use fewer than the
  default resources would offer exactly the same performance as the
  default. It must be stable and continue to operate reliably, but may
  respond more slowly.
* This enhancement does not address scaling the control plane **up**.

## Proposal

There are two aspects to manage scaling the control plane resource
requests. Most of the cluster operators are deployed by the
cluster-version-operator using static manifests that include the
resource requests. Those operators then deploy other controllers or
workloads dynamically. We need to account for both sets of components.

### Deployments managed by the cluster-version-operator or operator-lifecycle-manager

To start, we will [reduce the default
requests](https://bugzilla.redhat.com/show_bug.cgi?id=1920159) for
cluster operators deployed by the cluster-version-operator with static
manifests. By re-tuning those settings by hand, we may be able to
avoid implementing a more complex system to change them dynamically
within the cluster.

A similar approach can be taken for operators installed via the
operator-lifecycle-manager.

### Deployments managed by cluster operators

We are considering 2 approaches for operands of cluster operators.

#### Kubelet over-subscription configuration option

Today, kubelet subtracts reserved CPUs from the available resources
that it reports, under the assumption that they will be entirely
consumed by workloads. Sometimes workloads do not entirely consume
those CPUs, however, and non-guaranteed pods can also use the reserved
CPU pool. A change to kubelet to provide a static over-subscription
configuration option would provide a global approach to allowing the
CPU resources reported by kubelet to be adjusted by a factor defined
by the user to account for this difference, based on the workloads
planned for the cluster.

The new configuration option needs to be added to kubelet upstream in
order to be supported long-term in OpenShift. A KEP will be written
and submitted independently of this enhancement document.

In OpenShift, the option would be set during installation and would be
added to the control plane `MachineConfig`.

We would need to carefully consider how to document the use of this
option. Users will need to understand the minimum CPU resource
requests of a base OpenShift deployment and any additional add-on
operators before they will be able to tune the over-subscription
setting so that all the components will fit onto the number of CPUs
they require. We will need to keep the documentation of minimum CPU
resource requests for OpenShift up to date for each release, or
provide a tool to calculate the value from a default cluster.

Refer to the implementation history section below for details of a
proof-of-concept implementation of this approach.

#### Cluster-wide resource request limit API

The kubelet change in option 1 requires work to ensure the option is
accepted upstream. That requirement introduces risk, if the upstream
community does not like the option or if it takes a long time to reach
agreement. As an alternative approach, we are considering a new
cluster-wide API.

Tuning for operator-managed workloads should be controlled via an API,
so that operators can adjust their settings correctly. We do not want
users to change the setting after building the cluster (at least for
now), so the API can be a status field. As with other
infrastructure-related settings, the field will be added to the
`infrastructure.config.openshift.io` [informational
resource](https://docs.openshift.com/container-platform/4.6/installing/install_config/customizations.html#informational-resources_customizations).

The new `controlPlaneCPULimit` field will be a positive integer
representing the whole number of [Kubernetes
CPUs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu)
into which the control plane must fit. Expressing the API this way
makes it easy for the user to declare the requirements up front when
installing the cluster, without having to understand the basis for the
default settings and without having to calculate a scaling factor
themselves.

In order for operators to know how to use `controlPlaneCPULimit` to
compute a scaling factor, they need to know the default number of CPUs
assumed for a control plane that produced the default resource
limit. Rather than encoding that information separately in every
operator implementation, we should put it in one place so it can be
managed consistently. Using a `const` defined in the library with the
Infrastructure type makes it easy to upgrade operators if the value
changes across releases. So, a new `DefaultControlPlaneCPUCount`
constant will be added to the module with the Infrastructure type.

If `controlPlaneCPULimit == 0`, operators should use their
default configuration for their operands.

If `0 < controlPlaneCPULimit < DefaultControlPlaneCPUCount`, operators
should scale their operands down by around `(controlPlaneCPULimit /
DefaultControlPlaneCPUCount)` or more, if appropriate. When scaling,
the operators should continue to follow the other guidelines for
resources and limits in
[CONVENTIONS.md](CONVENTIONS.md#resources-and-limits).

If `controlPlaneCPULimit >= DefaultControlPlaneCPUCount`, operators
should use their default configuration for their operands. We do not
currently have a need to allow the control plane to scale up, although
this may change later. If we do have a future requirement to support
that, we could remove this restriction.

The logic described in this section could also be captured in a
library function. Something like

```go
cpuRequestValue, err := GetScaledCPURequest(kubeClientConfig, myDefaultValue)
```

where the function fetches the Infrastructure resource and applies the
algorithm to a default value provided by the caller. That would make
the code change to implement the approach in each operator smaller,
since we could conceivably wrap the existing static values with the
function call.

Users will specify the `controlPlaneCPULimit` when building a cluster
by passing the value to the installer in `install-config.yaml`.

### User Stories

#### Radio Access Network (RAN) Use Case

In the context of telcocommunications service providers' 5G Radio Access
Networks, it is increasingly common to see "cloud native" implementations
of the 5G Distributed Unit (DU) component. Due to latency constraints,
this DU component needs to be deployed very close to the radio antenna for
which it is responsible. In practice, this can mean running this
component on anything from a single server at the base of a remote cell
tower or in a datacenter-like environment serving several base stations.

A hypothetical DU example is an unusually resource-intensive workload,
requiring 20 dedicated cores, 24 GiB of RAM consumed as huge pages,
multiple SR-IOV NICs carrying several Gbps of traffic each, and
specialized accelerator devices. The node hosting this workload must
run a realtime kernel, be carefully tuned to ensure low-latency
requirements can be met, and be configured to support features like
Precision Timing Protocol (PTP).

The most constrained resource for RAN deployments is CPU, so for now
the focus of this enhancement is scaling CPU requests.  Kubernetes
resource requests for CPU time are measured in fractional "[Kubernetes
CPUs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu)". For
bare metal, 1 Kubernetes CPU is equivalent to 1 hyperthread.

Due to the resource-intensive workload, the overhead for platform
components such as the OpenShift platform and add-on operators is
severely restricted when compared to other uses. For example, one
customer has allocated 4 hyperthreads for all components other than
their own workloads. OpenShift currently requires 7.

### Implementation Details/Notes/Constraints

1. Add the new field to the `Infrastructure` API.
2. Expose the `controlPlaneCPULimit` setting through the installer via
   `install-config.yaml`.
3. Update operators to respond to the API, as described above.

### Risks and Mitigations

If we choose to add a cluster-wide API and update operators to use it
to configure their operands, then we may have to update a lot of
different operators. We could mitigate that by starting with some of
the most expensive operators and either having them adjust their
workloads based on the new API or use the existing infrastructure
topology API. Using the existing API would mean we could take more
time to think about what API we do want, but we would be giving up the
ability to tune the configuration of operators for more scenarios.

The operators with the highest excess requests compared to idle use
include:

| Namespace | Pod | OCP 4.6 Average Use at rest | Request Size | Difference | % Difference |
| --------- | --- | ----------------------------| -------------| ---------- | ------------ |
| openshift-etcd | etcd | 200 | 350 | 150 | 43% |
| openshift-oauth-apiserver | apiserver | 6 | 150 | 144 | 96% |
| openshift-apiserver | apiserver | 16 | 125 | 109 | 87% |
| openshift-controller-manager | controller-manager | 3 | 100 | 97 | 97% |
| openshift-ingress | router-default | 4 | 100 | 96 | 96% |
| openshift-dns | dns-default | 4 | 65 | 61 | 94% |
| openshift-kube-apiserver | kube-apiserver | 230 | 290 | 60 | 21% |
| openshift-kube-controller-manager | kube-controller-manager | 23 | 80 | 57 | 71% |

## Design Details

### Open Questions

1. If manually re-tuning the settings for the static manifests managed
   by the cluster-version-operator is not sufficient, what should we
   do?
2. How do we express to the cluster operators that they should scale
   their operands?
3. How dynamic do we actually want the setting to be? Should the user
   be able to change it at will, or is it a deployment-time setting?
4. How do we handle changes in the base request values for a cluster
   operator over an upgrade?
5. Are operators allowed to place a lower bound on the scaling? Is it
   possible to request too few resources, over subscribe a CPU, and
   cause performance problems for the cluster?

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

Clusters deployed from versions that do not support scaling will show
a value of `0`, causing operators that support scaling to use their
default settings.

Operators from versions that do not support scaling will ignore the
new field until they are upgraded.

The value of `DefaultControlPlaneCPUCount` constant may change between
releases. The value from the library will be vendored into each
operator, so the scaling formula for a given version of an operator
will be correct at all times during an upgrade, even if different
operators are using different base values.

## Implementation History

Some [work to reduce the static default requests for cluster
operators](https://bugzilla.redhat.com/show_bug.cgi?id=1920159) has
been done in the 4.8 release.

### Proof-of-concept for kubelet over-subscription option

A proof-of-concept implementation of a kubelet configuration option to
declare a single static over-subscription value has been
built.

#### Code changes

We want to increase (compensate) allocatable by the
`overSubscriptionCapacity` amount.  A change was made to the
`node_container_manager_linux.go` by introducing a static
oversubscription capacity of 1 CPU.

```diff
[root@cnfdd3-installer kubernetes]# git diff
diff --git a/pkg/kubelet/cm/cpumanager/cpu_manager.go b/pkg/kubelet/cm/cpumanager/cpu_manager.go
index 6acd8ebc660..6a8d2e94043 100644
--- a/pkg/kubelet/cm/cpumanager/cpu_manager.go
+++ b/pkg/kubelet/cm/cpumanager/cpu_manager.go
@@ -160,6 +160,9 @@ func NewManager(cpuPolicyName string, reconcilePeriod time.Duration, machineInfo
                // exclusively allocated.
                reservedCPUsFloat := float64(reservedCPUs.MilliValue()) / 1000
                numReservedCPUs := int(math.Ceil(reservedCPUsFloat))
+               if specificCPUs.Size() > 0 {
+                       numReservedCPUs = specificCPUs.Size()
+               }
                policy, err = NewStaticPolicy(topo, numReservedCPUs, specificCPUs, affinity)
                if err != nil {
                        return nil, fmt.Errorf("new static policy error: %v", err)
diff --git a/pkg/kubelet/cm/node_container_manager_linux.go b/pkg/kubelet/cm/node_container_manager_linux.go
index fc7864dacc2..78d8c7bbde6 100644
--- a/pkg/kubelet/cm/node_container_manager_linux.go
+++ b/pkg/kubelet/cm/node_container_manager_linux.go
@@ -211,6 +211,7 @@ func (cm *containerManagerImpl) getNodeAllocatableInternalAbsolute() v1.Resource
 func (cm *containerManagerImpl) GetNodeAllocatableReservation() v1.ResourceList {
        evictionReservation := hardEvictionReservation(cm.HardEvictionThresholds, cm.capacity)
        result := make(v1.ResourceList)
+       overSubscriptionCapacity := resource.NewMilliQuantity(1000, resource.DecimalSI)
        for k := range cm.capacity {
                value := resource.NewQuantity(0, resource.DecimalSI)
                if cm.NodeConfig.SystemReserved != nil {
@@ -222,6 +223,9 @@ func (cm *containerManagerImpl) GetNodeAllocatableReservation() v1.ResourceList
                if evictionReservation != nil {
                        value.Add(evictionReservation[k])
                }
+               if k == "cpu" {
+                      value.Sub(*overSubscriptionCapacity)
+                }
                if !value.IsZero() {
                        result[k] = *value
                }
```

#### Results

It can be seen that oversubscription capacity (1 cpu) has been added
to node allocatable ( Capacity = 104, reserved = 4, oversubscription =
1, allocatable = capacity - reserved + oversubscription = 104 - 4 + 1
= 101)

Node report:

```text
Capacity:
  cpu:                104
...
Allocatable:
  cpu:                101
```

#### Productization aspects

1. Provisioning of the oversubscription parameter

   The approach of providing the node a fixed amount of
   oversubscription capacity requires a new parameter provision to the
   kubelet. This new parameter must be added to both kubelet command
   line arguments and a corresponding parser should be added to the
   KubeletConfig resource handler. The parameter should be optional
   and can be set to zero if undefined.  The deployment will look as
   follows:

   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: KubeletConfig
   spec:
     kubeletConfig:
       cpuOverSubscriptionCapacity: 1.5
   ```

2. Node report

   The oversubscription capacity will alter the node reports and can
   potentially mislead cluster administrators and
   developers. Therefore it is essential that oversubscription
   capacity will be reflected in the node report, for example:

   ```text
   Capacity:
   cpu: 104
   cpu oversubscription: 1.000
   ```

3. Logging

   Kubelet logs reporting Allocatable, Capacity and Reservations
   should include the new parameter where applicable.

4. Testing

   Additional testing will be required to verify that oversubscription
   parameter addition does not cause regression in different
   configuration cases (at least all permutations of kube / system /
   explicitly reserved)

5. Documentation

   The parameter must be documented in the official documentation

6. Upgrades and downgrades

   The oversubscription capacity parameter should be seamless for
   upgrades and downgrades. Need to verify that kubelet doesn’t crash
   in case of an unknown command line argument.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

The best approach to achieve the goals described above is not clear,
so the alternatives section is a collection of candidates for
discussion.

### Expressing a need to scale using an API

These alternatives discuss what the API would look like, without
considering which CRD it would be on.

1. A CPU resource request size parameter

   We could define an API to give a general size parameter, using values
   like t-shirt sizes (large, medium, small) or flavors.

   Such an API is vague, and it would be up to teams to interpret the
   size value appropriately. We would also be limited in the number of
   sizes we could reasonably describe to users before it would become
   confusing (see Amazon EC2 flavors).

2. A CPU resource "request limit" parameter

   We could define an API to give an absolute limit of the CPU resource
   requests for the cluster services.

   Operators would need to scale the resource requests for their operands
   based on an understanding of how their defaults were derived. For
   example, today most values are a proportion of the resources needed
   for etcd on a 3 node control plane where each node has 2 CPUs, for a
   total of 6 CPUs. If the API indicates a limit of 2 CPUs, the operator
   would need to configure its operands to request 1/3 of the default
   resources.

3. A CPU resource scaling parameter

   We could define an API to give a scaling parameter that operators
   should use for their operands.

   The value would be applied globally based on understanding how the
   defaults are set for 6 CPUs and how many CPUs are available in the
   real system. We could document the 6 CPU starting point to allow users
   to make this calculation themselves.

   Operators could read the setting and use it for their operands, but
   not themselves.

Implementing any of these options would require modifying multiple
operators. It is likely that we would need to update all of them,
although we could prioritize the ones with the largest current resource
requests.

### Locations for API setting

These alternatives could apply to any of the API forms discussed in
the previous section.

1. Use a cluster-wide API

   We could add a field to the Infrastructure configuration
   resource. Many of our operators, including OLM-based operators,
   already consume the Infrastructure API.

2. Use an operator-specific API

   If we choose to implement a new operator to manage scaling for the
   control plane operators (see below), it could define a new API in a
   CRD it owns.

   Updating existing operators to read the new field and scale their
   operands and documenting an entirely new API might be more work
   than using the Infrastructure API that would only be useful if the
   operator responding to the API was an add-on.

### Options for declaring the base value for the CPU limit API

In order for an operator to use a cluster-wide CPU resource request
limit API, it needs to know the number of CPUs used for the
calculation of the default settings.

1. Expose the value through the Infrastructure config API.

   We could publish the value as part of the status fields of the
   Infrastructure API. This would make it easy for all operators to
   use it, assuming the value is correct and relevant (it may not be
   for OLM operators).

   The value would only need to be set once, in the installer, and
   could be changed in later versions without updating the source for
   any operators. In order to change the behavior in an upgraded
   cluster, something would have to modify the Infrastructure config
   resource in the existing cluster.

2. Expose the value as a `const` in the library where the
   Infrastructure config API is defined.

   We could publish the value only in `go` source code, using a
   `const` in the same module where the Infrastructure API `struct` is
   defined. Consumers of the API that import the code would be able to
   use the value, and receive updates by updating to a new version of
   the library.

   If the value changes between releases, upgraded operators would
   have the new value.

### Dynamic vs. Static Setting

It is not likely that users would need to change the setting after
building a cluster, so we could have the installer use a value from
the `install-config.yaml` to set a status field in whatever CRD we
choose for the API. If we do need to support scaling later, that work
can be described in another enhancement.

### Managing settings for operators installed by cluster-version-operator

The alternatives in this subsection cover the need to change the
settings for the static manifests currently managed by the
cluster-version-operator. These approaches would only apply to the
operators themselves, and not their operands.

1. Use a cluster profile to change the resource requests for control
   plane operators

   We could create a separate cluster profile with different resource
   requests for our control plane components.

   This would be quite a lot of work to implement and test.

   We may eventually need different profiles for different customers,
   further multiplying the amount of work.

   As an organization, we are trying to keep the number of profiles
   small as a general rule. The discussion about using a cluster
   profile for deploying single production instances provides some
   useful background. See
   [#504](https://github.com/openshift/enhancements/pull/504),
   [#560](https://github.com/openshift/enhancements/pull/560), and
   [single-node-deployment-approach](single-node/production-deployment-approach.md).


   Profile settings only apply to the cluster operators Deployments,
   and changing the resource requests in the static manifests does not
   signal to the operators that they need to change the settings for
   their operands.

2. Lower the resource requests for cluster operators statically in the
   default configuration

   We could apply changes to the default resource requests in the
   static manifests used by all deployments, instead of adding one or
   more new cluster profiles.

   This would affect all clusters, and if we are too aggressive with
   the changes we may lower values too much and cause important
   cluster operators to be starved for resources, triggering degraded
   cluster performance.

   The static settings only apply to the cluster operators
   Deployments, and changing them does not signal to the operators
   that they need to change the settings for their operands.

   With lowered CPU requests, the consumption of some operators may no
   longer match the requests closely, especially for periodically
   "bursty" workloads. One way to address that is by restructuring
   some operators to use secondary Jobs to run some expensive
   operations. For example, the insights operator could schedule a Job
   to collect and push data. The operator would consume relatively few
   resources, and the Job would be ephemeral. We will need to perform
   significant analysis to identify opportunities for these sort of
   changes.

   Some [work to reduce default
   requests](https://bugzilla.redhat.com/show_bug.cgi?id=1920159) has
   been done in the 4.8 release.

3. Have a new operator in the release payload perform the scaling for
   control plane operators

   It would use the new API field to determine how to scale (see above).

   We would need to change the cluster-version-operator to ignore
   resource request settings for cluster operators defined in static
   manifests.

   It would add yet another component to be running in the cluster,
   consuming resources.

   It could apply the request scaling to everything, a static subset,
   or more dynamically based on label selectors or namespaces.

   Having an operator change the settings on manifests installed by
   the cluster-version-operator may cause some "thrashing" during
   install and upgrade.

4. Have a webhook perform the scaling for control plane operators

   It would use the new API field to determine how to scale (see above).

   We would need to change the cluster-version-operator to ignore
   resource request settings for cluster operators defined in static
   manifests.

   It would add yet another component to be running in the cluster,
   consuming resources.

   It could apply the request scaling to everything, a static subset,
   or more dynamically based on label selectors or namespaces.

   We may find race conditions using a webhook to change settings for
   cluster operators.

5. Have the cluster-version-operator perform the scaling for control
   plane operators

   We could change the cluster-version-operator to scale the
   Deployments for control plane operators.

   It would use the new API field to determine how to scale (see above).

   This additional logic would complicate the CVO (*How much?*) and
   there is a general desire to avoid adding complexity to the main
   component responsible for managing everything else.

   One benefit of this approach is that installation and upgrade would
   be handled in the same way, without thrashing, race conditions, or
   requirements for add-on components.

   What would apply the scaling to operator-lifecycle-manager (OLM)
   operators?

6. Have an out-of-cluster actor perform the scaling for control plane
   operators

   An outside actor such as ACM policy applied via klusterlet or a
   one-time script could change the resource settings.

   An outside actor would not require an API change in the cluster.

   We would need to change the cluster-version-operator to ignore
   resource request settings for cluster operators defined in static
   manifests.

   Allowing an outside actor to change the resource requests turns
   them into something a user might change themselves, which may not
   be ideal for our testing and support matrix.

### Options for globally affecting resource limits

We could change some of the underlying components to affect scaling
without the knowledge of the rest of the cluster components.

1. Have a third-party, such as another operator, change the requests.

   We could investigate using the [vertical pod
   autoscaler](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler)
   or write our own operator focused on scaling control plane
   components.

   If we have 2 controllers trying to manage the same resource (the
   original operator that owns it and this new scaling operator),
   there will be contention for the definition. We would need to have
   all operators ignore the resource requests fields when comparing
   Deployments of their operands for equality.

   If operators cooperated with the resource request settings, a
   third-party actor could modify them reliably. We could use an
   annotation-based API to inform that operator of the base settings
   for each Deployment, so that during an upgrade if the base needs
   change they can be scaled appropriately.

2. Have the scheduler and kubelet perform the scaling based on
   priority class

   Changing kubelet by itself, or adding a plugin, would capture
   everything as it is launched, but only after a Pod is
   scheduled. That would mean we would need to leave enough overhead
   to schedule a large component, even if the request is going to be
   rewritten to ask for a fraction of that value. So, we have to
   change the scheduler to ask for less and kubelet to grant less,
   without changing the Pod definition and triggering the ReplicaSet
   controller to rewrite the Pod definition.

3. Have kubelet announce more capacity than is available

   We could have kubelet support "over subscription" of a node by
   reporting that the node has more capacity than it really does. This
   could be implemented a few ways.

   1. Kubelet could accept an option to report a static amount of extra
      capacity, for example always reporting 2 more CPUs than are present.
   2. Kubelet could not subtract the requests from some workloads, or
      only subtract part of the requests, leaving the capacity higher
      than it would if the request was included fully in the
      calculation. *See implementation history above for details about
      a proof-of-concept implementation of this approach.*
   3. Kubelet could add capacity to match the requests of some workloads,
      so that the apparent capacity of a node grows dynamically when
      system workloads are added.

   Changing kubelet in these ways means fewer changes to other
   components, so it may be implemented more quickly.

   Changing kubelet in these ways instead of changing other components
   means we lose optimization benefits in multi-node clusters.

   Presenting false data to influence the schedule may be very tricky to
   get right and would complicate debugging and support. We would need
   some way to expose the difference between the actual capacity and the
   capacity value being used for scheduling.

   It's not clear if we could implement these behaviors in a plugin, or
   if we could push the changes upstream. Carrying a fork of kubelet to
   implement the feature would result in additional release management
   work indefinitely.

4. Have a scheduler plugin ignore critical Pods when calculating free
   CPU resources on a node

   A scheduler plugin could skip the CPU requests for critical Pods
   when calculating free CPU on a Node. We would need to override the
   scheduler configuration to disable the `NodeResourcesFit` filter
   and enable the new one.

   More research is needed to understand how this scheduler plugin
   might be enabled on only some clusters.

## Infrastructure Needed

None
