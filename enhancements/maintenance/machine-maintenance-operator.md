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
last-updated: 2020-05-28
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

>2. The ability to declare maintenance windows is currently being discussed in OSD. Nothing has been designed in concrete yet, meaning that this proposal has an undesigned dependency. However, development could commence on this operator and have the maintenance functionality retroactively fitted. Is this a blocker for its acceptance?

>3. How applicable will this be to a cloud provider like GCP that performs live migrations of their VMs?

## Summary

This enhancement proposal explores the idea of having a machine-maintenance-operator(MMO) that will inspect each machine CR for scheduled events for each infra/worker machine in the cluster. Upon finding a maintenance for a machine, the MMO will seek to execute this maintenance manually at the earliest convenient time as defined by an administrator. 

A POC of the idea can be found [here](https://github.com/dofinn/machine-maintenance-operator).

## Motivation

Currently the machine-api marks an machine as stopped when they have been terminated/stopped by the cloud provider. A provider may terminate/stop and instance when there is an associated maintenance scheduled for it. This is also the case for users manually terminating/stopping machines via the console. The MMO is a proactive approach to executing the required (delete target machine CR)to enable the machine-api to manage machinesets. This operator would work in conjunction with MachineHealthCheck implimentation. 

## Requirements
That the machine-api collects schedules maintenaces from cloud providers and posts them in the status of the each machines CR

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?
The machine-maintenance-operator will:
* detect maintenances from machine CRs and execute them manually at their earliest and most convenient time

### Non-Goals

This is not a solution for managing maintenances or machine state for master machine roles. 

## Proposal

### User Stories [optional]

#### Story 1
As a customer using OKD/OCP/OSD, I want my cluster/s to be proactive in handling events that are initiated by the cloud provider hosting my cluster. This will assure me that OKD/OCP/OSD accounts for machines in the cluster that require terminating or stopping to be proactively maintained and addressed with intention. 

### Implementation Details/Notes/Constraints [optional]

Constraints: 
* This implimentation will require the machine-api to query cloud providers for scheduled maintenances and publish them in the machines CR. 
* GCP only allows maintenances to be queried from the node itself -> `curl http://metadata.google.internal/computeMetadata/v1/instance/maintenance-event -H "Metadata-Flavor: Google"`

This operator will iterate through the machineList{} and inspect each machine CR for scheduled maintenances. If a maintenance is found, the controller will validate the state of the cluster prior to performing any maintenance. For example; is the cluster upgrading? is the cluster already performing a maintenance?

These processes will only hold true for infra and worker roles of machines within the cluster. 

If a scheduled maintenance is detected for a master node, an alert should be raised.

Not every cloud provider has the same kind of maintenances that inherit that same degradation on the cluster.  Some of the features may need to be toggleable per provider. 

Example: (AWS maintenance)[https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/monitoring-instances-status-check_sched.html#types-of-scheduled-events] vs (GCP maintenance)[https://cloud.google.com/compute/docs/storing-retrieving-metadata#maintenanceevents].


### Risks and Mitigations

## Design Details

AWS event types:
* instance-stop
* instance-retirement
* instance-reboot
* system-reboot
* system-maintenance

### machinemaintenance controller
The machinemaintenance controller will iterate through machine CRs and reconcile identified mainteances. It will be responsible for first validating the state of the cluster prior to executing anything on a target object. Validating the state of the cluster will include will initially check for only is the cluster upgrading or is a maintenance already being performed. More use-cases can be added as seen fit. 

If the cluster fails validation (for example is upgrading), the controller will requeue the object and process it again according to its `SyncPeriod` which would currently be proposed at 60 minutes. 

After cluster validation, the controller will ascertain if its in a maintenance window where is can execute maintenances (See open question 2). If in the case no maintenance windows are defined, the controller will continue as true. If the maintenance window logic is only applicable in OSD, the operator could validate if its "managed" prior to expecting these resources.

The event type is then sourced from the CR and then is resolved by either deleting a target machine CR (so the machine-api creates a new one) or raising an alert for manual intervention (master maintenance scheduled). 

This is a very UDP type of approach. 

The MMo could also look to store state of its actions in its on machinemaintenance CR that it would create from a machine CR. 

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

The MMO would then delete this CR after validating the target machine has been deleted and new one created indicating that the maintenance is completed. 

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
