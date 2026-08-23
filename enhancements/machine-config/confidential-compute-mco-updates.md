---
title: confidenital-clusters-mco-updates
authors:
  - "@isabella-janssen"
reviewers:
  - "@confidential-cluster-team" # for interactions with the confidential cluster operator
  - "@coreos-team"               # for general confidential compute context
#   TODO: add team MCO reps, architects, and staff eng
approvers:
#   TODO: determine & add final reviewer
  - TBD
api-approvers:
  - None
creation-date: 2026-02-02
last-updated: 2026-02-05
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/MCO-2066
  - https://issues.redhat.com/browse/MCO-2074
see-also:
  - "enhancements/security/confidential-clusters.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Confidenital Clusters - MCO Updates - MVP GA

## Summary

This enhancement proposal aims to outline the changes required in the MachineConfigOperator to support
the [confidential clusters initiative](https://github.com/openshift/enhancements/pull/1878). For simplicity,
this enhancement ***will not*** outline the `bootc` support needed in the MCO for confidenitial
clusters. Instead, `bootc` work will be outlined in a seperate enhancement as part of
 [MCO-1238](https://issues.redhat.com/browse/MCO-1238).

## Motivation

The motivation for confidential clusters is clearly laid out in its corresponding 
[enhancement](https://github.com/openshift/enhancements/pull/1878). The goal of this specific 
enhancement is to agree on the scope of work needed in the MCO by team MCO to support the overall 
initiative. This should be a place to document changes to the MCO experience for confidenital 
clusters and to iron out some of the larger outstanding questions on implementation design.

### User Stories

TODO (ijanssen): Fill in section

### Goals

* This enhancement documents changes to the typical MCO expereince for confidential clusters.
* A clear design direction is agreed upon for the main changes required in the MCO.
* Tradeoffs to the decided on implementation are clearly documented for future reference.

### Non-Goals

* This enhancement does not cover ehancements needed to other operators and components, such as the
installer, to support confidential clusters.
* This enhancement does not cover the dependancy on bootc for supporting confidential clusters; see
[MCO-1238](https://issues.redhat.com/browse/MCO-1238) for information on the MCO's bootc plans.
* This enhancement is limted in scope to the changes needed in the MCO to support the initial MVP 
release of confidential clusters.
   * For example, this phase assumes that MachineConfigs (MC) served by the MachineConfigServer 
   (MCS) are valid, as the primary goal in this phase is guaranteeing confidentiality, not integrity.
* This enhancement does not aim to explain the primary goals and ideas of confidential clusters,
such as the attestation process and the new Trusted Execution Cluster Operator. For those details, 
see the [main confidential clusters enhancement](https://github.com/openshift/enhancements/pull/1878).

## Proposal

### Workflow Description

There are two categories of changes needed in the MCO to support the MVP release of confidential 
clusters.

First, some currently suported MC configs will not be supported in confidential clusters, so the 
machine config validation functionality needs to be updated to reject such configs. This 
requirement will be discussed further in the first subsection below.

Second, to guarantee confidentiality, MCs should only be served to machines that have been 
attested. This will require some changes in node scale up proccesses and require the introduction 
of new processes between the MCO and the Trusted Execution Cluster Operator. These changes will 
be detailed in the second subsection below.

#### Refuse MachineConfigs with configurations unsupported in confidential clusters

Not all currently supported MachineConfig configurations will be supported in confidential 
clusters. The currently supported MachineConfig configuration options, can be found in the 
[4.21 Openshift docs](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/machine_configuration/machine-config-index#what-can-you-change-with-machine-configs). 
The following config changes will not be supported in confidential clusters:

- kernelArguments
- kernelType
- fips
- extensions

The MCO has a [ValidateMachineConfig function](https://github.com/openshift/machine-config-operator/blob/c9188a480b4aed93dcf3ef3c20bbcb017c4c6aaa/pkg/controller/common/helpers.go#L447-L469)
that validates whether a user-provided MC config is valid in terms of formating and options. More 
specifically, the beginning steps of applying a MC are:

1. A MC is applied
2. The MachineConfigController's (MCC) RenderController validates whether the provided MC is valid.
   a. If the MC is valid, a rendered MC is created and the update can start rolling out to all 
   applicable nodes.
   b. If the MC is invalid, no rendered MC is created and the targeted MCP degrades.

Extending the functionality in this existing process to reject additional configs when a cluster 
is in confidential mode seems to be the most straightforward approach to handle this required 
change. It will allow the existing experience of MCPs degrading on the application of invlid MCs to 
persist and is straightforward from a development perspective.

#### Only serve MachineConfigs to attested nodes

The below steps describe the current process followed for the scale up of a new node:

1. Node scale up is triggered.
2. A new node boots and contains a stub ignition with a "merge" directive.
   - The "merge" directive in the config indicates that the config is not the full config for the 
   node and includes an endpoint to the MCS, where the full config can be found.
3. The node reaches out to the MCS to get the full config.
4. The MCS serves the rendered MC for the node, which is determined by the new
node's associated MachineConfigPool (MCP).
5. The node consumes the full config and joins the cluster.

To guarantee confidentiality in the node scale up process in confidential clusters, we need to 
ensure that MCs are only served to attested nodes. One shortcoming of the current MCS 
implementation with regards to this goal is that the MCS will serve MCs to any machine hitting the 
request endpoint. By design, the MCS assumes that all machines able to make a request to it should 
have access to the requested MCs, so it responds to the simple, no auth required GET requests. To 
align with the confidentiality goals for confidential clusters, we must ensure that MCs are only 
served to attested nodes.

The processes for attesting nodes is handled by the [Trusted Execution Cluster Operator](https://github.com/trusted-execution-clusters/operator), 
where nodes are attested on boot, before any MCO operations. Therefore, the MCO does not need to 
assume any of the responsibility of checking whether the node is attested.

The MCO does, however, need some process updates to ensure node attestation is complete before the 
machines are served their respective rendered MC. There were a few options discussed for how to 
best adress the goal of only serving MCs to attested nodes. However, in this section I will only 
outline the two "best" options being seriously considered. The other options will be outlined in 
the following [alternatives section](#alternatives-not-implemented).

##### Option 1: Teach the MCS to recognize connections from attested nodes

The general idea for this option is that the MCS will continue serving cofigs to new machines. 
However, instead of serving MCs to any machine, MCs should only be served once nodes are attested. 
The flow for node scale up in this option becomes:

1. Node scale up is triggered.
2. The node scale up request is directed to the Trusted Execution Cluster Operator, where a 
machine will be attested and booted.
   - The new machine will contain the typical stub ignition with a "merge" directive indicating 
   the respective MCS endpoint where the full config can be found.
   - The new machine will also contain information a proxy service can use to confirm a node is 
   attested before the MCS serves a MC.
3. The machine will make a request to the MCS through a new proxy service.
4. When the service gets a request for a MC from a node, it will first confirm with the 
Trusted Execution Cluster Operator's [attestation service](https://github.com/confidential-containers/trustee/tree/main/attestation-service) 
that the node is attested.
5. If the node requesting a MC is attested, the MCS will serve the config as normal. However, if 
the node is not attested, the MCS will serve the MC and the node scale up will be unsuccessful.

<!-- TODO: maybe add a mermaid diagram. -->

Taking this approach would require creating a new service to sit in front of the existing MCS that 
can speak with both the attestation service and the MCS. It's primary role would be to confrim 
machine attestation before allowing requests through to the MCS. This would allow the MCS to remain 
mostly the same as it is today, with the exception of updating it to only accept requests from the 
new proxy service.

**MCO Changes:**
- Create a proxy to understand whether a node is attested
- Update the MCS to only serve MCs on certain conditions (ex: only allow requests from the new 
service when node attestation has been confirmed)
- The MCS will need to handle a new error case (when a non-attested node is requesting MCs)

**Trusted Execution Cluster Operator Changes:**
- Allow requests to and from the MCO's new proxy service

**Benefits:**
- Maintains the MCS's role in serving MCs
- The attestation service's role would remain scoped to node attestation (no MC management)

**Drawbacks:**
- Adding a new service
- Changing what the MCS accepts requests from
- The MCs will remain homogeous per-MCP, so node-specific needs could not be handled

##### Option 2: Move the rendered MCs to the KBS

The main idea for this option is storing the rendered MCs traditionally served by the MCS in the 
Trusted Execution Cluster Operator trustee’s [key-value store (KBS)](https://github.com/confidential-containers/trustee/tree/main/kbs).
While the MCS would still play a role in serving MCs, it would transition from serving them 
directly to machines and instead to serving them in a way where they can be stored in the KBS. If 
we decide to go forward with this option, we will need to decide how the MCs must be fetched to 
understand the updates required by the MCO. Some options might include:

1. Request MCs from the MCS as needed (during the node scale up process)
2. Sync MCs in the KBS against those served by the MCS on a schedule
3. Push MCs to the KBS every time an MCP's rendered MC is updated

No matter the exact method for fetching MCs from the MCS, if we assume the KBS stores the MCs 
needed for nodes to join a cluster, the node scale up flow would become:

1. Node scale up is triggered.
2. The node scale up request is directed to the Trusted Execution Cluster Operator, where a 
machine will be attested and booted.
3. The machine will make a request to the KBS for its necessary config.
4. The node consumes the full config and joins the cluster.

<!-- TODO: maybe add a mermaid diagram. -->

**MCO Changes:**
- Only allow requests to the MCS from the Trusted Execution Cluster Operator
- If we must sync the MCs in the KBS with every rendered MC update in an MCP, we will need to add 
the necessary informers

**Trusted Execution Cluster Operator Changes:**
- The KBS will need the ability to store MCs that can be fetched on node scale up
- The Trusted Execution Cluster Operator will need to understand how to make requests to the MCS
- Depending on how the KBS syncs MCs, the Trusted Execution Cluster Operator might need a scheduler 
or service to request MCs from the MCS

**Benefits:**
- This keeps the node attestation flow fully within the Trusted Execution Cluster Operator's scope.
- The MCS currently only recognizes rendered MCs as homogenous per MCP. If any per-node MCs are 
needed, such updates can be handled in the Trusted Execution Cluster Operator.

**Drawbacks:**
- Pulling the entire node scale up process into the Trusted Execution Cluster Operator might 
duplicate logic currently in the MCO. 

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

As noted in the [main confidential clusters enhancement](https://github.com/openshift/enhancements/pull/1878),
confidential clusters are not planned to be supported on the hosted control plane topology in this phase.

#### Standalone Clusters

See the [main confidential clusters enhancement](https://github.com/openshift/enhancements/pull/1878) 
for more details, but this enhancement is currently intended for standalone clusters running on 
cloud providers that support confidential virtual machines.

#### Single-node Deployments or MicroShift

As noted in the [main confidential clusters enhancement](https://github.com/openshift/enhancements/pull/1878),
confidential clusters are not planned to be supported on the single node topology or in MicroShift 
in this phase.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

<!-- TODO: add here a bit about the bootc dependancy? -->

TODO (ijanssen): Fill in section

### Risks and Mitigations

TODO (ijanssen): Fill in section

### Drawbacks

TODO (ijanssen): Fill in section

## Alternatives (Not Implemented)

TODO (ijanssen): Fill in section with options not considered in the main proposal from 
[here](https://docs.google.com/document/d/1WuOgC_zJaH4lb5rqYGu4ZpuGroip2OcEbCDsthyt19A/edit?tab=t.0#heading=h.oq2r9sqwst35)

## Open Questions [optional]

TODO (ijanssen): Fill in section

## Test Plan

N/A for now as this enhancement proposal is for a TechPreview feature. Test cases will be 
determined through investigation work in [MCO-2078](https://issues.redhat.com/browse/MCO-2078).

## Graduation Criteria

TODO (ijanssen): Fill in section

### Dev Preview -> Tech Preview

TODO (ijanssen): Fill in section

### Tech Preview -> GA

TODO (ijanssen): Fill in section

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

TODO (ijanssen): Fill in section

## Version Skew Strategy

TODO (ijanssen): Fill in section

## Operational Aspects of API Extensions

TODO (ijanssen): Fill in section

## Support Procedures

TODO (ijanssen): Fill in section

## Infrastructure Needed [optional]

Currently, there is no new infrastructure needed to support this enhancement, though we plan to 
use [MCO-2078](https://issues.redhat.com/browse/MCO-2078) to understand if our current test 
infrastructure will support the testing needs for this enhancement.
