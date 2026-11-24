---
title: ovn-kubernetes-startup-readiness-timeout
authors:
  - "@cragr"
reviewers:
  - "@tssurya"
  - "@kyrtapz"
approvers:
  - "@tssurya"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-10-02
last-updated: 2026-10-02
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPBUGS-127208
see-also:
  - "/enhancements/network/user-defined-network-segmentation.md"
  - "/enhancements/network/multi-networkpolicy.md"
---

# OVN-Kubernetes Startup Readiness Timeout

## Summary

At startup, ovnkube-node waits up to a hard-coded 300 seconds for its
node's gateway and management port to be created in OVN. On clusters with
many user-defined networks (UDNs) and multi-network policies, a node that
starts with empty local OVN databases, such as a newly added node, can
take longer than that to finish its initial network sync. It then exits,
restarts from scratch, fails the same way, and never becomes ready. This
enhancement adds an optional `startupReadinessTimeoutSeconds` field to
`OVNKubernetesConfig` in the `network.operator.openshift.io` API, behind a
new `OVNKubernetesStartupReadinessTimeout` feature gate. The Cluster
Network Operator (CNO) passes it to ovnkube-node as the upstream
`--startup-readiness-timeout` flag. When unset, behavior is unchanged.

## Motivation

In interconnect mode, ovnkube-controller on each node creates the node's
gateway and management port only after the network manager's initial sync
has started every user-defined network. That sync processes one network
at a time, by design. Meanwhile ovnkube-node waits for the gateway and
management port in `newStartupWaiter()`, with a fixed 300-second deadline.

When the deadline passes, ovnkube-node fails with:

```text
failed to init default node network controller: error waiting for node readiness: context deadline exceeded
```

The process exits and restarts with an empty sync, so it hits the same
deadline on every attempt. Nodes that were already running are not
affected, because they add new networks one by one, and a warm restart is
fine because the gateway and management port already exist in the node's
northbound database. The failure affects cold starts: new nodes, and nodes
whose local OVN databases were wiped.

