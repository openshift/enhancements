---
title: single-node-openshift-day2-workers
authors:
  - "@omertuc"
reviewers:
  - "@romfreiman"
  - "@eranco74"
  - "@tsorya"
  - TBD
approvers:
  - TBD
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2021-01-05
last-updated: 2021-01-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MGMT-8414
see-also:
  - "https://github.com/openshift/enhancements/tree/master/enhancements/single-node"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/installer/single-node-installation-bootstrap-in-place.md"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/external-control-plane-topology.md"
replaces:
superseded-by:
---

# Single Node OpenShift Day-2 Workers

## Summary

This enhancemnet aims to formally enable adding workers to a single
control-plane node cluster by attempting to tackle multiple issues that arise
when worker nodes are added to such cluster.

## Motivation

Today OCP supports deploying a cluster which only has a single control-plane
node and no additional workers.

There's been recent demand from OCP to also formally support deployment of
single control-plane node clusters with additional (non control-plane) worker
nodes.

This is already easily done on (unsupported) cloud IPI-deployed single
control-plane node clusters by increasing the replica count of either of the
worker machinesets (there are two machinesets provided by default on cloud
clusters, one for each AZ, and in cloud single control-plane node clusters
they're both simply scaled to 0 by default). Doing this results in a single
control-plane node cluster with additional workers and that cluster works as
expected without any known issues.

Even on Assisted Installer installed single control-plane node clusters it's
trivial to add more workers by leveraging the Assisted Installer's day-2 worker
installation capabilities (after some minor DNS configuration issues which will
be improved by the Assisted-Installer team, separately from this enhancement).

OCP installations have an `infrastructures.config.openshift.io` CR
automatically deployed by the installer. This CR has two topology parameters in
its status called "Control Plane Topology" and "Infrastructure Topology" which,
as of today, may have one of three values:

- SingleReplica
- HighlyAvailable
- External (this value is unrelated to this enhancement and is only mentioned here for completeness' sake)

(See "see-also" enhancements links for more information about the topology
parameters and their possible values)

The "Control Plane Topology" parameter is used by control-plane operators (such
as the Cluster etcd Operator) to determine how many replicas they should give
their various Deployments / StatefulSets. The value of this parameter in single
control-plane node clusters is always simply "SingleReplica" and it is not
discussed further in this enhancement.

The "Infrastructure Topology" parameter is used by infrastructure operators
(such as the Cluster Ingress Operator or Cluster Monitoring Operator) to
determine how many replicas they should give their various Deployments /
StatefulSets. The value of this parameter is not clear-cut in single
control-plane node clusters and may currently change depending on how many
workers were present during installation. This enhancement aims to formally
define the value of this parameter under various circumstances.

In addition, on "none"-platform single control-plane node clusters, when adding
workers, the resulting cluster has an issue with the behavior of the ingress
pod, the following paragraphs explain the background for this issue and the
issue itself.

One of the benefits of installing single-node clusters is the simplicity of not
having to deal with load-balancing and virtual IPs, as these don't provide much
value when there's only a single node behind them.

As a result, current common ways of installing Single Node OpenShift today
(mainly the Assisted Installer) avoid the usage of load balancers or virtual
IPs for API and ingress. There have also been some recent effort to determine how
single control-plane node cluster deployments on clouds may be adjusted in order
to reduce their costs, and one of the conclusions is that getting rid of the load
balancer installed by default by the IPI installer results in major cost savings.

A user installing Single-Node OpenShift on "none"-platform will be tempted to
simply point their DNS entries directly at the IP address of the single node
that they just installed.

Similarly, in the Assisted Installer, the user is able to complete the
installation without needing to define any DNS entries. This is currently made
possible by injecting a `MachineConfig` manifest targeting the "master" pool.
The node is also configured with `/etc/resolv.conf` to use that dnsmasq server
for DNS resolution. The dnsmasq server is configured with DNS entries for
`api.<cluster>.<base>`, `api-int.<cluster>.<base>` and
`*.apps.<cluster>.<base>` which all point to the single control-plane node's IP
address. This allows the installation process and the resulting cluster to
conveniently work without the user having to think about and configure DNS for
their cluster (of course external access to the cluster requires the user to
configure DNS, but this can be done separately after the installation has
completed).

The issue with those approaches is that they assume that
`*.apps.<cluster>.<base>` should always point at the single control-plane
node's IP address. This is of-course correct when there's just that single node
in the cluster, but once you start adding worker nodes to the cluster it starts
causing a potential problem - the `router-default` deployment created by the
Cluster Ingress Operator, which is responsible for load balancing ingress
traffic, targets the "worker" pool using a node selector. As a result, under
some circumstances, that deployment's pods may eventually find themselves
running on the newly added worker nodes, as those nodes obviously also belong
to the worker pool (the reason the control-plane node was also in the worker
pool is that when during installation there are no worker nodes, the OpenShift
installer sets the Scheduler CR `.spec.mastersSchedulable` to `true` and as a
result the control-plane node is in both the "master" and "worker" pools).

This pod floating between the nodes is a problem because user ingress traffic
has to be directed at the node currently holding the `router-default` pods, and
since the DNS entries have been "naively" pointed at the original control-plane
node's IP address (which may no longer be running those pods), ingress traffic
can no longer work. This can be temporarily solved if DNS is adjusted to point
at the correct node currently holding the pod or a load-balancer / some virtual
IP solution is put in place and then the DNS entry can be directed at that
load-balancer / virtual IP instead of at the node. This enhancement will try
to prevent this floating altogether so these workarounds will not be needed.

Finally, performing bootstrap-in-place installation of a "none"-platform single
control-plane node cluster today using the OpenShift Installer does not provide
the user with worker node ignition files. As a result, the user is not able to
manually add worker nodes to the resulting cluster. All current official OCP
documentation points the user to use "the worker ignition files that get
generated during installation", but these are not currently generated in such
bootstrap-in-place installation.

### Goals

- Define a sensible value for the "Infrastructure Topology" parameter in the
various configurations a single control-plane node cluster may be deployed in.

- Deal with the "floating ingress" issue encountered in "none"-platform
single control-plane node clusters which have worker nodes added to them.

- Define how the installer may be modified to generate the worker ignition
manifest, even when doing bootstrap-in-place "none"-platform single
control-plane node cluster installation

### Non-Goals

- Deal with clusters that have less nodes than they had during day-1
installation. The rationale for this being a non-goal is that it seems that
removing the "initial" nodes of a cluster causes a lot of issues even in a
regular, multi-node installation. So dealing with similar issues in single
control-plane node clusters is also out of scope for this enhancement. For
example, installing a cluster with 3 control-plane nodes and 3 workers results
in unschedulable control-plane nodes. Scaling down the worker machine-sets from
3 to 0 post-installation does not magically make the control-plane nodes
schedulable, and as a result a lot of infrastructure workloads fail to schedule
and operators degrade. User intervention is required to fix this.

- Deal with expansion of the single-node control-plane by adding more control-plane nodes

- Deal with expansion of clusters that have been installed before the
implementation of this enhancement (if possible, their expansion may addressed
of with documentation outlining the steps that the user has to take to enable
expansion).

- Adjust the baremetal platform to support single control-plane node
installations. The baremetal platforms solves the "floating ingress" issue by
using virtual IP addresses/keepalived.

- Deal with users who want their ingress traffic to not go through the single
control-plane node.

- Deal with non-cloud, non-"none" platforms such as baremetal, vSphere, etc.

## Proposal

Set the "Infrastructure Topology" value to "SingleReplica" in almost all 
circumstances where the "Control Plane Topology" is also "SingleReplica".

Make sure the default `IngressController` CR created by the Cluster Ingress
Operator targets the "master" pool rather than the "worker" pool whenever the
"Infrastrcture Topology" is set to "SingleReplica". This will ensure that the
`router-default` deployment created by the Cluster Ingress Operator will always
run on the single control plane node, and as a result any
`*.apps.<cluster>.<base>` DNS entries which originally pointed at the single
control plane node will remain correct even in the face of newly added worker
nodes.

Make sure worker node ignition files are generated even in bootstrap-in-place
single control-plane node "none"-platform installation.

### User Stories

- As an OpenShift cluster administrator, I want to install a single control-plane
node cluster with additional worker nodes, so that my cluster will be able to
handle larger computation demands.

- As an OpenShift cluster administrator, I want to add worker nodes to my
existing single control-plane node cluster, so that it'll be able to meet
growing computation demands.

### API Extensions

This enhancement does not modify/add any API

### Implementation Details/Notes/Constraints

#### Infrastructure Topology value

The following table tries to cover the various installation scenarios, the
existing values for the "Infrastrcture Topology", the target pool in the
default `IngressController` and the new proposed values for those two parameters
under those scenarios.

The following abbreviations are used to get a more compact table -

- `D1W`/`D2W` = The amount of workers during installation / The amount of workers added post-installation

- `SR`/`HA` = SingleReplica / HighlyAvailable

- `CIT`/`PIT` = Current "Infrastrcture Topology" / Proposed "Infrastrcture Topology"

- `M`/`W` = Master / Worker

- `CTP`/`PTP` = Current `IngressController` target pool / Proposed `IngressController` target pool

- `SNO` = Single control-plane node cluster

|Scenario                                |Platform|D1W|D2W|CIT|PIT|CTP|PTP|Is a load balancer currently needed?             |Will a load balancer be needed after proposal?|
|----------------------------------------|--------|---|---|---|---|---|---|-------------------------------------------------|----------------------------------------------|
|None platform SNO no workers            |None    |0  |0  |SR |SR |W  |M  |No                                               |No                                            |
|Cloud platform SNO no workers           |Cloud   |0  |0  |SR |SR |W  |M  |No                                               |No                                            |
|None platform SNO day1 single worker    |None    |1  |1  |-  |SR |-  |M  |Yes, ingress floats                              |No, ingress pinned to master pool             |
|Cloud platform SNO day1 single worker   |Cloud   |1  |1  |SR |HA |W  |W  |Yes - provided, ingress floats                   |Yes - provided, there are two ingress replicas|
|None platform SNO day1 multiple workers |None    |2+ |2+ |-  |SR |-  |M  |Yes, ingress is on two different nodes           |No, one ingress and is pinned to master pool  |
|Cloud platform SNO day1 multiple workers|Cloud   |2+ |2+ |HA |HA |W  |W  |Yes - provided, ingress is on two different nodes|Yes - provided, there are two ingress replicas|
|None platform SNO only day2 workers     |None    |0  |1+ |SR |SR |W  |M  |Yes, ingress floats                              |No, ingress pinned to master pool             |
|Cloud platform SNO only day2 workers    |Cloud   |0  |1+ |SR |SR |W  |M  |Yes - provided, ingress floats                   |Yes - provided, ingress floats                |

Some notes about this table and the decisions made in it:

The table assumes that the "Infrastrcture Topology" value is read only. That
means an optimal value for this parameter must be determined during
installation. You can see the consequences of that, for example, in the last
row of the table - even though we add workers in day 2 to the cluster, the
"Infrastrcture Topology" stays "SingleReplica" even though that cluster could
benefit from the additional worker nodes for the purposes of highly-available
infrastructure (as it's running in the cloud, so it has a load-balancer). In
the future it may be worth revisiting making this value configurable.

The "None platform SNO day1 single/multiple worker" scenario rows aren't really
possible today as there is no real way to install this kind of cluster. That's
why their CIT / CTP have been left empty.

It's proposed that the only scenarios in which the "Infrastrcture Topology"
parameter will be "HighlyAvailable" are when installing a single control-plane
node cluster in the cloud with one or more day-1 workers. Cloud deployments
have a load balancer so they can benefit from having highly-available ingress.

Today, if you only have 1 day-1 worker in a single control-plane node cluster
in the cloud, then the "Infrastrcture Topology" is set to "SingleReplica" - I
don't believe there's a good reason for that, as both the control-plane node
and the worker node are actually in the worker pool (TODO: make sure this is
correct!, if not, this entire point is moot) - giving us a total of 2 worker
nodes, so there's nothing preventing us from setting "Infrastrcture Topology"
to "HighlyAvailable" in that case.

When installing day-1 workers on a single control-plane node cluster on the
"none"-platform, it's unlikely you have a load balancer / virtual IP set up, so
even if you have multiple workers to hold ingress pods, there would be no load
balancer to balance between them. So it makes sense to only have one ingress
replica. That's why we set "Infrastrcture Topology" to "SingleReplica" even in
the "none"-platform even though we have day-1 workers. This has the unfortunate
consequence that non-ingress infrastructure workloads (such as monitoring, for
example) can no longer benefit from spreading over the worker nodes - since
they will all also have just 1 replica (as a result of the topology). It's
worth mentioning that we don't bother pinning those non-ingress infra workloads
to any particular node, as that would have no benefit. The reason the replica
must be pinned specifically to the control plane node and not some other
arbitrary worker node is because it has to be pinned to *some* node, and for
the sake of simplicity and the sake of consistency with deployments that don't
have any workers (those with potential to have more workers added to them), we
choose the control-plane node as the node to which the ingress pod is being
pinned. We also assume the worker nodes are possibly disposable but the single
control-plane is "forever", so it wouldn't make sense to pin it to any
particular worker.

Looking at the PIT and PTP columns, you can see that under all circumstances
where the proposed-"Infrastructure Topology" is "SingleReplica" it also makes
sense to set target pool to "master". That's why it was chosen as the condition
for when to pin of the `IngressController` to the "master" pool.

TODO: Go into detail of what code should be changed in the installer to make
the above table happen.

#### IngressController default target pool

Making sure the default `IngressController` points at the `master` pool
whenever the "Infrastrcture Topology" is "SingleReplica" can be easily done by
adjusting the Cluster Ingress Operator's `ensureDefaultIngressController`
method to set the `.spec.nodePlacement.nodeSelector.matchLabels` map to contain
the `node-role.kubernetes.io/master` key when
`infraConfig.Status.InfrastructureTopology == configv1.SingleReplicaTopologyMode`. 

#### Bootstrap-in-place worker ignition

TBD

### Risks and Mitigations

This should make no noticable difference on "regular" single control-plane node
clusters which do not have any day-1 or day-2 worker nodes. The only difference
for those clusters would be the `IngressController` targeting the "master"
pool rather than the "worker" pool, but since the single control-plane node is
already both in the "master" and "worker" pools, that should make no practical
difference.

On single control-plane node clusters with workers, we've made the
"opinionated" decision of pinning the `IngressController` to the "master" pool.
Users who for some reason want their traffic to *not* go through the single
control-plane node can still do so by following the [existing](https://docs.openShift.com/container-platform/4.9/machine_management/creating-infrastructure-machinesets.html#moving-resources-to-infrastructure-machinesets)
OpenShift documentation on moving infrastructure workloads to particular nodes.

I do not believe this enhancement has any security implications.

## Design Details

### Open Questions

None that I can think of at the moment

### Test Plan

- Add unit tests to the Cluster Ingress Operator to make sure the
IngressController resource is generated as exepected.

- Add periodic nightly tests which install single-node in the cloud, add a few
worker nodes to it, then run conformance tests to make sure we don't run into
any problems not described in this enhancement.

- Add periodic nightly tests which install a single-node "none"-platform cluster,
add worker nodes to it, and check that ingress traffic still works as expected
and recovers even after the `router-default` pod gets deleted and rescheduled.
Make sure this is still true even after upgrades.

- Add tests on both cloud / "none"-platform that check that a single-node cluster
with additional workers recovers after the single control-plane node reboots by
running conformance tests post-reboot.

- Add tests on both cloud / "none"-platform that check that a single-node cluster
with additional workers recovers after an upgrade by running conformance tests
post-upgrade.

TODO: Describe day-1 tests?

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

In the non-goals section it's mentioned that this enhancement does not apply to
clusters which have been installed prior to the enhancement, so their upgrade
is not discussed.

This enhancement, to the best of my knowledge, should have no problems persisting
across any type cluster upgrades. The Test Plan section describes how this will be
tested.

### Version Skew Strategy

Does not apply, to the best of my understanding.

### Operational Aspects of API Extensions

This enhancement does not modify/add any API

#### Failure Modes

This enhancement does not modify/add any API

#### Support Procedures

This enhancement does not modify/add any API

## Implementation History

Not yet applicable

## Drawbacks

- The pinning of the `IngressController` to the "master" pool is another change
which would make single-node clusters slightly different from multi-node
clusters, and any such difference is naturally not ideal.

- TBD

## Alternatives

- Adjust the "baremetal" platform to support single-node installations and make
users and the Assisted-Installer use that platform instead of the "none"
platform for single control-plane node cluster installations. The baremetal
platform solves the issue described in this enhancement with virtual IP
addresses/keepalived. This approach was dismissed due to much higher
development efforts and additional processes that would need to run on the
already resource constrained single control-plane node. Furthermore, even once
the baremetal platform is adjusted to support single-node clusters, the
Assisted-Installer which is currently the main supported way with which users
install single control-plane node clusters would have to go through a lot of
development effort in order to make it use the baremetal platform rather than
the "none" platform currently used for single node installations. This may
happen in the future.

- We may also decide to set the "Infrastrcture Topology" to "SingleReplica" even
in cloud single control-plane node clusters which also have day-1 workers. The
motivation for that is to make the value of this parameter consistent across
all single control-plane node clusters. By doing that we're practically making
its value always tied to the value of the "Control Plane Topology" paramater
during installation. Then the only time in which the value of the two topology
parameters will ever differ is when we some day make the "Infrastructure
Topology" non-read only and allow the user to modify it (either during
installation or post-installation).

## Infrastructure Needed [optional]

N/A
