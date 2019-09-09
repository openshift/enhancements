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

Enable opt in for automated health checking and remediation of unhealthy nodes backed by machines.

## Motivation

Reduce administrative overhead to run a cluster and ability to respond to failures of machines.

### Goals

- Providing a default remediation strategy for public clouds - Machine deletion.

- Allowing to define different unhealthy criterias for different pools of machines.

- Allowing new and custom remediation strategies developed outside the machine-healthcheck controller, e.g baremetal reboot.

- Providing administrator-defined short-circuiting of automated remediation.


### Non-Goals

- Coordinating individual remediation systems. This must be driven by each system however they see fit, e.g. by annotating affected nodes with state and transition time.

- Recording long-term stable history of all health-check failures or remediations.

## Proposal

- As a customer I only care about my apps availability so I want my cluster infrastructure to be self healing and the nodes to be remediated automatically.

- As a cluster admin I want to define my unhealthy criteria as a list of node conditions https://github.com/kubernetes/node-problem-detector.

- As a cluster admin I want to define different unhealthy criteria for pools of machines targeted for different workloads.

- As a cluster admin I want to define different remediation strategies for different cloud environments. E.g deletion or reboot.

- As a cluster admin I want to skip remediation to optionally prevent automated remediation if some threshold of nodes are unhealthy, because this situation may require more careful intervention. E.g a large number of nodes in a single zone are down because it's most likely a networking issue.

Machine health checking is an integration point between node problem detection tooling and remediation strategies.

We will provide a default remediation strategy - delete. It will delete a machine when the backed node meets the unhealthy criteria defined by the user. The upper level machine controller, e.g machineSet will then reconcile towards satisfying the expected number of replicas and will create a new one.

### Implementation Details

#### MachineHealthCheck CRD:
- Enable watching a pool of machines (based on a label selector).
- Enable defining an unhealthy criteria (based on a list of node conditions).
- Enable choosing a remediation strategy.
- Enable setting a threshold for the number of unhealthy nodes. If the current number is at or above this threshold no further remediation 


E.g I want my worker machines in us-east-1a to be remediated when the backed node has `ready=false` condition for more than 10m.

```yaml
apiVersion: "healthchecking.openshift.io/v1beta1"
kind: "MachineHealthCheck"
metadata:
  name: workers-us-east-1a
  namespace: openshift-machine-api
spec:
  selector:
    role: worker
    zone: us-east-1a
  remediationStrategy: delete
  unhealthyConditions:
    - name: Ready
      status: false
      timeout: 10m
  maxUnavailable: 5
status:
  ExpectedHealthy: 10 // Total number of machines targeted by the label selector
  CurrentHealthy: 10
```

TODO: how to define OR relation between unhealthyConditions, e.g `ready=false || ready=unknown`

#### MachineHealthCheck controller:
- Watch MachineHealthCheck resources and nodes.
- Find machines backing nodes that meet the unhealthy criteria and signal remediaton systems with the chosen remediation strategy, e.g `healthchecking.openshift.io/remediation: reboot`.
- If the remediation is "delete" then it runs the remediation logic and set the machine for deletion. 

#### Out of tree remediation controller, e.g baremetal reboot:
- Watch machines signaled for remediation with `healthchecking.openshift.io/remediation: reboot` and react as they need.

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
