---
title: sno-none-platform-internal-dns
authors:
  - pmtk
reviewers:
  - TBD # MCO: FeatureGate and static-pod rendering.
  - TBD # Assisted Installer: dnsmasq compatibility.
  - TBD # Runtimecfg: node-IP discovery and Corefile rendering.
  - TBD # DNS/networking: resolver behavior and required records.
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-09-07
last-updated: 2026-09-14
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3609
see-also:
  - "/enhancements/network/on-prem-mutable-vips.md"
  - "/enhancements/network/baremetal-networking.md"
replaces: []
superseded-by: []
---

# None-platform SNO internal DNS

## Summary

This enhancement adds an opt-in, host-local CoreDNS service for None-platform
Single Node OpenShift (SNO). The Machine Config Operator (MCO) deploys CoreDNS
when a new FeatureGate is enabled, replacing in the future the Assisted
Installer-specific dnsmasq.

## Motivation

Assisted Installer and agent-based installations inject a dnsmasq
MachineConfig to provide the DNS records required by SNO. Other installation
methods require user-managed DNS. This makes SNO behavior depend on the
installation method and leaves dnsmasq as an exception to MCO's existing
CoreDNS-based on-premise networking stack.

Reusing MCO's CoreDNS assets provides one implementation for new None-platform
SNO clusters while retaining user-managed DNS and existing installations.

### User Stories

* As a None-platform SNO administrator, I want OpenShift to provide its
  internal DNS records, so that I do not need to manage a local DNS service.
* As an Assisted Installer user, I want the same DNS implementation as other
  None-platform SNO installations, so that cluster behavior does not depend on
  the installer.
* As an administrator with external DNS, I want to leave host-local DNS
  disabled, so that OpenShift does not run a competing resolver.

### Goals

* Deploy MCO-managed CoreDNS on None-platform SNO when the FeatureGate is
  enabled.
* Resolve `api`, `api-int`, and `*.apps` records to the node's primary IP.
* Forward other queries to the node's upstream resolvers.
* Preserve current behavior when the FeatureGate is disabled.

### Non-Goals

* Adding platform-agnostic DNS selection to the Infrastructure API. This Dev
  Preview phase validates CoreDNS on None-platform SNO; a follow-up enhancement
  will define the long-term API.
* Adding a cluster capability.
* Supporting day-2 transitions from dnsmasq to CoreDNS outside a topology
  transition.
* Creating API or ingress virtual IPs.
* Deploying keepalived, HAProxy, or another load balancer.
* Removing the legacy dnsmasq implementation.

## Proposal

MCO adds SNO-specific CoreDNS assets under `templates/common/sno/files/`.
It renders them when all of the following conditions are true:

* Control-plane topology is `SingleReplica`.
* Platform type is `None`.
* The new FeatureGate is enabled.

MCO applies these conditions when rendering the control-plane MachineConfig
during bootstrap and in-cluster reconciliation. A topology transition updates
the rendered DNS assets to match the new topology.

The CoreDNS static pod uses host networking. Its init container runs
baremetal-runtimecfg without VIPs. Runtimecfg reads the node's primary IP and
upstream resolvers, then renders the Corefile. CoreDNS returns the primary IP
for the cluster records and forwards all other queries upstream.

The design reuses only the MCO assets required for DNS. It does not render
`corednsmonitor`, keepalived, HAProxy, or VIP-related assets. The SNO address
does not move between nodes. Whether changes to upstream resolvers must be
reflected without restarting the pod remains an open question.

```text
 primary-ip --------+
                    +--> runtimecfg --> Corefile --> CoreDNS :53
 upstream resolvers-+                               ^       |
                                                    |       +--> upstream DNS
                                      host resolver-+       +--> primary-ip
```

The existing `sno-dnsmasq.conf.yaml` is unchanged. Assisted Installer evaluates
the FeatureGate from `install-config.yaml` before generating its additional
manifests. When the gate is enabled, it does not add the dnsmasq MachineConfig.
When the gate is disabled, it retains its current behavior.

For other installers, leaving the gate disabled selects user-managed DNS. MCO
then renders neither CoreDNS nor another local resolver.

### Workflow Description

#### CoreDNS installation

1. The cluster creator enables the FeatureGate for a None-platform SNO
   installation.
2. Bootstrap MCO renders the SNO CoreDNS and resolver assets.
3. Runtimecfg waits for the primary IP, reads upstream resolvers, and renders
   the Corefile.
4. The host resolver sends queries to CoreDNS.
5. CoreDNS answers cluster records locally and forwards other queries.

#### Assisted installation

1. Assisted Installer evaluates the FeatureGate from `install-config.yaml`.
2. When the gate is enabled, it omits the dnsmasq MachineConfig from its
   additional manifests.
