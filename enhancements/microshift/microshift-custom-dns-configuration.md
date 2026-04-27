---
title: microshift-custom-dns-configuration
authors:
  - "@pacevedom"
reviewers:
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@dfroehli, for customer requirements and use case validation"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2026-04-24
last-updated: 2026-04-24
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2998
see-also:
  - N/A
---

# MicroShift Custom DNS Configuration

## Summary
MicroShift's internal DNS uses a CoreDNS configuration that only allows
limited customization through the `dns.hosts` option. This enhancement allows
users to provide a custom Corefile via a filesystem path in the MicroShift
configuration, fully replacing the default CoreDNS configuration for advanced
use cases such as split-horizon DNS, custom forwarding rules, or conditional
zone delegation.

## Motivation
MicroShift deploys CoreDNS with a hardcoded Corefile that covers the common
case: cluster-local resolution via the `kubernetes` plugin, forwarding to the
host's `/etc/resolv.conf`, and optional hosts file injection. However, edge
deployments may have DNS requirements that do not fit this default:

- Split-horizon DNS where internal and external names resolve differently.
- Custom forwarding to specific upstream resolvers per zone.
- Conditional zone delegation for multi-site or air-gapped environments.
- Additional CoreDNS plugins (e.g., `rewrite`, `template`, `file`) that
  require Corefile-level configuration.

The existing `dns.hosts` feature allows injecting additional host entries, but
does not cover these advanced scenarios. Users need full control over the
CoreDNS configuration without MicroShift overwriting it.

### User Stories
As a MicroShift administrator, I want to provide a custom CoreDNS Corefile so
that I can configure DNS resolution behavior for my edge deployment's specific
requirements.

As a MicroShift administrator, I want changes to my custom DNS configuration
file to be reflected at runtime without restarting MicroShift, so that DNS
changes do not cause application downtime.

As a MicroShift administrator operating a fleet, I want to manage DNS
configuration through filesystem-level tooling (e.g., rpm-ostree, image
builder, Ansible) so that it integrates with my existing device management
workflow.

### Goals
* Allow users to specify a custom Corefile via the MicroShift configuration.
* Apply custom DNS configuration without requiring changes to MicroShift code
  when new CoreDNS features are needed.
* Reflect changes to the custom Corefile at runtime without restarting
  MicroShift.
* Maintain mutual exclusivity between custom DNS configuration and the
  `dns.hosts` feature to avoid conflicting Corefile definitions.

### Non-Goals
* Validating Corefile syntax. CoreDNS is responsible for its own configuration
  validation. MicroShift will not import CoreDNS parsing logic to avoid
  coupling.
* Partial Corefile modifications or plugin injection. The custom configuration
  is a full replacement of the default Corefile.
* ConfigMap-based delivery of the custom configuration. Only filesystem-based
  delivery is supported, consistent with the existing `dns.hosts` pattern.
* Supporting multiple Corefile sources or merging configurations.

## Proposal
A new configuration option `dns.configFile` is introduced, pointing to a
filesystem path containing a complete CoreDNS Corefile. When this option is
set, MicroShift reads the file contents and uses them as the `Corefile` key in
the `dns-default` ConfigMap in the `openshift-dns` namespace, replacing the
default template-rendered configuration entirely.

The proposed configuration addition:
```yaml
dns:
  # ...existing fields...
  configFile: /etc/microshift/dns/Corefile  # optional, empty by default
```

When `dns.configFile` is empty or not specified, MicroShift renders the default
Corefile from its embedded template, preserving current behavior.

When `dns.configFile` is set and `dns.hosts.status` is `Enabled`, MicroShift
startup MUST fail with a clear error message. These options are mutually
exclusive because the `dns.hosts` feature injects a `hosts` plugin block into
the default Corefile template; a user-provided Corefile replaces the template
entirely, making the injection point undefined.

### Workflow Description
**cluster admin** is a human user responsible for configuring a MicroShift
instance.

1. The cluster admin creates a Corefile at a path on the host filesystem (e.g.,
   `/etc/microshift/dns/Corefile`) with the desired CoreDNS configuration.
2. The cluster admin sets `dns.configFile` in the MicroShift configuration to
   point to that file.
3. On start, MicroShift validates mutual exclusivity with `dns.hosts`, reads
   the file, and creates the `dns-default` ConfigMap with the file contents.
