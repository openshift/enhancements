---
title: primary-user-defined-networks-for-virtualization-workloads
authors:
  - "@maiqueb"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@trozet"   # the integration with the SDN side
  - "@jcaamano" # the integration with the SDN side / focusing on the persistent IPs part
  - "@EdDev"    # the integration with the KubeVirt / CNV side
  - "@qinqon"   # everything
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@trozet"
api-approvers:
  - "@dougbtv"
creation-date: 2024-08-30
last-updated: 2024-08-30
tracking-link:
  - https://issues.redhat.com/browse/CNV-46779
see-also:
  - "/enhancements/network/user-defined-network-segmentation.md"
  - "/enhancements/network/persistent-ips-sdn-secondary-networks.md"
---

# Primary User Defined Networks for Virtualization workloads

## Summary

Live-migration is the bread and butter of any virtualization solution.
OpenShift Virtualization does not currently support live-migration over the
OpenShift default cluster network, for a variety of reasons. The scope of this
enhancement is to define how to use the existent
[primary UDN feature](https://github.com/openshift/enhancements/pull/1623/) to
enable virtualization workloads to migrate over the primary network.

## Motivation

Virtualization users are commonly used to having layer2 networks, and to having
their tenant networks isolated from other tenants. This is something that
opposes the Kubernetes networking model, in which all pods can reach other pods,
and security is provided by implementing Network Policy.

To streamline the migration experience of users coming from these traditional
virtualization platforms into OpenShift Virtualization we need to match the
user's expectations in terms of feature set, and user experience. Thus, we need
to provide them with isolated primary networks, that meet the live-migration
requirements.

Other type is users just want to have a more managed experience - they do not
want to have to manage their networks, e.g. deploying DHCP servers, DNS, and
so forth. They are hoping to have a streamlined experience, where they define
a (set of) subnets for their layer2 network, and the SDN itself is responsible
for assigning migratable IP addresses to the virt workloads, configure access
to outside the cluster, and, when configured by the user, be able to access an
application they are interested in exposing from the outside world.

### User Stories

- As the owner of an application running inside my VM, I want to have east/west
communication without NAT to other VMs and/or pods.
- As a developer who defined a custom primary network in their project, I want
to use the primary network to which the VM is connected for north/south, while
still being able to connect to KAPI and consume Kubernetes DNS.
- As a VM owner, I want the VM to have the same IP address before/after live
migrating.
- As a VM owner, I want to be able to specify the IP address for the interface
of my VM.
- As an owner of a VM that is connected only to the primary network, I want to 
fetch resources from outside networks (internet).
- As a developer migrating my VMs to OCP, I do not want to change my
application to support multiple NICs.
- As a VM owner, I want to expose my selected applications over the network to
users outside the cluster.
- As an admin, I'm limited by public cloud networking restrictions and I rely
on their LoadBalancer to route traffic to my applications.

### Goals

- The IP addresses on the VM must be the same before / after live-migration
- Live-migration without breaking the established TCP connections
- Provide a configurable way for the user to define the IP addresses on a VM's
interface.
- Native IPv6 integration
- Integration with service meshes, and OpenShift observability solutions

### Non-Goals

TODO

## Proposal

To compartmentalize the solution, the proposal will be split in three different
topics:
- [Extending networking from the pod interface to the VM](#extending-networking-from-the-pod-interface-to-the-VM)
- [Persisting VM IP addresses during the migration](#persisting-vm-ip-addresses-during-the-migration)
- Allow user to configure the VM's interface desired IP address

Before that, let's ensure the proper context is in place.

### Basic UDN context

As indicated in the
[user defined network segmentation proposal](https://github.com/openshift/enhancements/blob/master/enhancements/network/user-defined-network-segmentation.md#proposal),
the pods featuring a primary user defined network will feature two interfaces:
- an attachment to the cluster default network
- an attachment to the primary user defined network

Later on, in the
[services section](https://github.com/openshift/enhancements/blob/master/enhancements/network/user-defined-network-segmentation.md#services-1),
it is explained that all traffic will be send over the primary UDN interface.

### Basic virtualization context

In KubeVirt / OpenShift Virtualization, the VM runs inside a pod (named
virt-launcher). Thus, given OVN-Kubernetes configures the pod interfaces (and
is responsible for configuring networking up to the pod interface), we still
need to extend connectivity from the pod interface into the VM.

During live-migration - once it is is scheduled - and the destination
node is chosen, a new pod is scheduled in the target node (let's call this pod
the *destination* pod). Once this pod is ready, the *source* pod transfers the
state of the live VM to the *destination* pod via a connection proxied by the
virt-handlers (the KubeVirt agents running on each node) to the *destination*
pod.

### Extending networking from the pod interface to the VM

KubeVirt uses a concept called bind mechanisms to extend networking to the VM;
we plan on using the
[passt](https://passt.top/passt/about/#passt-plug-a-simple-socket-transport)
binding for this, which maps the layer2 network interface in the guest to
native layer4 sockets (TCP/UDP/ICMP) on the host where the guest runs.
To improve the user experience, the VM will have a single interface in it.

Passt relies on a user space program running on the pod namespace that maps
traffic from the guest to the respective socket in the host; it currently is
**not** migrated to the destination pod during migration, which causes the
established TCP connections to be severed when the VM is migrated.

This requires an enhancement on passt.

TODO: link the ticket to get an enhancement

### Persisting VM IP addresses during the migration

OpenShift already features the ability of providing persistent IP addresses
for layer2 **secondary** networks. It relies on the
[IPAMClaim CRD](https://github.com/k8snetworkplumbingwg/ipamclaims/blob/48c5a915da3b67f464a4e52fa50dbb3ef3547dcd/pkg/crd/ipamclaims/v1alpha1/types.go#L23)
to "tie" the allocation of the IP address to the VM.
In short, KubeVirt (or a component on behalf
of KubeVirt) creates an `IPAMClaim` resource for each interface in the VM the
user wants to be persistent, and KubeVirt instructs the CNI plugin of which
claim to use to persist the IP for the VM using an attribute defined in the
k8snetworkplumbingwg network-selection-elements (the
`k8s.v1.cni.cncf.io/networks` annotation). This protocol is described in depth
in
[the persistent IPs for virt workloads enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/network/persistent-ips-sdn-secondary-networks.md#creating-a-virtual-machine).

We want to re-use this mechanism to implement persistent IPs for UDN networks;
meaning KubeVirt (or a component on behalf of it) will create the `IPAMClaim`
and will instruct OVN-Kubernetes of which claim to use. Since for primary UDNs
we do **not** define the network-selection-elements, we need to use a new
annotation to pass this information along.

The proposed annotation value is `k8s.ovn.org/ovn-udn-ipamclaim-reference`.

These persistent IPs will be cleaned up by the Kubernetes garbage collector
once the VM to which they belong is removed.

All the other
[work required in OVN-Kubernetes](https://github.com/trozet/enhancements/blob/941f5c6391830d5e4a94e65d742acbcaf9b8eda9/enhancements/network/user-defined-network-segmentation.md#pod-egress)
to support live-migration was already implemented as part of the epic
implementing UDN.

### Workflow Description

Explain how the user will use the feature. Be detailed and explicit.
Describe all of the actors, their roles, and the APIs or interfaces
involved. Define a starting state and then list the steps that the
user would need to go through to trigger the feature described in the
enhancement. Optionally add a
[mermaid](https://github.com/mermaid-js/mermaid#readme) sequence
diagram.

Use sub-sections to explain variations, such as for error handling,
failure recovery, or alternative outcomes.

For example:

**cluster creator** is a human user responsible for deploying a
cluster.

**application administrator** is a human user responsible for
deploying an application in a cluster.

1. The cluster creator sits down at their keyboard...
2. ...
3. The cluster creator sees that their cluster is ready to receive
   applications, and gives the application administrator their
   credentials.

See
https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md#high-level-end-to-end-workflow
and https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md for more detailed examples.

### API Extensions

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

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

## Graduation Criteria

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

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

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
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
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

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

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

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

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

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