3. When the gate is disabled, it adds the dnsmasq MachineConfig as it does
   today.

#### User-managed DNS

1. The cluster creator leaves the FeatureGate disabled.
2. MCO renders no host-local DNS assets.
3. The cluster creator provides the DNS records required by installation.

### API Extensions

This enhancement adds a new FeatureGate to `openshift/api`, initially enabled
only in the `DevPreviewNoUpgrade` FeatureSet, targeted for OpenShift 5.1. The
gate is disabled by default.

Users enable the gate at installation by setting `featureSet: DevPreviewNoUpgrade`
in install-config.yaml.

Adding the gate to `openshift/api/features/features.go` constitutes an API
change and requires api-approvers review. No new CRDs, fields, or API
endpoints are proposed.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

Supported only on None-platform clusters with `SingleReplica` control-plane
topology.

#### Single-node Deployments or MicroShift

SNO is the target topology. The CoreDNS pod currently requests 100m CPU and
200Mi memory. Resource use will be measured without `corednsmonitor`.

MicroShift is not supported because it does not use MCO.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

#### Machine Config Operator

MCO will:

* Add `sno-coredns.yaml`.
* Add the Corefile and host-resolver assets used by the static pod.
* Render those assets from topology, platform, and FeatureGate state.
* Produce the same result during bootstrap and in-cluster reconciliation.

#### baremetal-runtimecfg

Runtimecfg adds a `--discover-node-ip` option. In this mode it waits for
`/run/nodeip-configuration/primary-ip`, renders that address into the Corefile,
and reads upstream resolvers from NetworkManager without requiring VIPs.

`nodeip-configuration.service` owns the primary-IP file. It runs after
NetworkManager is online and before `kubelet-dependencies.target`. It retries
until it finds a usable address, then writes the file. Kubelet requires that
target, so it cannot start the CoreDNS static pod before the file exists. The
file is recreated under `/run` after every boot.

#### Host resolver

MCO renders the minimum resolver-prepender assets needed to direct host
queries to CoreDNS. Their systemd ordering must prevent the host resolver from
using CoreDNS before the static pod is ready.

#### Assisted Installer

Assisted Installer generates `install-config.yaml` and invokes
`openshift-install`. It sets `featureSet: DevPreviewNoUpgrade` through its
install-config overrides, which enables all Dev Preview gates for the cluster.
Assisted Installer does not modify MCO's control-plane MachineConfig. It only
decides whether to add its separate dnsmasq MachineConfig based on the effective
FeatureGate.

#### Static-pod image

The CoreDNS image must be available before static-pod startup. If the release
image cache does not guarantee this, the implementation will reuse MCO's
on-premise image-pull service.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| CoreDNS and dnsmasq both bind port 53       | Assisted Installer omits dnsmasq when the FeatureGate is enabled. |
| The primary IP is not ready                 | Runtimecfg waits for the file; systemd ordering and bootstrap tests verify the contract. |
| The CoreDNS image is unavailable            | Verify release-image caching or reuse MCO's image-pull service. |
| The host resolver points to stopped CoreDNS | Render resolver and CoreDNS assets together; test reboot and failure behavior. |
| Upgrade enables CoreDNS unexpectedly        | Keep the gate disabled by default and in existing FeatureSets. |
| CoreDNS consumes excessive SNO resources    | Measure CPU and memory without `corednsmonitor`. |

Security review covers host networking, host-path mounts, and Linux
capabilities. Documentation lists the records required when the feature is
disabled.

### Drawbacks

SNO temporarily retains two host-local DNS implementations: CoreDNS for
FeatureGate-enabled clusters and dnsmasq for legacy assisted installations.

## Alternatives (Not Implemented)

### Retain assisted dnsmasq

This preserves installer-dependent behavior and does not provide local DNS to
other installation methods.

### Add an installer or Infrastructure API

An API would make DNS ownership explicit, but expands the first phase beyond
FeatureGate integration.

### Add a cluster capability

Cluster capabilities represent user-facing features, such as the web console.
Host-local DNS is an implementation detail of the SNO topology, not a product
feature.

Capabilities also cannot be disabled after they are enabled. A future topology
transition away from SNO would need to remove the host-local DNS assets. A
capability therefore has incompatible lifecycle semantics.

### Deploy corednsmonitor

The existing monitor tracks node addresses and NetworkManager's
`resolv.conf`. SNO does not need node-address tracking, but the monitor would
keep upstream resolvers current. Retaining the full sidecar adds its resource
cost to provide that update behavior.

## Open Questions [optional]

1. What is the FeatureGate name, and is it enabled by
   `DevPreviewNoUpgrade` or selected individually using `CustomNoUpgrade`?
