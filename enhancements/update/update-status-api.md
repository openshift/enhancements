---
title: update-status-api
authors:
  - "@petr-muller"
reviewers:
  - "@wking"     # OTA
  - "@joelspeed" # API approver
  - "@jupierce"  # Original design author
approvers:
  - TBD
api-approvers:
  - "@joelspeed"
creation-date: 2024-10-16
last-updated: 2024-10-16
tracking-link: 
  - https://issues.redhat.com/browse/OTA-1256
  - https://issues.redhat.com/browse/OTA-1339
see-also:
  # Update Health API Controller Proposal
  - https://docs.google.com/document/d/1SuV8mvEQbUMpcV1VTazuPlXgbMeUm2134SIV5so4a5M/edit
  # Update Health API Draft
  - https://docs.google.com/document/d/1aEIWkfhhaVSe-XlSXvmokymOe3X_969pRCJhfhPDwFQ/edit
  # Client-based prototype in 4.16: Easier OpenShift Update Troubleshooting with oc adm upgrade status
  - https://source.redhat.com/groups/public/openshift/openshift_blog/easier_openshift_update_troubleshooting_with_oc_adm_upgrade_status
---

# Update Status API and Controller

## Summary

This enhancement proposes a new API that exposes the status and health of the
OpenShift cluster update process. The API will be implemented as a new CRD that
will be managed by a new controller owned by the OpenShift OTA team. The
enhancement optimizes to allow early delivery of the API itself backed by
a simple controller to allow gathering feedback by potential API clients
(internal and external) before investing effort into building consensus on
the full, robust and modular implementation.

## Motivation

The OpenShift update process is complex and often challenging to troubleshoot,
involving multiple components, their hierarchies and differing interfaces.
Administrators need extensive knowledge of resources, conditions, and processes
to effectively manage updates across different cluster types and form factors.
This complexity creates a high barrier for troubleshooting and support.

There is an ask for a centralized API to evaluate update status and health
across different OpenShift tools and interfaces. This API would standardize the
interpretation of update processes, accommodate various components involved in
updates, and potentially integrate with related concepts like pre-checks and
maintenance windows. Such an interface would improve the user experience for
administrators and support.

### User Stories

* As a cluster administrator, I want to easily observe the update process so
  that I know if the update is going well or if there are issues to address
* As a cluster administrator, I want to observe the update process in a single
  place so that I do not have to dig through different command outputs to
  understand what is going on
* As a cluster administrator, I want to be clearly told if there are issues I
  need to address during the update so that I can do so.
* As a cluster administrator, I only want to be informed about issues relevant
  for the update and not be overwhelmed with things that may be a problem but
  are often not, so that I do not waste effort investigating items that are
  reported
* As an engineer building tooling and UXes that interact with OpenShift clusters
  I want the process and health information available though an API so that I
  build software to consume and expose this information
* As an engineer building tooling and UXes that interact with OpenShift clusters
  I want the information available through a designated API, so that there is
  a standardized interpretation of the update process state provided by the
  platform and I do not need to interpret individual component states

### Goals

* Provide a centralized API to expose cluster update status to consumers
* Back the API with a simple centralized controller to allow early feedback to
  drive eventual further development
* Provide features provided by the 4.16 client-based prototype through the API
  and a `oc` command consuming the API
* Design the API to be extensible enough to accommodate individual components
  contributing information to the API in the future

### Non-Goals

* Achieve consensus on how would the modular backend look like 
* Implement the modular backend where different components contribute to the
  API

## Proposal

Introduce a new `UpdateStatus` API (CRD) that exposes the status and health of
the OpenShift cluster upgrade process. In a typical OpenShift cluster, the API
will be a singleton with an empty `spec` and all information will be exposed
through its `status` subresource.

Introduce a new Update Status Controller (USC) component in OpenShift that will
directly inspect the cluster state during the update and populate the `UpdateStatus`
resource. The functionality of the controller will match the features provided
in the client-based prototype delivered in 4.16 and extended in 4.17 and 4.18:

- Monitor the control plane update process and report its progress, completion,
  duration, estimated finish and high-level healthiness of the process and
  cluster operators (cluster operators are treated as a control plane "unit")
- Monitor the node pool update process and report progress, completion and
  high-level health assessment of the process and individual nodes
- Monitor the cluster for potentially problematic states and surface them as
  "health insights" focused to be relevant and actionable

Implement a default `oc adm upgrade status` mode that will consume the API and
present the information in a human-targeted format, similar to the existing
client-based prototype implementation.

