---
title: microshift-certificate-rotation
authors:
  - "@eggfoobar"
reviewers:
  - "@pacevedom"
  - "@copejon"
  - "@pmtk"
approvers:
  - "@pacevedom"
  - "@pmtk"
api-approvers:
  - None
creation-date: 2026-09-08
last-updated: 2026-09-09
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2899
see-also:
  - /enhancements/microshift/microshift-apiserver-certs.md
  - /enhancements/microshift/microshift-apiserver-custom-certs.md
---

# MicroShift Controlled Certificate and CA Renewal

## Summary

MicroShift currently includes a certificate validity monitoring system that
forces an unplanned process exit when any certificate enters the "red zone"
(approaching expiry). This behavior causes unplanned downtime on edge devices
deployed in environments where maintenance windows must be tightly controlled,
such as telecommunications networks.

This enhancement introduces controlled, on-demand certificate and CA renewal via
the `microshift certs` CLI subcommand family, allowing administrators to check
certificate status and trigger renewal during planned maintenance windows. The
forced process exit on red-zone certificates is made configurable via a
`certificates.autoRotate` configuration option. By default, MicroShift switches
to a warn-only policy logging warnings and surfacing certificate zone status via
`certs status` and healthcheck while giving administrators control over when
renewals and the associated service restart occur. Administrators who prefer the
existing auto-restart behavior can re-enable it through the configuration.
Administrators can also configure the validity of internally generated serving
and CA certificates; the defaults remain one year and ten years, respectively.

## Motivation

### User Stories

#### Story 1: Certificate Status Visibility

As a MicroShift administrator, I want to check the status of all certificates
managed by MicroShift so that I can proactively plan maintenance windows before
certificates approach expiry.

#### Story 2: Serving Certificate Renewal

As a MicroShift administrator, I want to renew serving and client certificates
without renewing the CA chain so that I can perform a quick, low-impact renewal
during a short maintenance window.

#### Story 3: CA Certificate Renewal

As a MicroShift administrator, I want to renew CA certificates (and all their
descendant certificates) when needed, with clear understanding of the impacts
(service restart, kubeconfig redistribution), so that I can plan the maintenance
window appropriately.

#### Story 4: Dry-Run Validation

As a MicroShift administrator, I want to preview what a renewal operation would
do before executing it, so that I can validate the scope of changes and
communicate the expected downtime to stakeholders.

#### Story 5: Configurable Red-Zone Behavior

As a MicroShift administrator running MicroShift at edge sites, I want the
red-zone forced process exit to be disabled by default and configurable, so that
I can maintain my SLA commitments and renew certificates during planned
maintenance windows, while organizations that prefer automatic restarts can opt
in to that behavior.

#### Story 6: Configurable Certificate Validity

As a MicroShift administrator, I want to configure the validity of internally
generated serving certificates and CAs so that the cluster's certificate policy,
including short-lived serving certificates, meets my organization's security
requirements.

### Goals

1. Provide a CLI-based mechanism to inspect the status of all MicroShift-managed
   certificates, including their expiry dates and zone classification
   (green/yellow/red).

2. Provide a CLI-based mechanism to renew serving/client certificates
   independently of CA certificates.

3. Provide a CLI-based mechanism to renew CA certificates with cascading renewal
   of all descendant certificates.

4. Support `--dry-run` for all renewal operations to allow validation before
   execution.

5. Make the forced process exit on red-zone certificates configurable. By
   default, switch to a warn-only policy (log warnings, surface status in `certs
   status` and healthcheck). Provide a configuration option
   (`certificates.autoRotate`) for administrators who prefer the existing
   auto-restart behavior.

6. Preserve the existing behavior where yellow-zone certificates are
   automatically regenerated on service start.

7. Introduce a PKI inventory abstraction that decouples CLI operations from the
   specific certificate layout, enabling future CA consolidation work.

8. Allow administrators to configure the validity of all internally generated
   serving certificates and CAs. Preserve the current one-year serving
   certificate and ten-year CA defaults when the settings are omitted.

### Non-Goals

