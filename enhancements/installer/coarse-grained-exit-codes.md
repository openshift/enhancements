---
title: coarse-grained-exit-codes
authors:
  - "@deadsk2"
reviewers:
  - "@staebler"
approvers:
  - "@pdillon"
  - "@sdodson"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2022-03-16
last-updated: 2022-03-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
replaces:
superseded-by:
---

# Coarse Grained Exit Codes from the Installer

## Summary

`openshift-installer` callers will now get more granular exit codes.
Callers of `openshift-installer` can expect non-zero exit codes *that may change over time* as
the `openshift-installer` evolves over time.
`openshift-installer` provides *no guarantee* that exit codes will not change as result of changes like
greater granularity, lower granularity, re-organization, or other needs.
`openshift-installer` *does guarantee* that a particular exit code will not be re-used to have a
different meaning in the same y-level version.
`openshift-installer` will make reasonable efforts to avoid re-using a particular exit code to have
different meanings across y-level versions, but for sufficiently compelling reasons may do so.

## Motivation

Automated categorization of failures can assist in efforts like
1. reliability determination of provision on various clouds
2. directing effort to proper teams depending on failure modes
3. tracking of which installation stages have problems on various platforms
4. automated retry of infrastructure failures by CI step registry

These are reasons why the data is useful and by providing it, other teams will be able to build these
secondary system.
Other teams *must properly handle previously unknown exit codes*.

### Goals

1. Coarse grained (order of 10 or less values), stage-based exit codes to assist in categorizing
   failures, but not debugging failures.

### Non-Goals

1. Detailed, machine-parseable, cause-based failure information is outside the scope of this enhancement.

## Proposal

The installer knows what stage of the installation has failed when it exits.
The stages include
1. install-config handling
2. infrastructure creation
3. bootstrapping
4. wait-for-cluster-install
5. destroy
6. everything else

The installer will not introspect errors to provide deeper analysis, but these stages (and whatever future
ones the install team identifies) may have unique exit codes when they fail.
Categorizing failures in this way will allow machine categorization of the type of failure by other systems.
The CI step registry is a good example of a consumer, but issuing a destroy after an infrastructure creation
and retrying the `openshift-install` seems like a fairly common use-case since infrastructure creation
failures often happen for environmental reasons and are reasonably likely to succeed on a retry.

### Known Exit Codes

Key things to remember:
1. Callers of `openshift-installer` can expect non-zero exit codes *that may change over time* as
   the `openshift-installer` evolves over time.
2. `openshift-installer` provides *no guarantee* that exit codes will not change as result of changes like
   greater granularity, lower granularity, re-organization, or other needs.
3. `openshift-installer` *does guarantee* that a particular exit code will not be re-used to have a
   different meaning in the same y-level version.
4. `openshift-installer` will make reasonable efforts to avoid re-using a particular exit code to have
   different meanings across y-level versions, but for sufficiently compelling reasons may do so.
5. Consumers must properly handle previously unknown exit codes.

| Stage | Exit Code |
| --- | --- | 
| Generic | whatever other value is produced |
| infrastructure creation | 3 |
| bootstrapping | 4 | 
| wait-for-cluster-install | 5 |
| install-config verification | TBD |

### User Stories

### API Extensions

### Implementation Details/Notes/Constraints [optional]

Approval of the implementation lies with the installer team, but [openshift/installer#5702](https://github.com/openshift/installer/pull/5702)
demonstrates using `errors.Unwrap` and logical `errors.Is` to keep all return code logic contained in the command package.
This keeps exit code logic from leaking into the installer library itself.

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

The `openshift-installer` is only excecuted once and does not leave any persisted data
in the cluster itself, so there is not an upgrade/downgrade concern for changing
existing non-zero return codes into other non-zero return codes.

### Version Skew Strategy

The `openshift-installer` is only excecuted once and does not leave any persisted data
in the cluster itself, so there is no version skew concern for changing
existing non-zero return codes into other non-zero return codes.

Consumers that installer multiple levels of `openshift-installer` may see exit codes
change from one non-zero value to another as the product evolves, but since they are all
failing exit codes and the exit code is explicitly not determining cause, impacts are minimal.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Drawbacks

Consumers may expect to "pin" an exit code forever.
Since consumers know that our product evolves over time and since our exit codes are based on stages, not symptoms,
consumers must always handle novel exit codes as proper failures.
This ensures that any change in an exit code will fail in the safe direction.
This is also the case today.
In addition, if a consumer wishes to identify exact behavior, the `openshift-installer version` command
can be used to determine version and react accordingly.

## Alternatives

A more detailed solution involved structured, machine readable output could be created.
Such a solution would be more expressive, but additional expression isn't required for several use-cases today.
Building these exit codes does not make it any more difficult to provide more detailed structured output in the future.

## Infrastructure Needed [optional]

