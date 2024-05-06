---
title: admin-defined-node-disruption-policy
authors:
  - "@yuqi-zhang"
reviewers:
  - "@cgwalters"
approvers:
  - "@djoshy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-02-08
last-updated: 2024-02-08
tracking-link:
  - https://issues.redhat.com/browse/RFE-4079
see-also:
  - https://issues.redhat.com/browse/OCPSTRAT-380
  - https://issues.redhat.com/browse/MCO-507
  - https://github.com/openshift/api/pull/1764
replaces:
superseded-by:
---

# Admin Defined Node Disruption Policy

## Summary

This enhancement outlines an API/mechanism for users to define what actions to take upon a MachineConfigOperator driven change (e.g. a file change via a MachineConfig object). By default, all changes to MachineConfig fields require a drain and reboot. The user can use this new API, a Node Disruption Policy subfield in the MachineConfiguration object, to specify which MachineConfig changes to not disrupt their workloads at a cluster scope.

## Motivation

Historically, workload disruption (drains and reboots) can be very costly for OCP users, especially in bare-metal environments where the stopping and restarting of a node can take up to an hour. For small incremental changes to the host OS, many of them can take effect with a service reload or no action at all, and thus the ability to perform a non-disruptive update can be very beneficial to many users without any downside.                  

The MCO since OCP 4.7 has added the ability to perform disruptionless updates on select changes, see: [MCO doc on disruptionless updates](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigDaemon.md#rebootless-updates). This list however is by no means exhaustive, and it is not feasible for the MCO to hard-code all use cases for all users into the MCO. In addition, different users may have different expectations on what will cause a disruption (drains and reboots) or not. Thus there is a need for a user-defined, MCO operated list of "node disruption policies" on what to do after an OS update has been applied.

### User Stories

* As an Openshift cluster administrator, I have an environment with thousands of ICSP/IDMS mirror entries. These entries will be added and deleted constantly on a weekly basis. I understand the potential dangers of removing mirrors (can cause an image to no longer be accessible if no other mirror hosts it), but I accept that risk and would like my changes to apply without disrupting my workloads and only by reloading the crio service.

* As an application developer, I have certificates for my applications I need on-disk, which can be rotated safely and hot-loaded into the applications without any additional action. I do not wish to disrupt other applications when these update via the MCO.

### Goals

* Create a NodeDisruptionPolicyConfig subfield in the MachineConfiguration CRD to allow users to define what actions to take for minor MachineConfig updates
* Have users be able to define no-action and service reloads to specific MachineConfig changes
* Have users be able to easily see existing cluster non-disruptive update cases

### Non-Goals

* Have NodeDisruptionPolicy apply to non-MCO driven changes (e.g. SRIOV can still reboot nodes)
* Remove existing non-disruptive update paths (the user will be able to override cluster defaults)
* Design for image-based updates (live apply and bootc, will be considered in the future)
* Have the MCO validate whether a change can be successfully applied with the given NodeDisruptionPolicy (i.e. it is up to the responsibility of the user to ensure the correctness of their defined actions)

## Proposal

Create NodeDisruptionPolicy and NodeDisruptionPolicyStatus in the machineconfigurations.operator.openshift.io CRD, allowing specify user-provided NodeDisruptionPolicy, and view current version cluster defaults, as follows:

```console
apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
spec:
  nodeDisruptionPolicy:
    files:
      - path: "/etc/my-file"
        actions:
          - type: Reload
            reload: 
              serviceName: my.service
          - type: DaemonReload
status:
  nodeDisruptionPolicyStatus:
    files:
      - path: "/etc/my-file"
        actions:
          - type: Reload
            reload:
              serviceName: my.service
          - type: DaemonReload
      - path: /etc/mco/internal-registry-pull-secret.json
          actions:
            - type: None
        - path: /var/lib/kubelet/config.json
          actions:
            - type: None
        - path: /etc/machine-config-daemon/no-reboot/containers-gpg.pub
          actions:
            - type: Reload
              reload:
                serviceName: crio.service
        - path: /etc/containers/policy.json
          actions:
            - type: Reload
              reload:
                serviceName: crio.service
        - path: /etc/containers/registries.conf
          actions:
            - type: Special
    sshkeys:
        - actions:
          type: None
```

Which, when applied, will define actions for all future changes to these MachineConfig fields based on the diff of the update (rendered MachineConfigs).

For this initial implementation we will support MachineConfig changes to:
 - Files
 - Units
 - sshKeys

And actions for:
 - None
 - Draining a node
 - Reloading a service
 - Restarting a service
 - Daemon-reload and restarting of a service
 - Reboot (default)
 - Special (see workflow description)

In the future, we can extend this in a few ways:
 - allowing for directories, and other wildcarding
 - supporting more changes/actions (e.g. user defined script to run after a certain change)
 - supporting diffing between ostree images (for image based updates) and bootc managed systems (for bifrost)


### Workflow Description

An admin of an OCP cluster can at any time create/update/delete nodeDisruptionPolicy fields in the MachineConfiguration object to reconfigure how they wish updates to be applied. There isn’t any immediate effect so long as the CR itself is valid.

The MachineConfigOperator pod will process any update to the object and validate the NodeDisruptionPolicy for potential issues, although currently all error cases are covered by API-level validation. The MCO pod will then merge the user provided contents to the cluster defaults (user contents will always override cluster defaults) and populate the nodeDisruptionPolicyStatus.

The MachineConfigDaemon will process and apply the current status.nodeDisruptionPolicyStatus, which holds the previous validated object. The spec changes will be ignored until validated and populated by the controller.

When the user first enables the new nodeDisruptionPolicySpec via featuregate (or at some version, by default), the "default" MachineConfiguration object will be generated with the existing cluster defaults by the MCO into the status, so the user can also view the default policies that came with the cluster. The user can set their own spec for any additional policies they would like to take effect.

Note that /etc/containers/registries.conf has a “Special” keyword. This is due to the extra processing the MCO does today. No user defined policy may use the “Special” keyword, as it is a status only field.

User-provided changes will always overwrite the cluster defaults, reflected in the status.

The special MCO cert paths ("/etc/kubernetes/kubelet-ca.crt", "/etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem") are not listed in the clusterDefaultPolicies as they are no longer MachineConfig changes.

In terms of the policy itself, in case a change has multiple matching policies (e.g. two files changed, both with a different policy), all policies will be applied in order (e.g. reload multiple services). Any invalid policy, such as invalid actions or duplicate entries, will be rejected at the API level.

The MachineConfigNodes CR will later be enhanced to track the status of the upgrade, and will also be updated to indicate what the NodeDisruptionPolicy used for the update was.

NodeDisruptionPolicies are only processed in-cluster and cannot be used to skip the bootstrap pivot during cluster installation.

#### Variation and form factor considerations [optional]

This will work by default on standalone OCP and any other OCP variants that run the MCO in its entirety. (e.g. works for RHEL)

HyperShift deployments do not run the MCO pods directly and will not have MachineConfiguration objects.

### API Extensions

- Adds fields to the MachineConfiguration CRD. See: https://github.com/openshift/api/pull/1764

### Implementation Details/Notes/Constraints [optional]

This aims more to be a user-facing API to essentially map MachineConfig fields to what action should be taken on them, which aligns with existing MCO functionality to perform a hard-coded list of these policies. However, as we evolve the MCO to fit in with an image based OS update vision, we would need this to work with:
 - Existing MachineConfig based entries
 - User-provided base-image upgrade cases (live-apply)
 - A more unified update interface with bootc

Which we can definitely translate under-the-hood to adapt to the system, but file/unit specific entries like this may not work as effectively when it comes to image-to-image updates, especially at the daemon level.

As there is a need for this in 4.16, we will proceed with the format that works best with the existing MCO architecture, while leaving it open to change for the future.

#### Hypershift [optional]

As noted above, Hypershift cannot directly tap into this. In a bit more detail, Hypershift has 2 modes of operation for nodepools:

- Replace upgrades: since all machines are deleted and recreated in this model, there is no method of live-application today.
- Inplace upgrades: these actually use a special form of the daemon, and we can leverage that to update and perform necessary policies.

However we probably do not want hosted cluster admins to have direct control over this, much like MachineConfigs, so if we do want to add this we will need a new API in Hypershift to handle this. As such, we are not considering this at this time

### Risks and Mitigations

As noted in the non-goal sections, any NodeDisruptionPolicy is not verified for application correctness in the MCO, minus formatting. This means that if you accidentally define a change that needed a reboot to take effect, the cluster would “finish the update” without being able to use the new configuration.

The user thereby needs to understand that no additional verification is taking place, and that they accept any responsibility for NodeDisruptionPolicies. If any are applied to the cluster, a metric should be raised for debuggability.

The MCO rebootless apply and reboot apply actually does have 1 major difference still: the application of proper selinux labels. The MCO doesn’t currently have any ability to properly label files it writes, and some users have worked around this by relabelling via a service, which would trigger during the early boot process. With this, we will break that workflow unless the MCO explicitly has the ability to handle selinux labels.

### Drawbacks

There shouldn’t be many drawbacks (this is a pretty lightweight sub-feature that we already in-part ship today) other than what was outlined in Risks and Mitigations.

## Design Details

### Open Questions [optional]

The biggest question is the format and constraints we would like for the UX to be. Conceivably, we have the options to:

 - Allow users to wildcard paths
 - Allow users to define policies per pool instead of globally
 - Provide users more flexibility in defining the actions (e.g. provide a shell script to run to change selinux context)
 - Provide users more granularity on changes within a file (e.g. adding vs deleting image mirrors within registries.conf)

This current outline is a bit more restrictive in UX but aims to be a bit more "safe" in the actions that can be taken.

The question of image based updates is also important, but discussed in the above Implementation Details section.

Selinux labels are discussed in risks and mitigations.

### Test Plan

The MCO e2e tests will cover functionality for checking if the policy has taken effect.

### Graduation Criteria

We will add this behind a featuregate during the 4.16 timeframe. This at most only needs a tech preview phase, and then can be GA’ed since the feature is relatively isolated.

#### Dev Preview -> Tech Preview

- Having CI testing for the feature

#### Tech Preview -> GA

- Obtain feedback based on existing UX (skipping image regstry drains)

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

- Deprecate the "non-disruptive image registry changes configmap" we have put in place since 4.14, as a temporary workaround

#### Removing a deprecated feature



### Upgrade / Downgrade Strategy



### Version Skew Strategy



### Operational Aspects of API Extensions


#### Failure Modes



#### Support Procedures


## Implementation History


## Alternatives

#### A new NodeDisruptionPolicy CRD

This is essentially the same mechanism as outlined above, except instead of building  it into a subset of MachineConfiguration object's spec and status, a new NodeDisruptionPolicy object is created instead.

Benefits:
1. more flexibility and extensibility in the future, and can have per-pool selectors, etc. to allow for more granular application

Downsides:
1. requires a new CRD and API object

#### Per MachineConfig object annotation

Instead of having a new API object, we allow users to add annotations to existing MachineConfig objects in the cluster. When those object changes, the corresponding action is applied.

Benefits:
1. does not require any API changes
1. integrates better with how bootc envisions non-reboot updates

Downsides:
1. this does not fit with existing MCO architecture in two ways:
    * most templated machineconfigs (that ship with the cluster) ships as one giant 00-master/00-worker config, which won't have the same level of nuance
    * the daemons (performing the actual update) does the action calculation, and does it based on rendered config diffs instead of individual MachineConfigs
  These two issues can be solved in the future but will not allow much flexibility with use cases today
1. users can specify the same MachineConfig fields in multiple MachineConfigs. The merge logic will get complex with multiple fields defined in the same file.

#### Manual control

We could ship a general flag for 4.16 that allows users to take over for the MCO and manually perform any necessary actions on the nodes directly, and implement a better method of this after image-based MCO is implemented. This might not be a very safe design, though.

## Infrastructure Needed [optional]