1. Hot-reload of certificates without a service restart. Due to vendor code
   initialization semantics in the MicroShift binary, a full process restart is
   required after CA renewal. Hot-reload is deferred to a future enhancement.

2. Automated, scheduled certificate renewal. This enhancement provides the
   tooling for administrators to trigger renewal manually; automation of
   scheduling is out of scope.

3. Certificate Authority consolidation. The parallel effort tracked by
   OCPSTRAT-2900 will reduce the current 12 CAs to 1-3. This enhancement
   provides the PKI inventory abstraction that enables that work but does not
   implement the consolidation itself.

4. Custom certificate management. Custom certificates provided by administrators
   (covered by the `microshift-apiserver-custom-certs` enhancement) are not
   managed by this rotation mechanism.

5. Per-certificate validity overrides. Validity is configured once for all
   MicroShift-managed serving certificates and once for all MicroShift-managed
   CAs. User-provided `apiServer.namedCertificates`, ingress certificate
   secrets, and workload certificates are not affected.

## Proposal

### CLI Design

The `microshift certs` subcommand family follows existing CLI patterns
established by `microshift backup`, `microshift restore`, and `microshift
healthcheck`.

All commands require root privileges and operate on the MicroShift data
directory.

#### `microshift certs status`

Reports the status of all managed certificates. Safe to run while MicroShift is
running.

```console
$ sudo microshift certs status
CERTIFICATE              STATUS    EXPIRY                  REASON          MESSAGE
kube-apiserver-serving    Green     2027-03-15T10:30:00Z    NotExpiring     Valid for 553 days
etcd-serving              Yellow    2026-11-01T08:00:00Z    Expiring        Expires in 54 days
kubelet-client            Green     2027-03-15T10:30:00Z    NotExpiring     Valid for 553 days
admin-kubeconfig-signer   Green     2035-09-08T10:30:00Z    NotExpiring     Valid for 3287 days
service-ca                Green     2035-09-08T10:30:00Z    NotExpiring     Valid for 3287 days
...
```

#### `microshift certs renew --serving`

Renews all serving and client (leaf) certificates without touching the CA chain.
Requires MicroShift to be stopped.

```console
$ sudo systemctl stop microshift
$ sudo microshift certs renew --serving --dry-run
DRY RUN: Would renew the following certificates:
  - kube-apiserver-serving (expires 2027-03-15)
  - etcd-serving (expires 2026-11-01)
  - kubelet-client (expires 2027-03-15)
  ...
No CA certificates will be changed.

$ sudo microshift certs renew --serving
Renewed 18 serving/client certificates.
$ sudo systemctl start microshift
```

#### `microshift certs renew --ca`

Renews all rotatable CA certificates and cascades renewal to all descendant
serving and client certificates. Requires MicroShift to be stopped.

```console
$ sudo systemctl stop microshift
$ sudo microshift certs renew --ca --dry-run
DRY RUN: Would renew the following CAs and their descendants:
  CA: service-ca
    - openshift-controller-manager-serving
    - openshift-router-serving
    ...
  CA: kube-apiserver-lb-signer
    - kube-apiserver-lb-serving
  CA: admin-kubeconfig-signer
    - admin-kubeconfig-client
  ...
WARNING: After CA renewal, kubeconfigs stored outside the MicroShift data
directory must be manually re-copied.

$ sudo microshift certs renew --ca
Renewed 5 CAs and 18 descendant certificates.
WARNING: Kubeconfigs in /var/lib/microshift/resources/kubeadmin have been
regenerated. If you have copied kubeconfigs to other locations, you must
update those copies.
$ sudo systemctl start microshift
```

### Certificate Lifetime Configuration

MicroShift adds the following fields to `/etc/microshift/config.yaml`:

```yaml
certificates:
  autoRotate: false
  servingValidity: 8760h
  caValidity: 87600h
```

`servingValidity` applies to all serving certificates generated and managed by
MicroShift. `caValidity` applies to all CA certificates generated and managed by
MicroShift. The values use Go duration syntax. The defaults, `8760h` (365 days)
and `87600h` (3650 days), preserve the current one-year and ten-year validity. A
six-week serving-certificate policy is expressed as `servingValidity: 1008h`.

