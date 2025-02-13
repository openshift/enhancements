---
title: Control Group v2 Enablement
authors:
  - "@rphillips"
  - "@sairameshv"
reviewers:
  - "@mrunalp"
  - "@haircommander"
  - "@kikisdeliveryservice"
  - "@sinnykumari"
  - "@yuqi-zhang"
  - "@cgwalters"
approvers:
  - "@mrunalp"
  - "@yuqi-zhang"
api-approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2021-10-19
last-updated: 2025-02-05
status: implementable
---

# Control Group v2 Enablement on New Clusters

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Definitions and References

- cgroup: control group and is never capitalized
- cgroups: multiple control cgroups
- cgroup v1: references cgroup version 1 implementation
- cgroup v2: references cgroup version 2 implementation

[Upstream Docs](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)

## Summary

Control Group v2 (cgroup v2) enablement in Kubernetes has progressed to stable
[upstream](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2254-cgroup-v2).
cgroup v2 is enabled by default in all the freshly installed Openshift clusters >= 4.14.
Control Group v1 is a deprecated feature from OCP 4.16 and the support is intended to be removed from OCP >= 4.19

Note: This enhancement is focusing on `pure` mode cgroup v2. Mixed mode environments
may behave differently (metrics, vpa, hpa, etc) since cgroup v1 is not
compatible with cgroup v2.

## Motivation

Migrating to cgroup v2 will bring in many new features and fixes not found in
cgroup v1. cgroup v1 is considered 'legacy' and migrating to cgroup v2 is
necessary since RHEL8 ships with cgroup v2 on by default. (OpenShift 4.x
currently disables cgroup v2 in favor of v1).

Some features of cgroup v2 include:

* IO enhancements
* User based OOM killer
* cgroup aware OOM killer

