---
title: management-workload-partitioning
authors:
  - "@dhellmann"
  - "@mrunalp"
  - "@browsell"
  - "@haircommander"
  - "@rphillips"
  - "@lack"
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
  control plane and per-node components and that can be extended to
  other workload types in the future.

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
customer or by vendors who are not our partners. Therefore, while this
feature will be released as Tech Preview initially with limited formal
support, the APIs described are not internal or otherwise private to
OpenShift.

The basic proposal is to provide a way to identify different types of
workloads at runtime and to use CRI-O to run them on a user-selected
set of CPUs, while other workloads will be prevented from running
there. For the management workload case, this effectively gives 3
pools of CPUs

* **management** -- a user-defined cpuset that restricts where
  management components run
* **dedicated** -- CPUs used for exclusive workloads
* **shared** -- CPUs in the set `management + !dedicate`

Because the management CPU pool is restricted to management workloads,
the shared CPU pool will need other CPUs to support burstable or
best-effort workloads that are not identified as management workloads.

We want the isolation to be in place from the initial buildout of the
cluster, to ensure predictable behavior and to ensure that the cluster
can fully operate with workload partitioning. Therefore, the feature
must be enabled during installation. To enable the feature, the user
will specify the set of CPUs to run all management workloads. When the
management CPU set is not defined, the feature will be completely
disabled.

We generally want components to opt-in to workload partitioning and
especially to being considered management workloads. Therefore, for a
regular pod to be considered to contain a management workload it must
have an annotation configuring its *workload type*,
`target.workload.openshift.io/{workload-type}`. For now, we will focus on
management workloads via `target.workload.openshift.io/management`, but will
use a syntax that supports other types of workloads that may be
defined in future enhancements.

We want to be able to differentiate between "hard" and "soft"
partitioning requests for workload types. We also want to be able to
support other partitioning parameters in the future. Therefore,
workload type settings will use an annotation with a JSON value (see
below for details).

We want to treat all OpenShift components as management workloads,
including those that run the control plane. Therefore, kubelet will be
modified to partition static pods with the annotation based on their
workload type, when the feature is enabled. We will update operator
manifests and implementations to annotate all OpenShift components not
running in static pods has having the workload type "`management`",
and add a CI job to require that the annotation be present in all
workloads created from the release payload.

We need kubelet to know when the feature is enabled, but we cannot
change the configuration schema that kubelet inherits from
upstream. Therefore we will have kubelet look for a new configuration
file on startup, and the feature will only be enabled if that file is
found.

We want to give cluster administrators control over which workloads
are run on the management CPUs. Normal users cannot change the
metadata of a namespace without the right RBAC permissions. Therefore,
only pods in namespaces with an annotation
`allowed.workload.openshift.io: {comma_separated_list_of_type_names}`
will be subject to special handling.