Both durations must be greater than zero, and `caValidity` must be greater than
`servingValidity` so that a CA does not expire before a serving certificate it
issues. Invalid configuration prevents MicroShift from starting and identifies
the invalid field. A descendant certificate's `NotAfter` must never exceed its
signing CA's `NotAfter`.

The configured durations apply when MicroShift creates or renews a certificate.
Changing either value does not rewrite existing PKI material or restart
MicroShift. Existing certificates retain their original `NotBefore` and
`NotAfter` values until they are explicitly renewed or regenerated by the normal
rotation path. The `certs status` command evaluates each existing certificate
using the validity encoded in that certificate, not the currently configured
duration.

### Certificate Zone Model

MicroShift identifies a short term and long term certificate and automatically
regenerates them when _manually restarted_ **and** _if_ they are near expiry.

Formula to determine renewals:

```
Short_Term = < 5 years
Long_Term = > 5 years
Earliest_Restart = 4 Months Before Short_Term Expires OR 12 Months Before Long_Term Expires

if Short_Term has less then 7 Months left
   renew Short_Term

if Long_Term has less then 18 Months left
   renew Long_Term

start Earliest_Restart deadline to force restart MicroShift
```

| Zone   | Criteria                                                             |
| ------ | -------------------------------------------------------------------- |
| Red    | 1 Year left on _long term_ **or** 4 Months left on _short term_      |
| Yellow | 18 Months left on _long term_ **or** 7 Months left on _short term_   |
| Green  | 19+ Months left on _long term_ **or** 8+ Months left on _short term_ |

Certificates are classified into zones based on their remaining validity:

| Certificate Type            | Total Validity | Green               | Yellow              | Red               |
| --------------------------- | -------------- | ------------------- | ------------------- | ----------------- |
| Short-term (serving/client) | 1 year         | 0-5 months elapsed  | 5-8 months elapsed  | 8+ months elapsed |
| Long-term (CA)              | 10 years       | 0-8.5 years elapsed | 8.5-9 years elapsed | 9+ years elapsed  |

**Behavioral changes by zone:**

- **Green zone**: No action needed. Certificates are valid and not approaching
  expiry.
- **Yellow zone**: Warning logged. Certificates in this zone are automatically
  regenerated when MicroShift is manually started (preserving existing
  behavior).
- **Red zone (changed)**: Behavior is now governed by the
  `certificates.autoRotate` configuration option:
  - **`autoRotate: false`** (new default): Warning logged; MicroShift does _not_
    force a process exit. The cluster continues to run until certificates are
    actually invalid. Near-expiry is visible in logs, `certs status` output
    (zone, time until `NotAfter`, whether auto-restart would have fired), and
    healthcheck. The administrator decides when to perform renewal in a
    maintenance window.
  - **`autoRotate: true`** (legacy behavior): MicroShift cancels the run context
    via `WhenToRotateAtEarliest` / `context.WithDeadline` in `pkg/cmd/run.go`,
    causing systemd to restart the process. This preserves the pre-enhancement
    behavior for administrators who prefer automatic rotation at the cost of
    unplanned downtime.

  > **Note:** The existing yellow-zone behavior where certificates are
  > automatically regenerated on manual service start (`certsToRegenerate`) is
  > _not_ affected by this configuration. The `autoRotate` setting controls only
  > the in-process red-zone deadline, not the "regenerate on start" behavior.

### PKI Inventory

The implementation introduces a PKI inventory abstraction that catalogs all
managed certificates by role (CA, serving, client, peer) and tracks the
parent-child relationships between CAs and their issued certificates.

Current MicroShift PKI layout:

- **12 Certificate Authorities**: including service-ca,
  kube-apiserver-lb-signer, kube-apiserver-localhost-signer,
  kube-apiserver-service-network-signer, admin-kubeconfig-signer, and others.
