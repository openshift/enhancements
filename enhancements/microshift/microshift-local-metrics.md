---
title: microshift-local-metrics
authors:
  - "@eggfoobar"
reviewers: # Proposed reviewers; confirm participation during enhancement review.
  - "@pacevedom" # MicroShift lifecycle and observability integration.
  - "@pmtk" # Certificate lifecycle, configuration, and implementation scope.
  - "@copejon" # MicroShift architecture and operational behavior.
approvers:
  - "@pacevedom" # Proposed approver; not a record of approval.
api-approvers:
  - None
creation-date: 2026-10-30
last-updated: 2026-10-30
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2899
see-also:
  - /enhancements/microshift/microshift-certificate-rotation.md
---

# MicroShift Local Metrics and Certificate Observability

## Summary

Add an optional, loopback-only metrics endpoint directly to the MicroShift
process, with certificate health as its first collector. The endpoint exposes
Prometheus-format metrics that a host-local OpenTelemetry Collector can scrape;
it is not an OTLP receiver. It reuses the certificate inventory and status
calculation introduced by controlled certificate renewal, while keeping
observation read-only and separate from renewal and recovery. The listener does
not depend on kube-apiserver readiness, but shares MicroShift's process
lifetime. An isolated registry allows future MicroShift collectors to use the
same endpoint without modifying Kubernetes vendor code.

## Motivation

Controlled renewal lets an administrator prepare certificates while MicroShift
is running and activate them during a later maintenance restart. Administrators
can also disable the automatic restart at `ExpirationImminent`. These workflows
need a way to monitor certificate health across a fleet without repeatedly
invoking a privileged CLI on every node.

The existing `certs status` command remains the detailed, on-demand diagnostic
interface. Metrics provide periodic observations for customer dashboards, daily
summaries, and alerts. The customer chooses the collection pipeline, retention,
and notification policy; MicroShift does not install an alerting backend or
transmit these metrics externally by itself.

The tracking link identifies the originating certificate-renewal work. A
dedicated observability epic, release target, and ownership remain to be agreed;
this proposal does not add observability to that strategy's original acceptance
criteria implicitly.

### User Stories

- As a fleet administrator, I want to identify certificates approaching expiry
  and schedule renewal and activation before services are affected.
- As an administrator, I want pending renewal to be visible without making
  expiring active certificates appear healthy before activation.
- As an operator, I want to distinguish certificate health from failed
  collection, stale observations, and an unavailable MicroShift process.
- As an observability integrator, I want a local scrape target usable with an
  existing OTel Collector, without Kubernetes API authentication or new
  credentials.
- As a MicroShift developer, I want to add future bounded collectors to one
  owned endpoint without adding another listener or patching kube-apiserver.

### Goals

- Expose the managed certificate inventory's dates and existing five statuses.
- Read public certificates and pending metadata without changing certificate
  state or reading private keys.
- Serve cached observations independently of kube-apiserver readiness.
- Make collection failures and observation freshness explicit.
- Keep dependencies, configuration, labels, and extension points small.

### Non-Goals

- Automatic renewal, activation, recovery, or changes to restart policy.
- An endpoint that survives MicroShift exit or early initialization failure.
- Reusing the API-server `/metrics` handler or the global Prometheus registry.
- Remote listening, a Kubernetes Service/Route, or Kubernetes authentication.
- An embedded OTel SDK, OTLP receiver, exporter process, or systemd unit.
- Certificate events, live TLS probing, or monitoring arbitrary workload and
  customer-supplied certificates outside the managed inventory.
- A new status enum, configurable thresholds, or a new persistent status API.
- Backend alert evaluation, daily report generation, or remote export by
  MicroShift.

## Proposal

### Workflow Description

1. The administrator enables the endpoint in the existing MicroShift
   configuration file or a configuration drop-in, then restarts MicroShift:

   ```yaml
   metrics:
     status: Enabled
     listenAddress: 127.0.0.1:9090
   ```

2. After certificate initialization, MicroShift starts the listener before
   services whose startup waits for etcd or kube-apiserver readiness. Collection
   runs immediately in the background and then once per minute.
