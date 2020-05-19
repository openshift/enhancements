---
title: opt-out-machine-health-checking
authors:
  - "@JoelSpeed"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-05-18
last-updated: 2020-05-18
status: implementable
see-also:
  - "/enhancements/machine-api/machine-health-checking.md"  
---

# Opt-Out Machine Health Checking

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal explores the option to install Machine Health Checking by default within new OpenShift clusters.
This would allow clusters to automatically heal themselves when machines become unhealthy.

## Motivation

Improve the user experience of OpenShift by automatically healing Machines without requiring extra configuration from users.

### Goals

- Provide a sensible default MachineHealthCheck on new OpenShift clusters
- Allow worker Machines to recover automatically
- Allow users to opt-out of health checking

### Non-Goals

- Force MachineHealthChecks to always be present on clusters
- Provide MachineHealthChecks as part of upgrade paths

## Proposal

On cluster creation, the installer should create a MachineHealthCheck that targets all MachineSets created by the installer.
It will not target Control-Plane Machines created by the installer.

This HealthCheck will mark machines unhealthy based on the `Ready` condition on the Machine's Node.

It will also specify a sensible default for the `maxUnhealthy` field which will prevent remediation of unhealthy
Machines once a certain percentage of the cluster is considered unhealthy.

Should cluster administrators not be happy with the MachineHealthCheck, they will be free to modify or delete it as they wish.

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: machine-unready-5m
  namespace: openshift-machine-api
spec:
  nodeStartupTimeout: 10m # This is the default value
  maxUnhealthy: 40%
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: worker
      machine.openshift.io/cluster-api-machine-type: worker
    matchExpressions:
    - key: machine.openshift.io/cluster-api-machineset
      operator: In
      values:
      - <machineset-name-a>
      - <machineset-name-b>
      - <machineset-name-c>
  unhealthyConditions:	 
  - type:    Ready
    timeout: 300s
    status: "False"
  - type:    Ready
    timeout: 300s
    status: Unknown
```

### Implementation Details

A new MachineHealthCheck manifest will be added to the installer resources.
This MachineHealthCheck should **not** be managed by the Cluster Version Operator,
this would prevent users from adjusting it to suit their particular needs.

### Risks and Mitigations

#### MachineHealthCheck and Cluster Autoscaler interactions

Both the MachineHealthCheck and Cluster Autoscaler implement health checking for Machines in different ways.
The Cluster Autoscaler attempts to determine the health of MachineSets by looking at whether Machines are successfully
joining the cluster within a given period.

With MachineHealthChecks installed, they may intefere with the Cluster Autoscalers health check logic.
See [How does the ClusterAutoscaler interact with MachineHealthCheck](#how-does-th-clusterautoscaler-interact-with-machinehealthcheck)
for details of how they interfere.

Before a MachineHealthCheck can be recommended by default, the MachineHealthCheck controller should
[implement back off](https://issues.redhat.com/browse/OCPCLOUD-800) to prevent hot loops on broken user configuration.
Once this is implemented, it should be safe to use MachineHealthChecks and Cluster Autoscaler together.

## Design Details

### Test Plan

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

### Deploy a MachineHealthCheck for all non Control-Plane Machines

Alternatively to deploying a MachineHealthCheck that works for installer created MachineSets,
a MachineHealthCheck that targets all non Control-Plane Machines could be deployed instead.

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: worker-unready-5m
  namespace: openshift-machine-api
spec:
  selector:
    matchExpression:
    - key: machine.openshift.io/cluster-api-machine-type
      operator:  notin
      values:
      - master
  maxUnhealthy: 40%
  unhealthyConditions:
  - type: Ready
    status: "False"
    timeout: 300s
  - type: Ready
    status: "Unknown"
    timeout: 300s
```

