---
title: management-workload-partitioning
authors:
  - "@dhellmann"
  - "@mrunalp"
  - "@browsell"
  - "@haircommander"
  - "@rphillips"
reviewers:
  - "@deads2k"
  - "@staebler"
  - TBD
approvers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@markmc"
creation-date: 2021-03-18
last-updated: 2021-03-18
status: implementable
see-also:
  - "/enhancements/single-node-production-deployment-approach.md"
replaces:
  - https://github.com/openshift/enhancements/pull/628
---

# Management Workload Partitioning

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes an approach to allow us to isolate the
control plane services to run on a restricted set of CPUs. This will
be especially useful for resource-constrained enviornments, such as
single-node production deployments, where the user wants to reserve
most of the CPU resources for their own workloads and needs to
configure OpenShift to run on a fixed number of CPUs within the host.

One example of this use case is seen in telecommunication service
providers implementation of a Radio Access Network (RAN). This use case
is discussed in more detail below.

## Motivation

In constrained environments, management workloads, including the
OpenShift control plane, need to be configured to use fewer resources
than they might by default in normal clusters. After examining
[various approaches for scaling the resource
requests](https://github.com/openshift/enhancements/pull/628), we are
reframing the problem to allow us to solve it a different way.

Customers who want us to reduce the resource consumption of management
workloads have a fixed budget of CPU cores in mind. We want to use
normal scheduling capabilities of kubernetes to manage the number of
pods that can be placed onto those cores, and we want to avoid mixing
management and normal workloads there.

### Goals

* This enhancement describes an approach for configuring OpenShift
  clusters to run with management workloads on a restricted set of
  CPUs.
* Clusters built in this way should pass the same Kubernetes and
  OpenShift conformance and functional end-to-end tests as single-node
  deployments that are not isolating the management workloads.
* We have a goal of running on 4 hyperthreads today, but we do not
  know what constraints we might be given in the future so we need a
  solution that is not tied to the current known limit.
* We want a general approach, that can be applied to all OpenShift
  control plane and per-node components.

### Non-Goals

* This enhancement is focused on CPU resources. Other compressible
  resource types may need to be managed in the future, and those are
  likely to need different approaches.
* This enhancement does not address non-compressible resource
  requests, such as for memory.
* This enhancement does not address ways to disable operators or
  operands entirely.
* Although the results of this enhancement may be useful for
  single-node developer deployments, multi-node production
  deployments, and Internet-of-things devices, those use cases are not
  addressed directly in this document.
* This enhancement does not address reducing actual utilization,
  beyond providing a way to have a predictable upper-bounds. There is
  no expectation that a cluster configured to use a small number of
  cores for management services would offer exactly the same
  performance as the default. It must be stable and continue to
  operate reliably, but may respond more slowly.
* This enhancement assumes that the configuration of a management CPU
  pool is done as part of installing the cluster, and cannot be
  changed later. Future enhancements may address enabling or
  reconfiguring the feature described here on an existing cluster.
* This enhancement describes partitioning concepts that could be
  expanded to be used for other purposes. Use cases for partitioning
  workloads for other purposes may be addressed by future
  enhancements.

## Proposal

[Previous attempts](https://github.com/openshift/enhancements/pull/628)
to solve this problem focused on reducing or scaling requests so that
the normal scheduling criteria could be used to safely place them on
CPUs in the shared pool. This proposal reframes the problem, so that
instead of considering "scaling" or "tuning" the requests we think about
"isolating" or "partitioning" the workloads away from the non-management
workloads. This view of the problem is more consistent with how the
requirement was originally presented by the customer.

We want to define "management workloads" in a flexible way. For the
purposes of this document, "management workloads" include all
OpenShift core components necessary to run the cluster, any add-on
operators necessary to make up the "platform" as defined by telco
customers, and operators or other components from third-party vendors
that the customer deems as management rather than operational. It is
important to note that not all of those components will be delivered
as part of the OpenShift payload and some may be written by the
customer or by vendors who are not our partners.

The basic proposal is to provide a way to identify management
workloads at runtime and to use CRI-O to run them on a user-selected
set of CPUs, while other workloads will be prevented from running
there. This effectively gives 3 pools of CPUs

* **management** -- a user-defined cpuset that restricts where
  management components run
* **dedicated** -- CPUs used for exclusive workloads
* **shared** -- CPUs in the set `management + !dedicate`

Because the management CPU pool is restricted to management workloads,
the shared CPU pool will need other CPUs to support burstable or
best-effort workloads that are not identified as management workloads.

We want the isolation to be in place from the initial buildout of the
cluster, to ensure predictable behavior. Therefore, the feature must
be enabled during installation. To enable the feature, the user will
specify the set of CPUs to run all management workloads. When the
management CPU set is not defined, the feature will be completely
disabled.

We generally want components to opt-in to being considered management
workloads. Therefore, for a regular pod to be considered to contain a
management workload it must be labeled with a *workload label*,
`workload.openshift.io/target={workload_type}`. For now, we will focus on
`workload.openshift.io/target=management`, but this syntax supports other
types of workloads that may be defined in future enhancements.

We want to treat all OpenShift components as management workloads,
including those that run the control plane. Therefore, kubelet will be
modified to treat static pods with the label as management workloads,
when the feature is enabled. We will update operator manifests and
implementations to label all OpenShift components not running in
static pods with the label, and add a CI job to require that the label
be present in all workloads created from the release payload.

We need kubelet to know when the feature is enabled, but we cannot
change the configuration schema that kubelet inherits from
upstream. Therefore we will have kubelet look for a new configuration
file on startup, and the feature will only be enabled if that file is
found.

We want to give cluster administrators control over which workloads
are run on the management CPUs. Therefore, only pods in namespaces
labeled with a label
`workload.openshift.io/allowed={comma_separated_list_of_type_names}`
will be subject to special handling. Normal users cannot add a label
to a namespace without the right RBAC permissions.

We want to continue to use the scheduler for placing management
workloads, but we cannot rely on the CPU requests of those workloads
to accurately reflect the constrained environment in which they are
expected to run. Instead of scaling those CPU request values, we will
change them to request a new [extended
resource](https://kubernetes.io/docs/tasks/administer-cluster/extended-resource-node/)
called `cpu.workload.openshift.io/{workload_type}`.  We will modify
kubelet to advertise the `cpu.workload.openshift.io/management`
extended resource when the management workload partitioning feature is
enabled, using a value equivalent to the CPU resources for the entire
host. This large value should allow the scheduler to always be able to
place workloads, while still accurately accounting for those
requests. The naming convention of the resource will allow us to
support other CPU pools in future enhancements. We may change the
formula for the advertised resources in future enhancements.

We need to ensure fair allocation of CPU time between different
management workloads. CRI-O does not receive information about
extended resource reservations when a container is started, but it
does receive the content of annotations on the pod. Therefore, we will
copy the original CPU requests for containers in management workload
pods into a annotations that CRI-O can use to configure the CPU shares
when running the containers. We will use the naming convention
`cpu.workload.openshift.io/{container-name}` and create an annotation
for each container in the pod.

We need to change pod definitions as each pod is created, so that the
scheduler, kubelet, and CRI-O all see a consistently updated version
of the pod and do not need to make independent decisions about whether
it should be treated as a management workload. We need to intercept
pod creation for *all* pods, without race conditions that might be
introduced with traditional admission webhooks or controllers.
Therefore, we will build an admission hook into the [kubernetes API
server in
OpenShift](https://github.com/openshift/kubernetes/tree/master/openshift-kube-apiserver/admission),
to intercept API requests to create pods. We will update kubelet to
make the same changes to static pods.

The API server requires a configuration resource as input, rather than
a ConfigMap or command line flag.  Therefore, we need an API-driven
way to enable management workload partitioning in the admission
hook. We will add a [feature
set](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L102-L119)
with a feature gate to control whether the feature is on or off.

*We probably need more details about the feature gate.*

Some pods used to run OpenShift control plane components
(kubeapiserver, kube-controller-manager, scheduler, and etcd) are
started before the API server. Therefore, we will have to manually add
extra metadata to those pod definitions, instead of relying on the
admission hook to do it.

Some of the workloads that will be classified as management are
outside of the OpenShift release payload. Therefore we will need to
document this feature so that workload authors can use it. We will
also identify add-on components that we know our initial customers
will use and to which we contribute, and make the changes to add the
labels ourselves, starting with

* performance-addon-operator
* sriov-network-operator
* ptp-operator
* local-storage-operator
* cluster-logging (fluentd)
* klusterlet and klusterlet add-ons (RHACM)
* kubevirt
* N3000

### User Stories

#### Radio Access Network (RAN) Use Case

In the context of telcocommunications service providers' 5G Radio
Access Networks, it is increasingly common to see "cloud native"
implementations of the 5G Distributed Unit (DU) component. Due to
latency constraints, this DU component needs to be deployed very close
to the radio antenna for which it is responsible. In practice, this
can mean running this component on anything from a single server at
the base of a remote cell tower or in a datacenter-like environment
serving several base stations.

A hypothetical DU example is an unusually resource-intensive workload,
requiring 20 dedicated cores, 24 GiB of RAM consumed as huge pages,
multiple SR-IOV NICs carrying several Gbps of traffic each, and
specialized accelerator devices. The node hosting this workload must
run a realtime kernel, be carefully tuned to ensure low-latency
requirements can be met, and be configured to support features like
Precision Timing Protocol (PTP).

The most constrained resource for RAN deployments is CPU, so for now
the focus of this enhancement is managing CPU requirements.
Kubernetes resource requests for CPU time are measured in fractional
"[Kubernetes
CPUs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu)". For
bare metal, 1 Kubernetes CPU is equivalent to 1 hyperthread.

Due to the resource-intensive workload, the overhead for platform
components such as the OpenShift platform and add-on operators is
severely restricted when compared to other uses. For example, one
customer has allocated 4 hyperthreads for all components other than
their own workloads. OpenShift currently requires 7.

### Implementation Details/Notes/Constraints

#### High-level End-to-end Workflow

This section outlines an end-to-end workflow for deploying a cluster
with management-workload-partitioning enabled and how pods are
correctly scheduled to run on the management CPU pool.

1. User sits down at their computer.
2. The user creates their `install-config.yaml`, including extra
   values in the specifying the CPUs to include in the management CPU
   set. (See below for details.)
3. The user runs the installer.
4. The installer uses the management CPU pool settings to generate an
   extra machine config manifest to configure CRI-O to process
   management workloads in a special way.
5. The installer creates a machine config manifest to write a
   configuration file for kubelet. The file should only be readable by
   the kubelet.
6. The installer enables the feature set for the feature flag
   controlling management workload partitioning.
7. The kubelet starts up and finds the configuration file enabling the
   new feature.
8. The kubelet advertises `cpu.workload.openshift.io/management`
   extended resources on the node based on the number of CPUs in the
   host.
9. The kubelet reads static pod definitions. It replaces the `cpu`
   requests with `cpu.workload.openshift.io/management` requests of
   the same value and adds the
   `cpu.workload.openshift.io/{container-name}` annotations for CRI-O
   with the same values.
10. Something schedules a regular pod with the
    `workload.openshift.io/target=management` label in a namespace
    with the `workload.openshift.io/allowed=management` label.
11. The admission hook modifies the pod, replacing the CPU requests
    with `cpu.workload.openshift.io/management` requests and adding
    the `cpu.workload.openshift.io/{container-name}` annotations for
    CRI-O.
12. The scheduler sees the new pod and finds available
    `cpu.workload.openshift.io/management` resources on the node. The
    scheduler places the pod on the node.
13. Repeat steps 10-12 until all pods are running.
14. Single-node deployment comes up with management components
    constrained to subset of available CPUs

#### Pod mutation

The kubelet and API admission hook will change pods labeled with
`workload.openshift.io/management` so the CPU requests are replaced
with management CPU requests and an annotation is added with the same
value.

```yaml
requests:
  cpu:
    400m
```

becomes

```yaml
requests:
  cpu.workload.openshift.io/management: 400m
```

and

```yaml
annotations:
  cpu.workload.openshift.io/container-name: 400m
```


#### CRI-O Changes

*NOTE: This section needs to be updated to deal with the generalized
CPU pools.*

CRI-O will be updated to support new configuration settings for workload types.

```ini
[crio.runtime.workloads.{workload-type}]
  cpu_set = "0-1"
  label = "workload.openshift.io/target={workload-type}"
```

The `cpu_set` field describes the CPU set that workloads will be
configured to use, based on their type.

The `annotation` field gives the annotation prefix used to match pods
to the workload type. Specifying this in the configuration file makes
it easier to change later and keeps OpenShift-specific values out of
CRI-O.

In the management workload case, we will configure it with values like

```ini
[crio.runtime.workloads.management]
  cpu_set = "0-1"
  label = "workload.openshift.io/target=mangement"
```

CRI-O will be configured to support a new annotation on pods,
`cpu.workload.openshift.io/{container-name}`.

```ini
[crio.runtime.runtimes.runc]
  runtime_path = "/usr/bin/runc"
  runtime_type = "oci"
  allowed_annotations = ["cpu.workload.openshift.io"]
```

Pods that have the `cpu.workload.openshift.io` annotation will have
their cpuset configured to the value from the appropriate workload
configuration, as well as have their CPU shares configured to the
value of the annotation.

Note that this field does not conflict with the `infra_ctr_cpuset`
config option, as the infra container will still be put in that
cpuset.  They can be configured as the same value if the infra
container should also be considered to be managed.

#### API Server Admission Hook

A new admission hook in the kubernetes API server within OpenShift
will mutate pods when they are created to make 2 changes.

1. It will move CPU requests to workload CPU requests, so that
   the scheduler can successfully place the pod on the node, even
   though the sum of the CPU requests for all management workloads may
   exceed the actual CPU capacity of the management CPU pool.
2. It will add annotations with for each container with the value
   equal to the original CPU requests, so that CRI-O can use the value
   to configure the CPU shares for the container.

We will not change pods in a way that changes their quality-of-service
class. So, we would not strip CPU requests unless they also have
memory requests, because if we mutate the pod so that it has no CPU or
memory requests the quality-of-service class of the pod would be
changed automatically. Any labeled pod that is already BestEffort
would be annotated using `0` as the value so that CRI-O will have an
indicator to configure the CPU shares as BestEffort.

#### Kubelet Changes

Kubelet will be changed to look for a configuration file,
`/etc/kubernetes/management-pinning`, to enable the management
workload partitioning feature. The file should contain the cpuset
specifier for the CPUs making up the management CPU pool.

Kubelet will be changed so that when the feature is enabled, [when a
static pod definition is
read](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/config/file_linux.go)
it is mutated in a way similar to the API server admission hook will
mutate regular pods.

Kubelet will be changed so that when the feature is enabled, it
advertises a new extended resource of
`cpu.workload.openshift.io/management`, representing all of the CPU
capacity of the host (not just the management CPU pool).

#### Installer Changes

The installer will be changed to accept a new `workloadSettings`
parameter with a list of workload types and their settings. For now,
the only setting will be the name and the CPU sets to treat as
separate from the standard shared pool. The name `workloadSettings`
may change, based on discussion on the installer code changes.

```yaml
workloadSettings:
  - name: management
    cpuIDs: 0-1
```

The default is empty, and when the value is empty the management
workload partitioning feature is disabled.

For the first version, we will only support 1 workload type with the
name "management". That restriction may be lifted in the future.

The `cpuIDs` value is a CPU set specifier for the CPUs to add to the
isolated set used for management components, using the same cpuset
syntax used elsewhere in kubernetes. Asking for explicit IDs instead
of simply a count gives the user control in situations where they need
specific CPUs to be available because they're close to accelerators,
NICs, or other special hardware.

The installer will be changed to generate an extra machine config
manifest to configure CRI-O so that containers from pods with the
`cpu.workload.openshift.io/management` annotation are run on the
CPU set specified by the management workload settings.

The installer will be changed to create a machine config manifest to
write the `/etc/kubernetes/management-pinning` configuration file for
kubelet. The file will have SELinux settings configured so that it is
only readable by the kubelet.

The installer will be changed to support the feature flag for enabling
management workload partitioning.

### Risks and Mitigations

The first version of this feature must be enabled when a cluster is
deployed to work correctly. However, some of the enabling
configurations could be modified by an admin on day 2. We need to
document clearly that this is not supported. In particular, changing
the configuration and rebooting may cause normal pods to be isolated
but will not configure kubelet to also partition static pods.

It is possible to build a cluster with the feature enabled and then
deploy an operator or other workload that should take advantage of the
feature without first configuring the namespace properly.  We need to
document that the configuration only applies to pods created after it
is set, so if an operator is installed before the namespace setting is
changed the operator or its operands may need to be re-installed or
restarted.

The schedule for delivering this feature is very aggressive. We have
tried to minimize the number of places that need complex changes to
make it more likely that we can meet the deadline.

Many implementation details proposed here require us to carry patches
to components that we try to keep in sync with upstream versions. This
design isolates those patches more fully than the previous approaches
did, which should make carrying the patches easier.  This design also
provides an opportunity to upstream the feature in a way that other
approaches did not, which may mean we can avoid carrying the patches
indefinitely. Even if the approach is not accepted upstream exactly as
it is described here, it will be informative to share the experience
as we believe workload isolation is a common problem for other
kubernetes users.

## Design Details

### Open Questions [optional]

1. If we think this feature is eventually going to be useful in
   regular clusters, do we want the settings in the `bootstrapInPlace`
   section? Should we add a `managementWorkloadPartitioning` section,
   or something similar, and say for now that it only applies when the
   single-node deployment approach is used?
2. What should the new field in the Infrastructure CRD be?
3. Should we worry about heterogeneous control plane hardware in
   multi-node clusters? The install-config schema may need to support
   specifying the same pool as using different CPUs on different nodes
   in that case. For example

   ```yaml
   workloadCPUPools:
     - name: management
       nodes:
         - master-0
       cpuIDs: 0-1
     - name: management
       nodes:
         - master-1
         - master-2
       cpuIDs: 23-24
   ```
4. Is the feature flag enabled automatically when the user specifies
   the workload settings in the install-config, or do they need to
   enable it explicitly.

### Test Plan

We will add a CI job to ensure that all release payload workloads and
their namespaces are labeled with `io.openshift.management: true`.

We will add a CI job to ensure that single-node deployments configured
with management workload partitioning pass the compliance tests.

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

This new behavior will be added in 4.8 for single-node clusters only.

Enabling the feature after installation is not supported, so we do not
need to address what happens if an older cluster upgrades and then the
feature is turned on.

### Version Skew Strategy

N/A

## Implementation History

A CI job to identify pods that need a workload label and ensure it is
present: https://github.com/openshift/origin/pull/25992

Kubelet changes: https://github.com/openshift/kubernetes/pull/627

The first set of CRI-O changes to refactor annotation handling:
https://github.com/cri-o/cri-o/pull/4680

## Drawbacks

Several of the changes described above are patches that we may end up
carrying downstream indefinitely. Some version of a more general "CPU
pool" feature may be acceptable upstream, and we could reimplement
management workload partitioning to use that new implementation.

## Alternatives

### Force pods into the management CPU pool without opt-in

We could force all pods of a certain priority into the management CPU
pool. There are two issues with that approach:

1. There are no RBAC restrictions on selecting the priority for a
   pod. We could mitigate that by still using the namespace label to
   opt-in.
2. By not requiring opt-in, we make it harder for component authors to
   predict and understand how their code is run. The pods created from
   deployments managed by operators would not match the deployment
   settings, and it would not be clear why. By requiring opt-in,
   component authors have a chance to learn about and understand the
   feature.

Opt-in also gives us the opportunity to expand to more special-purpose
CPU pools in the future.

### Use PAO to configure this behavior day 2

We could use the PAO to apply some of the configuration for the
kubelet on day 2. That would require extra reboot(s), which we want to
avoid because the amount of time it takes to install is already too
long for the goals of some customers.

### CRI-O Alternatives

* We could define an entirely different runtime class for CRI-O to use
  for these workloads. That may conflict with our desire to use
  runtime classes for other purposes later, though, and isn't
  necessary.
* Have the cpuset configured on `runtime_class` level instead of
  top-level config
* Have the cpuset configured as the value of `io.openshift.management`
  instead of hard coded
  * This option is not optimal because it requires multiple locations
    where the cpuset has to be configured (in the admission controller
    that will inject the annotation)
* Have two different annotations, rather than just one.
  * This is only needed if we decide to configure the cpuset in the
    annotation.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
