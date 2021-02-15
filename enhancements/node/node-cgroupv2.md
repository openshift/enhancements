---
title: enable-cgroupv2
authors:
  - "@rphillips"
  - "@harche"
reviewers:
  - "@mrunalp"
approvers:
  - "@mrunalp"
creation-date: 2021-02-15
last-updated: 2021-02-15
status: implementable
see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=1857446
replaces:
superseded-by:
---

# Enable cgroup v2

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

cgroup v2 is the next version of the kernel control groups. The support for cgroup v2 has been in development in the container ecosystem from quite some time. But now with entire stack from `runc` to `kubernetes` and everything in between supports cgroup v2, it's about time we should enable support for cgroup v2 in OpenShift.


## Motivation

Cgroup v2 brings in bunch of improvements over it's predecessor. Please refer to the official kernel [documentation](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html) for more information.

### Goals

* Enable cgroup v2 across the cluster as an optional feature

### Non-Goals

* Until we have more in-depth understanding of the impact of enabling cgroup v2 on the workloads, we won't make it default and replace it with cgroup v1.

## Proposal

Cgroup v2 is enabled by booting the system with the following kernel arguments,

1. systemd.unified_cgroup_hierarchy=1
2. cgroup_no_v1="all"
3. psi=1

We will discuss how we can enable cgroup v2 in Openshift in day-1 and day-2 operation scenrios in the following subsections.

### Day-1 operation scenario

If the user wishes to enable `cgroup v2` during the installation of the cluster itself, they will have to indicate that using a field `cgroupsV2` in installation configuration. e.g.

```yaml
apiVersion: v1
baseDomain: gcp.devcluster.openshift.com
cgroupsV2: true
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  platform: {}
  replicas: 3
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform: {}
  replicas: 3
metadata:
  creationTimestamp: null
  name: testcluster1
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  gcp:
    projectID: openshift-gce-devel
    region: asia-south1
publish: External
pullSecret: </snip>
```

#### Proof of concept
@rphillips has a [proof of concept implementation](https://github.com/openshift/installer/pull/4648) changes described above.

These changes can be tested using the cluster-bot by command,

`launch https://github.com/openshift/installer/pull/4648`

### Day-2 operation scenario

Users can enable cgroup v2 in OpenShift cluster on Day-2 by using the [existing support](https://github.com/openshift/machine-config-operator/blob/906673c4cab9dc5ad3f50b7481c8f02d26240a5d/docs/MachineConfiguration.md#kernelarguments) for passing the kernel arguments in MCO.

#### Enable cgroup v2 on worker nodes
```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: enable-cgroupv2-workers
spec:
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ""
  kernelArguments:
    - systemd.unified_cgroup_hierarchy=1
    - cgroup_no_v1="all"
    - psi=1

```

#### Enable cgroup v2 on master nodes
```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: enable-cgroupv2-master
spec:
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/master: ""
  kernelArguments:
    - systemd.unified_cgroup_hierarchy=1
    - cgroup_no_v1="all"
    - psi=1

```

### Upgrade / Downgrade Strategy

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?

  Place holder. Needs more investigation, especially if any existing metrics will get affected.

- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?

  N/A

- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

  Place holder. Needs more investigation.


## Drawbacks

This change is very core to the container infrastructure, so we need to evaluate and monitor it's impact on existing ecosystem (e.g. metrics) carefully.

## Alternatives

Considering the boat load of improvements cgroup v2 brings on the table, staying with older cgroup v1 is not an option. The only option to upgrade from cgroup v1 is to go to cgroup v2, there is no alternative to that.