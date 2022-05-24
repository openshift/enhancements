---
title: pod-security-admission
authors:
  - "@s-urbaniak"
reviewers:
  - "@stlaz"
  - "@sttts"
  - "@ibihim"
  - "@slaskawi"
approvers:
  - "@mfojtik"
creation-date: 2021-09-14
last-updated: 2021-09-14
status: informational
see-also:
replaces:
superseded-by:
---

# PodSecurity admission in OpenShift

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Kubernetes recently gained PodSecurity policy admission as part of KEP 2579 [1].
It is a new pod security admission plugin to enforce Kubernetes PodSecurity standards [2].
It is meant to replace the existing PodSecurityPolicy admission mechanism in Kubernetes.

This enhancement describes the migration path to integrate PodSecurity admission inside OpenShift.
Special interest is put to design a migration scheme which lets PodSecurity coexist with the existing Security Context Constraints (SCC) mechanism.

[1] https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/2579-psp-replacement
[2] https://kubernetes.io/docs/concepts/security/pod-security-standards/

## Motivation

Unlike the deprecated (see [1]) "Pod Security Policy" admission plugin the goal is to enable the newer "Pod Security" admission plugin within OpenShift.
The new PodSecurity admissions plugin is a validating plugin only. This implies the opportunity to let it coexist with the existing SCC logic.

Note that PodSecurity admission is validating only while SCC is mutating admission. Hence, SCC by design always runs before Pod Security admission.

[1] https://kubernetes.io/blog/2021/04/06/podsecuritypolicy-deprecation-past-present-and-future/

### Goals

[1] Enable PodSecurity admission with "restricted" pod security profile by default
[2] Design an architecture that lets PodSecurity coexist with SCC logic

### Non-Goals

[1] Break existing SCC API

## Proposal

We propose introducing PodSecurity admission in a multi-step process. We suggest to follow a similar migration scheme as outlined in the PodSecurityPolicy migration recommendation [1]:

[1] https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/2579-psp-replacement#podsecuritypolicy-migration

For OpenShift, the following high level migration plan is envisioned:

1. Enable the PodSecurity admission plugin in no-op mode but with the ability to audit policy violations.

Initially, PodSecurity is configured with the following settings:

| Policy  | Profile |
| --------| --------|
| enforce | privileged |
| warn    | baseline |
| audit   | baseline |

This configures PodSecurity to run in "no-op" mode but will emit warnings and audit log events in case of "baseline" violations.

2. Analyze audit logs from CI runs for violations against the "baseline" policy level

From existing CI e2e runs we can introspect the amount of workloads that violate the "baseline" policy.
This gives us an indicator what workloads need to be annotated as privileged to pass PodSecurity admission.

3. Increase warn and audit profile to "restricted"

| Policy  | Profile |
| --------| --------|
| enforce | privileged |
| warn    | restricted |
| audit   | restricted |

Similar to step 2. we will reiterate on existing workloads and asses if there is necessary action to readjust Pod Security annotations.

For existing clusters that have Pod Security admission enabled in step 1. this can be configured at runtime using an unsupported config override:

```yaml
apiVersion: operator.openshift.io/v1
kind: KubeAPIServer
metadata:
  name: cluster
spec:
  unsupportedConfigOverrides:
    admission:
      pluginConfig:
        PodSecurity:
          configuration:
            kind: PodSecurityConfiguration
            apiVersion: pod-security.admission.config.k8s.io/v1alpha1
            defaults:
              audit: restricted
              audit-version: latest
              enforce: privileged
              enforce-version: latest
              warn: restricted
              warn-version: latest
```

4. Increase enforce profile to "restricted"

Finally, after iterating over and adjusting workloads the "restricted" pod security profile is going to be enabled with the following settings:

| Policy  | Profile |
| --------| --------|
| enforce | restricted |

The pod security label syncer mechanism (https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission-autolabeling.md) ensures that customer workloads won't break.

### User Stories

#### Core Workload Pod Security compliance

As core workload I want to comply and pass Pod Security admission.

### Risks and Mitigations

We want to test Pod Security admission by default in OpenShift as early as possible to detect potential issues.

## Design Details

Pod Security is an upstream feature, design details are available in the KEP [1].

[1] https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/2579-psp-replacement

### Open Questions

It is desirable to design a scheme where SCC and PodSecurity admission can coexist in OpenShift.
The envisioned goal is to have `PodSecurity` admission enabled by default in OpenShift next to SCC admission, desirably with `restricted` policy by default.

### Future work

Currently, SCC and Pod Security admission logic is configured independently.
To reduce redundant configuration efforts there is ongoing effort to design a mechanism which sync SCC configuration with Pod Security setttings.

### Test Plan

Pod Security admission test coverage is available as part of upstream e2e integration and unit tests.

### Graduation Criteria

We follow the graduation criteria as outlined in the upstream KEP [1].

[1] https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/2579-psp-replacement#graduation-criteria

For OpenShift this has the following implications:

| OpenShift | Kubernetes  | Maturity     |
|-|-|-|
| 4.10      | 1.23        | Beta         |
| 4.11      | 1.24        | GA (planned) |

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Pod Security being a label based API on namespaces does not impose a risk upon downgrade/upgrade.
Due to the pod security admission not being available in previous version workloads are guaranteed to be scheduled.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

N/A