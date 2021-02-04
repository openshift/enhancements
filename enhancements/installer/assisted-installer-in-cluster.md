---
title: assisted-installer-in-cluster
authors:
  - "@mhrivnak"
  - "@hardys"
  - "@avishayt"
reviewers:
  - "@eparis"
  - "@markmc"
  - "@dgoodwin"
  - "@ronniel1"

approvers:
  - TBD
creation-date: 2020-12-22
last-updated: 2021-02-03
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
see-also:
replaces:
superseded-by:
---

# Assisted Installer in-cluster

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Assisted Installer currently runs as a SaaS on cloud.redhat.com, enabling
users to deploy OpenShift clusters with certain customizations, particularly on
bare metal hardware. It is necessary to bring those capabilities on-premise in
users' clusters for two purposes:

1. A cluster created by the Assisted Installer needs the ability to add workers
on day 2.
2. Multi-cluster management, such as through hive and RHACM, should be
able to utilize the capabilities of the Assisted Installer.

This enhancement proposes the first iteration of running the Assisted Installer
in end-user clusters to meet the purposes above.

## Motivation

### Goals

* Expose the Assisted Installer's capabilities as a kubernetes-native API. Some
portion may be exposed through [hive](https://github.com/openshift/hive)'s API.
* Enable any OpenShift cluster deployed by the Assisted Installer to add worker nodes on day 2.
* Enable multi-cluster management tooling to create new clusters using Assisted Installer.
* Enable automated creation and booting of the assisted discovery ISO for bare-metal deployments.
* Ensure the design is extensible for installing on other platforms.

### Non-Goals

* Solve central machine management. That can be done with this effort, or after
this effort, but it is not strictly a requirement in order to deliver the
goals stated above.
* Run metal3 components on a non-baremetal cluster.

## Proposal

### User Stories

#### Day 2 Add Worker

After deploying a cluster with Assisted Installer, I can add workers from bare
metal hardware on day two using a similar workflow and tool set. I can either
obtain the discovery live ISO and use my own methods to boot it, or I can use
the baremetal-operator to boot the live ISO automatically.

#### Multi-cluster

As a user of Red Hat's multi-cluster mangement tools, I can use the assisted
installation agent-based workflow to create clusters from a pool of bare metal
inventory.

### Implementation Details/Notes/Constraints

#### Concurrent development with SaaS

The assisted service is currently deployed as a SaaS on cloud.redhat.com with a
non-k8s REST API and a SQL database. The software implementing that needs to
continue to exist with those design choices in order to meet the scale needs of
a service that runs on the internet.

Those design choices are not the best fit for an in-cluster service. Based on
OpenShift's approach to infrastructure management of using kubernetes-native
APIs, an implementation of the assisted service's features that is based on
CRDs is additionally necessary. Both implementations of similar functionality
need to be developed and maintained concurrently.

The first implementation of controllers that provide a CRD-based API will be
added to the existing
[assisted-service](https://github.com/openshift/assisted-service) code in a way
that allows the existing REST API to continue existing and running
side-by-side, including its database. An end user, whether human or automation,
will only need to interact with the CRDs; the database and REST API will be
implementation details that can be removed in the future. While continuing to
run a separate database and continuing to utilize parts of the existing REST
API are not ideal for an in-cluster service, this approach is the only
practical way to deliver a working solution in a short time frame.

While making a separate version that is Kubernetes-native with no SQL DB is on
the table for the long term, it is not feasible to implement and maintain such
a different pattern in the near-term.

#### Assisted Installer k8s-native API

The Assisted Installer has a [REST
API](https://generator.swagger.io/?url=https://raw.githubusercontent.com/openshift/assisted-service/master/swagger.yaml)
that is available at cloud.redhat.com. The service is implemented with a
traditional relational database. In order to integrate with OpenShift
infrastructure management tooling, it is necessary to additionally expose its
capabilities as Kubernetes-native APIs. They will be implemented as Custom
Resource Definitions and a controller with the assisted service's capabilities.

For the first deliverable, a local deployment of the backing service will
include new controllers that will operate to achieve the desired state as
declared in CRDs. Thus the service will expose both REST and Kubernetes-native
APIs, and both API layers will call the same set of internal methods. The local
service will include a database; sqlite if possible, or else postgresql.

    --------------------------
    |  REST API  |  k8s API  |
    --------------------------
    | Assisted Service logic |
    --------------------------
        |               |
        v               v
      SQL DB        File System
                      storage

For modest clusters that do not have persistent storage available but need
day-2 ability to add nodes, the database will need to be reconstructable in
case its current state is lost. Thus the CRs will be the source of truth for
the specs, and the controller will upon startup first ensure that the DB
reflects the desired state in CRs. The source of truth for the status, however,
is the actual state of the agents running on the hosts being installed and not
necessarily what was previously recorded.

For hub clusters that are used for multi-cluster creation and management,
persistent storage will be a requirement.

Agents that are running on hardware and waiting for installation-related
instructions will continue to communicate with the assisted service using the
existing REST API. In the future that may transition to using a CRD directly.
In the meantime, status will be propagated to the appropriate k8s resource when
an agent reports relevant information.

**InstallEnv**
This new resource, part of the assisted intaller's new operator, represents an
environment in which a group of hosts share settings related to networking,
local services, disk layout, etc. This resource is used to create a discovery
image that should be used for booting hosts. In the REST API this corresponds
to the "image" resource that is embedded in the "cluster" resource, but for
kubernetes-native it is more natural for it to be separate.

The discovery ISO can be downloaded from a URL that is available in the
resource's Status.

The details of this resource definition are being discussed [in a
pull request](https://github.com/openshift/assisted-service/pull/969/files).

**ClusterDeployment**
Hive's ClusterDeployment CRD will be extended to include all cluster details
that the agent-based install needs. There will not be a new cluster resource.
The contents of this API correlate to the "cluster" resource in the assisted
installer's current REST API.

The details of this are being discussed on [a hive
pull request](https://github.com/openshift/hive/pull/1247).


**Agent**
Agent is a new resource, part of the assisted installer's new operator, that
represents a host that is destined to become part of an OpenShift cluster, and
is running an agent that is able to run inspection and installation tasks. It
correlates to the "host" resource in the Assisted Installer's REST API.

The details of this API are being discussed in a [pull
request](https://github.com/openshift/assisted-service/pull/861) that
implements the CRD.


#### REST API Access

Some REST APIs need to be exposed in addition to the Kubernetes-native APIs
described below.
* Download ISO and PXE artifacts: These files must be available for download
via HTTP, either directly by users or by a BMC.  Because BMCs do not pass
authentication headers, the Assisted Service must generate some random URL or
query parameter so that the ISO’s location isn’t easily guessable.
* Agent APIs (near-term): Until a point where the agent creates and modifies
Agent CRs itself, the agent will continue communicating with the service via
REST APIs. Currently the service embeds the user’s pull secret in the discovery
ISO which the agent passes in an authentication header.  In this case the
service can generate and embed some token which it can later validate.


#### Hive Integration

Hive has a [ClusterDeployment
CRD](https://github.com/openshift/hive/blob/master/docs/using-hive.md#clusterdeployment)
resource that represents a cluster to be created. It includes a "platform"
section where platform-specific details can be captured. For an agent-based
workflow, this section will include a new `AgentBareMetal` platform that
contains such fields as:

* API VIP
* API VIP DNS name
* IngressVIP
* VIPDHCPAllocation

A new `InstallStrategy` section of the Spec enables the API to describe a way
of installing a cluster other than the default of using `openshift-install`.
The new field has an "agent" option that includes such fields as:

* AgentSelector, a label selector for identifying which Agent resources should
be included in the cluster.
* ProvisionRequirements, where the API user can specify how many agents to
expect before beginning installation for each of the control-plane and worker
roles.

#### Use of metal3

[metal3](https://metal3.io/) will be used to boot the assisted discovery ISO
for bare-metal deployments. Specifically, the BareMetalHost API will be used
from a hub cluster to boot the discovery ISO on hosts that will be used to
form new clusters.

For this first iteration, it is assumed that the hub cluster itself is using
the bare metal platform, and thus will have baremetal-operator installed
and available for use. In the future it may be desirable to make the metal3
capabilities available on hub clusters that are not using the bare metal
platform.

#### BareMetalHost can boot live ISOs

A [separate enhancement to
metal3](https://github.com/metal3-io/metal3-docs/pull/150) proposes a new
capability in the BareMetalHost API enabling it to boot live ISOs. That feature
is required so that automation in a cluster can boot the discovery ISO on known
hardware as the first step toward provisioning that hardware.

#### CAPBM

[Cluster-API Provider Bare
Metal](https://github.com/openshift/cluster-api-provider-baremetal/) will need
to gain the ability to interact with the new assisted installer APIs for day 2
"add worker" use cases. Specifically it will need to match a Machine with an
available assisted Agent that is ready to be provisioned. This will be in
addition to matching a Machine with a BareMetalHost, which it already does
today.

Other platforms may benefit from similar capability, such as on-premise virtualization
platforms. Ideally the ability to match a Machine with an Agent will be delivered
so that it can be integrated into multiple machine-api providers. Each platform would
perform this workflow:

1. A Machine gets created, usually as a result of a MachineSet scaling up.
1. The platform does whatever is necessary to boot the discovery live ISO on a host.
1. The platform waits for a corresponding Agent resource to appear.
1. The Agent gets associated with the Machine.
1. The agent-based provisioning workflow is initiated by the machine-api provider.

#### Host Approval

It is important that when an agent checks in, it not be allowed to join a
cluster until an actor has approved it. From a security standpoint, it is not
ok for anyone who can access a copy of a discovery ISO and/or access the API
where agents report themselves to implicitly have the capability to join a
cluster as a Node.

The Agent CRD will have a field in which to designate that it is approved.

#### Day 2 Add Node Boot-it-Yourself

This scenario takes place within a stand-alone bare metal OpenShift cluster.

1. The user downloads a discovery ISO from the cluster. The download is implemented by the assisted installer as a URL on a InstallEnv resource.
resource.
1. A host boots the live ISO, the assisted agent starts running, and the agent contacts the assisted service to register its existence. Communication utilizes the existing non-k8s REST API. The agent walks through the validation and inspection workflow as it exists today.
1. The assisted service creates a new Agent resource to be the k8s-native API for the agent.
1. A new Baremetal Agent Controller (eventually part of OpenShift's baremetal machine API provider) creates a BareMetalHost resource, setting the status annotation based on inspection information from the prior step.
1. The user approves the Agent for installation by setting a field in its Spec.
1. The user or an orchestrator scales up a MachineSet, causing a new Machine resource to be created.
1. CAPBM binds the Machine to the BareMetalHost, as it does today. It additionally finds the Agent CR and uses it to begin installation.
1. The assisted service initiates installation of the host.
1. CAPBM updates the status on the BareMetalHost to reflect that it has been provisioned.

#### Day 2 Add Node Virtualmedia Stand-alone

This scenario takes place within a stand-alone bare metal OpenShift cluster.

1. The user creates a BareMetalHost that includes BMC credentials and a label indicating it is associated with an InstallEnv.
1. The Baremetal Agent Controller gets a URL to the live ISO and adds it to the BareMetalHost, causing it to boot the host.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO.
1. The assisted agent starts running on the new hardware and runs through its usual validation and inspection workflow. The assisted service creates a new Agent resource to be the k8s-native API for the agent.
1. The user or an orchestrator scales up a MachineSet, resulting in a new Machine being created.
1. CAPBM does its usual workflow of matching the Machine to an available BareMetalHost. Additionally it uses the Agent CR to initiate provisioning of the host.
1. The assisted service provisions the host.

#### Personas for multi-cluster management

**Infra Owner** Manages physical infrastructure and configures hosts to boot the discovery ISO that runs the Agent.

**Cluster Creator** Uses running Agents to create and grow clusters.

#### Day 2 Add Node Virtualmedia Multicluster (add Remote Worker Node)

This scenario takes place from a hub cluster, adding a worker node to a spoke cluster.

1. Infra Owner creates a BareMetalHost resource with a label that matches an InstallEnv selector.
1. The Baremetal Agent Controller adds the discovery ISO URL to the BareMetalHost.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO on the BareMetalHost.
1. The Agent starts up and reports back to the assisted service, which creates an Agent resource in the cluster. The Agent is labeled with the labels that were specified in the InstallEnv's Spec.
1. The Agent's Role field in its spec is assigned a value if a corresponding label and value were present on its BareMetalHost. (only "worker" is supported for now on day 2)
1. The Agent is marked as Approved via a field in its Spec based on being recognized as running on the known BareMetalHost.
1. The Agent runs through the validation and inspection phases. The results are shown on the Agent's Status, and eventually a condition marks the Agent as "ready".
1. The Baremetal Agent Controller adds inspection data found on the Agent's Status to the BareMetalHost.
1. When the agent is in a ready state, installation of that host begins.

#### Create Cluster

This scenario takes place on a hub cluster where hive and possibly RHACM are
present. This scenario does not include centralized machine management.

1. Infra Owner creates an InstallEnv resource. It can include fields such as egress proxy, NTP server, ssh public key, ... In particular it includes an Agent label selector, and a separate field of labels that should be applied to Agents.
1. Infra Owner creates BareMetalHost resources that include BMC credentials. They are labeled so that they match the selctor on the InstallEnv.
1. A new controller, the Baremetal Agent Controller, sees the matching BareMetalHosts and boots them using the discovery ISO URL found in the InstallEnv's status.
1. The Agent starts up on each host and reports back to the assisted service, which creates an Agent resource in the cluster. The Agent is labeled with the labels that were specified in the InstallEnv's Spec.
1. The Agent's Role field in its spec is assigned a value if a corresponding label and value were present on its BareMetalHost.
1. The Agent is automatically marked as Approved via a field in its Spec based on being recognized as running on the known BareMetalHost.
1. The Agent runs through the validation and inspection phases. The results are shown on the Agent's Status, and eventually a condition marks the Agent as "ready".
1. Cluster Creator creates a ClusterDeployment describing a new cluster. It describes how many control-plane and worker agents to expect. It also includes a label selector to match Agent resources that should be part of the cluster.
1. Cluster Creator applies a label to Agents if necessary so that they match the ClusterDeployment's selector.
1. Once there are enough ready Agents of each role to fulfill the expected number as expressed on the ClusterDeployment, installation begins.

#### Static Networking

Some customers have asked for the ability to provide static network details
up-front for each host instead of using DHCP. They want to define this
configuration at the same time they define the corresponding BareMetalHost.

A net resource called NMStateConfig will have a Spec with the following
fields:

* MACAddress: a MAC address for any network device on the host to which this config should be applied. This value is only used to ensure that the config is applied to the intended host.
* Config: a byte array that can contain a raw [nmstate](https://www.nmstate.io/) network config.

nmstate is already in use within OpenShift for applying network configuration
to nodes via the [kubernetes-nmstate
operator](https://github.com/nmstate/kubernetes-nmstate).

Each NMStateConfig resource will have a label that corresponds to a InstallEnv.
The raw YAML configs for each matching resource will be rendered to a network
config by the assisted service and then embedded into the discovery ISO for
that InstallEnv. At runtime, the discovery ISO will find the config that
matches a MAC address on the current host and then apply the config. It does
not matter which interface has the matching MAC address; the matching is merely
used to identify that the current host corresponds to a given config.

The NMStateConfig resource design is being discussed [in a
pull request](https://github.com/openshift/assisted-service/pull/969/files).

#### Install Device

The Agent resource Spec will include a field on which to specify the storage device
where the OS should be installed. That field must be set by a platform-specific
controller or some other actor that understands the underlying host.

For bare metal, the BareMetalHost resource Spec already includes a
RootDeviceHints section that will be utilized. The new Baremetal Agent
Controller (previously described in the "Create Cluster" scenario) will use the
BareMetalHost's RootDeviceHints and the Agent's discovery data to populate this
field on the Agent.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Open Questions

#### Can we use sqlite?

It would be advantageous to use sqlite for the database especially on
stand-alone clusters, so that it is not necessary to deploy and manage
an entire RDBMS, nor ship and support its container image. The [gorm library
supports sqlite](https://gorm.io/docs/connecting_to_the_database.html#SQLite),
but it is not clear if assisted service is compatible. In particular, the [use
of FOR
UPDATE](https://github.com/openshift/assisted-service/blob/e70af7dcf59763ee6c697fb409887f00ab5540f5/pkg/transaction/transaction.go#L8)
might be problematic.

For hub clusters doing multi-cluster creation and management, there is an
expectation that persistent storage availability and scale concerns will be a
better fit for running a full RDBMS.

A suggestion has been made that in situations where there is a single process
running the assisted service, locking can happen in-memory instead of in the
database. Further analysis is required.

#### baremetal-operator watching multiple namespaces?

When utilizing baremetal-operator on a hub cluster to boot the discovery ISO on
hosts, should we be creating those BareMetalHost resources in a separate
namespace from those that are associated with the hub cluster itself?

What work is involved in having BMO watch additional namespaces?

Upstream, the metal3 project is already running baremetal-operator watching
multiple namespaces, so there should not be software changes required.

#### Centralized Machine Management

OpenShift's multi-cluster management is moving toward a centralized Machine
management pattern, as an addition to the current approach where Machines only
exist in the same cluster as their associated Node. This proposal should be
compatible with centralized Machine management, but it would be useful to play
through those workflows in detail to be certain.

#### Garbage Collection

Agent resources are not useful after provisioning except possibly as a
historical record. A plan should be in place for garbage collecting them.

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

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

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

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