3. A host-local collector scrapes `http://127.0.0.1:9090/metrics`. An
   administrator can also inspect it with:

   ```sh
   curl --noproxy '*' --fail --max-time 5 http://127.0.0.1:9090/metrics
   ```

4. Preparing renewal with `microshift certs renew --serving` or `--ca` leaves
   active certificate dates and statuses unchanged. A subsequent successful
   observation reports pending renewal separately.
5. The administrator restarts MicroShift during the maintenance window. Existing
   startup code validates and activates or rejects the pending generation. The
   new process observes the resulting active files; the metrics collector does
   not participate in activation.

Setting `metrics.status: Disabled` and restarting disables the listener. These
settings use the normal startup-loaded configuration; this proposal introduces
no hot reload. Enabling metrics does not change
`certificates.forceRestartOnExpirationImminent` or any certificate lifetime.

### API Extensions

There are no Kubernetes API extensions, CRDs, webhooks, or API-server endpoints.
The external interfaces are a MicroShift configuration section and metric names,
types, and labels. They require compatibility review even though they are not
Kubernetes API objects.

| Configuration field     | Default          | Validation and application                                      |
| ----------------------- | ---------------- | --------------------------------------------------------------- |
| `metrics.status`        | `Disabled`       | Exactly `Enabled` or `Disabled`; applied at service start.      |
| `metrics.listenAddress` | `127.0.0.1:9090` | Literal loopback IP and port 1–65535; applied at service start. |

IPv6 is supported by an address such as `[::1]:9090`. One address is bound, not
both families automatically. Hostnames, wildcard addresses, non-loopback IPs,
zone-qualified addresses, and port zero are rejected. Invalid settings follow
normal configuration validation and prevent startup; this differs from a valid
configuration whose port cannot be bound at runtime.

Configuration defaults, generated schema, packaged configuration, and user
documentation must remain consistent.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not applicable. This endpoint runs in the MicroShift host process, not a hosted
control plane or a management-cluster operator.

#### Standalone Clusters

This proposal does not change OpenShift Container Platform standalone cluster
monitoring, component metrics, or cluster-monitoring-operator configuration.

#### Single-node Deployments or MicroShift

MicroShift is the target. Collection requires no additional pod and works
without an installed OTel Collector. Disconnected deployments can scrape
locally; remote delivery is the collector's responsibility when connectivity is
available. OpenShift single-node deployments that do not run MicroShift are
unchanged. Validate both supported MicroShift CPU architectures and RPM/bootc
installations, including SELinux enforcing mode.

#### OpenShift Kubernetes Engine

Not applicable; this does not introduce a feature into OKE installations.

### Implementation Details/Notes/Constraints

#### Shared read-only observations

Move the existing managed-chain definition from `pkg/cmd` into
`pkg/admin/certificates.NewChains`. Startup, CLI operations, and observation use
the same builder. This is an extraction, not a second hierarchy or a change to
issuance, configured validity, signing relationships, or renewal selection.

`certificates.Observe` accepts the running process's startup configuration, data
directory, operation-lock path, and observation time. It returns the public
certificate inventory, pending-record presence, and separate inventory and
pending errors. It uses the supplied time at UTC second precision, matching the
CLI's status evaluation. Certificate status remains based on encoded
`NotBefore`/`NotAfter` and inventory rotation policy, not a newly edited config
file or the requested lifetime for a future renewal.

The observation path:

1. Opens the existing certificate operation lock without creating it and takes a
   non-blocking shared lock. Contention or a missing lock produces an
   unavailable observation, not a healthy or empty inventory.
2. Checks for an interrupted activation. If recovery is needed, both inventory
   and pending state are unavailable; observation never performs recovery.
3. Calls the existing builder's `LoadInventory` to read active public
   certificates.
4. Reads and validates the bounded pending JSON record, independently of the
   active inventory. It does not traverse the staged PKI tree, read keys,
   compute activation hashes, or validate staged certificate expiration.
