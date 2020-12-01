---
title: external-remediations
authors:
  - @slintes
reviewers:
  - @beekhof
  - @n1r1
  - TBD
approvers:
  - TBD
creation-date: 2020-11-29
last-updated: 2020-11-29
status: implementable
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191030-machine-health-checking.md
  - https://github.com/kubernetes-sigs/cluster-api/pull/3606
---

# External remediations

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

By using `MachineHealthChecks` a cluster admin can configure automatic remediation of unhealthy machines and nodes.
The machine healthcheck controller's remediation strategy is deleting the machine, and letting the cloud provider
create a new one. This isn't the best remediation strategy in all environments.

There is already a mechanism to provide an alternative, external remediation strategy, by adding an annotation to the
`MachineHealthCheck` and then to `Machine`s. However, this is isn't very maintainable.

With this enhancement we propose a better, future-proof mechanism, that aligns us with the mechanism implemented upstream.
This proposal is a backport of parts of the upstream machine healthcheck proposal [0], which
also is already implemented [1].

- [0] https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191030-machine-health-checking.md
- [1] https://github.com/kubernetes-sigs/cluster-api/pull/3606

## Motivation

- Environments consisting of hardware based clusters are significantly slower to (re)provision unhealthy machines,
so they have a need for a remediation flow that includes at least one attempt at power-cycling unhealthy nodes.
- Other environments and vendors also have specific remediation requirements, so there is a need to provide a generic
mechanism for implementing custom remediation logic.

### Goals

- Create the ability to define customized remediation flows outside of the Machine Health Check and CAPI codebase.
- Migrate the existing external remediation mechanism to the new one.

### Non-Goals

TBD

## Proposal

We propose modifying the MachineHealthCheck CRD to support a externalRemediationTemplate, an ObjectReference to
a provider-specific remediation template CR.

### User Stories 

#### Story 1

As an admin of a hardware based cluster, I would like unhealthy nodes to be power-cycled, so that I can recover
from transient errors faster and begin application recovery sooner.

#### Story 2

As an admin of a hardware based cluster, I would like unhealthy nodes to be power-cycled, so that I can detect
non-transient issues faster.

#### Story 3

As an admin of a hardware based cluster, I would like the system to keep attempting to power-cycle unhealthy nodes,
so that they are automatically added back to the cluster when I fix the underlying problem.

### Implementation Details/Notes/Constraints

If no value for externalRemediationTemplate is defined for the MachineHealthCheck CR, the existing remediation flow
is preserved.

If a value for externalRemediationTemplate is supplied and the Machine enters an unhealthy state, the template will
be instantiated, with the same name and namespace of the target Machine, which passes the remediation flow to an
External Remediation Controller (ERC) watching for that CR.

No further action (deletion or applying conditions) will be taken by the MachineHealthCheck controller until the
Node becomes healthy. After that, it will locate and delete the instantiated MachineRemediation CR.

When a Machine enters an unhealthy state, the MHC will:
* Look up the referenced template
* Instantiate the template (for simplicity, we will refer to this as a External Machine Remediation CR, or EMR)
* Force the name and namespace to match the unhealthy Machine
* Save the new object in etcd

We use the same name and namespace for the External Machine Remediation CR to ensure uniqueness and lessen the
possibility for multiple parallel remediations of the same Machine. 

The lifespan of the EMRs is that of the remediation process, and they are not intended to be a record of past events.
The EMR will also contain an ownerRef to the Machine, to ensure that it does not outlive the Machine it references.

The only signaling between the MHC and the external controller watching EMR CRs is the creation and deletion of the
EMR itself. Any actions or changes that admins should be informed about should be emitted as events for consoles
and UIs to consume if necessary. They are informational only and do not result in or expect any behaviour from the MHC,
Node, or Machine as a result.

When the external remediation controller detects the new EMR it starts remediation and performs whatever actions
it deems appropriate until the EMR is deleted by the MHC. It is a detail of the ERC when and how to retry remediation
in the event that a EMR is not deleted after the ERC considers remediation complete. 

The ERC may wish to register a finalizer on its CR to ensure it has an opportunity to perform any additional cleanup
in the case that the unhealthy state was transient and the Node returned to a healthy state prior to the completion
of the full custom ERC flow.

#### MHC struct enhancement

```go
    type MachineHealthCheckSpec struct {
        ...
    
        // +optional
        ExternalRemediationTemplate *ObjectReference `json:"externalRemediationTemplate,omitempty"`
    }
```

#### Example CRs

MachineHealthCheck:
```yaml
    kind: MachineHealthCheck
    apiVersion: cluster.x-k8s.io/v1alphaX
    metadata:
      name: REMEDIATION_GROUP
      namespace: NAMESPACE_OF_UNHEALTHY_MACHINE
    spec:
      selector:
        matchLabels: 
          ...
      externalRemediationTemplate:
        kind: Metal3RemediationTemplate
        apiVersion: remediation.metal3.io/v1alphaX
        name: M3_REMEDIATION_GROUP
```

Metal3RemediationTemplate:
```yaml
    kind: Metal3RemediationTemplate
    apiVersion: remediation.metal3.io/v1alphaX
    metadata:
      name: M3_REMEDIATION_GROUP
      namespace: NAMESPACE_OF_UNHEALTHY_MACHINE
    spec:
      template:
        spec:
          strategy:               escalate
          deleteAfterRetries:     10
          powerOnTimeoutSeconds:  600
          powerOffTimeoutSeconds: 120
```

Metal3Remediation:
```yaml
    apiVersion: remediation.metal3.io/v1alphaX
    kind: Metal3Remediation
    metadata:
      name: NAME_OF_UNHEALTHY_MACHINE
      namespace: NAMESPACE_OF_UNHEALTHY_MACHINE
      finalizer:
      - remediation.metal3.io
      ownerReferences:
      - apiVersion:cluster.x-k8s.io/v1alphaX
        kind: Machine
        name: NAME_OF_UNHEALTHY_MACHINE
        uid: ...
    spec:
      strategy:               escalate
      deleteAfterRetries:     10
      powerOnTimeoutSeconds:  600
      powerOffTimeoutSeconds: 120
    status:
      phase: power-off
      retryCount: 1
```

### Risks and Mitigations

No known risks

## Design Details

### Open Questions

See deprecation and upgrade

### Test Plan

The existing external remediation tests will be reviewed / adapted / extended as needed, and the upstream tests will
be backported as well. 

### Graduation Criteria

TBD

#### Examples

TBD

##### Dev Preview -> Tech Preview

TBD

##### Tech Preview -> GA 

TBD

##### Removing a deprecated feature

- The annotation based external remediation needs to be deprecated
- Open question: for how long do we need to support both mechanisms in parallel (if at all)?

### Upgrade / Downgrade Strategy

- Open question: do we need an automatic MHC conversion from the existing annotation based mechanism to the new one? 

### Version Skew Strategy

There is a dependency between the machine-api-operator (which contains the machine healthcheck controller) and
cluster-api-provider-baremetal (which provides the current external baremetal remediation controller), both are part
of the OCP release payload. That means that there can be a short living version skew during upgrades. This isn't a
problem though, because an updated MHC can only be applied (or an automatic conversion can only happen) when both
controllers and their CRDs are updated: the MHC needs to have the new template field, and the remediation CRD and
its template CRD need to exist.

## Implementation History

- [x] 11/30/2020: Opened enhancement PR

## Drawbacks

no known drawbacks

## Alternatives

- Keep the existing annotation based mechanism.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
