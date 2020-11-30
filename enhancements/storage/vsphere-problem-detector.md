---
title: vsphere-problem-detector
authors:
  - "@jsafrane"
reviewers:
  - "@hekumar"
  - "@jspeed"
  - "@jcpowermac"
  - "@staebler"
approvers:
  - TBD
creation-date: 2020-11-10
last-updated: 2020-11-13
status: provisional
see-also:
replaces:
superseded-by:
---

# vSphere monitoring for OCP clusters

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Below we propose to check if OpenShift clusters installed into vSphere environment is / can be fully functional by introducing a new operator and periodical checks.

## Motivation

Our support frequently faces OCP clusters on vSphere that is either mis-configured or uses unsupported features like storage migration.

* OCP lacks necessary permissions: [#1869035](https://bugzilla.redhat.com/show_bug.cgi?id=1869035) [#1879959](https://bugzilla.redhat.com/show_bug.cgi?id=1879959). [#1852304](https://bugzilla.redhat.com/show_bug.cgi?id=1852304).
* Datastore used in storage classes / OCP config is too long: [#1884202](https://bugzilla.redhat.com/show_bug.cgi?id=1884202).
* Users don't enable `disk.enableUUID=TRUE`: [#1434709](https://bugzilla.redhat.com/show_bug.cgi?id=1434709) [1850984](https://bugzilla.redhat.com/show_bug.cgi?id=1850984) [1727991](https://bugzilla.redhat.com/show_bug.cgi?id=1727991).
* Kubelet is configured without `--cloud-provider=vsphere`: [1632729](https://bugzilla.redhat.com/show_bug.cgi?id=1632729). This was frequent issue in 3.x, in 4.x the installer seems to set everything right. Still, this check should be very cheap.
* Storage Distributed Resource Scheduler (DRS) is not supported.
* vMotion (i.e. VM migration) is not supported.

Such support cases are often frustrating, as there is high variety of vSphere environments, error messages we get from vSphere are not always useful and at the same time users either can't use the cluster (can't create / attach PVs) or even lose data.

Secondary objective is that we (as OCP engineering / product management / support) do not know how our customers configure vSphere / OCP on top of it and if a future OCP release may break such OCP installation.

* We'd like to know HW version of virtual machines they use (necessary for vSphere CSI driver & removing in-tree cloud provider).
* We'd like to know what vSphere version they use (same as above).

### Goals

* User can detect and fix OCP / vSphere storage related configuration issues by themselves.
* Our support can easily check OCP was correctly configured and supportable.
* We get metrics about vSphere environments where OCP runs.
* Add pre-flight checks to installer to fail installation if provided vSphere cluster / credentials are not suitable.

### Non-Goals

* Any automatic fix of the environment.

## Proposal

Run a dedicated vsphere-problem-detector operator that periodically tests OCP / vSphere:

* The operator reports metrics, some of them sent via telemetry to us.
* The operator reports alerts for common configuration issues.
* The operator logs performed checks + more detailed info about OCP/vSphere config, so it's available in must-gather.

Add pre-flight checks to installer.

### Checks

These checks will be performed regularly and / or in the installer:

| Test | Periodic | Installer |
|-|-|-|
| OCP has enough permissions (e.g. list recent tasks and list datastores). | yes | yes |
| Kubelets run with cloud-provider configured (i.e. have `Node.Spec.ProviderID`). |  yes | no |
| All VMs have `disk.enableUUID=TRUE`. |  yes | no |
| Datastore name used in OCP config is not too long. |  yes | yes |
| Datastore names used in StorageClasses and PVs are not too long. |  yes | no |
| Stretch goal: OCP can create a volume + attach it to a VM (this usually works when the checks above pass). |  yes | no |
| Stretch goal: Was a VM / storage recently migrated using vMotion? We don't support this (yet). | yes | no |
| Stretch goal: Is Storage Distributed Resource Scheduler enabled? We don't support this. | yes | no |

When a previous test failed, the initial test frequency is once per 1 hour, doubling with every failed attempt up to once per 8 hours max.
When a previous test succeeded, the frequency is once per 8 hours.

### Metrics

We want these metrics to be transferred via telemetry to us:

* List of failed periodic tests.
* HW version of vSphere VMs.
* vCenter version.
* ESXi host version (of each host).
* List of installed storage plugins (3rd party vendor VIBs) - if possible.
* List of enabled features:
  * HA
  * DRS
  * SDRS w/DatastoreCluster


### User Stories

#### Story 1

As a support person, I can see in must-gather that OCP is configured correctly on vSphere.

I.e. results of the check(s) should be logged in must-gather.

#### Story 2

As OCP cluster admin, I want to know if I can create PVs and use them in pods. I.e. get an alert when something gets broken.

#### Story 3

As OCP PM, I want to know how many customers use an old vSphere version that may not be supported by a future OCP update and decide if I should push engineering harder to support the old vSphere version or convince few customers to upgrade.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

The operator will have direct access to vSphere API, which does not fine-grained permissions. As result, the permissions may be quite high. On the other hand, the operator exposes very little surface to attack.

## Design Details

All these are subject of change, see Open Questions below.

* It is a standalone operator.
* Running in namespace `openshift-cluster-storage-operator`.
* Started by cluster-storage-operator (CSO) when it detects it runs on vSphere.
* It reports its own health to CSO via CRD named `VSphereProblemDetector`. Provisional API:
    ```go
    type VSphereProblemDetector struct {
        metav1.TypeMeta   `json:",inline"`
        metav1.ObjectMeta `json:"metadata,omitempty"`

        Spec VSphereMonitoringSpec `json:"spec"`
        Status VSphereProblemDetectorStatus `json:"status"`
    }

    type VSphereProblemDetectorSpec struct {
        OperatorSpec `json:",inline"`
    }

    type VSphereProblemDetectorStatus struct {
        // Time of the last finished check
        LastCheck string
        // List of checks and their status
        Checks []VSphereCheckStatus

        // How the operator itself is doing
        OperatorStatus `json:",inline"`
    }

    type VSphereCheckStatus struct {
        Name string
        Result VSphereCheckResult
        // Why it failed
        Message string
    }

    type VSphereCheckResult string
    var (
        VSphereCheckPassed VSphereCheckResult = "Passed"
        VSphereCheckFailed VSphereCheckResult = "Failed"
        // The cluster may be unstable - for example: DRS is enabled - this is OK, as long as users don't actually use DRS to migrate a VM.
        VSphereCheckWarning VSphereCheckResult = "Warning"
    )
    ```
* The operator will use credentials provided by cloud-credentials-operator.

### Open Questions [optional]

* How to test the operator? Our shared vSphere account (VMC) does not allow us to set fine-grained permissions to make some checks fail.

* How often to run the checks?
  * Does user need a way how to initiate re-check?

### Test Plan

* We will run usual e2e-vsphere tests in CI, making sure the operator does not block cluster installation and all checks pass.

* There will be very little CI for error cases - our e2e env. does not allow us to make some (most?) checks fail.

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

* The operator is installed by default in all OCP vSphere clusters and documented in our docs.

##### Tech Preview -> GA

* More testing (upgrade, scale).
* At least 2 releases passed since tech preview to get feedback from customers & support.

##### Removing a deprecated feature

The operator is mostly "invisible" to customers, we can remove it without breaking anything.

### Upgrade / Downgrade Strategy

N/A - the operator is very simple and does not expose any critical API.

### Version Skew Strategy

N/A - the operator is very simple and does not interact with any other component.

## Implementation History

## Drawbacks

The operator will be idle for most of the time.

## Alternatives

We considered:

* Updating the installer only: this does not provide visibility through alerts + telemetry and users can (and do) change vSphere settings after installation.
* Running the check in cluster-storage-operator itself: bad separation of concerns.
* Shipping the checks as a cmdline tool / standalone image to run by customers when in troubles / when asked by our support: does not provide visibility through alerts + telemetry.

## Infrastructure Needed [optional]

Our shared VMC environment allows us to test only good cases. We need a vSphere cluster with more and less (!) privileges to test the error cases too.