5. Releases the operation lock after reading. Startup and CLI writers retain
   their existing exclusive-lock behavior.

The pending reader reuses the renewal metadata format and parser. The existing
administrative `LoadPending` path must retain its stronger staged-tree checks,
including classifying a missing stage as invalid metadata. Splitting out a
metadata-only reader must not weaken startup's rejection or recovery behavior.

A structurally valid record means **renewal preparation was recorded**, not that
activation is currently possible. Its stage may have been removed, become
expired, or no longer match configuration or active files. These checks remain
the responsibility of startup and administration. Malformed records, unsupported
versions, or read failures are unknown pending state, not pending zero.

#### Certificate metric contract

All six families are gauges. Per-certificate labels are `name`, `service`,
`role`, and `rotation_policy`, sourced from the managed inventory definition.
`status` adds a fifth label only to the status family. Node and fleet identity
belong in the collection pipeline, not in certificate-derived labels.

| Metric                                                            | Labels                        | Meaning                                                                                                                                    |
| ----------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `microshift_certificate_not_before_timestamp_seconds`             | Per-certificate               | Active certificate validity start, Unix seconds.                                                                                           |
| `microshift_certificate_not_after_timestamp_seconds`              | Per-certificate               | Active certificate expiry, Unix seconds.                                                                                                   |
| `microshift_certificate_status`                                   | Per-certificate plus `status` | Five zero/one series: `Healthy`, `ExpiresSoon`, `ExpirationImminent`, `Expired`, `NotYetValid`; exactly one is 1 per observed certificate. |
| `microshift_certificate_renewal_pending`                          | None                          | 1 for a structurally valid pending record; 0 when no record exists; omitted if pending state is unknown.                                   |
| `microshift_certificate_inventory_collection_success`             | None                          | 1 only when the latest active-inventory and pending-metadata observation both succeeded; otherwise 0.                                      |
| `microshift_certificate_inventory_last_success_timestamp_seconds` | None                          | Observation time of the last complete success; 0 until the first success in this process.                                                  |

Call the existing `CertificateInventoryEntry.StatusAt` rather than duplicating
threshold calculations. Re-evaluate on every refresh even if files did not
change, including when the clock moves. Expired or not-yet-valid certificates
are successful observations of unhealthy certificates, not collection failures.

For an inventory of N certificates, a complete snapshot contains `7*N + 3`
series. Serial numbers, fingerprints, paths, subjects, SANs, PEM contents,
credentials, arbitrary errors, and raw configuration are not labels or values.
Replacing a certificate does not create a new identity label. A replacement
snapshot removes identities no longer in the inventory.

These metrics describe **active files on disk**, consistently with the CLI; they
do not attest to certificates cached by every running component or presented on
a live TLS connection. A pending generation cannot improve active certificate
health before activation.

#### Refresh, partial results, and freshness

One background worker refreshes the snapshot immediately and every minute.
Scrapes read an immutable snapshot protected by a short mutex; they do not read
files, acquire certificate locks, generate keys, or invoke CLI commands.

| Latest observation                               | Certificate series           | Pending gauge   | Collection success | Last success |
| ------------------------------------------------ | ---------------------------- | --------------- | ------------------ | ------------ |
| Before the first observation                     | Omitted                      | Omitted         | 0                  | 0            |
| Inventory and pending state readable             | Current snapshot             | 0 or 1          | 1                  | Updated      |
| Inventory unreadable or not evaluable            | Omitted                      | 0 or 1 if known | 0                  | Unchanged    |
| Pending metadata unreadable or invalid           | Current snapshot if readable | Omitted         | 0                  | Unchanged    |
| Lock unavailable or activation requires recovery | Omitted                      | Omitted         | 0                  | Unchanged    |

Do not retain old certificate values after a completed failed refresh or convert
unknown state to healthy. Retain only the last-success timestamp across failed
refreshes. If a refresh itself stalls, scrapes continue to return the previous
snapshot; consumers must check its age, not only the success gauge. Wall-clock
changes can affect both freshness and certificate status and must be considered
when diagnosing alerts. The last-success value resets on process restart.

