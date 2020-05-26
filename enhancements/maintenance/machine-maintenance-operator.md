---
title: machine-maintenance-operator
authors:
  - "@dofinn"
reviewers:
  - "@cblecker"
  - "@jharrington22"
  - "@jewzaam"
  - "@jeremyeder"
  - "@michaelgugino"
  - "@derekwaynecarr"
approvers:
  - "@michaelgugino"
  - "@derekwaynecarr"
  - "@bison"
  - "@enxebre"
creation-date: 2020-05-26
last-updated: 2020-05-26
status: provisional

# machine-maintenance-operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions
>1. How applicable is this use-case to baremetal installations? 

>2. The ability to declare maintenance windows is currently being discussed in OSD. Nothing has be design in concrete meaning that this proposal has an undesigned depedency. However develop could commence on this operator and have the maintenance functionality retrospectively fitted. Is this a blocker for its acceptenance?

>3. How applicable will this be to a cloud provider like GCP that performs live migrations of their VMs?

## Summary

This enhancement proposal explores the idea of having a machine-maintenance-operator(MMO) that acts as a machine 'nanny'. The MMO will actively search for scheduled maintenances for each infra/worker machine in the cluster along with validating the state of these machines against the cloud providers API. Upon finding a maintenance for a machine, the MMO will seek to execute this maintenance manually at the earliest convenient time as defined by an administrator. 

A POC of the idea can be found [here](https://github.com/dofinn/machine-maintenance-operator).

## Motivation

Currently the machine-api fails to accurately detect the state of machines (validated in AWS) when they have been terminated/stopped by the cloud provider. A provider may terminate/stop and instance when there is an associated maintenance scheduled for it. This is also the case for users manually terminating/stopping machines via the console (validated in AWS). The MMO is a proactive approach to first detecting maintenances and/or machine state from the cloud provider, then executing the required to enable the machine-api to continue managing machines and machinesets. This operator would work in conjunction with MachineHealthCheck implimentation. 

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?
The machine-maintenance-operator will:
* detect maintenances and execute them manually at their earliest and most convenient time
* validate the state of the machine against its machine CR and the cloud providers API
* validate the state of the machine behind a cloud providers load balancers and provide an alert upon recognition of a miss-match of state

### Non-Goals

This is not a solution for managing maintenances or machine state for master machine roles. 

## Proposal

### User Stories [optional]

#### Story 1
As a customer using OKD/OCP/OSD, I want my cluster/s to be proactive in handling events that are initiated by the cloud provider hosting my cluster. This will assure me that OKD/OCP/OSD accounts for machines in the cluster that require terminating or stopping to be proactively maintained and addressed with intention. 

#### Story 2
As an SRE supporting OSD, I want automated detection of cloud provider scheduled maintenance events. This will enable automated processes to proactively handle the event and execute the required with intent and validation, while also alerting me if required. 

As an SRE supporting OSD, I want automated validation of machine state as advertised by the machine-api against the cloud provider. This will enable automated processes to proactively handle the event and execute the required with intent and validation, while also alerting me if required. 

As an SRE supporting OSD, I want automated validation of machine state as advertised by the machine-api against the cloud providers load balancers (health checks). This will enable automated processes to proactively handle the event and execute the required with intent and validation, while also alerting me if required. 

### Implementation Details/Notes/Constraints [optional]

The MMO will utilize a machineWatcher that retrieves a machineList{} from the machine-api and iterates through each machine while performing each check (maintenance/state/LB health). Once the machineWatcher has detected any of these events, a machinemaintenance CR will be posted to the openshift-machine-maintenance-operator namespace to be reconciled by the machinemaintenance controller. This controller will validate the state of the cluster prior to performing any maintenance. For example; is the cluster upgrading? is the cluster already performing a maintenance?

These processes will only hold true for infra and worker roles of machines within the cluster. 

If a scheduled maintenance is detected for a master node, an alert should be raised.

Not every cloud provider has the same kind of maintenances that inherit that same degradation on the cluster.  Some of the features may need to be toggleable per provider. 

Example: (AWS maintenance)[https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/monitoring-instances-status-check_sched.html#types-of-scheduled-events] vs (GCP maintenance)[https://cloud.google.com/compute/docs/storing-retrieving-metadata#maintenanceevents].

### Risks and Mitigations

## Design Details

### maintenanceWatcher
The maintenanceWatcher is a go routine that currently cycles every 60 minutes. It retrieves a list of machines and iterates through them to check if each machine has an associated scheduled maintenance. If a maintenance is found for a machine, a machinemaintenance CR is created with the event type from that cloud provider for reconciliation by the machinemaintenance controller. 

For example, AWS events will result in the event type being either:
* instance-stop
* instance-retirement
* instance-reboot
* system-reboot
* system-maintenance

### stateWatcher
The stateWatcher is a go routine that currently cycles every 10 minutes. It retrieves a list of machines and iterates through them to validate the state of machine from the machine-api with that of the cloud provider. If miss-match in state is found, machinemaintenance CR is created with event type "machine-state-missmatch" for reconciliation by the machinemaintenance controller.

### lbWatcher
The lbWatcher is a go routine that currently cycles every 10 minutes. It retrieves a list of machines and iterates through them to validate the health of machine from the machine-api with that of the cloud providers load balancers. This will only be targeted at infra and master machines. A machinemaintenance CR will be created with event type "lb-unhealthy" if a host is unexpectedly found unhealthy.

### machinemaintenance controller
The machinemaintenance controller will reconcile all machinemaintenance CRs within its namespace. It will be responsible for first validating the state of the cluster prior to executing anything on a target object. Validating the state of the cluster will include will initially check for only is the cluster upgrading or is a maintenance already being performed. More use-cases can be added as seen fit. 

After cluster validation, the controller will ascertain if its in a maintenance window where is can execute maintenances (See open question 2). If in the case no maintenance windows are defined, the controller will continue as true. If the maintenance window logic is only applicable in OSD, the operator could validate if its "managed" prior to expecting these resources.

The event type is then source from the CR and then is resolved by either deleting a target machine CR (so the machine-api creates a new one) or raising an alert for manual intervention (master maintenance scheduled or unhealthy LB target). 

Upon deleting a machine CR, the machinemaintenance CR status will be updated to in-progress. After deletion is confirmed, the controller will query the cloud provider API with the event ID to confirm it is no longer valid before deleting the CR. 

### Example machinemaintenance CR

```
apiVersion: machinemaintenance.managed.openshift.io/v1alpha1
kind: MachineMaintenance
metadata:
  name: mm-dofinn-20201705-blz22-worker-ap-southeast-2a-984h4
  namespace: machine-maintenance-operator
spec:
  maintenancescheduled: false
  eventcode: "instance-stop"
  eventid: "instance-event-01d0903276a5d038c"
  machineid: "i-04fdc4494938e5c4a"
  machinelink: "dofinn-20201705-blz22-worker-ap-southeast-2a-984h4"
status:
  maintenance: "in-progress"
```

### Alerting
An alert will be raised for the following conditions:

* Unable to schedule maintenance during customer defined window and prior to cloud provider maintenance deadline
* Post maintenance verification failed
* Node does not return after shutdown
* Node unable to drain
* Node unable to shutdown
* Same event still scheduled for machine after performing maintenance
* Maintenance taking longer then X minutes.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

The operator should follow (OLM)[https://github.com/operator-framework/operator-lifecycle-manager] for lifecycle management. 

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

It is not applicable to baremetal installations. 

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
