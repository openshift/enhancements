---
title: skip-firstboot-pivot
authors:
  - "@djoshy"
reviewers:
  - "@jlebon, for coreos/ostree and bootstrap overlay precedent"
  - "@patrickdillon, for installer context"
  - "@yuqi-zhang"
  - "@sdodson"
approvers:
  - "@yuqi-zhang"
api-approvers:
  - "@yuqi-zhang"
  - "@everettraven"
  - "@joelspeed"
creation-date: 2026-10-14
last-updated: 2026-10-14
tracking-link:
  - https://redhat.atlassian.net/browse/MCO-2574
see-also:
replaces:
superseded-by:
---

# Skip Firstboot Node Image Pivot

## Summary

This proposal adds an opt-in mechanism that lets a joining RHCOS node skip its firstboot reboot. Today, the node performs an `rpm-ostree rebase` and reboots to reconcile the boot image against the payload node image. With this feature, the node live-overlays `/usr` from the payload node image onto the running filesystem. The node stages the full `rpm-ostree` deployment for convergence on the next cluster upgrade. The feature is gated behind `SkipFirstbootPivot` under `TechPreviewNoUpgrade`. 

## Motivation

Every RHCOS node that joins an OpenShift cluster pays a full reboot at firstboot. The `machine-config-daemon-firstboot` service reads the encapsulated MachineConfig, performs an `rpm-ostree rebase` to the payload node image, and reboots before the node can start kubelet. This reboot exists purely to reconcile the boot image against the payload and adds significant time to cluster installation and scale-out operations.

