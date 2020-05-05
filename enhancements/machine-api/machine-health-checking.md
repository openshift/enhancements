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

- Enable experimental remediation systems to be developed outside the machine-healthcheck controller via annotation, e.g baremetal reboot.
  - If a strong need for multiple external remediation systems ever emerge a follow up proposal fleshing out stronger integration details must be provided for this to be supported as non-experimetal.


### Non-Goals

- Coordinating individual remediation systems. This must be driven and designed by each system as they see fit, e.g. by annotating affected nodes with state and transition time. This proposal focus on remediating by deleting.

- Recording long-term stable history of all health-check failures or remediations.

## Proposal

- As a customer I only care about my apps availability so I want my cluster infrastructure to be self healing and the nodes to be remediated automatically.

- As a cluster admin I want to define my unhealthy criteria as a list of node conditions https://github.com/kubernetes/node-problem-detector.

- As a cluster admin I want to define different unhealthy criteria for pools of machines targeted for different workloads.

The machine health checker (MHC) does a best effort to keep nodes healthy in the cluster.

It provides a short-circuit mechanism and limits remediation when `maxUnhealthy` threshold is reached for a targeted pool. This is similar to what the node life cycle controller does for reducing the eviction rate as nodes goes unhealthy in a given zone. E.g a large number of nodes in a single zone are down because it's most likely a networking issue.

The machine health checker is an integration point between node problem detection tooling expresed as node conditions and remediation to achieve a node auto repairing feature.

### Unhealthy criteria:
A machine/node target is unhealthy when:

- The node meets the unhealthy node conditions criteria defined.
- The Machine has no nodeRef.
- The Machine has nodeRef but node is not found.
- The Machine is in phase "Failed".

If any of those criterias are met for longer than given timeouts, remediation is triggered.
For the node conditions the time outs are defined by the admin. For the other cases opinionated values can be assumed.
For a machine with no nodeRef an opinionated value could be assumed e.g 10 min.
For a node notFound or a failed machine, the machine is considerable unrecoverable, remediation can be triggered right away.

### Remediation:
- The machine is requested for deletion.
- The controller owning that machine, e.g machineSet reconciles towards the expected number of replicas and start the process to bring up a new machine/node tuple.
- The machine controller drains the node.
- The machine controller provider implementation deletes the cloud instance.
- The machine controller deletes the machine resource.

### Implementation Details

#### MachineHealthCheck CRD:
- Enable watching a pool of machines (based on a label selector).
- Enable defining an unhealthy node criteria (based on a list of node conditions).
- Enable setting a threshold of unhealthy nodes. If the current number is at or above this threshold no further remediation will take place. This can be expressed as an int or as a percentage of the total targets in the pool.

E.g:
- I want my worker machines to be remediated when the backed node has `ready=false` or `ready=Unknown` condition for more than 10m.
- I want remediation to temporary short-circuit if the 40% or more of the targets of this pool are unhealthy at the same time.


```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: example
  namespace: openshift-machine-api
spec:
  selector:
    matchLabels:
      role: worker
  unhealthyConditions:
  - type:    "Ready"
    status:  "Unknown"
    timeout: "300s"
  - type:    "Ready"
    status:  "False"
    timeout: "300s"
  maxUnhealthy: "40%"
status:
  currentHealthy: 5
  expectedMachines: 5
```

#### MachineHealthCheck controller:
Watch:
- Watch machineHealthCheck resources
- Watch machines and nodes with an event handler e.g controller runtime `EnqueueRequestsFromMapFunc` which returns machineHealthCheck resources.

Reconcile:
- Fetch all the machines in the pool and operate over machine/node targets. E.g:
```
type target struct {
  Machine capi.Machine
  Node    *corev1.Node
  MHC     capi.MachineHealthCheck
}
```

- Calculate the number of unhealthy targets.
- Compare current number against `maxUnhealthy` threshold and temporary short circuits remediation if the threshold is met.
- Trigger remediation for unhealthy targets i.e request machines for deletion.

Out of band:
- The owning controller e.g machineSet controller reconciles to meet number of replicas and start the process to bring up a new machine/node.
- The machine controller drains the unhealthy node.
- The machine controller provider deletes the instance.
- The machine controller deletes the machine.

![Machine health check](./mhc.svg)

#### Out of tree experimental remediation controller, e.g baremetal reboot:
- An external remediation can plug in by setting the `machine.openshift.io/remediation-strategy` on the MHC resource.
- An external remediation controller remediation could then watch machines annotated with `machine.openshift.io/remediation-strategy: external-baremetal` and react as it sees fit.

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

Considered to bake this functionality into machineSets. This was discarded as different controllers than a machineSet could be owning the targeted machines. For those cases as a user you still want to benefit from automated node remediation.

Considered allowing to target machineSets instead of using a label selector. This was discarded because of the reason above. Also there might be upper level controllers doing things that the MHC does not need to account for, e.g machineDeployment flipping machineSets for a rolling update. Therefore considering machines and label selectors as the fundamental operational entity results in a good and convenient level of flexibility and decoupling from other controllers.

Considered a more strict short-circuiting mechanisim (currently feature gated) decoupled from the machine health checker i.e machine disruption budget analogous to pod disruption budget. This was discarded because it added an non justified level of complexity and additional API CRDs. Instead we opt for a simpler approach and will consider more concrete feature requests that requires additional complexity based on real use feedback.
This would introduce a MachineDisruptionBudget resource. This resource simply targets a set of machines with a label selector and continuously records status information on the readiness of the targets. It includes thresholds similar to the maxUnavailable option above, and provides an at-a-glance view of whether the targets are at that limit or not.

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
