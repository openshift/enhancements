---
title: add-bare-metal-host-to-vsphere
authors:
  - "rvanderp3"
reviewers: 
  - "gnufied"
  - "JoelSpeed"
approvers: 
  - "JoelSpeed"
api-approvers: 
  - None
creation-date: 2025-02-10
last-updated: 2025-02-10
tracking-link: 
  - https://issues.redhat.com/browse/OCPSTRAT-1917
see-also:
  - None
replaces:
  - None
superseded-by:
  - None
---

# Bare Metal Hosts on vSphere Integrated Cluster

## Summary

As customers begin to use alternate virtualization or deployment patterns in their cluster they may need the ability to provision bare metal hosts along side their cloud integrated nodes. This is needed if a customer is unable to readily migrate their workloads to another OpenShift cluster or if they have a specific need to schedule workloads to both integrated and non-integrated nodes.

## Motivation

### User Stories

As a maintainer of an OpenShift cluster I need the ability to add bare metal hosts to my integrated OpenShift installation so I can migrate workloads to bare metal hosts without having to reinstall the cluster.

### Goals

- Enable a migration path for existing clusters to leverage nodes without cloud provider integration.
- Cluster operators will not be `degraded` or stuck in `progressing`.

### Non-Goals

- Migrating the control plane and infrastructure nodes to `platform: none`. Atleast initially, the goal is to add nodes which lack cloud provider integration to an existing `platform: vsphere` cluster.
- Autoscaling or machine/cluster API management of nodes
- Bare metal API integration

## Proposal

Nodes added to a platform vSphere cluster are expected to be initialized by the cloud controller manager. Otherwise, the nodes will remain tainted and will not join the cluster. To ensure the bare metal nodes are initialized, they are ignited with `platform: metal`. Once the bare metal nodes join the cluster, the CCM recognizes the node is there but not a part of vCenter. While this results in warnings being logged by the CCM, no events are generated and the cloud controller manager operator remains available. 

The CSI operator will attempt to schedule daemonset pods on all nodes and [tolerates most taints](https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/4479e2418f38cb93b5da4df7e043aff71a20cccc/manifests/vanilla/vsphere-csi-driver.yaml#L565-L569). This results in the cluster storage operator progressing indefinitely since the CSI driver fails to schedule on the bare metal compute node. The CSI operator will need to be modified to allow the tolerations to be adjusted to allow a bare metal worker to be excluded given it carries the required taints.

### Workflow Description

#### Adding a New Node Without Cloud Provider Integration to a Platform vSphere cluster

1. Install a `platform: vsphere` cluster
2. Download the RHCOS Live CD which aligns with the installed version of OpenShift.
3. Obtain or create a worker.ign file. This will be used to bootstrap the bare metal node.
4. Boot the new bare metal host from the RHCOS Live CD.
5. Install RHCOS:
```bash=
coreos-installer install /dev/sdX --insecure-ignition --ignition-url=https://path-to-compute-ignition --platform=metal
```
6. Reboot the node.
7. Approve CSRs for the node
8. Apply a taint to the node to block workloads from being scheduled:
```yaml=
spec:
  taints:
    - key: bare-metal/no-vmware
      value: 'true'
      effect: NoExecute
```
Note: The [vSphere CSI driver daemonset](https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/4479e2418f38cb93b5da4df7e043aff71a20cccc/manifests/vanilla/vsphere-csi-driver.yaml#L565-L569) tolerates all taints. I was able to disable it by making the operator unmanaged and removing the tolerations.

### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

Does not have an impact.

#### Standalone Clusters

#### Single-node Deployments or MicroShift

Does not have an impact.

### Implementation Details/Notes/Constraints

**!!! TODO !!!**

### Risks and Mitigations

**!!! TODO !!!**

### Drawbacks

**!!! TODO !!!**

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

**!!! TODO !!!**

## Test Plan

**Note:** *Section not required until targeted at a release.*

**!!! TODO !!!**

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

**!!! TODO !!!**

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

**!!! TODO !!!**

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

**!!! TODO !!!**

## Support Procedures

**!!! TODO !!!**

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
