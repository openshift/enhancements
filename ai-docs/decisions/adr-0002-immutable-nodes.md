# ADR-0002: Immutable Node Infrastructure

**Status**: Accepted  
**Date**: 2026-06-24  
**Scope**: Cross-repository  

## Context

OpenShift nodes run the kubelet, container runtime (CRI-O), and other system-level services that must be consistent, predictable, and updatable across the cluster. Traditional Linux node management using package managers (yum/dnf) and configuration management tools (Ansible, Puppet) creates challenges:

- **Configuration drift**: Nodes diverge over time due to ad-hoc changes, failed partial updates, or inconsistent package versions
- **Non-atomic updates**: Package-based updates can fail partway through, leaving the system in an inconsistent state
- **Rollback difficulty**: Reverting a failed update requires understanding exactly which packages and configs changed
- **Testing combinatorial explosion**: Every combination of packages, versions, and configuration must be tested independently

## Decision

OpenShift nodes use an immutable infrastructure model built on four components:

1. **RHCOS (Red Hat CoreOS)**: A purpose-built, minimal operating system designed for running containerized workloads. Ships as a single versioned image tested with each OpenShift release.

2. **rpm-ostree**: Provides transactional, atomic operating system updates. The entire OS is deployed as an image-like tree — updates either fully succeed or fully roll back. Extensions (like `kernel-rt` or `usbguard`) can be layered atomically on top of the base image.

3. **Ignition**: Performs one-shot OS configuration at provision time based on the Ignition specification associated with each node's MachineConfigPool. This establishes the initial node state declaratively.

4. **Machine Config Operator (MCO)**: Manages day-2 node configuration changes through MachineConfig resources. The MCO renders desired configuration, and the MachineConfigDaemon (MCD) on each node applies changes by draining the node, applying updates (OS image, files, systemd units), rebooting, and uncordoning.

### Update flow

1. A new MachineConfig is created (by MCO or administrator)
2. MCO renders the desired configuration for each MachineConfigPool
3. MCD on each node detects the change
4. Nodes update one at a time per pool: drain → apply files and OS → reboot → uncordon
5. If the update fails, rpm-ostree rolls back to the previous OS tree on next boot

### Reboot-by-default principle

All MachineConfig changes require a node drain and reboot by default. This ensures the node reaches a known-good state rather than attempting live patching of a running system. Administrators can opt into specific no-reboot changes for selected files via Node Disruption Policy, but the immutable model remains the foundation.

## Rationale

- **Predictable state**: Every node in a pool runs the same OS image and configuration. There is no accumulated drift from manual edits or imperative commands.
- **Atomic rollback**: rpm-ostree's transactional model means a failed OS update automatically rolls back to the previous known-good tree. This is not possible with traditional package-by-package updates.
- **Reduced testing surface**: The entire OS stack (base + extensions) ships as a single version tested with each OpenShift release. All content is versioned with and tested with the OS, eliminating per-package combinatorial testing.
- **Security**: The read-only root filesystem and image-based deployment reduce the attack surface. Unauthorized modifications to the OS are detectable.

## Consequences

### Positive

- Nodes are reproducible — a new node joining a pool is identical to existing nodes
- OS updates are atomic and rollback-safe
- Configuration is declarative and auditable through MachineConfig resources
- Consistent across the fleet eliminates "works on my node" debugging

### Negative

- All configuration changes require a drain and reboot by default, which causes temporary workload disruption on that node
- Installing custom packages or agents requires building custom OS layers (on-cluster layering) rather than running `yum install`
- Ignition's one-shot model means some configuration fields are irreconcilable after initial provisioning — they cannot be changed without reprovisioning the node

### Neutral

- Administrators accustomed to SSH-based node management must adapt to the declarative MachineConfig model
- Debugging node-level issues uses `oc debug node/` rather than direct SSH access

## Alternatives Considered

### Alternative 1: Traditional package management (yum/dnf on RHEL)

**Description**: Run standard RHEL on nodes, manage packages via yum/dnf, configure via SSH or Ansible.

**Pros**:
- Familiar to Linux administrators
- Flexible, can install any package at any time
- No reboot required for many changes

**Cons**:
- Configuration drift over time
- Non-atomic updates can leave nodes in inconsistent state
- Rollback requires manual intervention
- Combinatorial testing burden across package versions

**Rejected because**: The operational cost of managing drift and non-atomic updates across a fleet of nodes outweighs the flexibility benefits. OpenShift's operator model depends on predictable, consistent node state.

### Alternative 2: Mutable configuration management (Ansible/Puppet/Chef)

**Description**: Use configuration management tools to continuously converge node state toward a desired specification.

**Pros**:
- Handles drift through continuous reconciliation
- Mature tooling ecosystem
- Does not require reboots

**Cons**:
- Convergence is not atomic — intermediate states during a run may be inconsistent
- Agent-based systems add operational complexity
- Configuration management DSLs add another abstraction layer alongside Kubernetes APIs

**Rejected because**: The Kubernetes operator model already provides declarative reconciliation. Adding a separate configuration management layer for nodes creates a split-brain between Kubernetes-managed and externally-managed state. MCO provides the same continuous reconciliation but integrated into the Kubernetes API model.

## References

- AI docs: [Design Philosophy — Immutable Infrastructure](../DESIGN_PHILOSOPHY.md)
- AI docs: [Upgrade strategies](../platform/openshift-specifics/upgrade-strategies.md)
- Enhancement: [Machine Config Node](../../enhancements/machine-config/machine-config-node.md)
- Enhancement: [RHCOS extensions](../../enhancements/rhcos/extensions.md)
- Enhancement: [On-cluster layering](../../enhancements/machine-config/on-cluster-layering.md)
- Enhancement: [Node Disruption Policy](../../enhancements/machine-config/admin-defined-node-disruption-policy.md)