The bootstrap node already avoids this reboot. [openshift/installer#9256](https://github.com/openshift/installer/commit/60c63bb8bc) introduced a live overlay mechanism. It pulls the node image via `ostree container image pull`, checks it out to disk, and bind-mounts the new `/usr` over the running system. CRI-O, kubelet, and all other OpenShift binaries then start from the overlaid filesystem with no reboot. This proposal generalizes that mechanism to control-plane and worker nodes.

The bootimage skew enforcement feature ([MCO-1983](https://redhat.atlassian.net/browse/MCO-1983)) keeps boot images within a bounded version skew of the payload. This makes the live overlay practical: the delta between boot image and payload is small and predictable, so `rpm-ostree apply-live` can bridge it reliably.

This addresses [RFE-817](https://issues.redhat.com/browse/RFE-817), open since 2020 with multiple enterprise cases requesting faster node provisioning. Removing one reboot per node at install and scale-out directly reduces cluster bring-up time, which is a top priority for large baremetal and edge deployments.

### User Stories

* As a cluster administrator scaling out worker nodes, I want newly provisioned nodes to skip the firstboot reboot, so that scale-out operations complete in less time and I can meet demand faster.

* As a cluster administrator installing Single Node OpenShift (SNO), where every reboot takes the whole cluster offline, I want the node to skip the firstboot reboot so that installation completes with less disruption.

* As a developer running short-lived clusters for CI, I want nodes to skip the firstboot reboot so that cluster spin-up time and the associated cloud costs are reduced.

### Goals

- If the feature is enabled, eliminate the firstboot reboot for joining RHCOS nodes.
- Provide a clean fallback. If the overlay fails, the node falls back to the existing rebase-and-reboot path and joins normally.
- Converge the overlaid node onto the payload node image on the next cluster upgrade. The standard upgrade reboot handles this.
- Surface the overlay and drift state in node annotations and MCD status. These must be readable with `oc` and captured in must-gather.
- Make sure that overlaid nodes do not trip config-drift detection or degrade the MachineConfigPool.

### Non-Goals

- This does not target hosted control planes (Hypershift).
- This does not default the behavior on. The feature is opt-in and requires an explicit API field.
- This does not aim to replace `bootc apply-live` once it becomes available. The `rpm-ostree apply-live` approach is the interim mechanism. When `bootc apply-live` is production-ready, a follow-up enhancement will evaluate migration.

## Proposal

The MCO's template controller always renders the overlay systemd services, scripts, and generator into the Ignition config served to all nodes. These units are inert on existing nodes because the systemd generator requires both a flag file and the firstboot sentinel to activate. The `skipFirstbootPivot` field on the `MachineConfiguration` operator resource controls only whether the flag file is rendered. This design means that enabling the feature on Day-2 adds a single small file to the rendered MachineConfig, not the entire unit set. A NodeDisruptionPolicy entry marks this file as a no-reboot change, so existing nodes do not roll out.

When both the flag file and the firstboot sentinel are present, the overlay chain runs:

1. A systemd generator detects the overlay flag and replaces the default boot target with a custom overlay target.
2. The overlay target brings up minimal services (networking, sshd) and then runs the overlay chain: the pull service runs `rpm-ostree rebase` to stage the target deployment, then `rpm-ostree apply-live --allow-replacement` to overlay the target image's `/usr` onto the running system. It saves and restores kernel modules (the target image has a different kernel version) and reloads SELinux policy.
3. After the pull service completes, the finalize service starts openvswitch and restarts NetworkManager (these packages were added by apply-live but their services were not running), writes a marker file at `/etc/machine-config-daemon/firstboot-pivot-skipped`, and isolates to `multi-user.target`. CRI-O, kubelet, and the rest of the node services start using the overlaid binaries.
4. The existing `machine-config-daemon-firstboot` service runs after the finalize service. It detects the marker file, skips the OS rebase (already staged by the pull service), applies kargs and MC file changes, loads sysctl values, and skips the reboot.
5. The node joins Ready. The staged deployment persists on disk and takes effect on the next reboot (which happens during the next cluster upgrade).

If any step in the overlay chain fails, the overlay services exit with an error. The systemd generator's fallback behavior allows the existing `firstboot-osupdate.target` path to run, and the node reboots as it does today. The failure is logged and surfaced in MCD logs.

### Workflow Description

#### Day-0: Install with overlay enabled

**Cluster creator** is a human user deploying a new cluster.

1. The cluster creator adds `skipFirstbootPivot: Enabled` to the `install-config.yaml`. The installer generates a `MachineConfiguration` manifest with `skipFirstbootPivot: Enabled`.
2. The installer plumbs this field into the `ControllerConfig` CR via the standard install-config -> global CRs -> MCO operator -> ControllerConfig path.
3. The MCO template controller always renders the overlay systemd units, scripts, and generator into the MachineConfigs for all pools. Because `skipFirstbootPivot` is `Enabled`, the template controller also renders the flag file.
4. The render controller merges these into the rendered MachineConfig.
5. The Machine Config Server (MCS) serves the rendered MachineConfig (including the overlay units and flag file via Ignition) and appends the encapsulated MachineConfig as usual.
6. A joining node boots, Ignition writes all files. The systemd generator finds both the flag file and the firstboot sentinel, and activates the overlay target.
7. The overlay chain runs: the pull service stages the deployment via `rpm-ostree rebase` and applies it live via `rpm-ostree apply-live --allow-replacement`. The finalize service starts OVS, restarts NetworkManager, writes the marker file, and isolates to `multi-user.target`.
8. `machine-config-daemon-firstboot` runs after the finalize service. It detects the marker file, skips the OS rebase, applies kargs and MC file changes, loads sysctls, and skips the reboot.
9. The node starts kubelet, registers with the API server, and joins Ready.

#### Day-2: Enable overlay on an existing cluster

**Cluster administrator** is a human user managing a running cluster.

1. The cluster administrator sets `skipFirstbootPivot: Enabled` on the `MachineConfiguration` operator resource.
2. The MCO operator propagates the flag to the `ControllerConfig`.
3. The template controller re-renders MachineConfigs. The overlay units, scripts, and generator are already present (they are always rendered). The only change is the addition of the flag file at `/etc/machine-config-daemon/skip-firstboot-pivot`.
4. The render controller produces new rendered MachineConfigs. The config hash changes because of the new flag file.
5. A NodeDisruptionPolicy entry for the flag file path marks this change as "None" (no drain, no reboot). The MCD writes the flag file to existing nodes without disruption.
6. Existing nodes are not affected. The systemd generator requires both the flag file and the firstboot sentinel to activate. The firstboot sentinel does not exist on existing nodes.
7. Any new node that joins the cluster from this point forward uses the overlay path.

#### Overlay failure fallback

1. The overlay pull or apply-live fails (network issue, corrupt image, disk full, or OS generation mismatch).
2. The pull service exits with a non-zero status.
3. The `OnFailure=firstboot-pivot-skip-fallback.service` triggers and isolates to `multi-user.target`.
4. `machine-config-daemon-firstboot` runs the existing rebase-and-reboot path. If the pull service staged a deployment before the failure, the MCD reboots into it. If no deployment was staged, the MCD runs its own rebase and reboots.
5. The node joins after one reboot, the same as the standard firstboot path.
6. The overlay failure is logged in the pull service journal and can be seen in must-gather.

#### Convergence on upgrade

1. A cluster upgrade delivers a new rendered MachineConfig with a new `OSImageURL`.
2. The MCD on the overlaid node detects the config change via the standard `triggerUpdate` path.
3. `update()` calls `updateLayeredOS()` which rebases to the new image and reboots.
4. The node reboots into the new OS deployment. The apply-live overlay is gone (it does not persist across reboots). The on-disk ostree deployment is now the source of truth.
5. The node is fully converged. The drift annotations are cleared. The marker file at `/etc/machine-config-daemon/firstboot-pivot-skipped` remains on disk. The normal update path does not read this file, so it is inert. It serves as a record that the node originally joined via the overlay path.

### API Extensions

#### MachineConfiguration field

A new field `skipFirstbootPivot` is added to `MachineConfigurationSpec`:

```go
// FirstbootPivotSkipPolicy controls whether joining nodes skip the
// firstboot reboot by applying a live overlay.
// +kubebuilder:validation:Enum=Enabled;Disabled
type FirstbootPivotSkipPolicy string

const (
    FirstbootPivotSkipEnabled  FirstbootPivotSkipPolicy = "Enabled"
    FirstbootPivotSkipDisabled FirstbootPivotSkipPolicy = "Disabled"
)

type MachineConfigurationSpec struct {
    // ... existing fields ...

    // skipFirstbootPivot controls whether joining nodes use a live overlay
    // instead of rebooting at firstboot to reconcile the boot image against
    // the payload node image. When set to "Enabled", nodes apply the payload
    // node image live via rpm-ostree apply-live and stage the full deployment
    // for convergence on the next upgrade. When set to "Disabled" or omitted,
    // nodes reboot at firstboot as they do today.
    // +openshift:enable:FeatureGate=SkipFirstbootPivot
    // +optional
    SkipFirstbootPivot FirstbootPivotSkipPolicy `json:"skipFirstbootPivot,omitempty"`
}
```

When the field is omitted, the MCO treats it as disabled. The MCO operator reads it and propagates it to the `ControllerConfig`.

#### ControllerConfig field

A corresponding field is added to `ControllerConfigSpec` to carry the flag to the template controller:

```go
type ControllerConfigSpec struct {
    // ... existing fields ...

    // SkipFirstbootPivot indicates that joining nodes will use a live
    // overlay at firstboot instead of rebooting.
    // +optional
    SkipFirstbootPivot FirstbootPivotSkipPolicy `json:"skipFirstbootPivot,omitempty"`
}
```

The template controller uses this field to conditionally render the flag file via `{{if eq .SkipFirstbootPivot "Enabled"}}`. The overlay units, scripts, and generator are rendered unconditionally.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature does not apply to Hypershift. Hosted control planes manage node provisioning through NodePools and the hosted cluster's own node lifecycle, which does not use `machine-config-daemon-firstboot`.

#### Standalone Clusters

This is the primary target. The feature supports multi-node, compact (3-node), and SNO deployments on self-managed standalone clusters.

#### Single-node Deployments or MicroShift

SNO deployments are explicitly in scope. The overlay reduces the SNO installation time by one reboot cycle.

MicroShift does not use the MCO or `machine-config-daemon-firstboot`, so this proposal does not affect it.

#### OpenShift Kubernetes Engine

This feature does not depend on capabilities excluded from OKE. It operates at the OS and MCO level, which is common to both OCP and OKE.

### Implementation Details/Notes/Constraints
The implementation has an proof of concept that can be found at [openshift/machine-config-operator#4083](https://github.com/openshift/machine-config-operator/pull/4083). 

#### The overlay mechanism

The overlay uses `rpm-ostree rebase` to stage the target deployment and `rpm-ostree apply-live --allow-replacement` to overlay the target image's `/usr` onto the running system. This differs from the bootstrap node's approach (which uses `ostree container image pull` + checkout + bind-mount) because `apply-live` is simpler and handles `/etc` changes automatically.

The mechanism consists of:

1. **Systemd generator** (`/etc/systemd/system-generators/firstboot-pivot-skip-generator`): The generator checks for the overlay flag file and the firstboot sentinel. If both files exist, it symlinks `default.target` to `firstboot-pivot-skip.target`. Only minimal services run during the overlay phase. The generator also creates a `multi-user.target.wants` symlink to `firstboot-osupdate.target`. The existing `firstboot-osupdate.target` is only `WantedBy=default.target`. After isolation back to `multi-user.target`, the firstboot chain needs this link to start. Without the link, `machine-config-daemon-firstboot.service` does not run after isolation. The two-condition gate means the generator only activates during firstboot on flagged nodes.

2. **Overlay target** (`firstboot-pivot-skip.target`): A custom systemd target that replaces `multi-user.target` during the overlay phase. It requires `basic.target`, `sshd.service`, `getty.target`, `systemd-user-sessions.service`, and `firstboot-pivot-skip-finalize.service`. CRI-O, kubelet, and other services do not start until the overlay completes. The overlay services are not individually enabled. The target's `Requires=` directives pull them in transitively.

3. **Pull service** (`firstboot-pivot-skip-pull.service`): Runs the pull script. An `ExecStartPre` applies `chcon --reference=/usr/bin/ostree` to set the correct SELinux label on the script. The service uses `Wants=network.target` and `After=network.target NetworkManager.service` so that networking is available but a NetworkManager restart does not cascade-kill this service. The service has `OnFailure=firstboot-pivot-skip-fallback.service` so that a pull failure triggers the fallback to the normal reboot path. The pull script does the following steps:
   - Extracts the OS image URL from the encapsulated MachineConfig via `jq`.
   - If `/var/run/NetworkManager/resolv.conf` exists, copies it to `/etc/resolv.conf`. Some platforms configure NetworkManager with `rc-manager=unmanaged`, which means NM does not write `/etc/resolv.conf`. Services that populate `/etc/resolv.conf` only run under `multi-user.target`, so the copy gives the pull service DNS resolution under the minimal target.
   - Saves `/usr/lib/modules` to `/run/modules-save`. The target image has a different kernel version, so the running kernel's module directory disappears after apply-live.
   - Runs `rpm-ostree rebase --experimental ostree-unverified-registry:<image>` to stage the target deployment in a retry loop with a 120-second timeout. This must run before any ostree container image pull, because pulling registers the image ref and causes rpm-ostree to think the deployment already tracks it. The retry loop handles platforms where the external network interface takes time to become routable after boot.
   - Runs `rpm-ostree apply-live --allow-replacement` to overlay the target image's `/usr` onto the running system. This creates a read-only overlay on `/usr`.
   - Bind-mounts the saved kernel modules back over `/usr/lib/modules`. apply-live replaced the modules with the target image's version, which is built for a different kernel.
   - Runs `semodule -R` to reload SELinux policy from the target image so the running policy matches the new binary labels in the overlay.

4. **Finalize service** (`firstboot-pivot-skip-finalize.service`): Runs after the pull service completes. It also has `OnFailure=firstboot-pivot-skip-fallback.service`. It performs the following steps:
   - `systemctl daemon-reload` to pick up new unit files from apply-live.
   - `systemctl start openvswitch` because the openvswitch package was added by apply-live but its service was not running.
   - `systemctl restart NetworkManager` to load the NetworkManager-ovs plugin that apply-live added. This runs in the finalize service (not the pull service) because the pull service has an ordering dependency on NetworkManager.
   - Creates the marker file at `/etc/machine-config-daemon/firstboot-pivot-skipped`.
   - Runs `systemctl --no-block isolate multi-user.target` to return to normal boot. The service uses `KillMode=none` so systemd does not kill child processes during the target transition. CRI-O, kubelet, and all node services start with the overlaid binaries.

5. **Fallback service** (`firstboot-pivot-skip-fallback.service`): A oneshot service that runs `systemctl --no-block isolate multi-user.target`. Both the pull service and the finalize service declare it as their `OnFailure=` target. If either service fails, the fallback isolates to `multi-user.target` and the node continues through the normal reboot path.

6. **Flag file** (`/etc/machine-config-daemon/skip-firstboot-pivot`): Ignition writes this file from the rendered MachineConfig. Setting `ControllerConfigSpec.SkipFirstbootPivot` to `Enabled` causes the template controller to render this file. This is the only component that is conditionally rendered. All other overlay units, scripts, and the generator are always present. This file bridges the cluster-side feature gate and the firstboot systemd environment where no API access exists. A NodeDisruptionPolicy entry for this file path uses the `None` action, so adding or removing it does not trigger a node drain or reboot.

#### Systemd ordering

```
boot -> systemd generator (detects flag + sentinel)
     -> firstboot-pivot-skip.target (minimal: basic, sshd, networking)
        -> firstboot-pivot-skip-pull.service (rpm-ostree rebase + apply-live)
        -> firstboot-pivot-skip-finalize.service (start OVS, restart NM, write marker, isolate)
     -> multi-user.target
        -> crio.service, kubelet.service, ...
        -> machine-config-daemon-firstboot.service (MC files, kargs, sysctl, skip reboot)
```

The existing `machine-config-daemon-firstboot.service` still runs. It has `After=firstboot-pivot-skip-finalize.service` so it waits for the overlay chain to complete. When it detects the marker file, it skips the OS rebase (already staged by the pull service), applies MC file changes and kargs, runs `sysctl --system` to load sysctl values (normally applied on reboot), and skips the final reboot call.

#### Drift suppression

After the overlay, the node's `bootedOSImageURL` does not match the payload's `OSImageURL` because the node has not rebooted into the new deployment. The following code paths must account for this:

- **`RunFirstbootCompleteMachineconfig()`**: When the marker exists, sets `oldConfig.Spec.OSImageURL = mc.Spec.OSImageURL` so the diff reports `osUpdate: false`. The MCD applies MC file changes and kargs but skips the OS rebase and the reboot.

- **`applyLayeredOSChanges()`**: Normally calls `rpm-ostree cleanup -p` to remove any pending deployment before applying changes. When the marker exists, this call is skipped. Without this skip, the MCD removes the staged target deployment that the pull service created.

- **`checkStateOnFirstRun()`**: When the marker exists, accepts the OS image mismatch between the booted image and the stored config. Sets the `machineconfiguration.openshift.io/firstboot-pivot-skipped` annotation on the node object.

- **`validateOnDiskStateImpl()`**: When the marker exists, accepts the OS image mismatch during on-disk validation.

- **Config drift monitor** at `config_drift_monitor.go`: Watches files from the current MachineConfig. File and unit content match after the overlay because apply-live handles `/usr` and the MCD writes MC files to `/etc`. The OS image mismatch is the only expected drift.

The overlay marker file (`/etc/machine-config-daemon/firstboot-pivot-skipped`) serves as the signal. When this file exists, the MCD treats the OS image mismatch as intentional and does not degrade the node. The marker persists on disk after the convergence reboot. The normal update path does not read this file, so it is inert after convergence. It remains as a record that the node originally joined via the overlay path.

#### Node annotations

The MCD sets `machineconfiguration.openshift.io/firstboot-pivot-skipped` on the node object during bootstrap when the overlay path is taken. 

#### `rpm-ostree rebase` and `rpm-ostree apply-live`

Both commands run in the pull service during firstboot:

- `rpm-ostree rebase`: Stages a new OS deployment from the target container image. The staged deployment takes effect on the next reboot. This must run before any `ostree container image pull` because pulling registers the image ref and causes rpm-ostree to report "Old and new refs are equal."

- `rpm-ostree apply-live --allow-replacement`: Applies the staged deployment's content to the running system. It creates a read-only overlay on `/usr` with the target image's files. This gives the node up-to-date binaries without rebooting.

The MCD firstboot service no longer needs to run `rpm-ostree rebase` because the pull service already staged the deployment. The MCD detects the marker file and skips the OS update.

### Risks and Mitigations

**Kernel mismatch**: The overlaid node runs the boot image's kernel with the payload's userspace. If the payload requires a newer kernel for specific features or drivers, those features are not available until convergence. This is mitigated by preserving `/usr/lib/modules` from the booted kernel (identical to the bootstrap node's approach). Kernel-dependent workloads (GPU operators, DPDK, custom kernel modules) should not use this feature in TP. This will be documented in the "do not use if" list.

**`apply-live` directory change limitation**: `rpm-ostree apply-live` cannot overlay directory structure changes between the boot image and the target image. It fails with `error: Changed directories are not supported yet`. Different OS generations trigger this error (for example, RHCOS 9.x to RHCOS 10.x). The fallback service handles this: the node reboots through the normal path and joins the cluster without degradation. The bootimage skew enforcement feature keeps boot images within a bounded version skew, which prevents this in normal operation.

**SELinux policy mismatch**: The overlay replaces `/usr`, which includes SELinux policy modules. The `semodule -R` call in the pull script reloads policy to match the new binaries.

**`bootc apply-live` availability**: The current implementation uses `rpm-ostree apply-live`. The long-term replacement is `bootc apply-live` (containers/bootc#76). If `bootc apply-live` becomes available before GA, the implementation can migrate to it without changing the API surface.

**In-memory process state after overlay**: Processes that loaded binaries before the overlay keep the old binaries in memory. This is safe because the overlay runs before any OpenShift services start (the systemd target isolation guarantees this). systemd itself continues running from its already-loaded binary.

### Drawbacks

The primary drawback is operational complexity. Nodes that skip the firstboot reboot are in a "split" state: the running userspace comes from the overlay, but the on-disk deployment is still the boot image. This state persists until the next cluster upgrade triggers a reboot. Operators and support engineers must understand this state when debugging node issues.

The `rpm-ostree apply-live` overlay creates a read-only overlayfs on `/usr`. This is robust against accidental remounts, but processes that started before the overlay keep the old binaries in memory until they restart.

## Open Questions

1. **Day-0 API placement**: Where exactly does `skipFirstbootPivot` live in `install-config.yaml`? This needs installer team input.

2. **Telemetry**: Should the MCO report telemetry on how many nodes joined via the overlay path? This would inform the GA decision. The node annotation provides the raw data, but a Prometheus metric may be more useful.

3. **Assisted-installer and agent-based installs**: These install methods have constrained memory environments (everything runs in RAM). The `rpm-ostree rebase` and `apply-live` operations require disk space for the staged deployment and the overlay. On disk-booted nodes this is not a concern. In live environments (ISO/PXE), additional memory is required. The overlay services must detect the live environment and handle it appropriately.

## Test Plan

**E2E tests**:
- Install a cluster with the feature enabled. Assert that all nodes joined without a firstboot reboot and are running the payload-version kubelet and CRI-O.
- Scale out worker nodes with the feature enabled. Assert zero firstboot reboots.
- Install SNO with the feature enabled. Assert one fewer reboot in the install sequence.
- Trigger a cluster upgrade on an overlaid cluster. Assert that nodes reboot into the correct OS deployment (convergence).
- Induce an overlay failure (e.g., break the node image pull). Assert that the node falls back to the rebase-and-reboot path and joins normally.


## Graduation Criteria

### Dev Preview -> Tech Preview

This feature will be introduced in TechPreview, so no graduation from DevPreview will be needed. 

### Tech Preview -> GA

The standard graduation criteria for promoting a feature will be followed, with e2es described in the Test Plan section above.

### Removing a deprecated feature

Not applicable. This is a new feature with no prior version to deprecate.

## Operational Aspects of API Extensions

This feature does not introduce a new API object. The `MachineConfiguration` resource already exists and is reconciled by the MCO operator.

The node annotation `machineconfiguration.openshift.io/firstboot-pivot-skipped` indicates that the overlay path was taken. If a node is Ready without this annotation, the overlay failed and the node fell back to the standard rebase-and-reboot path. Both outcomes produce a healthy node. 

The overlay units, scripts, and generator are always present in the rendered MachineConfig. They are inert on existing nodes because the systemd generator requires both the flag file and the firstboot sentinel. If `ControllerConfig` does not set `SkipFirstbootPivot` to `Enabled`, the flag file is not rendered and the generator does not activate. If the overlay fails at runtime, the `OnFailure` fallback isolates to `multi-user.target` and the node follows the standard reboot path. Both are safe failure modes.

A built-in NodeDisruptionPolicy entry marks the flag file path (`/etc/machine-config-daemon/skip-firstboot-pivot`) with the `None` action. This means that enabling or disabling the feature on Day-2 does not drain or reboot existing nodes.

The MCO team (Machine Config Operator component in Jira) owns this feature.

## Support Procedures

**Detecting overlay state**:
- Check the node annotation: `oc get node <name> -o jsonpath='{.metadata.annotations.machineconfiguration\.openshift\.io/firstboot-pivot-skipped}'`. If the annotation exists, the node joined via the overlay and has not yet converged.
- Check MCD logs on the node: `journalctl -u machine-config-daemon-firstboot` will show whether the overlay path was taken and whether the reboot was skipped.
- Check the pull service logs: `journalctl -u firstboot-pivot-skip-pull` will show the rebase, apply-live, module restore, and SELinux reload.
- Check the finalize service logs: `journalctl -u firstboot-pivot-skip-finalize` will show the OVS start, NM restart, and marker creation.

**Disabling the feature**:
- Set `skipFirstbootPivot: Disabled` on the `MachineConfiguration` resource. New nodes will join via the standard reboot path. Existing overlaid nodes are not affected.

**Known issues to check**:
- If a node is in overlay state and experiences issues, first check whether the problem is kernel-related (the boot image kernel is running, not the payload kernel). If so, the workaround is to reboot the node, which applies the staged deployment and converges the kernel.

## Upgrade / Downgrade Strategy

**Upgrade**: Nodes that joined via the overlay have their staged deployment applied on the next upgrade reboot. After convergence, the nodes are indistinguishable from nodes that joined via the traditional reboot path.

**Downgrade**: Downgrading from a version with this feature to one without it removes the overlay systemd units from the rendered MachineConfig. Nodes that already joined via the overlay are unaffected (they are already running). New nodes joining after the downgrade use the standard rebase-and-reboot path. The overlay marker file on existing nodes becomes inert.

## Version Skew Strategy

The overlay mechanism runs entirely at firstboot, before the node joins the cluster. There is no version skew concern between components because:

- The overlay uses the node image from the payload, which is the same version as all other cluster components.
- The `rpm-ostree rebase` stages the same image, so the on-disk deployment matches the payload.
- The MCD that runs after firstboot (in cluster mode) is from the payload image and understands the overlay marker.

The only skew is the kernel: the boot image's kernel runs until the next reboot. This is acceptable because the OpenShift control plane does not depend on a specific kernel version, and kernel-sensitive workloads are excluded from this feature in TP (documented in the "do not use if" list).

## Alternatives (Not Implemented)

### Wait for `bootc apply-live`

`bootc apply-live` ([containers/bootc#76](https://github.com/containers/bootc/issues/76)) will provide a first-class mechanism for applying an OS update to the running system without rebooting. It will replace the `rpm-ostree apply-live` approach with a supported, upstream operation.

This alternative was not chosen because `bootc apply-live` is not yet available in RHCOS. The `rpm-ostree apply-live` approach works today and can ship in the near term. The API surface in this proposal is compatible with a future migration to `bootc apply-live` without user-facing changes.

### Implement the overlay in the MCD's Go code

Instead of adding systemd services, the overlay logic can be implemented as a Go function in the MCD's `RunFirstbootCompleteMachineconfig()` path. This function calls `ostree` commands, performs mounts via `syscall.Mount()`, and runs rsync and semodule as subprocesses.

This alternative was not chosen because the overlay must happen before CRI-O and kubelet start. The MCD firstboot service runs inside a podman container. That container starts after the container runtime is available. The systemd-level approach uses target isolation to run the overlay before any OpenShift services. This matches the bootstrap node's architecture.