4. CoreDNS pods pick up the ConfigMap and load the custom configuration.
5. If the cluster admin modifies the Corefile on disk while MicroShift is
   running, the file watcher detects the change, updates the ConfigMap, and
   CoreDNS reloads the configuration automatically (provided the user includes
   the `reload` plugin in their Corefile).

#### Error handling
- If the file specified in `dns.configFile` does not exist at startup,
  MicroShift fails to start with an error.
- If the file is empty, not readable, or exceeds the 1 MiB ConfigMap size
  limit, MicroShift fails to start with a descriptive error.
- If the file is deleted or becomes unreadable while MicroShift is running, the
  watcher logs an error but the last known good ConfigMap remains in place.
- If the user provides a syntactically invalid Corefile, CoreDNS will fail to
  start or reload. MicroShift does not validate Corefile syntax.

### API Extensions
A new field is added to the MicroShift configuration file:
```yaml
dns:
  configFile: <string>  # optional, defaults to empty
```

This is a MicroShift-specific configuration option, not a Kubernetes API
extension. No CRDs, webhooks, or aggregated API servers are introduced.

### Topology Considerations
#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### OpenShift Kubernetes Engine
N/A. This enhancement is specific to MicroShift and does not depend on features
excluded from the OKE product offering.

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift. No additional resource
consumption beyond the existing CoreDNS deployment. The file watcher adds
negligible overhead, consistent with the existing hosts file watcher.

### Implementation Details/Notes/Constraints

#### Config struct changes
The `DNS` struct in `pkg/config/dns.go` gains a new field:
```diff
 type DNS struct {
     BaseDomain string      `json:"baseDomain"`
+    ConfigFile string      `json:"configFile,omitempty"`
     Hosts      HostsConfig `json:"hosts,omitempty"`
 }
```

#### Validation
The `validate()` method on `DNS` is extended with:
1. If `ConfigFile` is non-empty and `Hosts.Status` is `Enabled`, return an
   error: `"dns.configFile and dns.hosts are mutually exclusive"`.
2. If `ConfigFile` is non-empty:
   - The path must be absolute.
   - The file must exist and be readable.
   - The file must not be empty (0 bytes).
   - The file must not exceed 1 MiB (ConfigMap size limit).

These validations mirror the existing `dns.hosts` validation pattern.

#### Rendering pipeline
In `pkg/components/controllers.go`, the `startDNSController` function currently
renders the ConfigMap from the embedded template with `HostsEnabled` and
`ClusterIP` parameters. When `dns.configFile` is set, the rendering pipeline
skips the template and instead creates the ConfigMap directly from the file
contents, using the file content as the `Corefile` key value.

#### Runtime reload
A new file watcher, following the same pattern as `HostsWatcherManager` in
`pkg/controllers/hostswatcher.go`, watches the `dns.configFile` path using
`fsnotify`. On file change:
1. Compute SHA-256 hash and compare with last known hash.
2. If changed, read the file and update the `dns-default` ConfigMap in the
   `openshift-dns` namespace.
3. CoreDNS picks up the change via its `reload` plugin (if the user includes it
   in their Corefile).

The watcher also monitors the parent directory to handle file replacements
(e.g., atomic writes via `mv`), consistent with the hosts watcher.

#### Symlink considerations
If `dns.configFile` points to a symlink, `fsnotify` watches the symlink itself,
not the target. Changes to the target file may not trigger the watcher. This is
relevant for ostree/image-builder workflows where files may be symlinked. Users
should ensure the watched path is the actual file, or that their tooling
replaces the symlink itself (e.g., atomic symlink swap) rather than modifying
the target in place.

#### Configuration changes requiring restart
Adding or removing `dns.configFile` from the MicroShift configuration requires
a MicroShift restart to take effect. The file watcher is only started at boot
when `dns.configFile` is set; it cannot be started or stopped at runtime.
Similarly, reverting to the default Corefile by removing the `dns.configFile`
option requires a restart.

If the user does not include the `reload` plugin in their custom Corefile,
changes will only take effect after CoreDNS pods are restarted. This is a user
responsibility and will be documented.

