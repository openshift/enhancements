---
title: egress-flow
authors:
  - "@bnshr"
reviewers:
  - "@trozet"
  - "@danwinship"
  - "@msherif1234"
approvers:
  - "@trozet"
  - "@danwinship"
api-approvers:
  - None
creation-date: 2025-11-10
last-updated: 2025-11-12
tracking-link:
  - https://issues.redhat.com/browse/CNF-14073
status: implementable
---


# Communication egress flows matrix of OpenShift and Operators

## Summary

This enhancement allows to automatically generate the communication network communication in the 
product documentation for all egress flows of OpenShift (multi-node and
single-node deployments) and Operators.

## Motivation

Security-conscious customers need OpenShift flows matrix for regulatory reasons 
and/or to implement firewall rules to restrict traffic to the minimum set of
required flows only, on-node firewall or external.

### User Stories

- As an OpenShift cluster administrator, I want documentation on the expected 
  flows of traffic outgoing from to every OpenShift installation so I can set up 
  firewall rules such as nftables, NGFW, etc. to restrict traffic to the 
  minimum required set of flows only.

### Goals

- Provide a mechanism to automatically generate an accurate and up-to-date 
  OpenShift communication egress flows matrix.

- Keep the egress flow matrix documented in OpenShift release documents
  updated and validate it.

### Non-Goals
N/A

## Proposal

We propose to leverage OpenShift Network Observability Operator to collect the egress communication from the cluster to the outside world.

- A flow matrix describing the expected flows of outgoing traffic will 
  be included in every OpenShift release documentation.

### Workflow Description

An OpenShift administrator would like to get an accurate and up-to-date OpenShift 
communication egress flows matrix.

- The admin reviews OpenShift release documentation to get the included flow
  matrix describing the expected flows of outgoing traffic.

### API Extensions
N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes
Out of scope for this proposal.

#### Standalone Clusters

The egress matrix can be generated on standalone clusters.

#### Single-node Deployments or MicroShift

The egress matrix can be generated on single-node deployments and MicroShift.

### Implementation Details/Notes/Constraints

1. OpenShift CI installs the Network Observability Operator in the cluster in test. 
2. Through eBPF agent of the Network Observability Operator, the egress network data are collected. The data is aggregated through Loki. The retention of the flow logs in the Loki kept for 24 hours. `FlowCollector` is adjusted to capture all data with sampling rate 1.
3. CI job would run OpenShift tests to track any special flow that generates outgoing flow within the cluster.
4. The start and end time of the test result are captured and then we filter the Loki aggregated egress flow to process it.
5. The data processing would filter out only egress data from the OpenShift operators.

**Basic Loki query**

```{K8S_FlowLayer="infra", FlowDirection="1"} | json | DstSubnetLabel = "" | SrcSubnetLabel = "Pods" | line_format "{{.SrcAddr}},{{.SrcPort}},{{.DstAddr}},{{.DstPort}}" ```

This query would be readjusted to find the Operators that are generating the egress flow.


#### Architecture

![Alt text](./images/egress.drawio.svg)




### Risks and Mitigations

1. Having the sampling rate for flow capture may hit the peformance issue.
2. The small size Loki (1x.small) in the installed Loki may impose the risk of storage issue.
3. Loki could be down and hence debugging is necessary and data loss can occur. However, rerun of CI job is required in that case.

### Drawbacks
N/A

## Open Questions

1. What should be reporting strategy once we get the egress data report?
2. Should we automate the reporting the teams? If yes, how?
3. Do we need persistent storage for Loki and storage in the Cloud (maybe in AWS)?

## Test Plan

- E2E tests will be added to `openshift-tests`
  - Validate an up-to-date generated egress flow matches the 
    one documented in OpenShift release documents


## Graduation Criteria

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
N/A

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
N/A

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions
N/A

## Support Procedures
N/A

## Alternatives (Not Implemented)
N/A