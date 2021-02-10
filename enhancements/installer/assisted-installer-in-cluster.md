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
last-updated: 2021-02-10
status: implementable
see-also:
  - enhancements/installer/connected-assisted-installer.md
  - enhancements/installer/assisted-installer-bare-metal-validations.md
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
users' "Hub" clusters by installing clusters via Multi-cluster management, such
as through Hive and RHACM.

This enhancement proposes the first iteration of running the Assisted Installer
in end-user clusters to meet the purpose above.

## Motivation

The Assisted Installer running as a SaaS has demonstrated a new and valuable
way to install OpenShift clusters through an agent-based flow. The [Assisted
Installer for Connected Environments](connected-assisted-installer.md)
enhancement proposal elaborates on the value of the agent-based installations.

However many customers do not want a third party service running outside of
their network to be able to provision hosts inside their network. Reasons
include trust, control, and reproducibility. Those users will prefer to run the
service on-premises. There are also many customers whose network environments
have limited or no connectivity to the internet. Finally, it is desirable to
integrate this new installation method into RHACM, which runs on-premesis.

### Goals

* Expose the Assisted Installer's capabilities as a kubernetes-native API. Some
portion may be exposed through [Hive](https://github.com/openshift/hive)'s API.
* Enable multi-cluster management tooling to create new clusters using Assisted Installer.
* Enable adding workers to any OpenShift cluster deployed by the Assisted Installer via
  the same multi-cluster management tooling.
* Enable automated creation and booting of the Assisted discovery ISO for bare-metal deployments.
* Ensure the design is extensible for installing on other platforms.

### Non-Goals

* Remote Worker Node support requires solving specific problems that will be
addressed in a separate proposal.
* Solve central machine management. ("central machine management" involves
running machine-api-providers on a hub cluster in order to manage Machines on
the hub which represent Nodes in spoke clusters.) That can be done with this
effort, or after this effort, but it is not strictly a requirement in order to
deliver the goals stated above.
* Run metal3 components on a non-baremetal cluster.

## Proposal

### User Stories

#### Install cluster from hub

As a user of Red Hat's multi-cluster mangement tools, I can use the assisted
installation agent-based workflow to create clusters from a pool of bare metal
inventory.

#### Add Worker Node from Hub

After deploying a cluster with Assisted Installer, I can add workers from bare
metal hardware on day two using a similar agent-based workflow and tool set.

#### Run Agent via Boot It Yourself

Whether you are creating a new cluster or adding nodes to an existing cluster,
the workflow starts by booting a discovery ISO on a host so that it runs the
Agent. Many users have their own methods for booting ISOs on hardware. The
Assisted Service must be able to create a discovery ISO that users can take and
boot on hardware with their own methods.

#### Run Agent via Automation

Many users will want an integrated end-to-end experience where they describe
hardware and expect it to automatically boot an appropriate discovery ISO
without needing to provide their own mechanism for booting ISOs.

### Implementation Details/Notes/Constraints

#### Concurrent development with SaaS

The Assisted Service is currently deployed as a SaaS on cloud.redhat.com with a
non-k8s REST API and a SQL database. The software implementing that needs to
continue to exist with those design choices in order to meet the scale needs of
a service that runs on the Internet.

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
Resource Definitions and controllers with the Assisted Service's capabilities.

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

For hub clusters that are used for multi-cluster creation and management,
persistent storage will be a requirement for the database and file system
storage.

Agents that are running on hardware and waiting for installation-related
instructions will continue to communicate with the assisted service using the
existing REST API. In the future that may transition to using a CRD directly.
In the meantime, status will be propagated to the appropriate k8s resource when
an agent reports relevant information.

**InstallEnv**
This new resource, part of the assisted intaller's new operator, represents an
environment in which a group of hosts share settings related to networking,
local services, etc. This resource is used to create a discovery image that
should be used for booting hosts. In the REST API this corresponds to the
"image" resource that is embedded in the "cluster" resource, but for
kubernetes-native it is more natural for it to be separate.

The discovery ISO can be downloaded from a URL that is available in the
resource's Status.

The details of this resource definition are being discussed [in a
pull request](https://github.com/openshift/assisted-service/pull/969).

**ClusterDeployment**
Hive's ClusterDeployment CRD will be extended to include all cluster details
that the agent-based install needs. There will not be a new cluster resource.
The contents of this API correlate to the "cluster" resource in the assisted
installer's current REST API.

The details of this are being discussed on a Hive
[pull request](https://github.com/openshift/hive/pull/1247).


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
authentication headers, the Assisted Service must generate some URL or query
parameter so that the ISO’s location isn’t easily guessable.
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

A new `InstallStrategy` section of the Spec enables the API to describe a way
of installing a cluster other than the default of using `openshift-install`.
The new field has an "agent" option that would be common to any Agent-driven
installation, regardless of platform. Fields include:

* AgentSelector, a label selector for identifying which Agent resources should
be included in the cluster.
* ProvisionRequirements, where the API user can specify how many agents to
expect before beginning installation for each of the control-plane and worker
roles.

#### Use of metal3

[metal3](https://metal3.io/) will be used to boot the assisted discovery ISO
for bare-metal deployments. Specifically, the BareMetalHost API will be used
from a Hub cluster to boot the discovery ISO on hosts that will be used to
form new clusters.

For this first iteration, it is assumed that the hub cluster itself is using
the bare metal platform, and thus will have baremetal-operator installed
and available for use. In the future it may be desirable to make the metal3
capabilities available on hub clusters that are not using the bare metal
platform.

#### BareMetalHost can boot live ISOs

A separate [enhancement to
metal3](https://github.com/metal3-io/metal3-docs/pull/150) proposes a new
capability in the BareMetalHost API enabling it to boot live ISOs other than
the Ironic Python Agent. That feature is required so that automation in a
cluster can boot the discovery ISO on known hardware as the first step toward
provisioning that hardware.

#### Host Approval

It is important that when an agent checks in, it not be allowed to join a
cluster until an actor has approved it. From a security standpoint, it is not
OK for anyone who can access a copy of a discovery ISO and/or access the API
where agents report themselves to implicitly have the capability to join a
cluster as a Node.

The Agent CRD will have a field in which to designate that it is approved. If
the host was booted via the baremetal-operator, approval will be granted
automatically. The same automation that caused the host to boot would have the
ability to recognize the resulting Agent and mark it as approved.

#### Day 2 Add Node Boot-it-Yourself

#### Personas for multi-cluster management

**Infra Owner** Manages physical infrastructure and configures hosts to boot
the discovery ISO that runs the Agent.

**Cluster Creator** Uses running Agents to create and grow clusters.

#### Create Cluster

This scenario takes place on a hub cluster where hive and possibly RHACM are
present. This scenario does not include centralized machine management.

This scenario is an end-to-end flow that enables the user to specify everything
up-front and then have an automated process create a cluster. Because the
user's primary frame of reference is hardware-oriented, this flow enables them
to specify Node attributes such as "role" with their BareMetalHost definition.
Specifying those attributes together is particularly useful in a gitops
approach.

1. Infra Owner creates an InstallEnv resource. It can include fields such as egress proxy, NTP server, SSH public key, ... In particular it includes an Agent label selector, and a separate field of labels that will be automatically applied to Agents.
1. Infra Owner creates BareMetalHost resources with corresponding BMC credentials. They must be labeled so that they match the selctor on the InstallEnv and must be in the same namespace as the InstallEnv.
1. A new controller, the Baremetal Agent Controller, sees the matching BareMetalHosts and boots them using the discovery ISO URL found in the InstallEnv's status.
1. The Agent starts up on each host and reports back to the assisted service, which creates an Agent resource in the cluster. The Agent is automatically labeled with the labels that were specified in the InstallEnv's Spec.
1. The Baremetal Agent Controller sets the Agent's Role field in its spec to "master" or "worker" if a corresponding label was present on its BareMetalHost.
1. The Agent is automatically marked as Approved via a field in its Spec based on being recognized as running on the known BareMetalHost.
1. The Agent runs through the validation and inspection phases. The results are shown on the Agent's Status, and eventually a condition marks the Agent as "ready".
1. Cluster Creator creates a ClusterDeployment describing a new cluster. It describes how many control-plane and worker agents to expect. It also includes a label selector to match Agent resources that should be part of the cluster.
1. Cluster Creator applies a label to Agents if necessary so that they match the ClusterDeployment's selector.
1. Once there are enough ready Agents of each role to fulfill the expected number as expressed on the ClusterDeployment, installation begins.

#### Static Networking

Some customers have asked for the ability to provide static network details
up-front for each host instead of using DHCP. They want to define this
configuration at the same time they define the corresponding BareMetalHost.

A new resource called NMStateConfig will have a Spec with the following
fields:

* MACAddress: a MAC address for any network device on the host to which this config should be applied. This value is only used to ensure that the config is applied to the intended host.
* Config: a byte array that can contain a raw [nmstate](https://www.nmstate.io/) network config.

nmstate is already in use within OpenShift for applying network configuration
to nodes via the [kubernetes-nmstate
operator](https://github.com/nmstate/kubernetes-nmstate). It uses a similar
pattern of embedding a raw nmstate yaml structure as a byte array in the Spec.

Each NMStateConfig resource will have a label that corresponds to a InstallEnv.
The raw YAML configs for each matching resource will be rendered to a network
config by the Assisted Service and then embedded into the discovery ISO for
that InstallEnv. At runtime, the discovery ISO will find the config that
matches a MAC address on the current host and then apply the config. It does
not matter which interface has the matching MAC address; the matching is merely
used to identify that the current host corresponds to a given config.

The NMStateConfig resource design is being discussed [in a
pull request](https://github.com/openshift/assisted-service/pull/969).

#### Install Device

The Agent resource Spec will include a field on which to specify the storage device
where the OS should be installed. That field must be set by a platform-specific
controller or some other actor that understands the underlying host.

For bare metal, the BareMetalHost resource Spec already includes a
RootDeviceHints section that will be utilized. The new Baremetal Agent
Controller (previously described in the "Create Cluster" scenario) will use the
BareMetalHost's RootDeviceHints and the Agent's discovery data to populate this
field on the Agent.

#### Day 2 Add Node Virtualmedia Multicluster

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

For Hub clusters doing multi-cluster creation and management, there is an
expectation that persistent storage availability and scale concerns will be a
better fit for running a full RDBMS.

A suggestion has been made that in situations where there is a single process
running the assisted service, locking can happen in-memory instead of in the
database. Further analysis is required.

#### baremetal-operator watching multiple namespaces?

When utilizing baremetal-operator on a Hub cluster to boot the discovery ISO on
hosts, should we be creating those BareMetalHost resources in a separate
namespace from those that are associated with the Hub cluster itself?

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

### Expose a non-k8s-native REST API

The assisted installer service runs as a SaaS where it has a traditional
non-k8s-native REST API. This proposal includes exposing that same
functionality as a CRD-based API.

As an alternative, the SaaS's existing non-k8s-native REST API could be exposed
directly for use by hive, RHACM, the openshift console, and other management
tooling. However that would not match the OpenShift 4 pattern of exposing
management capabilities only as k8s-native APIs.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