#### Interaction with dns.hosts
The `dns.hosts` feature creates a separate ConfigMap (`hosts-file`) and injects
the `hosts` plugin into the default Corefile template. When `dns.configFile` is
set:
- The hosts watcher is not started (disabled).
- The `hosts-file` ConfigMap is deleted if it exists (cleanup from a prior
  configuration).
- The user is free to include the `hosts` plugin in their custom Corefile
  pointing to any source they choose.

### Risks and Mitigations

**Risk: Users break DNS with invalid configuration.**
An invalid Corefile will cause CoreDNS to enter CrashLoopBackOff, which means
no DNS resolution is available in the cluster. Since all in-cluster service
discovery depends on CoreDNS, this effectively renders the cluster
non-functional. MicroShift cannot automatically fall back to the default
configuration because the user has explicitly opted into full ownership of DNS.
Mitigation: Documentation will include a prominent warning that custom DNS
configuration is the user's responsibility and that an invalid Corefile will
break the cluster. The default Corefile is documented as a reference starting
point. Users are advised to validate their Corefile with CoreDNS tooling before
deploying it. Red Hat limits support to commercially reasonable efforts and may
request reproduction without the custom configuration.

**Risk: Missing `reload` plugin in custom Corefile prevents runtime updates.**
Mitigation: Documentation will recommend including the `reload` plugin and
explain the consequences of omitting it.

**Risk: Coupling with CoreDNS Corefile format across versions.**
Mitigation: Since MicroShift does not parse or validate the Corefile, there is
no coupling. Format changes in CoreDNS are the user's responsibility when using
a custom configuration.

**Risk: SELinux denials prevent reading the custom Corefile.**
On RHEL-based systems with SELinux enforcing, the custom Corefile must have the
appropriate SELinux context to be readable by the MicroShift process. A file
created without the proper context may cause silent read failures.
Mitigation: Documentation will include the required SELinux context for the
custom Corefile and provide instructions for setting it (e.g.,
`restorecon` or `semanage fcontext`).

**Risk: Invalid Corefile on bootc/ostree systems with greenboot.**
On systems using greenboot for health checking, a bad Corefile that breaks DNS
will cause health checks to fail, triggering an automatic rollback to the
previous deployment. This acts as a safety net, limiting the blast radius of
misconfigurations on edge devices. This behavior should be documented as an
expected interaction.

### Drawbacks
- Increases the support surface. Users with custom DNS configurations may
  report issues that stem from their own misconfiguration, requiring triage
  effort.
- Full Corefile replacement means users must track MicroShift's default Corefile
  across upgrades to ensure their custom configuration includes necessary
  plugins (e.g., `kubernetes` for cluster-local resolution).
- No syntax validation means failures are only observable at CoreDNS runtime,
  not at MicroShift startup.

## Design Details

#### Why filesystem-based delivery
MicroShift's configuration philosophy centers on filesystem-based management,
consistent with edge deployment patterns where devices are provisioned via
image builder, rpm-ostree, or configuration management tools. The existing
`dns.hosts` feature uses the same pattern: a file path in the MicroShift
config, watched for changes at runtime.

A ConfigMap-based approach (`dns.configMap` pointing to a `namespace/name`
reference) was considered but rejected because:
- It introduces a dependency on API server readiness for DNS configuration,
  creating a chicken-and-egg problem.
- It requires users to understand Kubernetes ConfigMap semantics for a
  host-level configuration task.
- It complicates persistence across reboots, as ConfigMaps live in etcd and
  must be restored or recreated.

#### Size limits
The 1 MiB limit is inherited from the Kubernetes ConfigMap size limit. This is
the same constraint applied to the `dns.hosts` file and is validated at
startup.

#### CoreDNS reload behavior
CoreDNS's built-in `reload` plugin checks the Corefile for changes every 30
seconds by default (configurable). When it detects a change, it gracefully
reloads the configuration. This means there is a delay of up to 30 seconds
between the ConfigMap update and CoreDNS applying the new configuration.

The `reload` plugin must be present in the user's custom Corefile for runtime
updates to work. If omitted, the user must restart CoreDNS pods manually.

## Open Questions
1. Should MicroShift provide a default Corefile as a reference file on disk
   (e.g., `/etc/microshift/dns/Corefile.default`) so that users have a starting
   point to copy and modify? This would help users who want to make small
   adjustments to the default without having to extract it from the source.