The drawback of this approach is that it will include all user created MachineSets as well as the defaults and would
likely not have appropriate granularity for the `maxUnhealthy` protection described in the
[Related Information](#how-does-the-maxunhealthy-field-work) section.
Users may want MachineHealthChecks to target smaller groups of Machines with particular special features (eg GPUs),
instead of having a single blanket policy.

## Related Information

### What do Machines/MachineSets in a default OpenShift cluster look like?

By default, in an OpenShift IPI installation, a MachineSet is created for each availability zone in the region that the
cluster is deployed into, eg, in AWS US-East-1, 6 MachineSets are created.
The installer will set the replica count to 1 for the first 3 MachineSets it creates and zero otherwise.

These MachineSets are intended to create generic worker Machines are are all identical apart from the availability
zone in which they are deployed.

Each of theses MachineSets will apply 4 labels to the Machines that it creates:
```yaml
machine.openshift.io/cluster-api-cluster: <cluster-name>
machine.openshift.io/cluster-api-machine-role: worker
machine.openshift.io/cluster-api-machine-type: worker
machine.openshift.io/cluster-api-machineset: <cluster-name>-worker-<availability-zone>
```

In addition to any Machines created by the MachineSets, the installer creates 3 Control-Plane Machines.
This means that typically a cluster will have 6 Machines present at time of install.

The Control-Plane machines are labelled:
```yaml
machine.openshift.io/cluster-api-cluster: <cluster-name>
machine.openshift.io/cluster-api-machine-role: master
machine.openshift.io/cluster-api-machine-type: master
```

Additionally all Machines are labelled with the following three labels:
```yaml
machine.openshift.io/instance-type: <instance-type>
machine.openshift.io/region: <availability-zone>
machine.openshift.io/zone: <availability-zone>
```

The labels applied to Machines are important as they allow users to determine which Machines are targeted by a Machine Health Check.

### How does a Machine Health Check determine which Machines it should target for health checking?

A MachineHealthCheck uses a `selector` to specify which Machines it should consider as its targets.
The selector supports [set-based requirements](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#resources-that-support-set-based-requirements)
so there are three options for specifying a selector for a Machine Health Check.

#### An Empty Selector

An empty selector matches all Machines. This means it would include the Control-Plane Machines,
all Machines created by the default MachineSets and any Machines that users create post installation.

#### Using exact labels

Using the `matchLabels` part of the selector allows specifying a list of key value pairs to check the presence of.
This means that Machines must have all of the labels specified to be considered to match the selector.
This could be used to specify that the MHC selects all Machines with the `cluster-api-machine-role: worker` label,
which would match all MachineSets created by the installer, but not Control-Plane nodes,
nor necessarily any MachineSets created by users post installation.

#### Using expressions

Using the `matchExpressions` part of the selector allows more complicated expressions to be created for selecting on labels.
Each expression consists of a key, operator and values.
This allows users to select a label with value that matches a set of values (operator: In),
a label with value that is not in a set of values (operator: NotIn). Or check that a label exists or not.

These expressions could be used to select all Machines that do not have the label `cluster-api-machine-role: master`,
ie, all Machines in the cluster that are not part of the Control-Plane.
This would include all MachineSets created by the installer and any created by users post installation.

### How does the ClusterAutoscaler interact with MachineHealthCheck?

One of the main concerns about installing a MachineHealthCheck by default is how it might interact with the ClusterAutoscaler.
They both monitor Machine’s and both have health checking built into them.

The ClusterAutoscaler uses its own health checking to determine if a MachineSet is unhealthy after scaling up.
Should it determine the MachineSet to be unhealthy,
it would scale the MachineSet back down and try another MachineSet to add new capacity to the cluster.

A concern is that MachineHealthChecks could interfere with this health checking logic and prevent the autoscaler from
adding new compute capacity. To ensure there are no conflicts,
several scenarios must be tested to check whether there may be conflicts

#### Scenarios
##### Machine fails to launch due to user config error

Steps to reproduce:
- Create cluster on AWS
- Deploy ClusterAutoscaler to cluster
- Deploy MachineHealthCheck to cluster
- Modify MachineSet to start on Spot instances with very low spot price
  ```yaml
  providerSpec:
    value:
      ...
      spotMarketOptions:
        maxPrice: "0.001" # Low to prevent EC2 instance being created
  ```
- Create deployment of dummy pods with some resources requested
- Scale deployment until ClusterAutoscaler scales modified MachineSet

What happens after Machine Fails:
- If the number of Failed Machines is greater than `maxUnhealthy`:
  - Example:
    - This may happen if a large number of nodes are scaled in a single MachineSet and they all fail to provision
      (eg AMI missing, or instance type not available in AZ)
	- MachineHealthCheck:
    - Remediation is short circuited, takes not action
  - Cluster Autoscaler:
    - Autoscaler determines nodes are unregistered
    - After 15 minutes (MaxNodeProvisionTime) autoscaler scales MachineSets back as expected and re-attempts scaling
	- Conclusion:
    - In this scenario, the MHC has no effect on the operation of the Cluster Autoscaler

- If the number of Failed Machine is less than `maxUnhealthy`:
  - Example:
    - This may happen under normal scaling, if a low number of machines are scaled and Fail,
      or if a number of machines are scaled, some Fail and the others eventually register
	- MachineHealthCheck:
    - While number of unhealthy machines exceed `maxUnhealthy`, no remediation occurs
    - Once `maxUnhealthy` not breached, hot loop of deletion and recreation of Machines caused by MachineHealthCheck
      immediately remediating failed Machines and MachineSet recreating Machines
	- Cluster Autoscaler:
    - Cannot track health of MachineSet as Machines are being deleted too quickly
    - Unable to determine broken MachineSet is unhealthy, therefore unable to attempt to scale alternate MachineSet as designed
	- Conclusion:
    - MachineHealthCheck breaks the assumptions of the Cluster Autoscaler and as such the Cluster Autoscaler cannot
      determine that the MachineSet is currently unhealthy
    - New capacity may never be added to the cluster if the Cluster Autoscaler choses a broken MachineSet,
      therefore the Cluster Autoscaler is broken by MachineHealthCheck at presents
    - This problem could potentially be resolved by introducing back-off for the MachineHealthCheck remediation controller
      (https://issues.redhat.com/browse/OCPCLOUD-800)
    - Alternatively this could be resolved by not remediating machines that have `Failed` but this would be a major change in behaviour

##### Machine fails to launch after being accepted by AWS

Steps to reproduce:
- Create cluster on AWS
- Deploy ClusterAutoscaler to cluster
- Deploy MachineHealthCheck to cluster
- Disable CVO
- Remove the following section from the `openshift-machine-api-aws` credentials request
  - `kubectl edit credentialsrequests -n openshift-cloud-credential-operator openshift-machine-api-aws`
 ```yaml
  - action:
    - kms:Decrypt
    - kms:Encrypt
    - kms:GenerateDataKey
    - kms:GenerateDataKeyWithoutPlainText
    - kms:DescribeKey
    effect: Allow
    resource: '*'
  - action:
    - kms:RevokeGrant
    - kms:CreateGrant
    - kms:ListGrants
    effect: Allow
    policyCondition:
      Bool:
        kms:GrantIsForAWSResource: true
    resource: '*'
  ```
- Create a KMS key and set a MachineSet to use this key to encrypt disks using this key
- Create deployment of dummy pods with some resources requested
- Scale Deployment until ClusterAutoscaler scales modified MachineSet

What happens:
- Example:
  - Credentials do not give permission to use KMS key given for decrypting EBS volume. AWS accepts instance request but fails to launch.
- Result:
  - AWS accepts instance launch request, returns provider ID
  - Machine controller updates Machine to Provisioned
  - AWS attempts to launch instance, fails, puts in terminated state
  - Machine controller marks Machine as Failed because instance is terminated
- Conclusion:
  - This scenario behaves in exactly the same way as the previous scenario

### How does a Machine Health Check interact with Control-Plane Machine?

At present, Machine Health Checks will not remediate Control-Plane machines.
Since Control-Plane Machines are not owned by any controller, deleting Control-Plane machines would have to be recreated manually.

Until automation is created (see [proposal](https://github.com/openshift/enhancements/pull/292))
for management of Control-Plane machines, they should be excluded from Machine Health Checks as health checking them will be redundant.

Once automation is created, a Machine Health Check could be used to remediate problems with the Control-Plane machines,
though it must ensure that the Etcd cluster remains healthy and as such,
should be managed by something to ensure the MaxUnhealthy parameter is set appropriately for the size of the Etcd cluster.

### How does the MaxUnhealthy field work?

Each MachineHealthCheck has a `maxUnhealthy` field.
This field can be used to prevent the MachineHealthCheck from remediating problems with Machines if there are
already too many unhealthy Machines within the cluster.

For example, if the value of `maxUnhealthy` was `50%`, and in a 10 node cluster, 6 nodes were determined to be unhealthy,
the `maxUnhealthy` value would prevent the MachineHealthCheck from making any remediation actions on the Machines.
This mechanism prevents the MachineHealthCheck from potentially worsening a catastrophic or cascading failure with a cluster.

An important note is that currently, the `maxUnhealthy` value is only respected for the single MachineHealthCheck that is being reconciled.
This means that, if two MachineHealthChecks covered the same Machine or Machines, one of the MachineHealthChecks could have
a less restrictive `maxUnhealthy` and allow a Machine to be remediated when the other MachineHealthCheck would have blocked remediation.

This means that, if a MachineHealthCheck covered all nodes, the value will include the Control-Plane machines
and they will be factored into the calculation to determine if MaxUnhealthy has been breached.
Because of the special nature of Control-Plane Machines, including them in an MHC with other nodes that would potentially
allow the MHC to remediate more than one Control-Plane machine at any one time (assuming three Control-Plane machines),
could cause the cluster to lose quorum for Etcd and potentially even lose the data altogether.

If Control-Plane Machines are to be covered by a MachineHealthCheck, the `maxUnhealthy` parameter must be set
appropriately to ensure that the MachineHealthCheck does not ever cause the Etcd cluster to lose quorum.
For this reason, Control-Plane Machines should not be mixed with worker Machines within an MHC
(Note however, iff the `maxUnhealthy` value were forced to be 1 Machine only, then this would be safe).