- **10 client certificates**: kubelet, admin-kubeconfig, controller-manager,
  etc.
- **6 serving certificates**: kube-apiserver (multiple signers), etcd,
  openshift-controller-manager, openshift-router.
- **2 peer certificates**: etcd peer, kubelet peer.

**Example Rotatable CAs** (renewed by `--ca`):

- `service-ca`
- `kube-apiserver-lb-signer`
- `kube-apiserver-localhost-signer`
- `kube-apiserver-service-network-signer`
- `admin-kubeconfig-signer`

This inventory abstraction decouples the CLI from the specific certificate
layout, enabling the parallel CA consolidation effort (OCPSTRAT-2900) to reduce
the number of CAs without modifying the CLI commands.

### Workflow Description

#### Serving/Client Certificate Renewal

1. Administrator runs `microshift certs status` to assess certificate state.
2. Administrator identifies certificates in yellow or red zone.
3. Administrator plans a maintenance window.
4. Administrator runs `microshift certs renew --serving --dry-run` to validate
   the scope.
5. Administrator stops MicroShift: `systemctl stop microshift`.
6. Administrator runs `microshift certs renew --serving`.
7. Administrator starts MicroShift: `systemctl start microshift`.
8. Workloads resume with renewed certificates. No kubeconfig redistribution
   needed.

#### CA Certificate Renewal

1. Steps 1-4 as above, using `--ca` flag.
2. Administrator communicates the maintenance window to dependent teams/systems,
   noting that kubeconfigs will be regenerated.
3. Administrator stops MicroShift: `systemctl stop microshift`.
4. Administrator runs `microshift certs renew --ca`.
5. Administrator starts MicroShift: `systemctl start microshift`.
6. Administrator copies regenerated kubeconfigs from the MicroShift data
   directory to any external locations where they were previously distributed.
7. Clients using the old kubeconfigs must obtain the updated copies.

#### Changing Certificate Validity

1. The administrator sets `certificates.servingValidity` and/or
   `certificates.caValidity` in `/etc/microshift/config.yaml`.
2. The administrator runs `sudo microshift show-config` to validate the file and
   inspect the effective durations. Existing certificates are not changed solely
   because their configured validity changed.
3. The administrator uses `microshift certs renew --serving` or `microshift
   certs renew --ca` during a maintenance window when the new validity should
   take effect.
4. Newly issued certificates use the configured duration. `certs status` reports
   their resulting `NotAfter` values and proportionally calculated zones.

### API Extensions

This enhancement does not introduce or modify Kubernetes API resources. It adds
the `certificates.autoRotate`, `certificates.servingValidity`, and
`certificates.caValidity` fields to the host-local MicroShift configuration
file.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A Not applicable to MicroShift.

#### Standalone Clusters

N/A Not applicable to MicroShift.

#### Single-node Deployments or MicroShift

This enhancement is exclusively for MicroShift. MicroShift runs as a single
process on a single node, so certificate renewal affects the entire cluster. The
service restart during CA renewal causes a brief, planned outage of the control
plane and all hosted workloads.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

#### Reuse of Existing PKI Functions

The implementation reuses the existing `certSetup` and `Regenerate` functions in
the MicroShift codebase. No new PKI writing mechanism is introduced, minimizing
the risk of divergence from tested code paths.

#### Cobra CLI Structure

The new subcommands follow the established cobra CLI patterns:

```
microshift
microshift backup
microshift restore
microshift healthcheck
microshift certs
microshift certs status
microshift certs renew
microshift certs renew --serving
microshift certs renew --serving --dry-run
microshift certs renew --ca
microshift certs renew --ca --dry-run
```

#### Service Stop Requirement

Actual renewal operations require MicroShift to be stopped. This is enforced
because:

1. Vendor code (etcd, kube-apiserver) initializes TLS materials at startup and
   does not support runtime reload.
2. Renewing certificates while the process holds open file handles to the old
   certificates could lead to inconsistent state.
3. The explicit stop/start flow gives administrators a clear boundary for their
   maintenance window.

The `status` subcommand and `--dry-run` flag are safe to use while MicroShift is
running.