## Test Plan
All changes will be tested within MicroShift's existing e2e scenario testing
framework using RobotFramework.

Planned test cases:
- **Custom DNS config file**: Provide a valid custom Corefile and verify
  CoreDNS uses it instead of the default.
- **Mutual exclusivity**: Set both `dns.configFile` and `dns.hosts.status:
  Enabled`, verify MicroShift fails to start with the expected error message.
- **Runtime reload**: Modify the custom Corefile while MicroShift is running and
  verify CoreDNS picks up the change without restart.
- **Invalid file path**: Set `dns.configFile` to a non-existent path and verify
  MicroShift fails to start with a clear error.
- **File size limit**: Provide a file exceeding 1 MiB and verify MicroShift
  rejects it at startup.
- **Upgrade from default**: Upgrade from a version without `dns.configFile` to
  one with it, verify default behavior is preserved when the option is not set.
- **Cluster-local resolution**: Verify that a custom Corefile with the
  `kubernetes` plugin still resolves cluster-local services correctly.

## Graduation Criteria
Targeting GA for MicroShift 5.0 release.

### Dev Preview -> Tech Preview
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA
- More testing (upgrade, downgrade)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
**Upgrade**: When upgrading from a MicroShift version without `dns.configFile`
support, the field is absent from the configuration and defaults to empty. The
default Corefile template continues to be used. No action required from the
user unless they want to adopt the new feature.

**Downgrade**: When downgrading to a version without `dns.configFile` support,
the field is ignored by the older version. MicroShift reverts to the default
Corefile template. The custom Corefile remains on disk but is not used. DNS
resolution returns to default behavior.

In both cases, the `dns-default` ConfigMap is overwritten by MicroShift on
startup, so there is no risk of stale configuration persisting.

## Version Skew Strategy
N/A. MicroShift is a single-node deployment and all components are deployed
from the same version.

## Operational Aspects of API Extensions
No Kubernetes API extensions are introduced. The configuration change is
MicroShift-specific and does not affect API server behavior.

### Failure Modes
* If the configured file does not exist or is not readable, MicroShift will
  fail to start. This is observable in the MicroShift journal logs.
* If the Corefile syntax is invalid, CoreDNS will fail to start or reload. This
  is observable in CoreDNS pod logs and the pod will enter CrashLoopBackOff.
* If the file is deleted while MicroShift is running, the watcher logs an error
  but the last known good ConfigMap remains in place. DNS continues to work
  with the previous configuration.

## Support Procedures
- **Detecting custom DNS configuration**: Check the MicroShift configuration
  for a non-empty `dns.configFile` field. If set, the user has opted into
  custom DNS management.
- **Diagnosing DNS issues with custom config**: Inspect the `dns-default`
  ConfigMap in `openshift-dns` to see the active Corefile. Compare with the
  file on disk. Check CoreDNS pod logs for parsing or plugin errors.
- **Reverting to default**: Remove or empty the `dns.configFile` field from the
  MicroShift configuration and restart MicroShift. The default Corefile template
  will be restored. The custom Corefile remains on disk but is no longer used.
- **Switching to custom DNS**: Set `dns.configFile` in the MicroShift
  configuration and restart MicroShift. The watcher is only created at boot, so
  a restart is required for the new configuration to take effect.
- **Support boundary**: When custom DNS configuration is in use, Red Hat limits
  support to commercially reasonable efforts and may request reproduction
  without the custom configuration.

## Implementation History
N/A

## Alternatives (Not Implemented)
- **ConfigMap reference (`dns.configMap`)**: Pointing to a `namespace/name`
  ConfigMap in the cluster. Rejected because it introduces API server readiness
  dependencies, complicates persistence, and is inconsistent with MicroShift's
  filesystem-based configuration pattern. See [Design Details](#why-filesystem-based-delivery).
- **Partial Corefile modification**: Allowing users to inject additional plugin
  blocks or override specific sections of the default Corefile. Rejected
  because it is error-prone, introduces complex merge logic, and limits user
  flexibility compared to full replacement.
- **CoreDNS syntax validation at startup**: Importing CoreDNS's config parser
  to validate the Corefile before applying it. Rejected to avoid coupling
  MicroShift with CoreDNS internals and the maintenance burden of tracking
  CoreDNS parser changes across versions.

## Infrastructure Needed
N/A
