---
title: node-swap
authors:
  - "@ehashman"
reviewers:
  - "@rphilips"
  - "@sjenning"
  - "???"
approvers:
  - "@mrunalp"
creation-date: "2021-06-23"
status: implementable
---

# OpenShift Node Swap Support

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Operational readiness criteria is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The upstream Kubernetes 1.22 release introduced alpha support for configuring
swap memory usage for Kubernetes workloads on a per-node basis.

Now that swap use on nodes is supported in upstream, there are a number of use
cases that would benefit from OpenShift nodes supporting swap, including
improved node stability, better support for applications with high memory
overhead but smaller working sets, the use of memory-constrained devices, and
memory flexibility.

## Motivation

See [KEP-2400: Motivation].

[KEP-2400: Motivation]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#motivation

### Goals

- Swap can be provisioned and configured for nodes to use in an OpenShift
  cluster.

### Non-Goals

- Workload-specific swap accounting.
- Any of the non-goals in [KEP-2400: Non-goals].

[KEP-2400: Non-goals]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#non-goals

## Proposal

### User Stories

See [KEP-2400: User Stories](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#user-stories).

### API Extensions

No new API extensions are required. For tech preview, the only update required
is the addition of a `NodeSwap` feature gate to the OpenShift API
`TechPreviewNoUpgrade` feature set. See [Design Details](#design-details) for
the specifics.

### Implementation Details/Notes/Constraints [optional]

See [KEP-2400: Notes/Constraints/Caveats](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#notesconstraintscaveats-optional).

### Risks and Mitigations

See [KEP-2400: Risks and Mitigations].

[KEP-2400: Risks and Mitigations]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#risks-and-mitigations

## Design Details

### Enable `NodeSwap` feature gate

We will add a new feature gate `NodeSwap` to the `TechPreviewNoUpgrade` feature
set to enable it for [Tech Preview].

Note that swap cannot be enabled or used without additional configuration, so
there isn't a risk of it interfering with other features when adding it to the
existing Tech Preview feature set.

Without this change, it is possible to enable the feature gate on any 4.9+
cluster by defining `NodeSwap` in a `customNoUpgrade` configuration:

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
    - NodeSwap
```

Once the `NodeSwap` feature flag is enabled, a cluster admin can enable swap
usage on the cluster as follows:

### Ensure component versions support swap

While swap support has been available in Kubernetes since the 1.22.0 release,
the container runtimes also need support in order for Kubernetes workloads to
be able to use the feature correctly.

Therefore, a user of OpenShift version >4.9.0 can enable the feature flag, but
they also will require a version of CRI-O that supports the
`MemorySwapLimitInBytes` for best results. This is only supported in the 1.23
and onwards releases of CRI-O, and should be supported in the first release of
OpenShift 4.10.

### Configure worker Kubelets

Swap behaviour on a node can be configured with
[`memorySwap.SwapBehavior`](https://kubernetes.io/docs/concepts/architecture/nodes/#swap-memory).

The most straightforward way to configure the kubelets with this feature is
with a custom `KubeletConfig` that will automatically be applied by the MCO:

```bash
# Enable the custom kubelet config on the worker pool
oc label machineconfigpool worker custom-kubelet=enabled
```

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: custom-config 
spec:
  machineConfigPoolSelector:
    matchLabels:
      custom-kubelet: enabled 
  kubeletConfig: 
    failSwapOn: false
    memorySwap:
      swapBehavior: UnlimitedSwap  # LimitedSwap is also supported
```

Note that enabling swap on control plane nodes is possible but **not** recommended.

### Add swap to nodes

There are a few different ways this can be accomplished. The most
straightforward is to add a kubelet configuration that enables a swapfile at
startup with a custom machine config, the same way we enable swap in upstream
CI:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 90-worker-swap
spec:
  config:
    ignition:
      version: 3.2.0
    systemd:
      units:
      - contents: |
          [Unit]
          Description=Enable swap on CoreOS
          Before=crio-install.service
          ConditionFirstBoot=no
          
          [Service]
          Type=oneshot
          ExecStart=/bin/sh -c "sudo dd if=/dev/zero of=/var/swapfile count=1024 bs=1MiB && sudo chmod 600 /var/swapfile && sudo mkswap /var/swapfile && sudo swapon /var/swapfile && free -h"
          [Install]
          
          WantedBy=multi-user.target
        enabled: true
        name: swap-enable.service
```

It is also possible to use [ignition
configs](https://coreos.github.io/ignition/configuration-v3_3/) to add swap
partitions to worker nodes with `filesystems.format = swap`.

Beyond tech preview, we may want to look into adding swap support to the
installer, and consider adding a partition on the root node, or perhaps
[provision and mount an NVMe
volume](https://github.com/openshift/machine-config-operator/issues/1619).

### Open Questions [optional]

- Will we eventually want to enable swap on all OpenShift nodes by default?
- Should swap just be limited to worker nodes, or should we consider adding it
  to control plane nodes too?

Testing in reliability clusters and in various configurations will help us
answer these questions. Stability metrics determined through the upstream beta
will assist.

### Test Plan

In addition to the upstream e2e tests, we will need to add e2e suites to
OpenShift in order to exercise provisioning and use of swap. This may include
unit tests where appropriate, such as the MCO.

### Graduation Criteria

#### Dev Preview -> Tech Preview

Requires alpha support in upstream Kubernetes (1.22+) and support in CRI-O
(1.23+).

- Support provisioning OpenShift nodes with swap enabled for all available
  upstream swap configurations (currently `LimitedSwap`, `UnlimitedSwap`).

JIRA: https://issues.redhat.com/browse/OCPNODE-470

_Graduation criteria below are tentative._

#### Tech Preview -> GA

Requires beta/GA support in upstream Kubernetes. (1.25?+)

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

Deprecation of this feature would likely follow upstream deprecation, which is
not currently planned.

### Upgrade / Downgrade Strategy

The `NodeSwap` feature flag is not supported in Kubernetes versions prior to
1.22/OpenShift 4.9. We will add the upstream `NodeSwap` feature flag as a
"NoUpgrade" feature flag to prevent upgrades.  

Note that swap support does not require coordination between components and the
configuration is limited to individual nodes/machine pools.

See also [KEP-2400: Upgrade/Downgrade Strategy].

[Tech Preview]: https://github.com/openshift/enhancements/blob/ce4d303db807622687159eb9d3248285a003fabb/guidelines/techpreview.md#official-processmechanism-for-delivering-a-tp-feature
[KEP-2400: Upgrade/Downgrade Strategy]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#upgrade--downgrade-strategy

### Version Skew Strategy

N/A, this is a compatible API change limited to the Kubelet that does not
require coordination with the API Server.

### Operational Aspects of API Extensions

None. The only API extension is a feature gate change. There are no plans to
enable the feature by default until extensive testing takes place and upstream
announces beta or GA support for the feature.

For potential operational impacts of enabling swap, see [KEP-2400: Risks and
Mitigations].

#### Failure Modes

See [KEP-2400: Rollout, Upgrade and Rollback Planning] and [KEP-2400:
Troubleshooting].

[KEP-2400: Rollout, Upgrade and Rollback Planning]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#rollout-upgrade-and-rollback-planning
[KEP-2400: Troubleshooting]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#troubleshooting

#### Support Procedures

See [KEP-2400: Production Readiness Review Questionnaire] and [KEP-2400:
Troubleshooting].

[KEP-2400: Production Readiness Review Questionnaire]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#production-readiness-review-questionnaire

## Implementation History

- [Upstream alpha swap support] completed in Kubernetes 1.22.
- [CRI-O support] added in 1.23.0.

[Upstream alpha swap support]: https://github.com/kubernetes/enhancements/issues/2400#issuecomment-884327938
[CRI-O support]: https://github.com/cri-o/cri-o/pull/5207

## Drawbacks

See [KEP-2400: Drawbacks].

[KEP-2400: Drawbacks]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#drawbacks

## Alternatives

See [KEP-2400: Alternatives].

[KEP-2400: Alternatives]: https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#alternatives

## Infrastructure Needed [optional]

- We will need to configure periodic e2e tests on VMs with swap enabled.
- We will need to enable swap on a [reliability cluster] to gauge long-term
  stability.

[reliability cluster]: https://issues.redhat.com/browse/OCPNODE-619