#### Listener ownership and lifecycle

Use the already vendored Prometheus client, `net/http`, and MicroShift's service
manager. `pkg/metrics.Service` owns an isolated registry and accepts a variadic
list of collectors through `NewService(cfg, collectors...)`. The `Collector`
interface embeds `prometheus.Collector` and adds `Run(context.Context)` for
background collection. The service registers and starts each collector
independently and cancels their contexts on shutdown. Do not register the global
Go/process or Kubernetes registries implicitly. Collectors must publish cached,
bounded observations rather than doing blocking I/O in `Collect`.

The listener has no etcd/API readiness dependency. It starts after startup
certificate recovery, activation, and initialization have completed and the
operation lock has been released. A failure before this phase means no metrics
endpoint. A later fatal core-service failure can still stop MicroShift and the
listener; this proposal does not change that policy.

First-pass HTTP limits are two concurrent scrapes, a five-second metrics-handler
timeout, five-second header-read timeout, ten-second read/write timeouts,
thirty-second idle timeout, and 16 KiB request headers. Gather errors may still
expose successful families from other registered collectors. A bind or serving
failure is logged and does not stop the control plane. There is no automatic
listener retry; an administrator fixes the problem and restarts.

Service cancellation closes the listener and stops scheduling observations. The
first pass uses ordinary filesystem reads: the non-blocking operation lock does
not impose a hard timeout on an already-running filesystem read. HTTP serving
and service shutdown must not wait for that read. Do not claim arbitrary
collector code or a stuck filesystem can always be forcibly canceled.

#### OpenTelemetry integration

The selected wire format is Prometheus exposition over HTTP, consumed by an OTel
Prometheus receiver. It is not necessary to introduce a new metrics SDK or an
OTLP endpoint in MicroShift. The endpoint itself requires no certificate or
Kubernetes credentials, so CA rotation creates no new scrape credential to
refresh for this target.

The existing `microshift-observability` small, medium, and large presets already
load Prometheus scrape drop-ins. A separate integration change can add this
target through that mechanism, preserve customer-owned configuration, and attach
stable node identity. This proposal's first implementation slice covers the
observation API, listener, and certificate metrics only; it does not yet install
a drop-in or change those presets. Existing authenticated scrape targets and the
collector's own service/credential lifecycle remain separate concerns.

