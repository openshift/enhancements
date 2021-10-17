---
title: short-circuiting backoff

authors:
  - @mshitrit

reviewers:
  - @beekhof
  - @n1r1
  - @slintes

approvers:
  - @JoelSpeed
  - @michaelgugino
  - @enxebre

creation-date: 2021-03-01

last-updated: 2021-03-01

status: implementable

see-also:
  - https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191030-machine-health-checking.md
---

# Support backoff when short-circuiting

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

Any Machine that enters the `Failed` state is remediated immediately, without waiting, by the MHC.
When this occurs, if the error which caused the failure is persistent (spot price too low, configuration error), replacement Machines will also be `Failed`.
As replacement machines start and fail, MHC causes a hot loop of Machine being deleted and recreated.
This hot looping makes it difficult for users to find out why their Machines are failing.
Another side effect of machines constantly failing, is the risk of hitting the benchmark of machine failures percentage - thus triggering the "short-circuit" mechanism which will prevent all remediations.

With this enhancement we propose a better mechanism.
In case a machine enters the `Failed` state and does not have a NodeRef or a ProviderID, it will not be remediated until after a certain time period has passed - thus allowing a manual intervention in order to break to hot loop.

## Motivation

- Preventing remediation hot loop, in order to allow a manual fix and prevent unnecessary resource usage.

### Goals

- Create the opportunity for users to enact custom remediations for Machines that enter the `Failed` state.

### Non-Goals

- This enhancement does not seek to create a pluggable remediation system in the MHC.

## Proposal

We propose modifying the MachineHealthCheck CRD to support a failed node startup timeout. This timeout defines the period after which a `Failed` machine will be remediated by the MachineHealthCheck.

### User Stories

#### Story 1

As an admin of a hardware based cluster, I would like failed machines to delay before automatically re-provisioning so I'll have a time frame in which to manually analyze and fix them.

### Implementation Details/Notes/Constraints

If no value for `FailedNodeStartupTimeout` is defined for the MachineHealthCheck CR, the existing remediation flow
is preserved.

In case a machine enters the `Failed` state and does not have a NodeRef or a ProviderID it's remediation will be requeued by `FailedNodeStartupTimeout`.
After that time has passed if the machine current state remains, remediation will be performed.


#### MHC struct enhancement

```go
    type MachineHealthCheckSpec struct {
        ...
    
        // +optional
        FailedNodeStartupTimeout metav1.Duration `json:"failedNodeStartupTimeout,omitempty"`
    }
```

#### Example CRs

MachineHealthCheck:
```yaml
    kind: MachineHealthCheck
    apiVersion: machine.openshift.io/v1beta1
    metadata:
      name: REMEDIATION_GROUP
      namespace: NAMESPACE_OF_UNHEALTHY_MACHINE
    spec:
      selector:
        matchLabels: 
          ...
      failedNodeStartupTimeout: 48h
```

### Risks and Mitigations
In case `FailedNodeStartupTimeout` is undefined default behaviour is preserved (i.e. remediation is not postponed).
The Pro is that naive users aren't being surprised with a new behavior however, the con is that naive users do benefit from the new behavior.

## Design Details

### Open Questions

See deprecation and upgrade.

### Test Plan

The existing remediation tests will be reviewed / adapted / extended as needed.

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

- [x] 03/01/2021: Opened enhancement PR

## Drawbacks

no known drawbacks

## Alternatives

In case a machine enters the `Failed` state and does not have a NodeRef or a ProviderID do not perform remediation on that machine.

This alternative is simpler since it does not require `FailedNodeStartupTimeout` however it does not allow an option to retain the system previous behaviour and relies completely on manual fix of the failed machines.