We want to continue to use the scheduler for placing management
workloads, but we cannot rely on the CPU requests of those workloads
to accurately reflect the constrained environment in which they are
expected to run. Instead of scaling those CPU request values, we will
change them to request a new [extended
resource](https://kubernetes.io/docs/tasks/administer-cluster/extended-resource-node/)
called `{workload-type}.workload.openshift.io/cores` (for example,
`management.workload.openshift.io/cores`).  We will modify kubelet to
advertise an extended resource for each workload type when the
workload partitioning feature is enabled, using a value equivalent to
the CPU resources for the entire host. This large value should allow
the scheduler to be able to place workloads without being constrained
by CPU requests while still taking other resource requests like memory
into account and while still accurately accounting for all
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
`io.openshift.workload.{workload-type}.cpushares/{container-name}` and
create an annotation for each container in the pod. See below for
details about this choice of name.

We need to change pod definitions as each pod is created, so that the
scheduler, kubelet, and CRI-O all see a consistently updated version
of the pod and do not need to make independent decisions about whether
it should be treated as a partitioned workload. We need to intercept
pod creation for *all* pods, without race conditions that might be
introduced with traditional admission webhooks or controllers.
Therefore, we will build an admission hook into the [kubernetes API
server in
OpenShift](https://github.com/openshift/kubernetes/tree/master/openshift-kube-apiserver/admission),
to intercept API requests to create pods. We will update kubelet to
make the same changes to static pods.

The admission hook receives the pod settings when the pod is created,
and does not know where the pod will be scheduled to run and cannot
make node-specific decisions about modifying its settings. Therefore,
we will treat partitioning as a cluster-wide feature and all nodes in
the cluster must have a CPU pool with a given workload type configured
before any workloads of that type will be run partitioned. Other than
the workload type name, the settings on each node do not need to be
the same, so different nodes can use different CPU sets for the same
workload type.

The API server needs to know when workload partitioning is enabled so
that it does not modify pod settings until the cluster is ready to
receive them with the new resource requests and nodes are configured
to actually partition the workloads. Therefore, the admission hook
will look at the workload type in the workload annotation and only
change the pod if all nodes in the cluster report the associated
resource. Future work may make this more flexible.

There are a lot of edge cases and unknowns around expanding a cluster
with workload partitioning enabled or disabling workload
partitioning. Therefore, in the initial implementation, after the
admission hook determines that partitioning is enabled for a given
workload type it will always modify pods with that workload type, even
if new nodes are added to the cluster without the configuration to
provide the associated resource type. This is likely to result in
unschedulable workloads, but we expect the resulting error message to
explain the problem well enough for admins to recover. Future work may
make this more flexible.

We are not prepared to commit to an installer API for this feature in
this initial implementation. Therefore, we will document how to create
the correct machine config manifests to enable it in `kubelet` and
CRI-O.

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
annotations ourselves, starting with

* performance-addon-operator
* sriov-network-operator
* ptp-operator
* local-storage-operator
* cluster-logging (fluentd)
* klusterlet and klusterlet add-ons (RHACM)
* kubevirt
* N3000

Due to time constraints, the first iteration of this work will also
require the
[performance-addon-operators](https://github.com/openshift-kni/performance-addon-operators)
to configure the reserved CPUs and enable the static CPU manager. We
expect those values to match the CPU set used for management
workloads. Future work will eliminate the need for the extra operator
(see GA graduation criteria).

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
with workload partitioning enabled and how pods are correctly
scheduled to run on the management CPU pool.

1. User sits down at their computer.
2. The user creates a machine config manifest to configure CRI-O to
   partition management workloads.
3. The user creates a machine config manifest to write a configuration
   file for kubelet to enable the same workload partition. The file
   should only be readable by the kubelet.
4. The user runs the installer to create the standard manifests, adds
   their extra manifests from steps 2 and 3, then creates the cluster.
5. The kubelet starts up and finds the configuration file enabling the
   new feature.
6. The kubelet advertises `management.workload.openshift.io/cores`
   extended resources on the node based on the number of CPUs in the
   host.
7. The kubelet reads static pod definitions. It replaces the `cpu`
   requests with `management.workload.openshift.io/cores` requests of
   the same value and adds the
   `io.openshift.workload.management.cpushares/{container-name}`
   annotations for CRI-O with the same values.
8. Something schedules a regular pod with the
   `target.workload.openshift.io/management` annotation in a namespace with
   the `allowed.workload.openshift.io: management` annotation.
9. The admission hook modifies the pod, replacing the CPU requests
   with `management.workload.openshift.io/cores` requests and adding
   the `io.openshift.workload.management.cpushares/{container-name}`
   annotations for CRI-O.
10. The scheduler sees the new pod and finds available
    `management.workload.openshift.io/cores` resources on the node. The
    scheduler places the pod on the node.
11. Repeat steps 8-10 until all pods are running.
12. Single-node deployment comes up with management components
    constrained to subset of available CPUs.

#### Workload Annotation

The `target.workload.openshift.io` annotation on each pod needs to allow us
to add new parameters in the future, so the value will be a
struct. Initially, the annotation will encode 2 values.

The *workload type* for the workloads is the suffix of the annotation
name to make it easy for CRI-O and other lower-level components to
detect it without understanding the full JSON struct in the value.

The `effect` field will control whether the request is a soft or hard
rule. It can contain either `PreferredDuringScheduling` for soft
requests or `RequiredDuringScheduling` for hard requests. Initially we
will only implement the soft request mode and that will be the
default.

```yaml
metadata:
  annotations:
    target.workload.openshift.io/management: |
      {"effect": "PreferredDuringScheduling"}
```

The admission hook will reject pods with multiple annotations, for
now. Future work may allow multiple workload types with different
priorities to support clusters with different types of configurations.

#### Pod mutation

The kubelet and API admission hook will change pods annotated with
`target.workload.openshift.io/management` so the CPU requests are replaced
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
  management.workload.openshift.io/cores: 400
```

and

```yaml
annotations:
  io.openshift.workload.management.cpushares/{container-name}: 400
```

The annotation name used to set the resource requests is reversed
(when compared to the other workload annotation or the resource
request name) so that CRI-O can be configured with a prefix value to
simplify the way it processes the strings without having to understand
their content.

The new request value and annotation value are scaled up by 1000 from
the original CPU request input because opaque resources do not support
units or fractional values.

As a special case, kubelet and the API admission hook should strip any
annotations from pods trying to set the "cpuset" resource. See the
next section for more details.

#### CRI-O Changes

CRI-O will be updated to support new configuration settings for
workload types.

```ini
[crio.runtime.workloads.{workload-type}]
  activation_annotation = "target.workload.openshift.io/{workload-type}"
  annotation_prefix = "io.openshift.workload.{workload-type}"
  resources = { "cpushares": "", "cpuset": "0-1" }
```

The `activation_annotation` field is used to match pods that should be
treated as having the workload type. The annotation key on the pod is
compared for an exact match against the value specified in the
configuration file.

The `annotation_prefix` is the start of the annotation key
used to pass settings from the admission hook down to CRI-O.

The `resources` map associates annotation suffixes with default
values. CRI-O will define a well-known set of resources and other
values will not be allowed. In OpenShift, we do not want to allow
arbitrary pods to set their own `cpuset` but other CRI-O users may
want that ability.

To pass a setting into CRI-O, the pod should have an annotation made
by combining the `annotation_prefix`, the key from
`resources`, and the container name, like this:

```text
io.openshift.workload.management.cpushares/container_name = {value}
```

In the management workload case, we will configure it with values like

```ini
[crio.runtime.workloads.management]
  activation_annotation = "target.workload.openshift.io/management"
  annotation_prefix = "io.openshift.workload.management"
  resources = { "cpushares" = "", "cpuset" = "0-1" }
```

CRI-O will be configured to support a new annotation on pods,
`io.openshift.workload.management.cpushares/{container-name}`.

```ini
[crio.runtime.runtimes.runc]
  runtime_path = "/usr/bin/runc"
  runtime_type = "oci"
```

Pods that have the `target.workload.openshift.io/management` annotation will
have their cpuset configured to the value from the appropriate
workload configuration. The CPU shares for each container in the pod
will be configured to the value of the annotation with the name
created by combining the `annotation_prefix`, `"cpushares"`
and the container name (for example,
`io.openshift.workload.management.cpushares/my-container`).

Note that this field does not conflict with the `infra_ctr_cpuset`
config option, as the infra container will still be put in that
cpuset.  They can be configured as the same value if the infra
container should also be considered to be managed.

#### API Server Admission Hook

A new admission hook in the kubernetes API server within OpenShift
will mutate pods when they are created to make 3 changes.

1. It will move CPU requests to workload CPU requests, so that
   the scheduler can successfully place the pod on the node, even
   though the sum of the CPU requests for all management workloads may
   exceed the actual CPU capacity of the management CPU pool.
2. It will strip all container-specific workload annotations from new
   pods. This both prevents a user from configuring a pod incorrectly
   and prevents unprivileged users from running their pods in a CPU
   pool they should not have access to.
3. It will add annotations with for each container with the value
   equal to the original CPU requests * 1000, so that CRI-O can use
   the value to configure the CPU shares for the container.

We will not change pods in a way that changes their [Quality of
Service](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/)
class. So, we would not strip CPU requests unless they also have
memory requests, because if we mutate the pod so that it has no CPU or
memory requests the quality-of-service class of the pod would be
changed automatically. Any pod that is already BestEffort
would be annotated using `0` as the value so that CRI-O will have an
indicator to configure the CPU shares as BestEffort.

#### Kubelet Changes

Kubelet will be changed to look for a configuration file,
`/etc/kubernetes/openshift-workload-pinning`, to enable the management
workload partitioning feature. The file should contain the cpuset
specifier for the CPUs making up each CPU pool.

Kubelet will be changed so that when the feature is enabled, [when a
static pod definition is
read](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/config/file_linux.go)
it is mutated in a way similar to the API server admission hook will
mutate regular pods.

Kubelet will be changed so that when the feature is enabled, it
advertises a new extended resource of
`{workload-type}.workload.openshift.io/cores` for each CPU pool defined in the
configuration file, representing all of the CPU capacity of the host
(not just that CPU pool).

#### Example Manifests

To enable workload partitioning, the user must provide a
`MachineConfig` manifest during installation to configure CRI-O and
kubelet to know about the workload types.

The manifest, without the encoded file content, would look like this:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 02-master-workload-partitioning
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,encoded-content-here
        mode: 420
        overwrite: true
        path: /etc/crio/crio.conf.d/01-workload-partitioning
        user:
          name: root
      - contents:
          source: data:text/plain;charset=utf-8;base64,encoded-content-here
        mode: 420
        overwrite: true
        path: /etc/kubernetes/openshift-workload-pinning
        user:
          name: root
```

The contents of `/etc/crio/crio.conf.d/01-workload-partitioning`
should look like this (the `cpuset` value will vary based on the
deployment):

```ini
[crio.runtime.workloads.management]
activation_annotation = "target.workload.openshift.io/management"
annotation_prefix = "io.openshift.workload.management"
resources = { "cpushares" = "", "cpuset" = "0-1,52-53" }
```

The contents of `/etc/kubernetes/openshift-workload-pinning` should look like
this:

```json
{
  "management": {
    "cpuset": "0-1,52-53"
  }
}
```

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

It is possible to build a cluster with the feature enabled and then
add a node in a way that does not configure the workload partitions
only for that node. This may make the node appear as not ready and
workloads in daemon sets may not be able to be scheduled on the
node. We expect the resulting error message to explain the problem
well enough for admins to recover. Future work may make this more
flexible.

## Design Details

### Open Questions [optional]

None

### Test Plan

We will add a CI job to ensure that all release payload workloads and
their namespaces have the `target.workload.openshift.io/management`
annotation.

We will add a CI job to ensure that single-node deployments configured
with management workload partitioning pass the compliance tests.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- Eliminate the need to have the performance-addon-operators configure
  the kube-reserved and system-reserved CPU set.
- Eliminate the need for the user to manually create manifests by
  adding an installer interface for enabling the feature.
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

This new behavior will be added in 4.8 for single-node clusters only.

Enabling the feature after installation is not supported in 4.8, so we
do not need to address what happens if an older cluster upgrades and
then the feature is turned on.

### Version Skew Strategy

N/A

## Implementation History

A CI job to identify pods that need a workload label and ensure it is
present: https://github.com/openshift/origin/pull/25992

Kubelet changes: https://github.com/openshift/kubernetes/pull/627

The first set of CRI-O changes to refactor annotation handling:
https://github.com/cri-o/cri-o/pull/4680

Final CRI-O PR: https://github.com/cri-o/cri-o/pull/4725

Installer changes: https://github.com/openshift/installer/pull/4802

Admission API hook: https://github.com/openshift/kubernetes/pull/632

Assisted service updating install-config:
https://github.com/openshift/assisted-service/pull/1431

Zero Touch Provisioning automation: https://github.com/redhat-ztp/ztp-cluster-deploy/pull/88/

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
   pod. We could mitigate that by still using the namespace annotation
   to opt-in.
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

### Enable the admission hook with a cluster-wide API

We could use a cluster-wide API (CRD or feature gate) to enable
workload partitioning. However, we eventually want to allow the
feature to be enabled on a running cluster and it takes time to roll
the machine configuration out to all of the nodes in the cluster. If
the admission hook sees a cluster-wide API that says the feature is
on, but the nodes have not been updated and are not configured to
partition the workloads, then pods would either be unschedulable or
would be scheduled as BestEffort but not partitioned.

### Have the installer configure the feature

Instead of requiring the user to provide machine config manifests to
configure kubelet and CRI-O to enable the feature, we could have the
installer generate those manifests. This early in the life of the
feature, we are reluctant to commit to what an install-config API
would look like, but this is one option.

The installer will be changed to accept a new `workload.partitions`
parameter for each [machine
pool](https://github.com/openshift/installer/blob/master/docs/user/customization.md#machine-pools)
with a list of workload types and their settings. For now, the only
setting will be the name and the CPU sets to treat as separate from
the standard shared pool. The name `workload.partitions` may change,
based on discussion on the installer code changes.

```yaml
controlPlane:
  name: master
  workload:
    partitions:
      - name: management
        cpuIDs: 0-1
compute:
  - name: worker
    workload:
      partitions:
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
write the `/etc/kubernetes/openshift-workload-pinning` configuration file for
kubelet. The file will have SELinux settings configured so that it is
only readable by the kubelet.

The installer will be changed to support the feature flag for enabling
management workload partitioning.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
