---
title: auto-node-sizing
authors:
  - "@harche"
reviewers:
  - "@rphillips"
approvers:
  - "@rphillips"
creation-date: 2021-02-11
last-updated: 2021-02-11
status: implementable
see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=1857446
replaces:
superseded-by:
---

# Kubelet Auto Node Sizing

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

Nodes should have an automatic sizing calculation mechanism, which could give kubelet an ability to scale values for memory and cpu system reserved based on machine size.

Today the sizing values are passed manually to kubelet using `--kube-reserved` and `--system-reserved` flags. Many cloud providers provide reference values for their customers to help them select optimal values based on the node sizes. e.g. [GKE](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-architecture#memory_cpu), [AKS](https://docs.microsoft.com/en-us/azure/aks/concepts-clusters-workloads#resource-reservations)

This enhancement proposes a mechanism to automatically determine the optimal sizing values for any node size irrespective of the cloud provider.

## Motivation

Kubelet’s `system reserved` and `kube reserved` play a crucial role in the OOMKilling the resource intensive pods. Without an adequate enough `system reserved` and `kube reserved` we risk freezing the node making it completely unavailable for other pods.

We have observed that scaling the value of `system reserved` and `kube reserved` with respect to the installed capacity of the node helps to deduce optimal values. Larger nodes have capacity for more pods and will require larger system reserved values.

Currently, the only way to customize the `system reserved` and `kube reserved` limits is to pre-calculate the values manually prior to Kubelet start.

### Goals

* Enable Kubelet systemd service to determine the value of the `system reserved` automatically during start up.

### Non-Goals

* For now the systemd service will only be used for calculating the values of `system reserved`. Similar approach can be taken to dynamically fetch the values of other parameters of the kubelet (e.g. `evictionHard`) but they are out of scope of this enhancement.
* Strictly from the OpenShift's point of view, we only need to take care of `system reserved`, and not `kube reserve`. Hence this proposal will not deal with generating optimal values for `kube reserve`

### User Stories

* User wants to enable auto node sizing on nodes of the cluster to start the kubelet with optimal system reserved values.

## Proposal

* New script that will be placed on the node that can calculate the system reserved values based on the node capacity
* New auto node sizing service to execute that script which will result in storing the system reserved values in a file.
* Modify kubelet service to read that file and use the generated values to start kubelet daemon.

### Graduation Criteria

#### Dev Preview -> Tech Preview
* Successfully calculate and set the optimal system reserved values.
* End user documentation

#### Tech Preview -> GA
* More testing (upgrade, downgrade, scale)
* Optinally make it available during installation

#### Removing a deprecated feature

N/A

## Design Details

### Auto Node Sizing Enabler

During the cluster installation a file will be placed at the location `/etc/node-sizing-enabled.env` with following content,

```bash
NODE_SIZING_ENABLED=false
SYSTEM_RESERVED_MEMORY=1Gi
SYSTEM_RESERVED_CPU=500m
```
Initially we would like the `Auto Node Sizing` to be an optional feature, so the value of the variable `NODE_SIZING_ENABLED` will be set to `false` during the installation along with the existing default values for system reserved memory and cpu. To enable this feature, the value of the variable `NODE_SIZING_ENABLED` can be set to `true` by using following `KubeletConfig`.

```yaml
kind: KubeletConfig
metadata:
  name: dynamic-node
spec:
  autoSizingReserved: true
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ""
```
This will enable `Auto Node Sizing` on all the worker nodes. A similar approach can be taken to enable it on the `master` nodes or on a custom machine config pool.

### Auto Node Sizing Script

This script can be found on the node at the location, `/usr/local/sbin/dynamic-system-reserved-calc.sh`

When the `Auto Node Sizing` is enabled, script will probe the host to get the installed resource capacity (such as, installed amount of RAM) and use well tested guidance on the optimal values for the corresponding system reserved.

Some of the examples of the guidance values for system reserved provided by [GKE](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-architecture#memory_cpu) and [AKS](https://docs.microsoft.com/en-us/azure/aks/concepts-clusters-workloads#resource-reservations)

And when the `Auto Node Sizing` is disabled, the script will output the existing default values for the system reserved.

The script will output the values in the following format at the location `/etc/node-sizing.env`,

```bash
$ cat /etc/node-sizing.env
SYSTEM_RESERVED_MEMORY=3.5Gi
SYSTEM_RESERVED_CPU=0.09
```
### Kubelet Auto Node Sizing Service

A new service `kubelet-auto-node-size.service` that will run `before` the existing kubelet service to calculate the optimal values of system reserved.

```toml
[Unit]
Description=Dynamically sets the system reserved for the kubelet
Wants=network-online.target
After=network-online.target ignition-firstboot-complete.service
Before=kubelet.service crio.service
[Service]
# Need oneshot to delay kubelet
Type=oneshot
RemainAfterExit=yes
EnvironmentFile=/etc/node-sizing-enabled.env
ExecStart=/bin/bash /usr/local/sbin/dynamic-system-reserved-calc.sh ${NODE_SIZING_ENABLED}
[Install]
RequiredBy=kubelet.service
```
This service will write recommended values of system reserved to the location `/etc/node-sizing.env`. It depends on another systemd environment file `/etc/node-sizing-enabled.env` mentioned above to determine if the user has enabled the `Auto Node Sizing` feature. In case user has not opted to enable it, this service will output the default values of the system reserved used today in `/etc/node-sizing.env`.

### Changes to Existing Kubelet Service

```toml
[Unit]
Description=Kubernetes Kubelet
Wants=rpc-statd.service network-online.target
Requires=crio.service kubelet-auto-node-size.service
After=network-online.target crio.service kubelet-auto-node-size.service
After=ostree-finalize-staged.service
[Service]
Type=notify
ExecStartPre=/bin/mkdir --parents /etc/kubernetes/manifests
ExecStartPre=/bin/rm -f /var/lib/kubelet/cpu_manager_state
EnvironmentFile=/etc/os-release
EnvironmentFile=-/etc/kubernetes/kubelet-workaround
EnvironmentFile=-/etc/kubernetes/kubelet-env
EnvironmentFile=/etc/node-sizing.env

ExecStart=/usr/bin/hyperkube \
    kubelet \
      --config=/etc/kubernetes/kubelet.conf \
      --bootstrap-kubeconfig=/etc/kubernetes/kubeconfig \
      --kubeconfig=/var/lib/kubelet/kubeconfig \
      --container-runtime=remote \
      --container-runtime-endpoint=/var/run/crio/crio.sock \
      --runtime-cgroups=/system.slice/crio.service \
      --node-labels=node-role.kubernetes.io/worker,node.openshift.io/os_id=${ID} \
{{- if eq .IPFamilies "DualStack"}}
      --node-ip=${KUBELET_NODE_IPS} \
{{- else}}
      --node-ip=${KUBELET_NODE_IP} \
{{- end}}
      --address=${KUBELET_NODE_IP} \
      --minimum-container-ttl-duration=6m0s \
      --volume-plugin-dir=/etc/kubernetes/kubelet-plugins/volume/exec \
      --cloud-provider={{cloudProvider .}} \
      {{cloudConfigFlag . }} \
      --pod-infra-container-image={{.Images.infraImageKey}} \
      --system-reserved=cpu=${SYSTEM_RESERVED_CPU},memory=${SYSTEM_RESERVED_MEMORY} \
      --v=${KUBELET_LOG_LEVEL}

Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Node sizing values, `SYSTEM_RESERVED_CPU` and `SYSTEM_RESERVED_MEMORY`, above will be read from environment file `/etc/node-sizing.env`

### Test Plan
The following workload can be used to test the automatically generated node sizing values.

```yaml
apiVersion: v1
kind: ReplicationController
metadata:
  name: badmem
spec:
  replicas: 1
  selector:
    app: badmem
  template:
    metadata:
      labels:
        app: badmem
    spec:
      containers:
      - args:
        - python
        - -c
        - |
          x = []
          while True:
            x.append("x" * 1048576)
        image: registry.redhat.io/rhel7:latest
        name: badmem

```
After submitting this ReplicationController the node should not end up in `NotReady` state. See https://bugzilla.redhat.com/show_bug.cgi?id=1857446 for more information.


### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?

  This functionality only modifies the systemd service file of the kubelet. It tries to supply values of `--system-reserved` kubelet flag. As long as kubelet keeps `--system-reserved` flag in place, version skew should not have any impact on this work.

- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?

  N/A

- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

  No



### Risks and Mitigations

When auto node sizing is enabled, any bug in the script that calculates the optimal system reserved can yield incorrect results. This could lead to node performing with degraded performance or even a complete outage.

Users can mitigate this by disabling auto node sizing.

### Upgrade / Downgrade Strategy

Since this feature is controlled using the `KubeletConfig`, upgrade/downgrade strategies applicable for the `KubeletConfig` are applicable here too.

## Drawbacks

This solution utilizes kubelet command line flags. Kubelet command line flags have been deprecated in favour of config file, so there is risk for this solution if those flags are actually purged. Having said that, those flags are quite widely used today. So there has not been much traction on actually removing those flags even though they have been marked deprecated.

## Alternatives

1. Enhance kubelet itself to be more smart about calculating node sizing values. We have an actively debated [KEP](https://github.com/kubernetes/enhancements/pull/2370) in sig-node around this idea.
2. Modify MCO the way it handles kubeletconfig. Instead of passing `--system-reserved` argument to the kubelet, maybe there is a possibility to make sure MCO is more tolerant of changes to the kubelet config file. This way we will modify the config file to add system reserve values instead of passing them as `--system-reserved`.

## Implementation History

See https://github.com/openshift/machine-config-operator/pull/2466