See the existing [scrape-drop-in
integration](https://github.com/openshift/microshift/blob/main/docs/contributor/observability_metric_exporters.md)
and [certificate administration
guide](https://github.com/openshift/microshift/blob/main/docs/user/howto_certificates.md).

### Risks and Mitigations

- **Local disclosure:** HTTP is unauthenticated. Any local user or workload
  sharing the host network may read the metrics. Loopback is not authentication;
  restrict output to the approved metadata, default to disabled, and obtain
  security review. Do not create a remote exposure mechanism.
- **False confidence:** pending is not activation readiness, scrape success is
  not collection success, and on-disk health is not live TLS health. Document
  these distinctions and make failures/freshness explicit.
- **PKI interference:** reuse the operation lock and public-certificate loader;
  keep mutation, private-key reads, full-tree hashing, and recovery out of the
  observer. Preserve stronger validation in administrative paths.
- **Resource pressure:** refresh once per minute, bound labels and requests,
  replace snapshots, and keep scrape work memory-only. Measure overhead on
  supported edge hardware before settling resource budgets.
- **Process coupling:** metrics disappear with MicroShift. Operators must
  monitor scrape failure or missing nodes separately; do not relax core-service
  failure policy just to keep this listener alive.
- **Platform policy:** verify binding and public-PKI reads on SELinux-enforcing
  RPM and bootc hosts. No policy change is assumed necessary or approved here;
  add only a demonstrated, narrowly scoped permission if testing requires it.

### Drawbacks

An additional listener and periodic reader add resource usage and a local
information surface. Cached observations can lag changes by a refresh interval.
Starting after certificate initialization deliberately leaves early startup
failures outside the endpoint's availability. A dedicated listener does not
solve monitoring during a complete MicroShift process outage.

## Alternatives (Not Implemented)

- **API-server metrics:** shares existing infrastructure but couples access to
  API startup/authentication and risks vendor modifications. The team selected a
  MicroShift-owned endpoint instead.
- **Separate exporter/systemd watcher:** can survive MicroShift exit, but adds
  another lifecycle, packaging, privilege, and compatibility boundary. That
  availability guarantee is outside the selected first scope.
- **Scheduled CLI/textfile collection:** useful for ad hoc automation, but CLI
  preparation can include recovery and is not the read-only observation API
  needed by an in-process collector.
- **OTLP push or an embedded OTel SDK:** adds pipeline configuration and remote
  dependencies to MicroShift despite an existing collector that can scrape.
- **Kubernetes events only:** useful supplementary context, but not a durable,
  periodic inventory and unavailable when the API cannot accept events.

## Open Questions

- Confirm security and observability reviewers for the endpoint and metric
  contract defined in this enhancement.
- Agree on an observability tracking epic, release target, and owners for
  collector packaging, QE, and product documentation.
- Set measured CPU, incremental memory, and scrape-latency budgets for minimum
  supported hardware. Request/time/series bounds above are implementation
  limits, not evidence that a performance budget has been met.
- Confirm metadata-only pending presence is sufficient for the first release;
  pending expiry or rejection-reason metrics would be a separate scope decision.
- Select the default scrape integration and node-identity convention, including
  custom collector configurations and upgrades. No remote backend is mandated.

## Test Plan

Reuse existing status, renewal, recovery, and configuration tests to verify that
extracting the chain definition and metadata parser preserves merged behavior.
The observability-specific coverage should focus on these boundaries:

- At a supplied time, metrics match `StatusAt` for all five statuses, both
  rotation policies, short/custom encoded lifetimes, and clock movement without
  certificate replacement. Check metric types, labels, zero/one encoding, and
  removal of obsolete identities.
- Observation reads public certificates even with invalid private-key contents,
  leaves files unchanged, never creates a missing operation lock, never repairs
  an interrupted activation, and does not access staged trees for metadata-only
  pending reporting. Administrative readers still reject a missing stage.
- Malformed/unsupported pending records, missing certificates, lock contention,
  and partial failures follow the table above. A subsequent successful refresh
  restores current data and advances last success.
- Exercise concurrent scrapes and refreshes with the race detector. Demonstrate
  that a slow observation does not block a second cached collector or listener
  shutdown; distinguish cancellation of scheduling from in-flight filesystem
  I/O.
- Validate defaults, configuration drop-ins, rejected addresses, IPv4/IPv6
  loopback listeners, disabled operation, busy-port behavior, and operation
  without API readiness or an installed OTel Collector.
- On a real MicroShift host, compare inventory identities and dates to CLI
  output; prepare renewal while running and verify active series do not change;
  restart and verify activated/rejected pending state on the next observation.
  Include custom validity and warn-only restart policy.
- Run RPM/bootc and architecture coverage with SELinux enforcing. Verify no
  non-loopback listener, secret-bearing output, new unsolicited outbound
  traffic, or change to startup/renewal/recovery semantics.
- In the later integration slice, verify all three bundled presets and a
  customer-managed host collector, stable fleet identity, customized config
  preservation, scrape/collection failures, and resource overhead.

Unit tests and a listener smoke test are not substitutes for host lifecycle,
SELinux, full renewal, or end-to-end OTel validation.

## Graduation Criteria

The feature targets GA directly, following the certificate-renewal enhancement.
The criteria below define release readiness, not a separate preview rollout. The
endpoint remains opt-in and disabled by default.

### Dev Preview -> Tech Preview

N/A. This feature is targeted for GA directly.

### Tech Preview -> GA

- Shared read-only certificate observations, the local metrics service, and
  certificate metrics are implemented with the configuration and metric contract
  defined here.
- Automated unit and end-to-end coverage runs in CI on supported RPM/bootc
  deployments and architectures with SELinux enforcing. Coverage includes
  renewal, activation/recovery, custom validity, restart policy, API-server
  unavailability, and upgrade/rollback behavior without changing the existing
  certificate lifecycle.
- End-to-end scraping through an OTel Collector is validated. The separate
  integration slice covers bundled presets and customer-managed collectors,
  preserves customized configuration, and provides stable node identity.
- ProdSec review is completed, including unauthenticated local access,
  loopback-only binding, and the absence of private keys or sensitive
  configuration in output.
- CPU, incremental memory, and scrape latency are measured on minimum supported
  hardware and meet budgets agreed with the MicroShift team.
- Metric names, types, labels, and status values are documented and validated in
  CI, including partial failures, freshness, and active-versus-pending state.
- End-user documentation is published in `openshift-docs`, covering enablement,
  collector setup, upgrade/rollback, troubleshooting, and alerting examples that
  distinguish unhealthy certificates from missing or stale observations.

### Removing a deprecated feature

Not applicable to this addition. A future removal or incompatible metric change
requires a separately documented deprecation and consumer migration plan.

## Upgrade / Downgrade Strategy

The endpoint is disabled by default and requires no new persistent observation
state. Existing certificate files and renewal records keep their formats and
owners; metrics do not write them. Existing administrators retain their current
renewal and restart policy unless they explicitly enable metrics.

Preserve user configuration during package upgrades. Downgrading to a version
without this feature removes the endpoint; remove its configuration section and
scrape target as appropriate for that version. Do not promise older binaries
recognize the new settings. With bootc/OSTree, deployment rollback is not a PKI
rollback: certificate material in persistent data remains subject to the
existing certificate lifecycle.

Collector packaging changes must preserve customized presets/drop-ins and
document how operators opt into a new scrape target. They are not part of the
initial endpoint-only implementation.

## Version Skew Strategy

There is no cross-node protocol: each collector scrapes its local MicroShift
instance. Older MicroShift versions will have no target or certificate families;
fleet queries must handle missing nodes and mixed rollout versions. Metric
names, types, labels, and status values follow the contract defined here. Once
released, incompatible changes require a compatibility/deprecation plan.

Shared parsing avoids a second interpretation of renewal metadata. An unknown
pending version makes that signal unavailable without changing or quarantining
the record. A readable active inventory can still be exported independently.

## Operational Aspects of API Extensions

No Kubernetes API objects or API availability guarantees are added. Configure
the listener through host configuration and restart MicroShift to apply changes.
Authentication, TLS credentials, Kubernetes RBAC, and API-server audit requests
are not part of this local endpoint. Existing CLI structured-output contracts
are unchanged.

Endpoint availability, successful collection, freshness, certificate status, and
pending presence are distinct signals. Backends must not treat an absent metric
as zero or a successful HTTP response as proof of healthy certificates.

## Support Procedures

1. Confirm the running MicroShift version, startup-loaded metrics configuration,
   and whether the process is alive. Editing configuration alone does not change
   the active listener.
2. Check the journal for listener startup/bind errors and inspect the loopback
   listener. If startup failed before certificate initialization completed,
   diagnose that failure through the existing logs and certificate CLI.
3. Scrape locally without a proxy. Check collection success and last-success age
   before interpreting certificate metrics; account for system clock changes.
4. Use `microshift certs status` for detailed active and pending information.
   Lock contention can temporarily make collection unavailable during a renewal;
   allow a subsequent refresh after that operation finishes.
5. Do not delete or repair certificate transaction files to restore metrics. Use
   documented certificate administration and startup recovery procedures.
6. If local scraping works but fleet data is missing, investigate the optional
   collector's target, process, identity, and export pipeline separately.

Support bundles should include relevant configuration and service diagnostics
using existing redaction rules, never private keys or raw credential-bearing
configuration added specifically for this collector.

## Infrastructure Needed

No new production service or hosted infrastructure is required. Use existing
MicroShift host/bootc CI for lifecycle and SELinux coverage, plus the existing
observability test infrastructure for the later collector-integration slice.