2. What `productScope` applies? `ocpSpecific` seems right choice given
   MCO is OCP-specific.
3. Must changes to NetworkManager's upstream resolvers take effect without a
   CoreDNS pod restart? The existing `corednsmonitor` provides this behavior.

## Test Plan

Unit tests cover:

* MCO rendering across topology, platform, and FeatureGate combinations.
* Equal bootstrap and in-cluster rendering.
* Runtimecfg primary-IP discovery and Corefile generation for IPv4, IPv6, and dual-stack.
* Mutual exclusion of CoreDNS and dnsmasq assets.
* Assisted Installer omits dnsmasq when the FeatureGate is enabled and retains
  it when the gate is disabled.

End-to-end tests cover:

* New IPv4 and IPv6 None-platform SNO installations.
* Resolution of `api`, `api-int`, and `*.apps` records.
* Dual-stack SNO installations (resolution to both addresses).
* Forwarding to upstream resolvers.
* Absence of keepalived, HAProxy, and `corednsmonitor`.
* User-managed DNS with the gate disabled.
* Assisted CoreDNS and legacy dnsmasq installations.
* Reboot and static-pod image availability.

Upgrade tests verify that a disabled or absent gate adds no assets and that an
enabled gate affects only None-platform SNO. OCPEDGE-3016 tracks the detailed
test plan.

## Graduation Criteria

### Dev Preview -> Tech Preview

* API review is complete.
* Upgrade behavior is documented and tested (the `DevPreviewNoUpgrade`
  FeatureSet blocks both upgrades and downgrades during Dev Preview).
* Resource use is measured on constrained SNO hardware.
* End-to-end IPv4 and IPv6 tests pass.
* Assisted Installer prevents dnsmasq and CoreDNS from running together.

### Tech Preview -> GA

Not proposed in this enhancement.

### Removing a deprecated feature

Not applicable. Removing dnsmasq requires a separate enhancement with a
migration and support policy.

## Upgrade / Downgrade Strategy

This behavior is not enabled by default; existing clusters are unaffected.

Enabling the gate requires selecting the `DevPreviewNoUpgrade` (or
`CustomNoUpgrade`) FeatureSet, which irreversibly prevents cluster upgrades and
downgrades at the FeatureSet level. The individual gate does not block upgrades
by itself; the FeatureSet selection does.

Because upgrades are blocked, there is no code path for "upgrade with this gate
enabled" in the Dev Preview phase. MCO does not need to handle adding CoreDNS
assets to an already-running cluster during an upgrade at this stage.

A follow-up enhancement will define upgrade and downgrade transitions before
enabling the feature by default or promoting it beyond Dev Preview.

## Version Skew Strategy

Bootstrap and in-cluster MCO must make the same rendering decision. A newer
MCO renders CoreDNS only when the FeatureGate is enabled.

During Dev Preview this is an install-time-only decision, so there is no
version skew between components on a running cluster. Graduation to Tech
Preview or GA — where the gate may be enabled on existing clusters during
upgrade — will introduce version skew between the upgrading MCO and nodes
still running the previous version. The version skew section must be revisited
at that stage.

## Operational Aspects of API Extensions

The FeatureGate is install-time configuration with no day-2 transition.
Enabling it creates an MCO-rendered MachineConfig, a CoreDNS static pod,
runtimecfg initialization, and host-resolver configuration.

#### Failure modes

* CoreDNS static pod fails to start: the host resolver is configured to
  send queries to CoreDNS. If the pod is down, all host DNS resolution fails.
  Kubelet restart of the static pod is the primary recovery path.
* MachineConfig fails to render: MCO reports a Degraded condition on the
  machine-config ClusterOperator. The node remains in the previous
  configuration.
* Primary-IP file missing at boot: runtimecfg blocks until the file appears.
  systemd ordering prevents kubelet (and therefore the CoreDNS static pod)
  from starting before `nodeip-configuration.service` completes.

#### Monitoring

* This enhancement does not deploy `corednsmonitor`. CoreDNS health is
  observable through kubelet static-pod status and CoreDNS's own health
  endpoint. Whether additional monitoring is needed will be evaluated during
  Dev Preview.

## Support Procedures

Support inspects:

* The rendered MachineConfig and CoreDNS static-pod manifest.
* CoreDNS and runtimecfg logs.
* `/run/nodeip-configuration/primary-ip`.
* NetworkManager resolver state and `/etc/resolv.conf`.
* The legacy dnsmasq MachineConfig.
* The process bound to port 53.

## Infrastructure Needed [optional]

CI requires None-platform SNO coverage for IPv4, IPv6, dual-stack, upgrades,
user-managed DNS, Assisted CoreDNS, and legacy dnsmasq. Resource tests require
constrained SNO hardware.