#### Configurable Certificate Validity

The configuration implementation replaces hard-coded validity values for managed
serving certificates and CAs with the resolved `servingValidity` and
`caValidity` values. The existing expiration alignment performed by `certSetup`
is retained.

The PKI inventory records certificate role and rotation policy explicitly. This
replaces `IsCertShortLived` duration-based classification, which would
misclassify a CA configured with a validity shorter than five years. Both
`certsToRegenerate` and `WhenToRotateAtEarliest` calculate their thresholds as
fractions of each certificate's `NotAfter - NotBefore` validity, using the
policy recorded in the inventory.

Configuration generation adds the new fields and their defaults to the sample
configuration and schema. Configuration merging distinguishes an omitted value
from an invalid zero duration, and validation occurs before PKI initialization.

### Risks and Mitigations

**Risk**: Administrator forgets to redistribute kubeconfigs after CA renewal.

**Mitigation**: The CLI prints a clear warning after CA renewal, listing the
paths where kubeconfigs have been regenerated and reminding the administrator to
update external copies.

---

**Risk**: Running renewal while MicroShift is still running could corrupt state.

**Mitigation**: The `renew` subcommand checks whether the MicroShift process is
running and refuses to proceed if it is. Only `status` and `--dry-run` are
permitted while the service is active.

---

**Risk**: PKI layout changes (e.g., CA consolidation) break the renewal
commands.

**Mitigation**: The PKI inventory abstraction decouples the CLI from the
specific certificate layout. The inventory is the single source of truth for
which certificates exist and their parent-child relationships.

---

**Risk**: A very short configured validity increases renewal frequency and can
cause certificates to enter warning zones soon after issuance.

**Mitigation**: Zone thresholds scale with each certificate's encoded validity,
configuration validation rejects non-positive or inconsistent CA/serving
durations, and `certs status` exposes the resulting absolute expiration time.

### Drawbacks

- Requiring a full service restart for CA renewal introduces planned downtime.
  However, this is strictly better than the current behavior of unplanned forced
  exits.
- The host-local administrative approach requires access to the edge device,
  which may not be available in all deployment models. Future work could expose
  this functionality via an API.
- Global serving and CA validity settings do not allow different policies for
  individual managed certificates.

## Alternatives (Not Implemented)

### Alternative A: Hot-Reload of Certificates

Dynamically reload certificates in the running process without a restart. This
was rejected because:

1. Vendor code (etcd, kube-apiserver libraries) initializes TLS at startup and
   does not expose runtime reload hooks.
2. Implementing hot-reload would require significant upstream changes or
   forking.
3. The maintenance window approach aligns with telecommunications operator
   workflows where planned maintenance is standard practice.

Hot-reload remains a candidate for future enhancement.

### Alternative B: Automated Scheduled Renewal

Automatically renew certificates on a schedule (e.g., cron-based). This was
rejected because:

1. Edge devices may have constrained maintenance windows that do not align with
   a fixed schedule.
2. Automatic renewal without administrator awareness could disrupt dependent
   systems that rely on the kubeconfigs.
3. The controlled approach gives administrators explicit control over when the
   service restart occurs.

### Alternative C: Maintain Forced Exit on Red-Zone

Keep the existing behavior of forcing a process exit when certificates enter the
red zone. This was rejected because:

1. Telecommunications customers reported that unplanned restarts may violate
   their SLA commitments.
2. Forced exits provide no administrator control over timing.
3. The warning-based approach preserves observability while respecting operator
   maintenance windows.

### Alternative D: Retain Fixed Certificate Validity

Keep the current one-year serving-certificate and ten-year CA validity without
configuration. This was rejected because it does not allow security-sensitive
deployments to adopt shorter certificate lifetimes. The existing durations are
retained as defaults instead.

## Open Questions

1. Should `microshift certs renew` support renewing a specific certificate by
   name rather than the broad `--serving` / `--ca` categories?

2. What is the recommended integration with configuration management tools
   (e.g., Ansible, RHEL for Edge image builder) for automating the renewal
   workflow across a fleet?

