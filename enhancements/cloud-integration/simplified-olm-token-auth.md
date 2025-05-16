---
title: simplified-olm-token-auth
authors:
  - Jeremiah Stuever
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
replaces:
  - "enhancements/cloud-integration/tokenized-auth-enablement-operators-on-cloud.md"
superseded-by:
---
# Simplified OLM token authentication

## Summary

OpenShift previously gained the ability to use short-lived tokens for authentication to various cloud providers. The core operators were included in this effort and are already capable of doing so. Operators managed by the Operator Lifecycle Manager (OLM) can benefit from this integration as well. Several have already done so using a previously defined process. It includes steps where the operator creates a CredentialRequest and then waits for the Cloud Credential Operator (CCO) to do nothing more than translate that into a secret. This enhancement is to simplify this by removing CCO from this part of the process and having the OLM operators generate the secret directly.

## Motivation

The intent presented in the original enhancement was to unify the process across operators so users of several of them have the same experience and similar steps to perform. This enhancement does not change that. Nor does it remove CCO from the process altogether; it is still valuable to have the `ccoctl` tool as part of the process. What this enhancement proposes is to remove CCO's in-cluster role of translating the OLM CredentialRequests into secrets.

One reason CCO was placed into the workflow at this point appears to be to enable CCO to provide consolidated logic for creating the token enabled secrets. In theory, this reduces effort by providing shared code and by enabling future changes to happen in a single place. In practice, this appears to have increased the up-front effort for each operator while adding continual maintenance requirements. Instead of creating a relatively simple secret (available in k8s libraries), these operators now have to create a credentialRequest. This forces CCO to be a dependency of the operator (the credentialRequest definition currently lives in the CCO repo). In addition, the interface between these operators and CCO adds several more points of failure. This increases the effort required from end users, support, and engineers to understand why these things break when they do. This is a significant increase in known effort with the hopes of reducing hypothetical future effort. By removing CCO from this part of the framework, we reduce the known effort significantly.

One of the hypothetical situations presented was in the case where the format of a secret changes. In this scenario, CCO would presumably be the only place where this change would need to take place. In practice, an operator only needs the new format if it itself has changed to do so. This actually highlights a compatibility nightmare created by the current framework. If CCO starts creating the secrets in a new format, what happens to the OLM operators that are still using the old? Would we update the operator to understand both formats? Would we update CCO to learn from the operator which format it needs? How would the operator relay that requirement to CCO? Either way, it appears the OLM operator would need additional changes in order to handle this situation. As a result, the desired benefit is not realized. By having the operator manage the secret directly, we guarantee that it is compatible with itself and enable it to move freely through this space unimpeded by CCO.

Another hypothetical situation presented was to enable CCO to validate the credential specified in the credentialRequest prior to creating the secret. This is a good idea, in theory, because it allows the workflow to fail early and provide the end user with a clear reason, if they know where to look. In practice, this requires CCO to have permissions in the cloud provider that it would not otherwise need. CCO currently requires no permissions in the cloud provider. Customers who are choosing to use STS authentication are doing so, in part, to minimize security risks. Having additional permission requirements goes against this. Realizing this benefit would be at the expense of security.

CCO was designed to take no actions when in manual mode. This changed when it was modified to handle OLM operator credentialRequests. All manual mode clusters now use the mint-mode execution path with injected logic to handle these special use cases. This has caused unintended consequences causing several bugs. The resolution of some of these bugs will require significant refactoring of CCO. By removing CCO from this part of the framework, we reduce the effort required to perform this refactoring.


### User Stories

TBD

### Goals

TBD

### Non-Goals

TBD

## Proposal

TBD

### Workflow Description

TBD

### API Extensions

TBD

### Topology Considerations

TBD

#### Hypershift / Hosted Control Planes

TBD

#### Standalone Clusters

TBD

#### Single-node Deployments or MicroShift

TBD

### Implementation Details/Notes/Constraints

TBD

### Risks and Mitigations

TBD

### Drawbacks

TBD

## Alternatives (Not Implemented)

TBD

## Open Questions [optional]

TBD

## Test Plan

TBD

## Graduation Criteria

TBD

### Dev Preview -> Tech Preview

TBD

### Tech Preview -> GA

TBD

### Removing a deprecated feature

TBD

## Upgrade / Downgrade Strategy

TBD

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

TBD

## Support Procedures

TBD

## Infrastructure Needed [optional]

TBD