All functionality will be delivered under new `UpdateStatus` [Capability](https://docs.openshift.com/container-platform/4.17/installing/overview/cluster-capabilities.html)
so that clusters that do not want the functionality do not need to spend
resources on running the new controller. The capability will be a part of the
`vCurrent` and a `v4.X` capability sets, which means the functionality will be
enabled by default, and admins need to opt-out at installation or update time.
Like all capabilities, once enabled they cannot be disabled.

This proposal does not intend to capture the entire roadmap for a system reporting
update health and status. There is a vision that the system where external,
responsible components closer to their domain, produce update insights into the
`UpdateStatus` API.  This enhancement proposal only corresponds to the Stage 1
of the implementation roadmap outlined in the [Update Health API Controller Proposal](https://docs.google.com/document/d/1aEIWkfhhaVSe-XlSXvmokymOe3X_969pRCJhfhPDwFQ/edit#heading=h.9g05u56hri6y).
There is not a consensus on further stages of the implementation outlined in that
document and this enhancement does not aim to achieve it. Any hypothetical
modular system will be a much larger effort to implement and its design will
benefit from API consumed by users, backed by the proposed simple implementation
in the USC.

### Workflow Description

1. On a cluster where Status API is enabled, the CVO manages a new Update Status
   Controller (USC) deployment as its operand
1. USC monitors the cluster and maintains a `UpdateStatus` resource called
   `cluster`
1. While there is no update happening, respective conditions convey this:
   1. `.status.controlPlane.conditions` has `Updating=False` condition
   1. `.status.workerPools[*].conditions` each has `Updating=False` condition
1. When a user runs `oc adm upgrade status` the client reads the `UpdateStatus`
   and reports that no update is happening
1. User triggers the update
1. USC monitors `ClusterVersion`, `ClusterOperator`, `MachineConfigPool` and
   `Node` resources and reflects the state of the update through the `UpdateStatus`
   resource through a set of status and health insights
1. When user runs `oc adm upgrade status` the client reads the `UpdateStatus`
   and uses status insights to convey progress and health insights to convey
   issues the admin needs to address.

### API Extensions

Introduce a new `UpdateStatus` CRD. The CRD is namespaced to allow usage in 
Hosted Control Plane management clusters as a part of the control plane.

The `UpdateStatus` CRD primary focus is to convey status information. For now its
`spec` is empty, but can in the future be used to configure the desire the form
or content of the information surfaced in `status`.

The `status` subresource is the main value of the enhancement. The status and
health information about the update are expressed through the `status`.

Introduce a new `update.openshift.io` group for `UpdateStatus`. There does not
seem to be a suitable existing OpenShift API group suitable where `UpdateStatus`
would fit. The `ClusterVersion` CRD used to convey _some_ information about
update process is in the `config` group which is a poor fit for `UpdateStatus`
because it provides no configuration capabilities. This new group is well-suited
to contain some of the other update-related APIs needed for further incoming
features like maintenance windows or update pre-checks.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The enhancement allows adoption in HCP as an extension. The API and controller
will be extended to handle the resource that represent control plane and worker
pools (`HostedCluster` and `NodePool` respectively) and surface update progress
through status insights for these resources. 

The Update Status Controller will run as CVO operand as a part of the hosted
cluster control plane. `UpdateStatus` resource was made namespaced in order to
be utilized in HCP. It will reside in management cluster, which is natural
because the information it conveys targets primarily cluster administrators
tasked with updating clusters.

#### Standalone Clusters

Standalone clusters are the primary target of the functionality. Using namespaced
`UpdateStatus` CRD for a singleton resource is awkward, but it is a tradeoff to
make the resource usable in HCP. We do not expect users to directly interact
with `UpdateStatus` resource; the API is intended to be used mainly by tooling.
The `openshift-cluster-version` namespace is suitable to contain the resource.

#### Single-node Deployments or MicroShift

No special considerations

### Implementation Details/Notes/Constraints

Full API proposal: https://github.com/openshift/api/pull/2012

#### `UpdateStatus` API Overview

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
metadata:
  name: cluster
  namespace: openshift-cluster-version
spec: { }
status:
  controlPlane:
    ...control-plane-relevant fields managed by the controller...
    informers:
    - name: cvo-example-informer
      insights: <list of insights reported by the informer>
    - name: mco-example-informer
      insights:
      - type: ClusterVersion # CV status insight
        ...
      - type: ClusterOperator # CO status insight
        ...
      - type: UpdateHealth # General update health insight
        ...
    conditions: <list of standard kubernetes conditions>
  workerPools:
  - name: workers
    ...pool-relevant fields managed by the controller...
    informers: <list of informers with reported insights>
    conditions: <list of standard kubernetes conditions>
  - name: infra
    ...
  conditions: <list of standard kubernetes conditions>    
```

The API has three conceptual layers:

1. The innermost layer `.status.controlPlane.informers` and `.status.workerPools[].informers`:
   Through this layer, the API exposes detailed information about individual
   concerns related to the update, called “Update Insights”. The API is prepared
   to allow multiple, external informers to contribute insights, but in this
   enhancement the only informer is the USC itself.
1. The aggregation layer `.status.controlPlane` and `.status.workerPools[]` 
   reports higher-level information about the update through this layer, serving
   as the USC interpretation of all insights.
1. The outermost layer controlled by USC `.status.conditions` used to report
   operational matters related to the USC itself (health of the controller and
   gathering the insights, not health of the update process).

#### Update Insights

Update Insights are units of update status/health information. There are two
types of update insights: status insight and health insights.

##### Update Status Insights
 
Update Status Insights are tied directly to the process of the update, no matter
if it goes well or not. Status Insights expose the status of a single resource
that is directly involved in the process of the update, usually a resource that:

1. either has a notion of “being updated”, such as a `Node` or `ClusterOperator`
2. or represents a higher-level abstraction, such as a `ClusterVersion` resource
   (represents a control plane) or `MachineConfigPool` (represents a node pool).

Initially, the USC will create status insights for `ClusterVersion`, `ClusterOperator`,
`MachineConfigPool` and `Node` resources.

##### Update Health Insights

Update Health Insights are reporting a state/condition existing in the cluster
that is abnormal, negative, and either affects or is affected by the update.
Ideally, none would be generated in a standard healthy update. Health insights
should be communicating a condition worth an immediate attention
by the administrator of the cluster, and should be actionable. Health insights
should be accompanied by links to resources helpful in the given situation.
Health insights can carry references to multiple resources involved
in the situation (for example, the health insight exposing a failure to drain
a node, it would carry a reference to the `Node`, the `Pod` that fails to be
drained, and `PodDisruptionBudget` that protects it). Individual resources can
be involved in multiple insights.

#### Update Status Controller

The Update Status Controller (USC) is a new component in OpenShift that will be
deployed directly by the CVO, being treated as its operand. This is uncommon in
OpenShift, CVO typically deploys only second-level operators as its operands. In
that model, the USC (providing cluster functionality) would typically be an _operand,_
and we would need to invent a new cluster operator to manage it. Because CVO is
directly involved in updates, such additional layer does not seem to have value.

Update Status Controller will be running a primary controller that will maintain
the `UpdateStatus` resource. The resource has no `.spec` so the controller will
just ensure the resource exists and its `status` content is up-to-date and correct.
It will determine the `status` content by receiving and processing insights from
the other controllers in USC.

Update Status Controller will have two additional control loops, both serving as
producers of insights for the given domain. One will monitor the control plane
and will watch `ClusterVersion` and `ClusterOperator` resources. One will
monitor the node pools and will watch `MachineConfigPool` and `Node` resources.
Both will report their observations as status or health insights to the primary
controller so it can update the `UpdateStatus` resource. These control loops will
reuse existing cluster "analysis" code implemented in the client-side `oc adm upgrade status`
prototype.

### Risks and Mitigations

The proposal to deliver the API and a controller that both manages the API and
monitors the cluster (produces insights) before achieving consensus on the
architecture eventual modular system risks that existing API will not accommodate
the future architecture well. We are making a tradeoff to deliver the API early
to start providing value which also allows us to learn about how and if such API
is really consumed. Early delivery direclty addresses the risk of investing effort
into building a much larger system that may not address the real user needs. 

### Drawbacks

The pattern of CVO deploying a non-operator component directly is unusual in OpenShift.
We could introduce an entirely new Cluster Operator to manage the USC but because
the update functionality is so closely tied to the CVO, an additional layer seems
excessive and unnecessary. Adding this layer can be considered in the future if
the proposed model turns out to be problematic.

Placing the cluster inspection logic directly into the USC places OTA team into
a position where we need to maintain the logic while not being experts in a
significant part of the domain (interpreting MCP and Node states). This is fine
as long as we _eventually_ move the logic to the MCO to server as an insight
producer, which depends on the future architecture of the system.

API-backed `oc adm upgrade status` will lose the ability to run against older
clusters that do not have the API (or against non-TechPreview clusters, while 
the feature is still in TechPreview). The feature was asked to be an API from
the start and the client-based prototype was meant to be a temporary solution.

## Open Questions [optional]

- When to deprecate and remove the client-based prototype?
- What is the best architecture for the future system where USC only aggregates
  and summarizes information (possibly provided in the form of Update Insights)
  provided by external components that want to contribute update-related 
  information.

## Test Plan

- All code will be appropriately unit-tested.
- Tests for the Status API will be added to openshift/origin testsuite, to both
  update and non-update path. In non-update paths we will simply test that the
  API correctly reports the cluster as not updating. In update paths, the
  presence of the expected status insights will be monitored and their properties
  checked.
- The client `oc adm upgrade status` code that interprets the API to human-readable
  output will receive similar [integration example-based tests](https://github.com/openshift/oc/tree/master/pkg/cli/admin/upgrade/status/examples)
  like the client-based implementation has.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A - the `UpdateStatus` feature gate is already Tech Preview

### Tech Preview -> GA

- API exists and is marked v1
- USC is running in the cluster and maintains the `UpdateStatus` resource in standalone
- Appropriate API support level is decided
- UpdateStatus capability exists, is enabled by default but allows admins to opt-out through standard mechanisms
- Clean plan to achieve HyperShift support is in place
- The `oc adm upgrade status` consumes the Status API by default and has at least feature parity with the client-based prototype
- Meets TRT criteria: e2e tests exist in openshift/origin and a result data corpus proves the feature works and does not lower platform stability

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy


## Version Skew Strategy



## Operational Aspects of API Extensions

## Support Procedures

## Alternatives

## Infrastructure Needed [optional]

N/A