[Kubernetes On Cgroup v2 - Video](https://www.youtube.com/watch?v=u8h0e84HxcE)

### Goals

- [ ] Enable cgroup v2 within the Openshift API
- [ ] Add kernel flags to MCO to enable cgroup v2 on nodes
- [ ] Add an admission webhook to deny updating `CgroupMode` of the `nodes.config.openshift.io` object to `CgroupMode_V1`
- [ ] Block the upgrades of the OCP clusters that are using cgroup v1 until migrated to cgroup v2

### Non-Goals

Mixed mode cgroup modes are not 100% compatible with each other. We need data
around how cgroup v2 runs in a pure mode before we can allow mixed mode
environments. Since, Red Hat is steering upstream cgroup v2 adoption, and we do
not have data around the pure mode environment yet, there needs to be a platform
to gather data from.

## Proposal

- The option to enable cgroup v2 resides in a centralized location i.e. [OpenShift Node config
object](https://github.com/openshift/api/blob/master/config/v1/types_node.go)
- Set the upgrade ability of the MCO cluster operator to `false` if a cluster is on cgroup v1
- An admission hook is introduced to allow if a cluster is transitioning from "cgroup v1" -> "cgroup v2" and deny the other way of "cgroup v2" -> "cgroup v1"

### API Extensions

Create an additional module within openshift/api as `config/v1/types_node.go`:

```go
type CgroupMode string

const (
  CgroupMode_Empty = "" // Empty string will always use the Default
  CgroupMode_v1 = "v1"
  CgroupMode_v2 = "v2"
  CgroupMode_Default = CgroupMode_v1
)

type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec NodeSpec `json:"spec"`
}

type NodeSpec struct {
  CgroupMode CgroupMode `json:"cgroupMode,omitempty"`

  // an eventual additional option might be crun in the future. This explains
  //   why a new struct may be necessary
  // CrunEnabled bool ...
}
```

### Operational Aspects of API Extensions

- MCO reads the configured object and
sets the appropriate kernel options (on bootstrap).
- The MCO will report an error
if a user tries to modify/add cgroup related kargs within a MachineConfig.
- An admission hook is introduced to allow if a cluster is transitioning from "cgroup v1" -> "cgroup v2" and deny the other way of "cgroup v2" -> "cgroup v1"
- MCO also reports error if a user tries to set the `CgroupMode` to `CgroupMode_V1` and sets the cluster operator's status condition of `Upgradeble=False`
- The following kernel command line arguments would be observed on the machine config pools by default and also when `CgroupMode_v2` is enabled:
```yaml
  kernelArguments:
    - systemd.unified_cgroup_hierarchy=1
    - cgroup_no_v1="all"
    - psi=1 
```
#### Failure Modes

N/A

#### Support Procedures

### User Stories

#### 1. As a user, I would like to install a cluster that uses cgroup v2

### Risks and Mitigations

The primary risk of cgroup v2 is some workloads not supporting the changed
cgroup filesystem paths. This is the reason why it is off by default and has to
be enabled at install time.

## Design Details

### Test Plan

Testing should be thoroughly done at all levels, including unit, end-to-end, and
integration.

### Graduation Criteria

#### Dev Preview -> Tech Preview

cgroup v2 will be dev preview on its initial release. Internal and customer
usage will be critical to gather information on bugs and enhancements to the
underlying subsystem.

Graduation requirements to Tech Preview are:

* No regressions from cgroup v1 to cgroup v2
* Processes are correctly OOMed
* CPU management works as expected
* No performance issues - PSAP and QE teams will be asked to test their suites for regressions
* Metrics are accurate and correctly submitted to monitoring

#### Tech Preview -> GA

With sufficient internal testing and customer feedback the feature will graduate
to Tech Preview.

Upon graduation to GA the feature will still be turned off by default until we define another enhancement to specify how cgroup v2 is enabled by default within OpenShift.

Graduation requirements to GA:
* Upstream graduation to GA
* Internal stakeholders are using cgroup v2 without issue
* Tech Preview Graduation requirements are still good
* Add blocking cgroup v2 upgrade jobs
* CI OpenShift cgroup v2 upgrade jobs pass percentage is similar or better than the OpenShift cgroup v1 upgrade job pass percentage

The following jobs will be run against cgroup v2 periodically and with a minimum of 100 runs. The test pass percentage for each test/job tuple must not be demonstrably worse for any of the following jobs:

- periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-upgrade
- periodic-ci-openshift-release-master-ci-4.10-e2e-azure-ovn-upgrade
- periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-gcp-ovn-upgrade
- periodic-ci-openshift-release-master-ci-4.10-e2e-aws-ovn-upgrade
- periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-aws-ovn-upgrade
- periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-azure-upgrade
- periodic-ci-openshift-release-master-ci-4.10-e2e-gcp-upgrade
- periodic-ci-openshift-release-master-nightly-4.10-upgrade-from-stable-4.9-e2e-metal-ipi-upgrade-ovn-ipv6
- periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-vsphere-upgrade

### Upgrade / Downgrade Strategy

- Downgrading a cluster to an OpenShift version not containing cgroup v2 support
is unsupported.
- Upgrading a cluster which is on cgroup v1 to a version >= 4.19 is blocked until it is migrated to cgroup v2.
In such case, the MCO cluster operator's status condition is set to `Upgradeable=False` blocking the upgrade.
- A user/admin has to manually set the `CgroupMode` from `v1` to `v2` as follows to make it `Upgradeable=True` again.
```shell
# fetch the nodes.config.openshift.io object
oc edit nodes.config.openshift.io cluster
# Update the `CgroupMode` field inside the spec to "v2"
# Wait for the MCO to rollout cgroup v2 related kernel arguments on all the machine config pools
```
### Version Skew Strategy

A cluster installed with cgroup v2 will abide by the usual skew upgrade path.

#### Removing a deprecated feature
cgroup v1 support would be removed from the future versions of RHEL and hence the setting of `CgroupMode_V1` would be denied by an admission webhook from OCP clusters(>= 4.19)

Note: Even after removing the support to configure cgroup v1 from OCP, the `CgroupMode_V1` is not deleted as one of the enum values for the `CgroupMode` in the interest of non-breaking API changes.

## Implementation History
Following code change inside the MCO [operator](https://github.com/openshift/machine-config-operator/blob/master/pkg/operator/status.go#L265)'s `pkg/operator/status.go` sets the ClusterOperator's `Upgradeable` status to `False` if the cluster is found to be using `CgroupMode_V1`
```go
configNode, err := optr.configClient.ConfigV1().Nodes().Get(context.Background(), ctrlcommon.ClusterNodeInstanceName, metav1.GetOptions{})
if err != nil {
	return err
}
if configNode.Spec.CgroupMode == configv1.CgroupModeV1 {
	coStatusCondition.Status = configv1.ConditionFalse
	coStatusCondition.Reason = "ClusterOnCgroupV1"
	coStatusCondition.Message = "Cluster is using cgroup v1 and is not upgradable. Please update the `CgroupMode` in the `nodes.config.openshift.io` object to 'v2'. Once upgraded, the cluster cannot be changed back to cgroup v1"
}
```
## Alternatives

## Drawbacks