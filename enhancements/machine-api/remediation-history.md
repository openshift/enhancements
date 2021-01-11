---
title: remediation-history
authors:
  - "@slintes"
reviewers:
  - "@beekhof"
approvers:
  - "@JoelSpeed"
  - "@michaelgugino"
  - "@enxebre"
creation-date: 2020-12-15
last-updated: 2020-12-15
status: implementable
---

# Remediation history

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Record and show limited remediation history in baremetal environments.

## Motivation

With machine health checks administrators can configure automatic remediation of node problems. There is no good
overview though for when and why nodes were remediated. Recording and showing a history of remediations would allow
administrators to review when and why machines were remediated, and to discover patterns like e.g. machineA always
reboots at 2am on Wednesdays.

### Goals

- Record a limited remediation history.

- Show remediation history in the UI.

### Non-Goals

- Record an *unlimited* history of remediations.
- Collect or provide information on *why* a node has a certain node condition which triggered remediation.

## Proposal

The machine health check holds the configuration for automatic remediation of nodes. We want to extend the status of
machine health checks to contain the last x remediation events, with information about which node is affected, which
condition triggered the remediation, and the timestamps of
- when the triggering condition was detected
- when remediation was started
- when the node was succesfully fenced (deleted / powered off)
- when the node is healthy again.

This information can be displayed in the UI in a table on a machine healthcheck details page.

MHC type enhancement:

```go

// MachineHealthCheckStatus defines the observed state of MachineHealthCheck
type MachineHealthCheckStatus struct {

	[...]

	// History of remediations triggered by this machine health check
	RemediationHistory []Remediation `json:"remediationHistory,omitempty"`
}

// Remediation tracks a remediation triggered by this machine health check
type Remediation struct {
	// the kind of the remediation target, usually node or machine
	// +kubebuilder:validation:Type=string
	TargetKind string `json:"targetKind"`

	// the name of the machine or node which is remediated
	// +kubebuilder:validation:Type=string
	TargetName string `json:"targetName"`

	// the condition type which triggered this remediation
	// +kubebuilder:validation:Type=string
	ConditionType *corev1.NodeConditionType `json:"conditionType,omitempty"`

	// the condition status which triggered this remediation
	// +kubebuilder:validation:Type=string
	ConditionStatus *corev1.ConditionStatus `json:"conditionStatus,omitempty"`

	// the reason for the remediation if not a condition
	// +kubebuilder:validation:Type=string
	Reason string `json:"reason,omitempty"`

	// the time when the unhealthy condition was detected
	Detected *metav1.Time `json:"detected"`

	// the time when remediation started
	Started *metav1.Time `json:"started,omitempty"`

	// the time when the node is fenced
	Fenced *metav1.Time `json:"fenced,omitempty"`

	// the time when the machine or node is healthy again
	Finished *metav1.Time `json:"finished,omitempty"`

	// the type of remediation, e.g. "machineDeletion" or "external"
	// +kubebuilder:validation:Type=string
	Type string `json:"remediationType,omitempty"`
}

```

Example MHC:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  annotations:
    machine.openshift.io/remediation-strategy: external-baremetal
  creationTimestamp: "2020-11-24T16:31:22Z"
spec:
  maxUnhealthy: 100%
  nodeStartupTimeout: 60m
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: worker
  unhealthyConditions:
  - status: "False"
    timeout: 20s
    type: Ready
  - status: Unknown
    timeout: 20s
    type: Ready
status:
  conditions:
  - lastTransitionTime: "2020-11-24T17:17:32Z"
    status: "True"
    type: RemediationAllowed
  currentHealthy: 2
  expectedMachines: 2
  remediationHistory:
  - conditionStatus: Unknown
    conditionType: Ready
    detected: "2020-11-24T17:33:43Z"
    fenced: "2020-11-24T17:34:35Z"
    finished: "2020-11-24T17:35:09Z"
    remediationType: external
    started: "2020-11-24T17:34:04Z"
    targetKind: Node
    targetName: worker-1
  remediationsAllowed: 2
```

Example UI: TBD (copy from "KNI Node & Host Management Designs" doc?)

### User Stories

#### Story 1

As a cluster administrator I want to have an overview over when and why nodes failed and were remediated, in order
to detect patterns, and to be able to troubleshoot possible root causes.

### Implementation Details/Notes/Constraints [optional]

The machine healthcheck controller [0] is responsible for triggering remediations based on machine health checks and node
conditions. This will also be the place to track remediations:

- in the `needsRemediation` func:
  - for failed machines and unhealthy node conditions: track detection
  - for missing nodes after remdiation start: track successful fencing
  - for healthy nodes: track finished remdiation
    
- in the `remediate` and `remediationStrategyExternal` funcs: track remediation start

[0] https://github.com/openshift/machine-api-operator/blob/master/pkg/controller/machinehealthcheck/machinehealthcheck_controller.go

### Risks and Mitigations

We are not aware of any risks.

## Design Details

### Open Questions [optional]

TBD

### Test Plan

TBD

### Graduation Criteria

TBD

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

TBD

## Implementation History

TBD

## Drawbacks

## Alternatives

- Using events
  
  - The UI already displays events (in its own generic events tab), but events only have a short TTL (configurable,
    defaults to 3 hours in OpenShift), which makes it hard to see which nodes are having recurring issues over a longer duration.
  - Parsing events and composing a remediation overview is harder on the UI than to just display the MHC status.


- Using metrics

  - Metrics are a good tool for recording e.g. how often remediations are triggered and how long they take in average, but
  not so much for tracking single remediations in the desired detail.
  - Parsing getting and parsing metrics, and composing a remediation overview from them, is harder on the UI than to just display the MHC status.
