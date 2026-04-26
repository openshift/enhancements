---
title: nri-plugin-mutation-policy
authors:
  - "@amritansh1502"
  - "@ngopalak-redhat"
reviewers:
  - "@haircommander"
  - "@sgrunert"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-04-15
last-updated: 2026-04-15
status: provisional
tracking-link:
  - TBD
see-also: []
replaces: []
superseded-by: []
---

# NRI plugin mutation policy

## Summary

CRI-O merges NRI plugin adjustments and validates the combined result in a single step. This enhancement proposes a small, standalone NRI policy plugin—not policy logic embedded in the CRI-O daemon—that decides whether a workload’s Kubernetes namespace may receive that merged change. Policy is held in configuration outside the core daemon, with only minimal CRI-O settings to enable NRI and load the plugin. How that configuration is delivered and updated on OpenShift (for example node provisioning / Machine Config versus cluster API / Operator / CRD) is left to the enhancement design and alternatives.

## Motivation

Cluster administrators need a clear contract for which namespaces may receive NRI-driven container adjustments, and whether the cluster is observe-only or enforcing. That decision must apply to the **merged** adjustment from all plugins, not to a single plugin’s isolated `CreateContainer` contribution—otherwise other plugins’ changes can still be present in the merge and a reliable “block the combined mutation” rule is not possible.

### User Stories

- As a cluster administrator, I want to deploy a standalone NRI policy plugin and manage its rules through the OpenShift-aligned path we agree in this enhancement (node-level config and/or API-driven rollout), and to run permissive mode while tuning and strict mode when enforcing, so that I can control whether merged NRI container adjustments apply per namespace without relying on per-plugin `CreateContainer` logic that cannot see the full merged result.

### Goals

- Standalone NRI plugin that enforces namespace policy against **merged** container adjustments at the validation stage.
- Strict and permissive modes; namespace-scoped rules in v1.

### Non-Goals

## Proposal

### Enforcement point

The plugin participates in the NRI stage where CRI-O validates **merged** container adjustments after all contributing plugins have applied. In the NRI API this corresponds to **`ValidateContainerAdjustment`**—policy is evaluated there so approve/reject applies to the combined adjustment, not to one plugin’s slice of it.

### Policy schema (v1)

The on-disk file and any future CR **spec** should share the same shape so plugin behavior is identical regardless of delivery path. Use a `policies[]` (or equivalent) list so v1 can focus on namespace selection while later releases can add per-hook or per–mutation-type fields without a breaking redesign.

```yaml
spec:
  mode: strict
  policies:
    - namespaceSelector:
        matchNames: ["allowed-ns", "kube-system"]
    - namespaceSelector:
        matchNames: ["other-ns"]
```


### Proposal A: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node configuration. CRI-O enables NRI and loads the plugin through the normal NRI integration path.

- At validation time, the plugin evaluates the workload namespace against the configured rules (see **Policy schema (v1)**).

- Policy is read from a single canonical path on the node, e.g. `/etc/crio/nri_plugins/<PluginName>/config.yaml` (exact plugin name TBD).

- Changes ship via the Machine Config Operator: a `MachineConfig` for the target pool (typically `worker`) uses Ignition `storage.files` to install:
  1. the plugin binary at a stable, documented path on RHCOS (prefer paths aligned with node/runtime guidance, e.g. a dedicated directory under `/usr/local` or an agreed OCP path; avoid ad-hoc writes under `/usr` from operators);
  2. the policy file;
  3. a CRI-O drop-in under `/etc/crio/crio.conf.d/` that enables NRI and points CRI-O/NRI at the plugin and config as required by the implementation.

### Proposal B: API-managed policy (Operator + reconciliation)

- Expose mutation policy as a first-class Kubernetes/OpenShift API so administrators can create, update, and observe policy without authoring raw Ignition/MachineConfig for every change. Enforcement remains the same standalone NRI plugin; the Operator renders the **Policy schema (v1)** document for the plugin to read.

- Introduce a CRD for a cluster-scoped or namespaced policy object (exact name, scope, group/version TBD; API review). The **spec** matches the shape above.

- An Operator watches policy CRs, validates spec, and reconciles desired state to the fleet: reject invalid config with clear conditions; compute effective per-node policy (defaults, ordering, conflicts—TBD); update CR status (Available, Progressing, Degraded, per-node summary as appropriate); emit Kubernetes events for notable failures.

- The Operator runs a DaemonSet (one pod per node) that writes the rendered plugin config to a documented host path readable by CRI-O/NRI (and optionally verifies binary presence).


## Test Plan

.

## Graduation Criteria



### Dev Preview -> Tech Preview



### Tech Preview -> GA



**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature



## Upgrade / Downgrade Strategy



Upgrade expectations:


Downgrade expectations:


## Version Skew Strategy



## Operational Aspects of API Extensions





## Support Procedures


  

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