3. Should the ProdSec review (OCPEDGE-3002) identify any certificates that
   require different rotation policies?

## Test Plan

### Unit Tests

- PKI inventory construction and parent-child relationship tracking.
- Zone classification for standard and extended policies across default and
  custom certificate durations.
- Certificate configuration defaulting and validation, including zero, negative,
  malformed, and serving-validity-greater-than-CA-validity values.
- Dry-run output generation.
- Service-running detection guard.

### Integration Tests

- End-to-end `certs status` output validation against a running MicroShift
  instance.
- End-to-end `certs renew --serving` followed by service restart, verifying all
  renewed certificates are valid and workloads resume.
- End-to-end `certs renew --ca` followed by service restart, verifying CA chain
  is renewed, descendant certificates are re-issued, and kubeconfigs are
  regenerated.
- Dry-run produces accurate output matching what the actual renewal would do.
- Renewal is refused while MicroShift is running.
- Validation that red-zone certificates produce warnings but do not force a
  process exit.
- A custom six-week `servingValidity` is reflected in newly issued serving
  certificates without placing them immediately in the yellow or red zone.
- A custom `caValidity` is reflected in newly issued CAs, and no descendant
  certificate expires after its signer.

### Scenario Tests

- Simulate certificate approaching red zone, verify warning behavior, execute
  planned renewal, verify recovery.
- Simulate CA renewal, verify kubeconfig regeneration, verify clients can
  authenticate with new kubeconfig.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A This feature is targeted for GA directly.

### Tech Preview -> GA

- All CLI commands implemented and tested (`certs status`, `certs renew
--serving`, `certs renew --ca`, `--dry-run`).
- Red-zone forced exit replaced with warnings.
- PKI inventory abstraction implemented and validated.
- Configurable serving and CA validity implemented with one-year and ten-year
  defaults and proportional rotation zones.
- ProdSec review completed (OCPEDGE-3002).
- Documentation published (OCPEDGE-3003).
- Automated test coverage in CI (OCPEDGE-3001).

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrade

On upgrade to a MicroShift version containing this enhancement:

- The `microshift certs` CLI subcommands become available.
- The red-zone forced exit behavior is replaced with warnings.
- No certificate renewal is triggered by the upgrade itself.
- Existing certificates remain valid with their current expiry dates.
- Omitted lifetime settings resolve to the existing one-year serving and
  ten-year CA defaults. Setting a custom lifetime affects only certificates
  issued after the setting is applied.

### Downgrade

On downgrade to a MicroShift version without this enhancement:

- The `microshift certs` CLI subcommands are no longer available.
- The red-zone forced exit behavior is restored.
- Certificates renewed by this enhancement remain valid; no rollback of
  certificate state occurs.
- The older binary ignores the unsupported `certificates` fields and returns to
  its hard-coded issuance and forced-restart behavior. Administrators should
  remove the fields to avoid assuming they are still effective. Certificates
  already issued with custom validity retain their encoded `NotAfter` values.

## Version Skew Strategy

N/A MicroShift is a single-binary deployment with no multi-component version
skew considerations.

## Operational Aspects of API Extensions

N/A No API extensions are introduced.

## Support Procedures

### Diagnosing Certificate Issues

1. Run `sudo microshift show-config` to verify the effective configured validity
   durations.
2. Run `sudo microshift certs status` to view the actual validity and state of
   each existing certificate.
3. Check MicroShift logs for certificate-related warnings: `journalctl -u
microshift -g "certificate"`.
4. If certificates are in red zone, plan a maintenance window and use the
   renewal commands.

### Recovery from Expired Certificates

1. Stop MicroShift: `sudo systemctl stop microshift`.
2. Renew all CAs and certificates: `sudo microshift certs renew --ca`.
3. Start MicroShift: `sudo systemctl start microshift`.
4. Redistribute regenerated kubeconfigs to external consumers.

## Infrastructure Needed

No additional infrastructure is required. All testing can be performed on
existing MicroShift CI infrastructure.
