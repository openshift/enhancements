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
last-updated: 2020-12-22
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
2. Multi-cluster management, such as through hive and ACM, should be
able to utilize the capabilities of the Assisted Installer.

This enhancement proposes the first iteration of running the Assisted Installer
in end-user clusters to meet the purposes above.

## Motivation

### Goals

* Expose the Assisted Installer's capabilities as a kubernetes-native API.
* Enable any OpenShift cluster deployed by the Assisted Installer to add worker nodes on day 2.
* Enable multi-cluster management tooling to create new clusters using Assisted Installer.
* Utilize [metal3](https://metal3.io/) to boot the assisted discovery ISO for bare-metal deployments.
* Ensure the design is be extensible for installing on other platforms.
* Utilize the Assisted Service and Agent with minimal changes to avoid
adversely affecting the cloud.redhat.com deployment.  While making a version
that is Kubernetes-native with no SQL DB is on the table for the long term,
it is not feasible to implement and maintain two versions in the near-term.

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
the specs, and the controller will upon statup first ensure that the DB
reflects the desired state in CRs. The source of truth for the status, however,
is the actual state of the agents running on the hosts being installed and not
necessarily what was previously recorded.

Agents that are running on hardware and waiting for installation-related
instructions will continue to communicate with the assisted service using the
existing REST API. In the future that may transition to using a CRD.

**AgentDrivenInstallationImage**
This resource represents a discovery image that should be used for booting
hosts. In the REST API this resource is embedded in the "cluster" resource, but
for kubernetes-native is more natural for it to be separate.

Spec:
 * clusterName: string
 * sshPublicKey: string
 * pullSecretName: string (reference to k8s secret)
 * ignitionOverrides: object
 * staticIpConfiguration: object

Status:
 * state: string
 * sizeBytes: integer
 * downloadUrl: string
 * createdAt: string
 * expiresAt: string

**AgentDrivenClusterInstallation**
This resource correlates to the "cluster" resource in the assisted installer's
current API.

Spec:
 * name: string
 * approved: boolean (used for starting installation)
 * openshiftVersion: string
 * baseDnsDomain: string
 * clusterNetworkCidr: string
 * clusterNetworkHostPrefix: integer
 * serviceNetworkCidr: string
 * apiVip: string
 * apiVipDnsName: string
 * ingressVip: string
 * machineNetworkCidr: string
 * sshPublicKey: string
 * vipDhcpAllocation: boolean
 * httpProxy: string
 * httpsProxy: string
 * noProxy: string
 * userManagedNetworking: boolean
 * additionalNtpSource: string
 * installConfigOverrides: string

Status:
* state: string enum
* stateInfo: string
* host_networks: array of (cidr, host_ids)
* install_started_at: string
* hosts: int
* progress:
  * progress_info: string
  * progress_updated_at: string
* validation_info:
  * configuration:
    * is_pull_secret_set: [success, failure, pending, error]
  * host_data:
    * all_hosts_are_ready_to_install: [success, failure, pending, error]
    * sufficient_masters_count: [success, failure, pending, error]
  * network:
    * machine_cidr_defined: [success, failure, pending, error]
    * cluster_cidr_defined: [success, failure, pending, error]
    * service_cidr_defined: [success, failure, pending, error]
    * no_cidrs_overlapping: [success, failure, pending, error]
    * network_prefix_valid: [success, failure, pending, error]
    * machine_cidr_equals_to_calculated_cidr: [success, failure, pending, error]
    * api_vip_defined: [success, failure, pending, error]
    * api_vip_valid: [success, failure, pending, error]
    * ingress_vip_defined: [success, failure, pending, error]
    * ingress_vip_valid: [success, failure, pending, error]
    * dns_domain_defined: [success, failure, pending, error]
    * ntp_server_configured: [success, failure, pending, error]
* connectivity_majority_groups: string
* updated_at: string
* created_at: string
* install_started_at: string
* install_completed_at: string
* controller_logs_collected_at: string

**AgentDrivenHostInstallation**
AgentDrivenHostInstallation represents a host that is destined to become part
of an OpenShift cluster, and is running an agent that is able to run inspection
and installation tasks. It correlates to the "host" resource in the Assisted
Installer's REST API.

Spec:
 * clusterName: string
 * role: [auto-assign, master, worker]
 * hostname: string
 * machineConfigPool: string
 * disks:
   * diskId: string
   * role: [none, install]
 * enabled: boolean
 * ignitionConfigOverrides: string
 * installerArgs: string

Status:
 * state: string enum
 * stateInfo: string
 * stateUpdatedAt: string
 * logsCollectedAt:string
 * installerVersion: string
 * createdAt: string
 * updatedAt: string
 * checkedInAt: string
 * hostname: string
 * bootstrap: boolean
 * discoveryAgentVersion: string
 * inventory:
   * timestamp: string
   * hostname: string
   * bmcAddress: string
   * bmcV6Address: string
   * memory:
     * physicalBytes: integer
     * usableBytes: integer
   * cpu:
     * count: integer
     * frequency: number
     * flags: array of strings
     * modelName: string
     * architecture: string
   * interfaces: array
     * ipv6Addresses: array of strings
     * vendor: string
     * name: string
     * hasCarrier: bool
     * product: string
     * mtu: integer
     * ipv4Addresses: array of strings
     * biosDevname: string
     * clientId: string
     * macAddress: string
     * flags: array of strings
     * speedMbps: integer
   * disks: array
     * driveType: string
     * vendor: string
     * name: string
     * path: string
     * hctl: string
     * byPath: string
     * model: string
     * wwn: string
     * serial: string
     * sizeBytes: integer
     * bootable: boolean
     * smart: string
   * boot:
     * current_boot_mode:string
     * pxeInterface:string
   * system_vendor:
     * serialNumber:string
     * productName:string
     * manufacturer: string
     * virtual: boolean
 * validation_info:
   * hardware:
     * hasInventory: [success, failure, pending, error]
     * hasMinCpuCores: [success, failure, pending, error]
     * hasMinMemory: [success, failure, pending, error]
     * hasMinValidDisks: [success, failure, pending, error]
     * hasCpuCoresForRole: [success, failure, pending, error]
     * hasMemoryForRole: [success, failure, pending, error]
     * isHostnameValid: [success, failure, pending, error]
     * isHostnameUnique: [success, failure, pending, error]
     * isPlatformValid: [success, failure, pending, error]
   * network:
     * isConnected: [success, failure, pending, error]
     * isMachineCidrDefined: [success, failure, pending, error]
     * belongsToMachineCidr: [success, failure, pending, error]
     * isApiVipConnected: [success, failure, pending, error]
     * belongsToMajorityGroup: [success, failure, pending, error]
     * isNtpSynced: [success, failure, pending, error]
  * progress:
    * currentStage: enum
    * progressInfo: string
    * stageStartedAt: string
    * stageUpdatedAt: string
 * connectivity: array of
   * host
   * l2Connectivity
     * outgoingNic: string
     * outgoingIpAddress: string
     * remoteIpAddress: string
     * remoteMac: string
     * successful: boolean
   * l3Connectivity
     * outgoingNic: string
     * remoteIpAddress: string
     * successful: boolean
 * apiVipConnectivity: boolean
 * ntpSources: array of
   * sourceName: string
   * sourceState: [synced, combined, not_combined, error, variable, unreachable]


#### REST API Access
Some REST APIs need to be exposed in addition to the Kubernetes-native APIs
described below.
* Download ISO and PXE artifacts: These files must be available for download
via HTTP, either directly by users or by a BMC.  Because BMCs do not pass
authentication headers, the Assisted Service must generate some random URL or
query parameter so that the ISO’s location isn’t easily guessable.
* Agent APIs (near-term): Until a point where the agent creates and modifies
AgentDrivenHostInstallation CRs itself, the agent will continue communicating
with the service via REST APIs.  Currently the service embeds the user’s pull
secret in the discovery ISO which the agent passes in an authentication
header.  In this case the service can generate and embed some token which it
can later validate.


#### Hive Integration

Hive has a [ClusterDeployment
CRD](https://github.com/openshift/hive/blob/master/docs/using-hive.md#clusterdeployment)
resource that represents a cluster to be created. It includes a "platform"
section where platform-specific details can be captured. For an agent-based
workflow, this section will include whatever information is necessary to get an
ISO and boot hosts.

A new field in the ClusterDeployment spec will optionally reference an
AgentDrivenClusterInstallation resource, which describes everything that the
agent-based install tooling needs to know about the cluster that it will be
creating. Some or all of that new resource may be folded into ClusterDeployment
in the future, but in the immediate term, keeping it separate will enable much
quicker development.

Hive or ACM will optionally orchestrate the booting of bare metal hosts by:

1. Get an ISO URL from assisted operator.
1. Use metal3's BareMetalHost to boot the live ISO on some hardware.

#### BareMetalHost can boot live ISOs

A [separate enhancement to
metal3](https://github.com/metal3-io/metal3-docs/pull/150) proposes a new
capability in the BareMetalHost API enabling it to boot live ISOs. That feature
is required so that automation in a cluster can boot the discovery ISO on known
hardware as the first step toward provisioning that hardware.

#### CAPBM 

[Cluster-API Provider Bare Metal](https://github.com/openshift/cluster-api-provider-baremetal/) will need to gain the ability to interact with the new assisted installer APIs. Specifically it will need to match a Machine with an available assisted Agent that is ready to be provisioned. This will be in addition to matching a Machine with a BareMetalHost, which it already does today.

#### Day 2 Add Node Boot-it-Yourself

This scenario takes place within a stand-alone bare metal OpenShift cluster.

1. The user downloads a discovery ISO from the cluster. The download is implemented by the assisted installer as a URL on a AssistedInstallCluster
resource.
1. A host boots the live ISO, the assisted agent starts running, and the agent contacts the assisted service to register its existence. Communication utilizes the existing non-k8s REST API. The agent walks through the validation and inspection workflow as it exists today.
1. The wrapping controller creates a new AssistedInstallAgent resource to be the k8s-native API for the agent.
1. CAPBM creates a new BareMetalHost resource, setting the status annotation based on inspection information from the prior step.
1. The user approves the host for installation. (TODO: how?)
1. The user or an orchestrator scales up a MachineSet, causing a new Machine resource to be created.
1. CAPBM binds the Machine to the BareMetalHost, as it does today. It additionally finds the CR representing the agent and uses it to begin installation.
1. The assisted service initiates installation of the host.
1. CAPBM updates the status on the BareMetalHost to reflect that it has been provisioned.

#### Day 2 Add Node Virtualmedia Stand-alone

This scenario takes place within a stand-alone bare metal OpenShift cluster.

1. The user creates a BareMetalHost that includes BMC credentials and a label indicating it should be used with assisted installer.
1. CAPBM gets a URL to the live ISO and adds it to the BareMetalHost, causing it to boot the host.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO.
1. The assisted agent starts running on the new hardware and runs through its usual validation and inspection workflow. The wrapping controller creates a new agent resource to be the k8s-native API for the agent.
1. The user or an orchestrator scales up a MachineSet, resulting in a new Machine being created.
1. CAPBM does its usual workflow of matching the Machine to an available BareMetalHost. Additionally it uses the agent CR to initiate provisioning of the host.
1. The assisted service provisions the host.

#### Day 2 Add Node Virtualmedia Multicluster

This scenario takes place from a hub cluster, adding a worker node to a spoke cluster.

1. User creates or modifies a BareMetalAsset on the hub cluster so that its cluster field references the desired ClusterDeployment.
1. Hive creates a BareMetalHost resource corresponding to the BareMetalAsset and uses it to boot the live ISO.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO.
1. Agent running on the host contacts assisted service, resulting in an AssistedInstallAgent resource being created.
1. Assisted Installer walks the agent through discovery and validation workflows.
1. When the agent is in a ready state, hive or ACM initiates provisioning, referencing the target cluster's ignition.

#### Create Cluster

This scenario takes place on a hub cluster where hive and possible ACM are
present. This scenario does not include centralized machine management.

1. User creates BareMetalAssets containing BMC credentials.
1. User creates AssistedInstallCluster resource describing their desired cluster.
1. User creates ClusterDeployment describing a new cluster. The Platform section indicates that metal3 should be used to boot hosts with a live ISO. A new field references the AssistedInstallCluster resource.
1. User associates some BareMetalAssets with the ClusterDeployment.
1. Hive creates BareMetalHost resources corresponding to each BareMetalAsset, and uses them to boot the live ISO.
1. baremetal-operator uses redfish virtualmedia to boot the live ISO.
1. Agent running on each host contacts assisted service, resulting in an AssistedInstallAgent resource being created.
1. Assisted Installer walks each agent through discovery and validation workflows.
1. When each agent is in a ready state, hive or ACM initiates cluster creation with the AssistedInstallCluster.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Open Questions

#### Can we use sqlite?

It would be advantageous to use sqlite for the database. The [gorm library
supports sqlite](https://gorm.io/docs/connecting_to_the_database.html#SQLite),
but it is not clear if assisted service is compatible. In particular, the [use
of FOR
UPDATE](https://github.com/openshift/assisted-service/blob/e70af7dcf59763ee6c697fb409887f00ab5540f5/pkg/transaction/transaction.go#L8)
might be problematic.

#### baremetal-operator watching multiple namespaces?

When utilizing baremetal-operator on a hub cluster to boot the discovery ISO on
hosts, should we be creating those BareMetalHost resources in a separate
namespace from those that are associated with the hub cluster itself?

What work is involved in having BMO watch additional namespaces?

#### BareMetalAsset: does it have value in these scenarios?

BareMetalAsset is a CRD that is part of ACM. It represents an invetory of
hardware that ACM can use to provision with IPI and add BareMetalHosts to spoke
clusters. ACM does not use BareMetalAsset to interact with or manage hardware;
it only uses it to store hardware information that gets either passed to
`openshift-install` or added directly to a spoke cluster in the form of a
BareMetalHost.

Scenario A: user maintains an inventory of hosts as BareMetalAssets, all in one
namespace, or grouped by organization. At install time, a BareMetalHost is
created (in the namespace appropriate to the cluster) for each selected
BareMetalAsset, and then the "Create Cluster" scenario takes place.
BareMetalHost resources get garbage collected once provisioning is complete.

Scenario B: user maintains an inventory of hosts as BareMetalHosts. They are
directly used to boot live ISOs as part of agent-based provisioning. When a
user selects BMHs for cluster creation, they may be moved into a new
cluter-specific namespace by deleting and re-creating them.

#### Host Approval

In the "Day 2 Add Node Boot-it-Yourself", how should the user approve a host
for installation?

#### Centralized Machine Management

OpenShift's multi-cluster management is moving toward a centralized Machine
management pattern, as an addition to the current approach where Machines only
exist in the same cluster as their associated Node. This proposal should be
compatible with centralized Machine management, but it would be useful to play
through those workflows in detail to be certain.

#### Garbage Collection

Agent resources are not useful after provisioning except possibly as a
historical record. 

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
