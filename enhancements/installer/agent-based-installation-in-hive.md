---
title: agent-based-installation-in-hive
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
  - "@markmc"
creation-date: 2020-12-22
last-updated: 2021-03-01
status: implementable
see-also:
  - enhancements/installer/connected-assisted-installer.md
  - enhancements/installer/assisted-installer-bare-metal-validations.md
---

# Agent-based installation workflow in Hive

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
as through [Hive](https://github.com/openshift/hive/) and RHACM (Red Hat
Advanced Cluster Management).

This enhancement proposes to add the agent-based installation workflow to Hive.

## Motivation

The Assisted Installer running as a SaaS has demonstrated a new and valuable
way to install OpenShift clusters through an agent-based flow. The [Assisted
Installer for Connected Environments](connected-assisted-installer.md)
enhancement proposal elaborates on the value of the agent-based installations.

However many customers do not want a third party service running outside of
their network to be able to provision hosts inside their network. Reasons
include trust, control, and reproducibility. Those users will prefer to run the
service on-premises. There are also many customers whose network environments
have limited or no connectivity to the internet.

[Hive](https://github.com/openshift/hive/) is the kubernetes-native API for
installing OpenShift, and it is the natural place to introduce an on-premises
capability for agent-based cluster installation.

### Goals

* Expose agent-based installation capabilities as a kubernetes-native API
through [Hive](https://github.com/openshift/hive).
* Enable using agent-based installation capabilities to add workers to clusters
that have been installed using - or adopted by - Hive.
* Enable automated creation and booting of the Assisted discovery ISO for bare-metal deployments.
* Ensure the design is extensible for installing on other platforms.

### Non-Goals

* Remote Worker Node support requires solving specific problems that will be
addressed in a separate proposal. For example, it requires the ability to
ensure that the Node joins with an appropriate role such that it will not be
schedulable for specific workloads, such as the ingress controller.
* Solve central machine management. ("central machine management" involves
running machine-api-providers on a hub cluster in order to manage Machines on
the hub which represent Nodes in spoke clusters.) While this is a foundational
element to solving central machine management, this enhancement doesn't attempt
to address the topic in its totality.
* Run metal3 components on a non-baremetal cluster. Some parts of this proposal
assume that the baremetal-operator is present on the hub cluster. Today that is
only possible when the hub cluster itself is on the bare metal platform. Future
work may propose running baremetal-operator on non-baremetal hub clusters.
* Applying static network configuration to hosts. This is a desired feature
but will be addressed in a future proposal.

## Proposal

### User Stories

Personas for multi-cluster management:

**Infra Owner** Manages physical infrastructure and configures hosts to boot
the discovery ISO that runs the Agent.

**Cluster Creator** Uses running Agents to create and grow clusters.

#### Run Agent via Boot It Yourself

Whether you are creating a new cluster or adding nodes to an existing cluster,
the workflow starts by booting a discovery ISO on a host so that it runs the
Agent. Many users have their own methods for booting ISOs on hardware. The
Assisted Service must be able to create a discovery ISO that users can take and
boot on hardware with their own methods.

*As an Infra Owner, I can make Agents available for provisioning by creating a
discovery ISO and using my own tooling to boot it on hosts.*

#### Run Agent via Automation

Many users will want an integrated end-to-end experience where they describe
hardware and expect it to automatically boot an appropriate discovery ISO
without needing to provide their own mechanism for booting ISOs.

*As an Infra Owner, I can maintain an inventory of available hardware and use
provided automation to run the Agent on that hardware.*

#### Install cluster from Hub

*As a Cluster Creator, I can select a group of Agents and use them to create a
new OpenShift cluster.*

#### Add Worker Node from Hub

*As a Cluster Creator, I can add workers by associating Agents either with a
cluster installed by Hive via an agent-based workflow, or an existing cluster
adopted by Hive.*

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

#### Kubernetes-native APIs for Agent Based Cluster Provisioning

The Assisted Installer has a [REST
API](https://generator.swagger.io/?url=https://raw.githubusercontent.com/openshift/assisted-service/master/swagger.yaml)
that is available at cloud.redhat.com. The service is implemented with a
traditional relational database. In order to integrate with OpenShift
infrastructure management tooling, it is necessary to additionally expose its
capabilities as Kubernetes-native APIs. They will be implemented as Custom
Resource Definitions and controllers with the Assisted Service's capabilities.

Hive's ClusterDeployment resource will be extended to serve as the cluster
definition for agent-based installations. Additional CRDs described below will
be used by other controllers and actors to perform agent-based provisioning.

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

The details of this resource definition were discussed [in a
pull request](https://github.com/openshift/assisted-service/pull/969).

Summary of fields:

Spec
* egress proxy settings
* a list of NTP sources (hostname or IP) to be added to all cluster
hosts. They are added to any NTP sources that were configured through other
means.
* a list of SSH public keys that will be added to all agents for use in debugging.
* pull secret reference
* a label selector for Agents. This is how Agent resources are identified as
belonging to a particular InstallEnv.
* labels to add to Agent resources upon creation. These labels should match
the label selector above.
* Array of ClusterDeployment references. This is a whitelist of ClusterDeployments
that are authorized to consume Agents from this InstallEnv. See the section below
on [Namespaces and Multi-tenancy](#namespaces-and-multi-tenancy) for details.

Status
* a URL to download the discovery ISO produced for this InstallEnv.

**Agent**
Agent is a new resource, part of the assisted installer's new operator, that
represents a host that is destined to become part of an OpenShift cluster, and
is running an agent that is able to run inspection and installation tasks. It
correlates to the "host" resource in the Assisted Installer's REST API.

The details of this API are available [as
code](https://github.com/openshift/assisted-service/pull/861), with some
details yet to be changed.

Summary of fields:

Spec
* Role that the Node will have
* Hostname
* Approved
* Install device, set automatically when there is platform-integrated
automation booting the discovery ISO, or otherwise set by the user in a
boot-it-yourself scenario.

Status
* Comprehensive inspection and validation data

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

The details of this were discussed on a Hive
[pull request](https://github.com/openshift/hive/pull/1247).

#### REST API Access

Some REST APIs need to be exposed in addition to the Kubernetes-native APIs
described below.
* Download discovery ISO and rootfs: These files must be available for download
via HTTP, either directly by users or by a BMC.  Because BMCs do not pass
authentication headers, the Assisted Service must generate some URL or query
parameter so that the ISO’s location isn’t easily guessable.
* Agent APIs (near-term): Until a point where the agent creates and modifies
Agent CRs itself, the agent will continue communicating with the service via
REST APIs. Currently the service embeds the user’s pull secret in the discovery
ISO which the agent passes in an authentication header.  In this case the
service can generate and embed some token which it can later validate.

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

#### Namespaces and Multi-tenancy

While we expect a ClusterDeployment to have its own namespace (which will be
increasingly valuable once we have central machine management and need to run
different machine-api components per cluster), Agents will be created in a
separate namespace where their InstallEnv exists. "Late binding" in particular
is a scenario where Agents will exist prior to a cluster. And of course a
collection of Agents may be created in advance of allocating them to various
clusters.

Thus when a Cluster Creator creates a namespace and a new ClusterDeployment,
that ClusterDeployment will need to consume Agents from a different namespace.
But the Infra Owner may not want them to have the ability to consume any Agent
from any namespace.

To designate that a ClusterDeployment is authorized to consume Agents from a
particular InstallEnv, a reference to that ClusterDeployment must be present
in an array on the InstallEnv. The reference proves that someone with write
access to the InstallEnv approves of associating it with the cluster.

The restriction will be enforced by the agent-related controllers at the time
of matching Agents to a particular ClusterDeployment. The controllers will
still have access to all namespaces.

InstallEnv and Agent resources are both namespaced. Agents will be created in
the same namespace as the InstallEnv from which they were created.

In the 4.8 timeframe, because "late binding" does not exist and thus each
InstallEnv must be associated to a single cluster, this array of references
will be required to have exactly one member. Once late binding is supported,
that restriction will be removed. ("late binding" refers to the ability to
create a discovery ISO and boot Agents with it prior to associating them with a
cluster.)

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
1. The Agent starts up on each host and reports back to the assisted service, which creates an Agent resource in the same namespace as the InstallEnv. The Agent is automatically labeled with the labels that were specified in the InstallEnv's Spec.
1. The Baremetal Agent Controller sets the Agent's Role field in its spec to "master" or "worker" if a corresponding label was present on its BareMetalHost.
1. The Agent is automatically marked as Approved via a field in its Spec based on being recognized as running on the known BareMetalHost.
1. The Agent runs through the validation and inspection phases. The results are shown on the Agent's Status, and eventually a condition marks the Agent as "ready".
1. Cluster Creator creates a ClusterDeployment describing a new cluster. It describes how many control-plane and worker agents to expect. It also includes a label selector to match Agent resources that should be part of the cluster.
1. Cluster Creator or Infra Owner adds a reference to the ClusterDeployment onto the InstallEnv. This reference confirms that the ClusterDeployment, which resides in a different namespace, is authorized to consume Agents from this InstallEnv.
1. Cluster Creator applies a label to Agents if necessary so that they match the ClusterDeployment's selector.
1. Once there are enough ready Agents of each role to fulfill the expected number as expressed on the ClusterDeployment, installation begins.

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

This scenario takes place from a hub cluster, adding a worker node to a spoke
cluster. This assumes that a ClusterDeployment exists in the hub cluster and
has the baremetal-operator component installed (which currently requires
setting the infrastructure platform to "baremetal").

1. Infra Owner creates a BareMetalHost resource with a label that matches an InstallEnv selector.
1. The Baremetal Agent Controller adds the discovery ISO URL to the BareMetalHost.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO on the BareMetalHost.
1. The Agent starts up and reports back to the assisted service, which creates an Agent resource in the hub cluster. The Agent is labeled with the labels that were specified in the InstallEnv's Spec, optionally including a label to match the AgentSelector field on the ClusterDeployment.
1. If the Agent does not get a default label matching a ClusterDeployment, a user or other automation must add a label to match the AgentSelector field on the appropriate ClusterDeployment.
1. Cluster Creator or Infra Owner adds a reference to the ClusterDeployment onto the InstallEnv if it is not already present. This reference confirms that the ClusterDeployment, which resides in a different namespace, is authorized to consume Agents from this InstallEnv.
1. The Agent's Role field in its spec is assigned a value if a corresponding label and value were present on its BareMetalHost. (only "worker" is supported for now on day 2)
1. The Agent is marked as Approved via a field in its Spec based on being recognized as running on the known BareMetalHost.
1. The Agent runs through the validation and inspection phases. The results are shown on the Agent's Status, and eventually a condition marks the Agent as "ready".
1. The Baremetal Agent Controller adds inspection data found on the Agent's Status to the BareMetalHost.
1. When the agent is in a ready state, installation of that host begins.

#### Future Use of MachinePools and Machine API

This section discusses potential future use of hive's MachinePool and the
Machine API as a preview of how the proposed agent-based installation can later
fit a Machine-driven provisioning workflow.

Hive has a MachinePool resource type that exists on a hub cluster and offers a
way to manage MachineSets on spoke clusters. This proposal does not utilize
any part of the Machine API because it does not (yet) add agent-based
provisioning support to Machine API providers.

A separate effort is underway to implement Centralized Machine Management,
where Machine API providers and related resources would all be implemented on
the hub cluster instead of the spoke cluster. Rather than use a MachinePool on
the hub as a proxy for a remote MachineSet resource, the entire Machine API
affecting a spoke cluster would be available for use on the Hub. That will be
the starting point for integrating agent-based installation with Machines.

Typically with the Machine API, a MachineSet gets scaled up, one or more
Machines get created, and then a controller (part of a particular platform's
machine-api-provider) interacts with a provider API (such as a public cloud
API) to create or obtain a host per Machine that can be provisioned as Nodes in
the cluster.

Integrating agent-based provisioning into that flow will involve matching each
Machine resource with an Agent resource in a similar way to how the bare metal
provider currently matches a Machine with an availbale BareMetalHost. On a bare
metal platform, Agents will likely continue to be created in advance,
independently of a Machine. It is possible that other platforms may create
Agents on-demand when a Machine appears.

#### Hive Cluster Adoption

Hive is able to adopt existing clusters that it did not create. This is done by
creating a ClusterDeployment where `spec.installed == true` and providing a
kubeconfig.

In order to adopt a cluster and then add workers using an agent-based approach,
the user must additionally:
* specify the agent label selector on the ClusterDeployment's Spec
* add a reference to the ClusterDeployment on one or more InstallEnvs

Otherwise there are no differences in the InstallEnv or the workflow as
described above.

### Risks and Mitigations

#### API Versions and Potential Change

Agent-based installation is a significant feature set with significant API
surface area. There is a strong potential for the need to change these new APIs
after receiving feedback from users. However there is a competing desire to
support these APIs when they are delivered in the OpenShift 4.8 timeframe,
which rules out the use of an "alpha" API version due to its insufficient level
of commitment.

New CRDs will have version "v1beta1" during the OpenShift 4.8 timeline (though
they will be distributed as part of RHACM). This will enable them to be
supported in production while allowing greater flexibility for changes than if
they were released as "v1".

The agent-specific portions of the hive ClusterDeployment will inherit the
ClusterDeployment's current version of "v1".

#### Agent REST API Auth

Agents will communicate with the assisted service using the non-kubernetes REST
API that is currently in use by the SaaS. The backend assisted service will be
listening on a socket that is routable from anywhere Agents are expected to
run, so it needs to be secured from other potential actors on the network.

When an Agent connects to report its existence, send logs, send inspection
data, etc. it will need to authenticate.

An authentication artifact will be embedded into the discovery ISO for use by
the agent. As long as the user protects the contents of the discovery ISO
itself, they can be confident that any client interacting with the backend
service's REST API is the expected Agent running as part of the live discovery
ISO. Details of the artifact are still being decided.

#### Other REST API Access

Where the assisted service has capabilities which are not (yet) expressed
through kubernetes APIs, users may be tempted to access those capabilities
directly via the assisted servuce REST API, thereby taking a dependency on the
capability and its REST API. This would limit our ability to evolve those APIs
in future.

To mitigate this, the assisted service's REST API, except for the Agent API as
described above, will limited to internal use only. It may be limited through
authorization mechanisms or by blocking entire URL path space from external
access.

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

There is some discussion in the
[Future Use of MachinePools and Machine API](#future-use-of-machinepools-and-machine-api)
section.

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

New APIs will start at alpha versions with all of the functionality described
in this proposal. Releases will be made available to key stakeholders for
evaluation and testing in order to gather feedback. APIs may change during this
time, but disruption will be minimized so as to encourage and facilitate
testing.

Once sufficient feedback has been collected from users (including lessons
learned from ongoing formal testing), and a set of changes have been agreed
upon, beta versions of APIs will be implemented and released alongside a future
OpenShift version.

### Upgrade / Downgrade Strategy

The assisted service, its new controllers, and its CRDs will be distributed
with Hive as part of RHACM's operator installed by Operator Lifecycle Manager.
They will continue to follow the upgrade approach that Hive already follows.

If the backend service is unavailable for a short time during an upgrade, any
agents trying to connect to the REST API will continuously re-try.

### Version Skew Strategy

Distributing the new components together with Hive, as described above, will
prevent skew from being a problem between these components.

The SaaS already is maintaining backward-compatibility for the REST API to
which agents make requests, so that when the service gets upgraded,
previously-generated discovery ISOs can continue to work. That backward
compatibility will likewise be important in this scenario when Hive and the
agent-based components get upgraded.

For situations where backward compatibility cannot be maintained, the agent
REST API is able to identify the version of an agent and instruct it to upgrade
or downgrade itself by running a different container image. That feature will
likewise be valuable when running the backend service alongside Hive.

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
