---
title: machine-health-checking
authors:
  - @bison
  - @ingvagabund
  - @enxebre
  - @cynepco3hahue
reviewers:
  - @derekwaynecarr
  - @michaelgugino
  - @beekhof
  - @mrunalp
approvers:
  - @enxebre
  - @bison
  - @derekwaynecarr
  - @michaelgugino

creation-date: 2019-09-09
last-updated: 2019-09-09
status: implementable
see-also:
replaces:
superseded-by:
---

# Machine health checking

## Release Signoff Checklist

 - [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs


## Summary

Enable opt in for automated health checking and remediation of unhealthy nodes backed by machines. a.k.a. node auto repair.

## Motivation

Reduce administrative overhead to run a cluster and ability to respond to failures of machines and keep the nodes healthy.

### Goals

- Providing a default remediation strategy for public clouds - Machine deletion.

- Allowing to define different unhealthy criterias for different pools of machines.

- Providing administrator-defined short-circuiting of automated remediation when multiple nodes are unhealthy at the same time.

- Allowing a new and custom remediation developed outside the machine-healthcheck controller and enable its use via annotation, e.g baremetal reboot.


### Non-Goals

- Coordinating individual remediation systems. This must be driven and designed by each system as they see fit, e.g. by annotating affected nodes with state and transition time. This proposal focus on remediating by deleting.

- Recording long-term stable history of all health-check failures or remediations.

## Proposal

- As a customer I only care about my apps availability so I want my cluster infrastructure to be self healing and the nodes to be remediated automatically.

- As a cluster admin I want to define my unhealthy criteria as a list of node conditions https://github.com/kubernetes/node-problem-detector.

- As a cluster admin I want to define different unhealthy criteria for pools of machines targeted for different workloads.

- The MHC does a best effort to short-circtuit and it limits remediation when `maxUnhealthy` threshold is reached for a targeted pool. This is similar to what the node life cycle controller does for reducing the eviction rate as nodes goes unhealthy in a given zone. E.g a large number of nodes in a single zone are down because it's most likely a networking issue.

Machine health checking is an integration point between node problem detection tooling and remediation to achieve node auto repairing.

### Unhealthy criteria:
A machine/node target is unhealthy when:

- The Node meets the unhealthy node conditions criteria.
- Machine has no nodeRef.
- Machine has nodeRef but node is not found.
- Machine is in phase "Failed".

If any of those criterias are met for longer than a given timeout, remediation is triggered.

### Remediation:
- The Machine is requested for deletion.
- MachineSet controller reconciles expected number of replicas brining up a new machine/node tuple.
- Machine controller drains node.
- Machine controller deletes machine.

### Implementation Details

#### MachineHealthCheck CRD:
- Enable watching a pool of machines (based on a label selector).
- Enable defining an unhealthy criteria (based on a list of node conditions).
- Enable setting a threshold for the number of unhealthy nodes. If the current number is at or above this threshold no further remediation will take place. This can be expressed as a percentage of the targeted pool.


E.g I want my worker machines belonging to example-machine-set to be remediated when the backed node has `ready=false` condition for more than 10m.

```yaml
apiVersion: healthchecking.openshift.io/v1alpha1
kind: MachineHealthCheck
metadata:
  name: example
  namespace: openshift-machine-api
spec:
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machineset: example-machine-set
  unhealthyConditions:
  - type:    "Ready"
    status:  "Unknown"
    timeout: "300s"
  - type:    "Ready"
    status:  "False"
    timeout: "300s"
  maxUnhealthy: "40%"
```

#### MachineHealthCheck controller:
- Watch MachineHealthCheck resources and operate over machine/node targets.
- Calculate the number of unhealthy targets.
- Compares current number against `maxUnhealthy` threshold and temporary short circuits remediation if the threshold is met.
- Triggers remediation for unhealthy targets i.e requests machines for deletion.
- MachineSet controller reconciles to meet number of replicas and bring up a new machine/node.
- Machine controller drains the unhealthy node.
- Machine is deleted.

#### Out of tree remediation controller, e.g baremetal reboot:
- An external remediation can plug in by setting the `healthchecking.openshift.io/strategy: reboot` on the MHC resource.
- An external remediation controller remediation could then watch machines annotated with `healthchecking.openshift.io/remediation: reboot` and react as it sees fit.

### Risks and Mitigations


## Design Details

### Test Plan
This feature will be tested for public clouds in the e2e machine API suite as the other machine API ecosystem https://github.com/openshift/cluster-api-actuator-pkg/tree/8250b456dec7b2fb06c591738518de1265e84a2c/pkg/e2e

### Graduation Criteria
An implementation of this feature is currently gated behind the `TechPreviewNoUpgrade` flag. This proposal wants to remove the gating flag and promote machine health check to a GA status with a beta API.

### Upgrade / Downgrade Strategy

The machine health check controller lives in the machine-api-operator image so the upgrades will be driven by the CVO which will fetch the right image version as usual. See:

https://github.com/openshift/machine-api-operator/blob/474e14e4965a8c5e6788417c851ccc7fad1acb3a/install/0000_30_machine-api-operator_01_images.configmap.yaml

https://github.com/openshift/machine-api-operator/blob/474e14e4965a8c5e6788417c851ccc7fad1acb3a/pkg/operator/operator.go#L222

For supported out of tree remediation controllers, e.g baremetal reboot, the binary needs to be deployed in one of the available bare metal images or specify its own one. See:

https://github.com/openshift/machine-api-operator/blob/master/pkg/operator/baremetal_pod.go

https://github.com/openshift/machine-api-operator/blob/474e14e4965a8c5e6788417c851ccc7fad1acb3a/pkg/operator/baremetal_pod.go

### Version Skew Strategy

## Implementation History

Initial implementation:

https://docs.google.com/document/d/1-RPiXfc33SyM7Gn-dogCWNigEWpSxXV_OZ7UKsQtI4E/edit#

Related discussions:

https://docs.google.com/document/d/10kauaJiXaWpvmd_qVsgIoZBNQLJMXMsQga3exV04ZTY/edit?ts=5d0b91e6#heading=h.4ifefbk4b4y2

https://gist.github.com/bison/403bb921e1d5ed72f7edec2ccb47471c#remediationstrategy

## Drawbacks

## Alternatives

Decoupling the administrator-defined short-circuiting logic.
Separate MachineDisruptionBudget resource. Similar to a PodDisruptionBudget, this resource simply targets a set of machines with a label selector and continuously records status information on the readiness of the targets. It includes thresholds similar to the maxUnavailable option above, and provides an at-a-glance view of whether the targets are at that limit or not.

Example:

```yaml
apiVersion: "healthchecking.openshift.io/v1beta1"
kind: "MachineDisruptionBudget"
metadata:
  name: workers-us-east-1a
  namespace: openshift-machine-api
spec:
  selector:
    role: worker
    zone: us-east-1a
  maxUnavailable: 5
```  

### Pros:

More expressive, i.e. the decision on whether to perform remediation can take into account the status of an arbitrary set of machines, including machines that may not qualify for automated remediation.
Could potentially be used by controllers other than health-checker.
Separates this bit of bookkeeping logic from the health-checker.

There's a feature gated version already.

### Cons:

Introduces a new type and controller.

### MachineDisruptionBudget use-cases:

I have a cluster spanning 3 zones in my data center. I want health-checking with remediation on all nodes. I want to skip remediation if a large number of nodes in a single zone are down because it's most likely a networking issue.

Solution: I create a MachineHealthChecker targeting all nodes, and I create a MachineDisruptionBudget for each zone with an appropriate maxUnavailable setting. To achieve the same effect without MachineDisruptionBudget I have to duplicate the MachineHealthChecker for each zone.

I want automated remediation on some nodes, but not all -- I may have "pet" machines that require special care. Whether remediation of machines should proceed or not depends on the health of my "pet" machines, i.e. do not remediate if a total of 5 or more machines are down across both the "cattle" and "pet" nodes.

Solution: Create a MachineDisruptionBudget targeting both the "cattle" and "pets" with appropriate thresholds. This is not possible without MachineDisruptionBudget or, possibly, one-off opt-out annotations.

## Infrastructure Needed
