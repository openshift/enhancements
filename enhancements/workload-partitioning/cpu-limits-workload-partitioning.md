---
title: cpu-limits-workload-partitioning
authors:
  - "@eggfoobar"
reviewers:
  - "@jerpeter1"
  - "@mrunalp"
  - "@rphillips"
  - "@browsell"
  - "@haircommander"
  - "@MarSik"
  - "@Tal-or"
approvers:
  - "@jerpeter1"
  - "@mrunalp"
api-approvers:
  - "None"
creation-date: 2024-01-24
last-updated: 2024-01-24
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-57
see-also:
  - "/enhancements/workload-partitioning"
---

# CPU Limits for Workload Partitioning

## Summary

This enhancements builds on top of the [Management Workload
Partitioning](management-workload-partitioning.md) enhancement to provide the
ability for workload partitioning to take into account CPU limits during Pod
admission. Currently only CPU requests are used during Pod admission and any Pod
that uses CPU limits is ignored. With this change the Pod admission webhook will
take into account CPU limits and use the existing mechanism to pass the CPU
limits information to the underlying container runtime.

## Motivation

Workload partitioning currently does not support mutating containers that have
CPU limits. The original premise was that all OCP Pods did not set limits, if
limits were present then the default behavior would set the CPU `requests.cpu`
to the limit value and thus getting in the way of the scheduler to use the
`cpushares` so it was decided to avoid modifying those pods.
([commit](https://github.com/openshift/kubernetes/commit/c6395e702e5f02c11ebb7659a18cef0b24609bfb)).
However it has been found that at least one cluster container does set limits. A
proper solution is required to deal with these exceptions. In addition, the
desire in the future is to support different workload types which would require
limit support.

### User Stories

As a cluster admin I want to make sure that on a CPU partitioned OpenShift
cluster, Pods that set CPU limits are also modified with the correct annotation
for workload partitioning. Furthermore, the CPU limit of those Pods should be
respected by the container run time so that they are bound to that limit.

### Goals

The goals only apply to clusters where the workload partitioning is enabled.

- Pods that set CPU limits and are annotated for workload partitioning will be
  modified for workload partitioning.
- We will not modify the QoS of Pods and guaranteed Pods will not be modified.
- We will update existing e2e tests to account for this new behavior.
- Clusters built in this way should pass the same kubernetes and OpenShift
  conformance and functional end-to-end tests as similar deployments that are
  not isolating the management workloads.

### Non-Goals

- This enhancement is focused on CPU resources. Other compressible resource
  types may need to be managed in the future, and those are likely to need
  different approaches.
- This enhancement does not address non-compressible resource requests, such as
  for memory.

## Proposal

In workload partitioning we currently set a custom resource type
`{workload-type}.workload.openshift.io/cores` for `requests` and `limits`. In
this process we remove the `requests.cpu` so that we can utilize the existing
scheduler for assigning pods to nodes. The admission webhook currently skips
over Pods that set a `limits.cpu` because the default behavior for kubernetes is
to add `requests.cpu` when `limits.cpu` is set and since we strip the `cpu`
resource to utilize the scheduler this causes a problem for our desired
scheduling behavior.

In order to support limits we will need to expose the runtime spec option for
`CPUQuota` to the workload configuration. There are two components we need to
modify for this change, CRI-O and the workloads admission webhook.

CRI-O will be updated to expose the CPU quota at the container runtime level via
the existing workloads configuration. We will modify the existing configuration
to support the new value. We will make the default behavior to be 0 so existing
CRI-O configuration files will not need to be modified.

The admission webhook will be altered to no longer ignore modifying CPU limit
requests. CPU limits will be used and added as annotation to the Pod similar to
how CPU shares are currently added. The QoS of the Pod shall not be changed, and
guaranteed Pods will continue to not be altered.

In short CRI-O and the admission webhook will be modified in the following way.

1. CRI-O Workload Resource Configuration
   - Expose the CPU quota runtime option by adding the `CPUQuota` to the
     resource configuration.
   - Update Mutating Spec call to modify CPU Quota
   - Update Cgroup Manager to set the CPUQuota for workload partitioned
     containers
   - Default value of 0 will be assumed for
     `[crio.runtime.workloads.resources.cpuquota]`
2. Admission Webhook
   - No longer ignore Pods with CPU limits defined
   - Add the adjusted value for CPU quota as an annotation to the Pod
   - Containers that do not contain limits will not have the `cpuquota`
     attribute set
   - Make sure a Pods QOS is not altered during this process

### Workflow Description

As it currently stands the proposed addition in this enhancement will not
require any change in how the user interacts with the cluster. Workload
partitioning is currently used by platform pods, with the new addition in
behavior platform pods can set limits, existing and future pods will no longer
need any special handling so they are not ignored by the webhook.

#### Variation and form factor considerations [optional]

The addition here does not alter the existing behavior of the other variations.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

### CRI-O - Workload Resource Configuration

We will need to update the CRI-O workload configuration to expose the cgroup CPU
quota option.

In the code we will update the top level configuration `struct` to include the
`CPUQuota` option.

[workloads.go](https://github.com/cri-o/cri-o/blob/f243ba712d58d106dea1ba7adf33ed0911a3e563/pkg/config/workloads.go#L45-L50)

```go
type Resources struct {
	// Specifies the number of CPU shares this pod has access to.
	CPUShares uint64 `json:"cpushares,omitempty"`
    // Specifies the CPU quota this pod is limited to in microseconds.
	CPUQuota int64 `json:"cpuquota,omitempty"`
	// Specifies the cpuset this pod has access to.
	CPUSet string `json:"cpuset,omitempty"`
}
```

We'll need to make sure the `MutateSpec` is updated to modify the CPU Resources
with the correct quota.

[workloads.go](https://github.com/cri-o/cri-o/blob/f243ba712d58d106dea1ba7adf33ed0911a3e563/pkg/config/workloads.go#L173-L183)

```go
func (r *Resources) MutateSpec(specgen *generate.Generator) {
	if r == nil {
		return
	}
	if r.CPUSet != "" {
		specgen.SetLinuxResourcesCPUCpus(r.CPUSet)
	}
	if r.CPUShares != 0 {
		specgen.SetLinuxResourcesCPUShares(r.CPUShares)
	}
    if r.CPUQuota != 0 {
		specgen.SetLinuxResourcesCPUQuota(r.CPUQuota)
	}
}
```

The Cgroup manager will then be updated when calling `setWorkloadsSettings` to
utilize the top level config when creating the Cgroup.

[cgroupfs_linux.go](https://github.com/cri-o/cri-o/blob/f243ba712d58d106dea1ba7adf33ed0911a3e563/internal/config/cgmgr/cgroupfs_linux.go#L211-L234)

```go
func setWorkloadSettings(cgPath string, resources *rspec.LinuxResources) (err error) {
	if resources.CPU == nil {
		return nil
	}
	cg := &cgcfgs.Cgroup{
		Path: "/" + cgPath,
		Resources: &cgcfgs.Resources{
			SkipDevices: true,
			CpusetCpus:  resources.CPU.Cpus,
		},
		Rootless: rootless.IsRootless(),
	}
	if resources.CPU.Shares != nil {
		cg.Resources.CpuShares = *resources.CPU.Shares
	}
	if resources.CPU.Quota != nil {
		cg.Resources.CpuQuota = *resources.CPU.Quota
	}

	mgr, err := libctrCgMgr.New(cg)
	if err != nil {
		return err
	}
	return mgr.Set(cg.Resources)
}
```

This will all be exposed to the user via the `toml` configuration.

```toml
[crio.runtime.workloads.management]
activation_annotation = "target.workload.openshift.io/management"
annotation_prefix = "resources.workload.openshift.io"
resources = { "cpushares" = 0,  "cpuquota" = 0, "cpuset" = "0-1,52-53" }
```

### Admission Webhook

The admission webhook will be updated to use the CPU limit resource information
to add the annotations to the Pod for each container. Pods with cpu limits will
no longer be ignored and their limits will correctly be represented with the
`management.workload.openshift.io/cores` in the same way that `requests` are.
Containers that do not include `limits` will not have a `cpuquota` set. We will
be utilizing the helper function in
[cm.MilliCPUToQuota](https://github.com/openshift/kubernetes/blob/0e0d15b865ffc36177dc8770b4723dc14476a630/pkg/kubelet/cm/helpers_linux.go#L58-L82)
to correctly pass along the CPU quota to the container runtime, but we will
maintain the `milli` value for the `requests.limits.cpu` value so as to not
cause confusion by changing the unit integer value. That is an implementation
detail that does not need to be exposed to the user.

In the example below, The resulting Pod from the given Deployment will now
correctly translate the `resources.limits.cpu` to
`management.workload.openshift.io/cores` to `cpuquota`. The Pod will then be
annotated to use the new `cpuquota` attribute for the `busybox` container for
the container runtime to alter the Cgroup
`resources.workload.openshift.io/busybox: '{"cpushares": 20, "cpuquota":
3000}'`. Note, the `cpuquota` correctly reflects the `30m` in microsecends. When
no CPU limits are specified then the old behavior will still be in effect where
no `cpuquota` is included, `resources.workload.openshift.io/busybox-no-limits:
'{"cpushares": 20}'`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: busybox-deployment
spec:
    ...
  template:
    metadata:
        ...
      annotations:
        target.workload.openshift.io/management: '{"effect": "PreferredDuringScheduling"}'
    spec:
      containers:
      - ...
        name: busybox
        resources:
          requests:
            cpu: 20m
            memory: 50Mi
          limits:
            cpu: 30m
            memory: 50Mi
      - ...
        name: busybox-no-limits
        resources:
          requests:
            cpu: 20m
            memory: 50Mi
```

```yaml
apiVersion: v1
kind: Pod
metadata:
  annotations:
    resources.workload.openshift.io/busybox: '{"cpushares": 20, "cpuquota": 3000}'
    resources.workload.openshift.io/busybox-no-limits: '{"cpushares": 20}'
    target.workload.openshift.io/management: '{"effect":"PreferredDuringScheduling"}'
...
spec:
  containers:
  - ...
    name: busybox
    resources:
      limits:
        management.workload.openshift.io/cores: "30"
        memory: 50Mi
      requests:
        management.workload.openshift.io/cores: "20"
        memory: 50Mi
  - ...
    name: busybox
    resources:
      limits:
        management.workload.openshift.io/cores: "20"
      requests:
        management.workload.openshift.io/cores: "20"
        memory: 50Mi
```

#### Hypershift [optional]

N/A

### Risks and Mitigations

The addition of this change should not pose any major problems, with this
approach we are essentially emulating what Kubernetes does when limits are set.

A thing to note is this is under the assumption that CPU Period is fixed at `100
milliseconds`, from my investigations that does seem to be the case for RHCOS in
OCP, but I am not sure if there are deployments out there that run with a
different CPU period, those would need to be taken into account.

Currently workload partitioning is only used by platform pods, any platform pods
that use `limits` will correctly be modified to have limits and request be
applied via `cpuquota` and `cpushares`. Since one of the key attributes of
workload partitioning is pinning a workload to a specific CPU set, then those
pods will correctly be moved over to those CPU sets and have limits imposed this
might cause issues in performance for those pods.

### Drawbacks

A draw back is that we will be committing to carry the admission webhook patch
downstream indefinitely. Futhermore, since we are not doing anything more than
what the `limits` would do in Kubernetes, we are essentially re-applying that
functionality and using our own existing workloads mechanism to pass the
`limits` information down to the container runtime. This doubling of efforts is
currently required in order to avoid the default behavior of any missing
`limits.cpu` being applied to `requests.cpu`, since such an upstream change
would not be favorable.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

We will update the origin tests for workload partitioning to also include checks
for cpu limits on clusters where workload partitioning is enabled.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

In a failure situation, we want to try to keep the cluster operational.
Therefore, there are a few conditions under which the admission hook will strip
the workload annotations and add an annotation `workload.openshift.io/warning`
with a message warning the user that their partitioning instructions were
ignored. These conditions are:

1. When a Pod has the Guaranteed QoS class
2. When mutation would change the QoS class for the Pod
3. When the feature is inactive because not all nodes are reporting the
   management resource

#### Support Procedures

N/A

## Implementation History

WIP

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A