Upstream ovn-kubernetes made the timeout configurable in
[ovn-kubernetes#7034](https://github.com/ovn-kubernetes/ovn-kubernetes/pull/7034).
Measurements from that PR (interconnect mode, 3 control-plane and 2 worker
nodes with 16 vCPU and 32 GiB each, 148 localnet NADs, MultiNetworkPolicies
each selecting 147 of the NADs):

| MultiNetworkPolicies | NB ACLs / port groups per node | Cold start, default 300s | Cold start, timeout 900s |
| --- | --- | --- | --- |
| 500 | 74.6k / 85.2k | ready after 85s | not tested |
| 1,000 | 148k / 159k | ready after 167s | not tested |
| 2,000 | 295k / 306k | deadline at 5m02s, crash-loops | not tested |
| 3,150 | 464k / 475k | deadline at 5m02s, crash-loops | ready after 8m51s, no restarts |

Sync time grew linearly with policy count (about 0.165s per policy at 147
networks), so the default ran out at about 1,800 policies. On OpenShift,
CNO renders the ovnkube-node configuration and reverts manual changes, so
administrators have no supported way to raise the timeout today.

### User Stories

* As a cluster administrator running many user-defined networks and
  multi-network policies, I want to raise the ovnkube-node startup
  readiness timeout, so that nodes I add to the cluster become ready
  instead of crash-looping.
* As a cluster administrator recovering a node whose OVN databases were
  rebuilt, I want the node to finish its initial sync on the first
  attempt, so that recovery does not depend on restarts.
* As a support engineer, I want ovnkube-node to log the timeout it is
  using, so that I can tell whether a readiness failure needs a larger
  value or has another cause.

### Goals

* Administrators can set the ovnkube-node startup readiness timeout
  through the `network.operator.openshift.io` API.
* Clusters that do not set the field keep the current 300-second
  behavior.
* The setting survives upgrades and is reconciled by CNO like other
  OVN-Kubernetes options.

### Non-Goals

* Making the initial UDN sync faster or parallel. That is a separate
  upstream effort, and this timeout is still needed while sync time grows
  with scale.
* Changing other ovnkube-node waits, such as the 30-second patch-port wait
  for UDN gateways.
* Choosing the timeout automatically from cluster size.
* Exposing the setting in MicroShift or in the HyperShift `HostedCluster`
  API in this phase.

## Proposal

1. **openshift/api:** add an optional
   `startupReadinessTimeoutSeconds` field to `OVNKubernetesConfig`, gated
   by a new `OVNKubernetesStartupReadinessTimeout` feature gate, enabled
   in TechPreviewNoUpgrade and DevPreviewNoUpgrade first.
2. **ovn-kubernetes:** carry upstream ovn-kubernetes#7034 into
   openshift/ovn-kubernetes. It adds the `--startup-readiness-timeout`
   flag (seconds, default 300, values of 0 or less rejected) and logs the
   timeout in use.
3. **cluster-network-operator:** when the field is set, render
   `--startup-readiness-timeout <value>` into the ovnkube-node start
   script. When it is unset, render nothing, so the binary default
   applies.

A command-line flag is used rather than the `ovnkube-config` ConfigMap,
because only ovnkube-node reads the setting and the ConfigMap has no
`[ovnkubenode]` section today. The same pattern is already used for
`egressIPConfig.reachabilityTotalTimeoutSeconds`.

### Workflow Description

**cluster administrator** is a human user responsible for the cluster's
network configuration.

1. The cluster administrator sees a new node crash-looping, with
   ovnkube-node logs showing `error waiting for node readiness: context
   deadline exceeded`.
2. The cluster administrator sets the timeout:

   ```shell
   oc patch network.operator.openshift.io cluster --type merge -p \
     '{"spec":{"defaultNetwork":{"ovnKubernetesConfig":{"startupReadinessTimeoutSeconds":600}}}}'
   ```

3. CNO re-renders the ovnkube-node DaemonSet, which rolls out across
   nodes one at a time.
4. The restarted ovnkube-node logs
   `Waiting up to 10m0s for gateway and management port readiness...`
   and the new node becomes ready once its initial sync completes.

To return to the default, the administrator removes the field. CNO rolls
the DaemonSet again without the flag.

### API Extensions

This enhancement modifies the `Network` CRD in
`network.operator.openshift.io` by adding one optional field. It adds no
webhooks, finalizers, or new CRDs.

```go
type OVNKubernetesConfig struct {
	// ...existing fields...

	// startupReadinessTimeoutSeconds is the number of seconds ovnkube-node waits
	// at startup for its node's gateway and management port to be created in OVN
	// before it exits and restarts.
	// Clusters with many user-defined networks or network policies may need a
	// higher value, because a newly added node creates them only after it has
	// synced every network.
	// When omitted, this means no opinion and the platform is left to choose a
	// reasonable default, which is subject to change over time.
	// The current default is 300 seconds.
	// When set, the value must be between 1 and 3600 seconds, inclusive.
	// Changing this value restarts the ovnkube-node pods.
	// +openshift:enable:FeatureGate=OVNKubernetesStartupReadinessTimeout
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	// +optional
	StartupReadinessTimeoutSeconds int32 `json:"startupReadinessTimeoutSeconds,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

ovnkube-node runs on the guest cluster's nodes and is rendered by CNO, so
the flag works the same way once the field is set. This proposal does not
add the field to the `HostedCluster` API, so hosted cluster
administrators cannot set it in this phase. See Open Questions.

#### Standalone Clusters

This change is relevant for standalone clusters, which are the primary
target.

#### Single-node Deployments or MicroShift

There is no added resource use. The field only changes how long
ovnkube-node waits at startup. On single-node OpenShift, a larger value
delays restart of a node that cannot become ready, which is the intended
trade-off.

MicroShift configures OVN-Kubernetes through its own configuration file,
not through CNO, so it is not affected. Exposing the option there is out
of scope.

#### OpenShift Kubernetes Engine

OVN-Kubernetes is the default network plugin in OKE, so the field is
available there. It is most useful with user-defined networks and
multi-network policies.

### Implementation Details/Notes/Constraints

**openshift/api.** Register the gate in `features/features.go` with
`reportProblemsToJiraComponent("Networking/ovn-kubernetes")`,
`contactPerson("cragr")`, `productScope(ocpSpecific)`, and
`enhancementPR(...)` pointing at this enhancement. Run `make update` to
regenerate the CRD manifests for each feature set, and add the API
validation tests under
`operator/v1/tests/networks.operator.openshift.io/OVNKubernetesStartupReadinessTimeout.yaml`.

**cluster-network-operator.**

* In `renderOVNKubernetes()` in `pkg/network/ovn_kubernetes.go`, add
  `data.Data["StartupReadinessTimeoutSeconds"]` from the new field.
* In `start-ovnkube-node()` in
  `bindata/network/ovn-kubernetes/common/008-script-lib.yaml`, add the
  flag when the value is set. Guard it with a check that the binary
  supports the flag, as the script already does for
  `--enable-interconnect`:

  ```bash
  startup_readiness_timeout_opt=
  {{- if .StartupReadinessTimeoutSeconds }}
  if /usr/bin/ovnkube --help 2>&1 | grep -q -- '--startup-readiness-timeout'; then
    startup_readiness_timeout_opt="--startup-readiness-timeout {{.StartupReadinessTimeoutSeconds}}"
  fi
  {{- end }}
  ```

* Add `${startup_readiness_timeout_opt}` to the `exec /usr/bin/ovnkube`
  arguments.
* The field is mutable. `isOVNKubernetesChangeSafe()` does not need to
  block changes, which roll the ovnkube-node DaemonSet.

**ovn-kubernetes.** The upstream change is integer seconds rather than a
`time.Duration`, because the upstream config parser reads a
`time.Duration` from the config file as nanoseconds.

### Risks and Mitigations

* **A large value delays recovery from unrelated failures.** If a node
  cannot become ready for another reason, ovnkube-node waits longer
  before exiting and restarting. The maximum of 3600 seconds bounds this,
  and the default is unchanged.
* **The wait does not respond to cancellation.** The existing readiness
  wait is not tied to the startup context, so a longer timeout can delay
  graceful shutdown during a rollout. This predates the change and is
  bounded by the pod's termination grace period.
* **Mixed versions during upgrade.** The `--help` guard prevents an older
  ovnkube binary from failing on an unknown flag.

Security impact is limited to a cluster-scoped configuration field that
only cluster administrators can change. No new permissions are added.

### Drawbacks

This adds a tuning knob for a symptom whose cause is the serial initial
sync. Administrators must notice the failure and pick a value. A faster
or parallel sync would remove the need for the knob, but it is a larger
upstream change, and the knob remains useful as a safety valve.

## Alternatives (Not Implemented)

* **Raise the default in CNO.** CNO could pass a larger value on every
  cluster without an API change. This changes behavior everywhere and
  delays restarts for unrelated failures on clusters that do not need it.
* **Raise the default upstream.** Same trade-off as above, for all
  ovn-kubernetes users.
* **Edit the ovnkube-config ConfigMap.** This requires stopping CNO
  reconciliation, which is unsupported and blocks upgrades.
* **Scale the timeout with the number of networks.** This is harder to
  predict, and sync time also depends on policy count and node size.

## Open Questions [optional]

1. Should the field also be exposed through the HyperShift `HostedCluster`
   network operator configuration?
2. Is 3600 seconds the right maximum, or should very large clusters be
   allowed more?

## Test Plan

* **openshift/api:** validation tests for the new field: accepted values,
  rejection of values below 1 and above the maximum, and absence when the
  gate is disabled.
* **cluster-network-operator:** unit tests that render the ovnkube-node
  script with the field unset (no flag), set (flag present with the
  value), and with the gate on and off.
* **ovn-kubernetes:** upstream unit tests cover config parsing, CLI
  precedence, validation, and that the waiter fails at the configured
  deadline.
* **End to end:** a TechPreview CI job sets the field and verifies that
  ovnkube-node starts and logs the configured timeout. Reproducing the
  original failure needs a large scale environment, so it is covered by
  scale testing rather than per-PR e2e.

## Graduation Criteria

### Dev Preview -> Tech Preview

* The field is available behind the feature gate in TechPreviewNoUpgrade.
* CNO renders the flag, with unit tests.
* The carried upstream change is in openshift/ovn-kubernetes.

### Tech Preview -> GA

* e2e coverage in TechPreview CI jobs meets the feature gate promotion
  threshold.
* A scale test reproduces the cold-start failure and shows that raising
  the field fixes it.
* User documentation in openshift-docs describes the field and when to
  raise it.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

On upgrade, clusters keep the current behavior until an administrator sets
the field. No action is needed to keep previous behavior.

On downgrade to a release without the field, the API server drops the
unknown field and the older CNO renders no flag, so ovnkube-node returns
to the 300-second default.

## Version Skew Strategy

During an upgrade, the new CNO can render the script before every node
runs an ovnkube binary with the flag. The `--help` guard in the script
omits the flag on older binaries, so they start with the default instead
of failing. Only ovnkube-node reads the setting, so there is no skew
between the control plane and nodes.

## Operational Aspects of API Extensions

The change adds one optional field to an existing CRD. It does not affect
API throughput or availability. Changing the field rolls the ovnkube-node
DaemonSet, which restarts each node's ovnkube-node pod in turn, in the
same way as other OVN-Kubernetes configuration changes. Escalations go to
the OVN-Kubernetes networking team.

## Support Procedures

* **Detecting the failure:** ovnkube-node logs
  `error waiting for node readiness: context deadline exceeded` and the
  pod restarts repeatedly on a new or rebuilt node. Each attempt logs
  `Waiting up to <timeout> for gateway and management port readiness...`,
  which shows the timeout in use.
* **Checking the setting:**
  `oc get network.operator.openshift.io cluster -o jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.startupReadinessTimeoutSeconds}'`
* **Disabling:** remove the field. CNO rolls ovnkube-node back to the
  default. Running workloads are not affected, apart from the usual
  ovnkube-node restart during the roll.

## Infrastructure Needed [optional]

None.
