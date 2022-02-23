---
title: single-node-openshift-with-workers
authors:
  - "@omertuc"
reviewers:
  - "@romfreiman"
  - "@eranco74"
  - "@tsorya"
  - "@dhellmann"
  - "@Miciah"
  - "@bparees"
  - "@JoelSpeed"
  - "@staebler"
  - "@derekwaynecarr"
approvers:
  - TBD
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2022-01-06
last-updated: 2022-02-12
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MGMT-8414
see-also:
  - "https://github.com/openshift/enhancements/tree/master/enhancements/single-node"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/installer/single-node-installation-bootstrap-in-place.md"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/external-control-plane-topology.md"
replaces:
superseded-by:
---

# Single Node OpenShift With Workers

## Summary

This enhancement aims to enable adding workers to a single control-plane node
cluster by attempting to tackle multiple issues that arise when worker nodes
are added to such clusters.

## Motivation

Today OCP supports deploying a cluster which only has a single control-plane
node and no additional workers.

There's been recent demand for OCP to also formally support deployment of
single control-plane node clusters with additional (non control-plane) worker
nodes.

This is already easily done on (unsupported) cloud IPI-deployed single
control-plane node clusters by increasing the replica count of one of the
worker machinesets (there are multiple machinesets provided by default on cloud
clusters, one for each AZ, and in cloud single control-plane node clusters
they're both simply scaled to 0 by default). Doing this results in a single
control-plane node cluster with additional workers and that cluster works as
expected without any known issues.

Even on Assisted Installer installed single control-plane node clusters it's
trivial to add more workers by leveraging the Assisted Installer's day-2 worker
installation capabilities (after some minor DNS configuration issues which will
be improved by the Assisted Installer team, separately from this enhancement).

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
control-plane node clusters is always simply "SingleReplica".

The "Infrastructure Topology" parameter is used by infrastructure operators
(such as the Cluster Ingress Operator or Cluster Monitoring Operator) to
determine how many replicas they should give their various Deployments /
StatefulSets. The value of this parameter is a function of how many workers
were present during installation.

On "none"-platform single control-plane node clusters, when adding
workers, the resulting cluster has an issue with the behavior of the ingress
pod, the following paragraphs explain the background for this issue and the
issue itself.

One of the benefits of installing single-node clusters is the simplicity of not
having to deal with load-balancing and virtual IPs, as these don't provide much
value when there's only a single node behind them.

As a result, current common ways of installing single control-plane node
clusters today (mainly the Assisted Installer) avoid the usage of load
balancers or virtual IPs for API and ingress. There has also been some recent
effort to determine how single control-plane node cluster deployments on clouds
may be adjusted in order to reduce their costs, and one of the conclusions is
that getting rid of the load balancer installed by default by the IPI installer
results in major cost savings.

A user installing a single control-plane node cluster on "none"-platform will
be tempted to simply point their DNS entries directly at the IP address of the
single node that they just installed.

Similarly, in the Assisted Installer, the user is able to complete the
installation without needing to define any DNS entries. This is currently made
possible by injecting a `MachineConfig` manifest targeting the "master" pool
containing configuration for a dnsmasq server. The dnsmasq server is configured
with DNS entries for `api.<cluster>.<base>`, `api-int.<cluster>.<base>` and
`*.apps.<cluster>.<base>` which all point to the single control-plane node's IP
address. This allows the installation process and the resulting cluster to
conveniently work without the user having to think about and configure DNS for
their cluster (of course external access to the cluster requires the user to
configure DNS, but this can be done separately after the installation has
completed).

The issue with those approaches is that they assume that
`*.apps.<cluster>.<base>` should always point at the single control-plane
node's IP address. This is of course correct when there's just that single node
in the cluster, but once you start adding worker nodes to the cluster it starts
causing a potential problem - the `router-default` deployment created by the
Cluster Ingress Operator, which is responsible for load balancing ingress
traffic, targets the "worker" pool using a node selector. As a result, under
some circumstances, that deployment's pod may eventually find itself running
also on the newly added worker nodes, as those nodes obviously also belong to
the worker pool (the reason the control-plane node was also in the worker pool
is that when during installation there are no worker nodes, the OpenShift
installer sets the Scheduler CR `.spec.mastersSchedulable` to `true` and as a
result the control-plane node is in both the "master" and "worker" pools).

This pod floating between the nodes is a problem because user ingress traffic
has to be directed at the node currently holding the `router-default` pods, and
since the DNS entries have been "naively" pointed at the original control-plane
node's IP address (which may no longer be running those pods), ingress traffic
may no longer work. This can be temporarily solved if DNS is adjusted to point
at the correct node currently holding the pod or a load-balancer / some virtual
IP solution is put in place and then the DNS entry can be directed at that
load-balancer / virtual IP instead of at the node. This enhancement will try
to prevent this floating altogether so these workarounds will not be needed.

Finally, performing bootstrap-in-place installation of a "none"-platform single
control-plane node cluster today using the OpenShift Installer does not provide
the user with a worker node ignition file. As a result, the user is not able to
manually add worker nodes to the resulting cluster. All current official OCP
documentation points the user to use "the worker ignition file that get
generated during installation", but that is not currently generated in such
bootstrap-in-place installation.

### Goals

- Deal with the "floating ingress" issue encountered in "none"-platform
single control-plane node clusters which have worker nodes added to them.

- Define how the installer may be modified to generate the worker ignition
manifest, even when doing bootstrap-in-place "none"-platform single
control-plane node cluster installation

### Non-Goals

- Deal with the scalability of single control-plane node clusters that have
additional workers added to them. The absence of multiple control-plane nodes
means that the number of workers/pods that can be supported on such a topology is
even more limited than in a regular 3 control-plane node cluster. Relevant
documentation may have to point out that the pod limits (or other control-plane
related limits) that apply to a single control-plane node cluster with no
workers also apply globally across all additional added workers. If a large
amount of workers/pods is desired the user would have to re-install the cluster
as a regular multi control-plane node cluster.

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
implementation of this enhancement (if possible, their expansion may be
addressed by an upgrade followed with documentation outlining the steps that
the user has to take to enable expansion, e.g. patch the default
`IngressController` to have node selector targetting the master pool rather
than the worker pool).

- Adjust the baremetal platform to support single control-plane node
installations. The baremetal platform solves the "floating ingress" issue by
using virtual IP addresses/keepalived.

- Deal with users who want their ingress traffic to not go through the single
control-plane node.

- Deal with non-cloud, non-"none" platforms such as baremetal, vSphere, etc.

## Proposal

- Create a new `IngressPlacement` API parameter.

- Make sure worker node ignition files are generated even in bootstrap-in-place
single control-plane node "none"-platform installation.

### User Stories

- As an OpenShift cluster administrator, I want to install a single control-plane
node cluster with additional worker nodes, so that my cluster will be able to
handle larger computation demands.

- As an OpenShift cluster administrator, I want to add worker nodes to my
existing single control-plane node cluster, so that it'll be able to meet
growing computation demands.

### API Extensions

Introduce a new topology field in the Ingress config CR
(`config.openshift.io/v1/ingresses`) called `IngressPlacement`.

### Implementation Details/Notes/Constraints

This new field will have one of these values - `ControlPlane` or `Workers`. 

In addition, continue to allow the `.spec.replicas` and `.spec.nodePlacement` parameters in
`operator.openshift.io/v1/ingresscontrollers` CRs to be omitted, but change the defaulting behavior for these fields.

The value of the `IngressPlacement` field will affect the defaulting behavior
of the `IngressController`'s `.spec.replicas` and `.spec.nodePlacement`
parameters.  In the absence of an `IngressController` resource created by
the user/installer, or when the user/installer creates an `IngressController`
with these two parameters omitted, the Cluster Ingress Operator will choose the
default values for those parameters based on the value of `IngressPlacement`.

If the value of `IngressPlacement` itself is omitted, it is defaulted
to `Workers`.

When the value of `IngressPlacement` is `Workers`, the defaulting behavior of
`.spec.replicas` and `.spec.nodePlacement` will be the same as it is today:
`.spec.replicas` will be chosen according to the value of
`InfrastructureTopology`, namely `1` when `SingleReplica` or `2` when
`HighlyAvailable`. `.spec.nodePlacement` will always just be:

```yaml
nodePlacement:
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
      node-role.kubernetes.io/worker: ''
```

However, if the value of `IngressPlacement` is `ControlPlane`, the defaulting
behavior will be different: `.spec.replicas` will be chosen instead according to the
value of `ControlPlaneTopology`; again, `1` when `SingleReplica` or `2` when
`HighlyAvailable`. `.spec.nodePlacement` will be always just be:

```yaml
nodePlacement:
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
      node-role.kubernetes.io/master: ''
```

(Note that the `kubernetes.io/os: linux` label is mentioned just because it's
the current behavior, it has no importance in this enhancement)

The installer will detect situations in which it's unlikely the user will want
to set up a load-balancer. Those situations currently include installation of
single control-plane node cluster deployments on "on-prem" platforms such as
"none" or "vSphere" (although today single control-plane node clusters are only
possible on the "none" platform). In those situations, the installer will set
`IngressPlacement` to be `ControlPlane`. Since there's just a single
control-plane node, `ControlPlane` topology would be `SingleReplica` and the
combined effect would be that the `IngressController` will have just a single
replica and be pinned to the single control-plane node. This will then ensure
that the `router-default` deployment created by the Cluster Ingress Operator
will always run on the single control-plane node, and as a result any
`*.apps.<cluster>.<base>` DNS entries which originally pointed at the single
control-plane node will remain correct even in the face of newly added worker
nodes.

### Risks and Mitigations

This should make no noticable difference on "regular" single control-plane node
clusters which do not have any day-1 or day-2 worker nodes. The only difference
for those clusters would be the `IngressController` targeting the "master"
pool rather than the "worker" pool, but since the single control-plane node is
already both in the "master" and "worker" pools, that should make no practical
difference.

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

TODO: Describe day-1-workers tests?

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

This enhancement, to the best of my knowledge, should have no problems
persisting across cluster upgrades. The Test Plan section describes how this
will be tested.

### Version Skew Strategy

Does not apply, to the best of my understanding.

### Operational Aspects of API Extensions

TBD

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

TBD

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

TBD

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

Not yet applicable

## Drawbacks

- The pinning of the `IngressController` to the "master" pool is another change
which would make single-node clusters slightly different from multi-node
clusters, and any such difference is naturally not ideal.

- The proposed defaulting behavior for the discussed `IngressController` parameters is
complicated and dependent on all three topology parameters (infra topology, control-plane, topology, and 
ingress placement) - such complexity would probably have to be documented in the CRD
definitions and may confuse users.

## Alternatives

- Even when users need to add just one extra worker, require them to add yet
another worker so they could just form a compact 3-node cluster where all
nodes are both workers and control-plane nodes. This kind of topology is
already supported by OCP. This will avoid the need for OCP to support yet
another topology. It has the obvious downside of requiring a "useless" node
the user didn't really need. It also means the user now has to run more
control-plane workloads to facilitate HA - for example, 2 extra replicas of
the API server which consume a lot of memory resources. From an engineering
perspective, it would require us to make the "Control-plane Topology"
parameter dynamic and make sure all operators know to react to changes in that
parameter (it will have to change from "SingleReplica" to "HighlyAvailable"
once those 2 new control-plane nodes join the cluster). I am not aware of the
other engineering difficulties we would encounter when attempting to support
the expansion of a single-node control-plane into a three-node control-plane,
but I think they would not be trivial.

- Adjust the "baremetal" platform to support single control-plane node
installations and make users and the Assisted Installer (the current popular,
supported method to install single control-plane node clusters) use that
platform instead of the "none" platform. The baremetal platform solves the
issue described in this enhancement with virtual IP addresses/keepalived. This
approach was dismissed due to much higher development efforts and additional
processes that would need to run on the already resource constrained single
control-plane node. Furthermore, even once the baremetal platform is adjusted
to support single-node clusters, the Assisted Installer would have to go
through a lot of development effort in order to make it use the baremetal
platform rather than the "none" platform currently used for single node
installations. This may happen in the future.

## Infrastructure Needed [optional]

N/A
