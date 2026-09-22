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
last-updated: 2026-09-22
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
`certificates.forceRestartOnRedZone` configuration option. The option defaults
to `true` to preserve existing behavior on upgrades. Administrators can set it
to `false` to use a warn-only policy that surfaces certificate zone status via
`certs status` and healthcheck without forcing an unplanned restart.
Administrators can also configure the validity of internally generated serving
and CA certificates; the defaults remain one year and ten years, respectively.
All certificate status, dry-run, and renewal operations provide JSON output for
fleet automation.

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
red-zone forced process exit to be configurable so that I can opt out of
unplanned restarts, maintain my SLA commitments, and renew certificates during
planned maintenance windows without changing the default behavior of existing
deployments.

#### Story 6: Configurable Certificate Validity

As a MicroShift administrator, I want to configure the validity of internally
generated serving certificates and CAs so that the cluster's certificate policy,
including short-lived serving certificates, meets my organization's security
requirements.

#### Story 7: Fleet Status Automation

As an administrator responsible for a fleet of MicroShift devices, I want
machine-readable certificate status and renewal results so that I can aggregate
expiry and impact data in external automation without parsing human-readable
tables.

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
   default, preserve the existing auto-restart behavior. Provide a
   `certificates.forceRestartOnRedZone` configuration option that administrators
   can disable to log warnings and surface status in `certs status` and
   healthcheck without forcing a restart.

6. Preserve the existing behavior where yellow-zone certificates are
   automatically regenerated on service start.

7. Introduce a PKI inventory abstraction that decouples CLI operations from the
   specific certificate layout, enabling future CA consolidation work.

8. Allow administrators to configure the validity of all internally generated
   serving certificates and CAs. Preserve the current one-year serving
   certificate and ten-year CA defaults when the settings are omitted.

9. Provide stable JSON output for certificate status, renewal dry-runs, and
   completed renewals so fleet-management systems can consume the results.

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

6. Selecting certificates for renewal by service or function. Renewal operates
   on the complete certificate category: `--serving` renews all managed leaf
   certificates, and `--ca` renews all managed CAs and their descendants. For
   example, this enhancement does not provide an etcd-only renewal operation.

7. Providing a fleet-management service, transport, or central data store.
   MicroShift provides machine-readable local command output; external systems
   are responsible for command execution, device identity, aggregation,
   retention, and alerting across the fleet.

## Proposal

### CLI Design

The `microshift certs` subcommand family follows existing CLI patterns
established by `microshift backup`, `microshift restore`, and `microshift
healthcheck`.

All commands require root privileges and operate on the MicroShift data
directory.

#### Output Formats

> Note: Since YAML is a superset of JSON, we are adding it as an output format
> but all examples and descriptions will call out JSON only for simplicity.

The `status` command and every `renew` operation, including `--dry-run`, accept
`-o json|yaml` or `--output=json|yaml`. Human-readable tables remain the default
when the flag is omitted. Successful JSON output writes exactly one JSON
document to standard output; progress and diagnostics go to standard error.
Warnings are included in the JSON document rather than mixed with standard
output. On failure, the command exits non-zero, leaves standard output empty,
and writes a JSON error object to standard error when JSON output was requested.
In that case, no additional human-readable diagnostics are written beside the
error document.

The JSON contract is versioned independently from the human-readable table.
Fields may be added compatibly, but existing fields and enum values are not
removed or redefined within a version. Timestamps use RFC 3339. Configured
validity durations use Go duration strings, while calculated remaining durations
use integer seconds. Unless a field is explicitly marked nullable below, it is
required and must not be `null`.

##### Error Document

JSON-mode failures return the following `Error` document on standard error:

| Field         | Type            | Nullable | Required value or meaning                         |
| ------------- | --------------- | -------- | ------------------------------------------------- |
| `apiVersion`  | string          | No       | `microshift.openshift.io/v1alpha1`                |
| `kind`        | string          | No       | `Error`                                           |
| `generatedAt` | RFC 3339 string | No       | Time at which the error document was generated    |
| `code`        | string          | No       | One of the stable codes defined below             |
| `message`     | string          | No       | Non-empty human-readable description              |
| `details`     | object          | Yes      | Structured context; `null` when none is available |

Allowed `Error.code` values are:

| Code                         | Meaning                                                             |
| ---------------------------- | ------------------------------------------------------------------- |
| `InvalidArguments`           | Command arguments or output options are invalid                     |
| `InvalidConfiguration`       | Effective MicroShift certificate configuration is invalid           |
| `InsufficientPrivileges`     | The command is not running with the required root privileges        |
| `MicroShiftRunning`          | Renewal was refused because the MicroShift service is active        |
| `CertificateInventoryFailed` | Managed certificate inventory or certificate data could not be read |
| `RenewalFailed`              | Renewal planning, validation, staging, or commit failed             |
| `RecoveryFailed`             | An interrupted renewal transaction could not be recovered           |
| `InternalError`              | An unexpected error not represented by another stable code          |

The error code identifies the failure class for automation; callers must not
parse `message`. Additional properties in `details` are code-specific and are
not part of the stable vocabulary unless separately documented.

```json
{
  "apiVersion": "microshift.openshift.io/v1alpha1",
  "kind": "Error",
  "generatedAt": "2026-09-08T10:30:00Z",
  "code": "MicroShiftRunning",
  "message": "MicroShift must be stopped before certificates can be renewed.",
  "details": {
    "service": "microshift.service",
    "state": "active"
  }
}
```

##### Status Document

`microshift certs status -o json` returns a `CertificateStatusList` document:

```json
{
  "apiVersion": "microshift.openshift.io/v1alpha1",
  "kind": "CertificateStatusList",
  "generatedAt": "2026-09-08T10:30:00Z",
  "config": {
    "forceRestartOnRedZone": true,
    "servingValidity": "8760h",
    "caValidity": "87600h"
  },
  "items": [
    {
      "service": "etcd",
      "name": "etcd-serving",
      "role": "peer",
      "rotationPolicy": "extended",
      "zone": "yellow",
      "notBefore": "2016-11-03T08:00:00Z",
      "notAfter": "2026-11-01T08:00:00Z",
      "remainingSeconds": 4656600
    }
  ],
  "warnings": []
}
```

##### Renewal Result Document

`microshift certs renew --serving|--ca [--dry-run] -o [json|yaml]` returns a
`CertificateRenewalResult` document with these top-level fields:

| Field         | Type             | Nullable | Required value or meaning                                           |
| ------------- | ---------------- | -------- | ------------------------------------------------------------------- |
| `apiVersion`  | string           | No       | `microshift.openshift.io/v1alpha1`                                  |
| `kind`        | string           | No       | `CertificateRenewalResult`                                          |
| `generatedAt` | RFC 3339 string  | No       | Time at which the result was generated                              |
| `mode`        | string           | No       | `serving` or `ca`                                                   |
| `status`      | string           | No       | `validated` or `completed`                                          |
| `dryRun`      | boolean          | No       | Whether the operation made no persistent changes                    |
| `items`       | non-empty array  | No       | All certificates selected by the operation                          |
| `impact`      | object           | No       | Operational actions required after the planned or completed renewal |
| `warnings`    | array of strings | No       | Human-readable warnings; an empty array when none apply             |

Every member of `items` contains:

| Field             | Type            | Nullable | Required value or meaning                                       |
| ----------------- | --------------- | -------- | --------------------------------------------------------------- |
| `service`         | string          | No       | Owning service or function                                      |
| `name`            | string          | No       | Stable inventory name                                           |
| `role`            | string          | No       | `ca`, `serving`, `client`, or `peer`                            |
| `parentCA`        | string          | Yes      | Parent CA inventory name; `null` when there is no parent        |
| `currentNotAfter` | RFC 3339 string | No       | Expiry before the planned or completed operation                |
| `newNotAfter`     | RFC 3339 string | No       | Proposed expiry for `validated`; applied expiry for `completed` |
| `changed`         | boolean         | No       | Whether this invocation persistently replaced the item          |

The required `impact` object contains the boolean fields
`serviceRestartRequired`, `kubeconfigRedistributionRequired`, and
`applicationReloadMayBeRequired`.

Only these result-state combinations are valid:

| `status`    | `dryRun` | `changed` | Meaning                                     |
| ----------- | -------- | --------- | ------------------------------------------- |
| `validated` | `true`   | `false`   | Validation succeeded; no files were changed |
| `completed` | `false`  | `true`    | The transaction committed and was validated |

`validated` with `dryRun: false`, `completed` with `dryRun: true`, or mixed
`changed` values are schema-invalid. Failed or partially committed operations do
not return `CertificateRenewalResult`; they return an `Error` document with a
non-zero exit code. Transaction recovery completes or rolls back before a later
result is emitted.

Example planned renewal:

```json
{
  "apiVersion": "microshift.openshift.io/v1alpha1",
  "kind": "CertificateRenewalResult",
  "generatedAt": "2026-09-08T10:30:00Z",
  "mode": "ca",
  "status": "validated",
  "dryRun": true,
  "items": [
    {
      "service": "service-ca",
      "name": "service-ca",
      "role": "ca",
      "parentCA": null,
      "currentNotAfter": "2035-09-08T10:30:00Z",
      "newNotAfter": "2036-09-05T10:30:00Z",
      "changed": false
    }
  ],
  "impact": {
    "serviceRestartRequired": true,
    "kubeconfigRedistributionRequired": true,
    "applicationReloadMayBeRequired": true
  },
  "warnings": [
    "Kubeconfigs stored outside the MicroShift data directory must be copied again after renewal."
  ]
}
```

Example completed renewal:

```json
{
  "apiVersion": "microshift.openshift.io/v1alpha1",
  "kind": "CertificateRenewalResult",
  "generatedAt": "2026-09-08T10:31:00Z",
  "mode": "ca",
  "status": "completed",
  "dryRun": false,
  "items": [
    {
      "service": "service-ca",
      "name": "service-ca",
      "role": "ca",
      "parentCA": null,
      "currentNotAfter": "2035-09-08T10:30:00Z",
      "newNotAfter": "2036-09-05T10:30:00Z",
      "changed": true
    }
  ],
  "impact": {
    "serviceRestartRequired": true,
    "kubeconfigRedistributionRequired": true,
    "applicationReloadMayBeRequired": true
  },
  "warnings": [
    "Kubeconfigs stored outside the MicroShift data directory must be copied again."
  ]
}
```

#### `microshift certs status`

Reports the status of all managed certificates, grouped and sorted by service or
function and then by certificate name. Safe to run while MicroShift is running.

```console
$ sudo microshift certs status
SERVICE            CERTIFICATE              STATUS    EXPIRY                  REASON          MESSAGE
authentication     admin-kubeconfig-signer   Green     2035-09-08T10:30:00Z    NotExpiring     Valid for 3287 days
etcd               etcd-signer               Green     2035-09-08T10:30:00Z    NotExpiring     Valid for 3287 days
etcd               etcd-serving              Yellow    2026-11-01T08:00:00Z    Expiring        Expires in 54 days
kube-apiserver     kube-apiserver-serving    Green     2027-03-15T10:30:00Z    NotExpiring     Valid for 188 days
kubelet            kubelet-client            Green     2027-03-15T10:30:00Z    NotExpiring     Valid for 188 days
service-ca         service-ca                Green     2035-09-08T10:30:00Z    NotExpiring     Valid for 3287 days
...
```

#### `microshift certs renew --serving`

Renews all serving and client (leaf) certificates without touching the CA chain.
Requires MicroShift to be stopped.

```console
$ sudo systemctl stop microshift
$ sudo microshift certs renew --serving --dry-run
DRY RUN: Would renew the following certificates:
SERVICE            CERTIFICATE              CURRENT EXPIRY           PROPOSED EXPIRY
etcd               etcd-serving              2026-11-01T08:00:00Z    2027-09-08T10:30:00Z
kube-apiserver     kube-apiserver-serving    2027-03-15T10:30:00Z    2027-09-08T10:30:00Z
kubelet            kubelet-client            2027-03-15T10:30:00Z    2027-09-08T10:30:00Z
  ...
No CA certificates will be changed.
WARNING: Applications that cache certificates or CA bundles may need to be
reloaded or restarted after renewal.

$ sudo microshift certs renew --serving
Renewed 18 serving/client certificates.
SERVICE            CERTIFICATE              STATUS    EXPIRY                  REASON          MESSAGE
etcd               etcd-serving              Green     2027-09-08T10:30:00Z    NotExpiring     Valid for 365 days
kube-apiserver     kube-apiserver-serving    Green     2027-09-08T10:30:00Z    NotExpiring     Valid for 365 days
kubelet            kubelet-client            Green     2027-09-08T10:30:00Z    NotExpiring     Valid for 365 days
  ...
WARNING: Applications that cache certificates or CA bundles may need to be
reloaded or restarted after renewal.
$ sudo systemctl start microshift
```

After a successful renewal, the command prints the same status fields as
`microshift certs status`, limited to the renewed certificates.

#### `microshift certs renew --ca`

Renews all rotatable CA certificates and cascades renewal to all descendant
serving and client certificates. Requires MicroShift to be stopped.

```console
$ sudo systemctl stop microshift
$ sudo microshift certs renew --ca --dry-run
DRY RUN: Would renew the following CAs and their descendants:
TYPE           PARENT CA                    CERTIFICATE                              CURRENT EXPIRY           PROPOSED EXPIRY
CA             -                            admin-kubeconfig-signer                   2035-09-08T10:30:00Z    2036-09-05T10:30:00Z
Certificate    admin-kubeconfig-signer      admin-kubeconfig-client                   2035-09-08T10:30:00Z    2027-09-08T10:30:00Z
CA             -                            kube-apiserver-lb-signer                  2035-09-08T10:30:00Z    2036-09-05T10:30:00Z
Certificate    kube-apiserver-lb-signer     kube-apiserver-lb-serving                 2027-03-15T10:30:00Z    2027-09-08T10:30:00Z
CA             -                            service-ca                                2035-09-08T10:30:00Z    2036-09-05T10:30:00Z
Certificate    service-ca                   openshift-controller-manager-serving      2027-03-15T10:30:00Z    2027-09-08T10:30:00Z
Certificate    service-ca                   openshift-router-serving                  2027-03-15T10:30:00Z    2027-09-08T10:30:00Z
...
WARNING: After CA renewal, kubeconfigs stored outside the MicroShift data
directory must be manually re-copied.
WARNING: Applications that cache certificates or CA bundles mounted into pods
must reload that material or be restarted after renewal.

$ sudo microshift certs renew --ca
Renewed 5 CAs and 18 descendant certificates.
SERVICE            TYPE           CERTIFICATE                        EXPIRY
authentication     CA             admin-kubeconfig-signer             2036-09-05T10:30:00Z
authentication     Certificate    admin-kubeconfig-client             2027-09-08T10:30:00Z
kube-apiserver     CA             kube-apiserver-lb-signer            2036-09-05T10:30:00Z
kube-apiserver     Certificate    kube-apiserver-lb-serving           2027-09-08T10:30:00Z
service-ca         CA             service-ca                          2036-09-05T10:30:00Z
service-ca         Certificate    openshift-router-serving            2027-09-08T10:30:00Z
  ...
WARNING: Kubeconfigs in /var/lib/microshift/resources/kubeadmin have been
regenerated. If you have copied kubeconfigs to other locations, you must
update those copies.
WARNING: Applications that cache certificates or CA bundles mounted into pods
must reload that material or be restarted after renewal.
$ sudo systemctl start microshift
```

After a successful CA renewal, the command lists the new expiry date for every
renewed CA and descendant certificate.

### Certificate Lifetime Configuration

MicroShift adds the following fields to `/etc/microshift/config.yaml`:

```yaml
certificates:
  forceRestartOnRedZone: true
  servingValidity: 8760h
  caValidity: 87600h
```

`forceRestartOnRedZone` controls only whether entering the red zone forces
MicroShift to restart. It defaults to `true` to preserve the behavior of
existing deployments. It does not control regeneration during a later manual
service start.

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

Zone thresholds are derived from the validity encoded in each certificate so
they remain meaningful for configured durations such as a six-week serving
certificate. For each item, total validity is `NotAfter - NotBefore`, remaining
validity is `NotAfter - now`, and remaining percentage is remaining validity
divided by total validity.

The PKI inventory records certificate role (CA, serving, client, or peer) and
rotation policy as separate attributes. Role determines how a certificate is
reported and selected for renewal; rotation policy determines its zone
thresholds. Certificate duration is not used to infer either attribute:

| Rotation policy | Assignment                                        | Green           | Yellow                            | Red           |
| --------------- | ------------------------------------------------- | --------------- | --------------------------------- | ------------- |
| Standard        | Serving and explicitly assigned client/peer certs | More than 58.3% | More than 33.3% and at most 58.3% | At most 33.3% |
| Extended        | All CAs and explicitly assigned client/peer certs | More than 15%   | More than 10% and at most 15%     | At most 10%   |

All CAs use the extended policy and all serving certificates use the standard
policy. This intentionally normalizes existing signer CAs that currently use the
short-lived constant. Client and peer certificates are not assigned a policy
solely from their non-CA role: existing long-lived entries such as
`admin-kubeconfig-client`, `apiserver-etcd-client`, `etcd-peer`, and
`etcd-serving` retain the extended policy. New client and peer inventory entries
must declare their policy explicitly.

For the default validity durations, these percentages retain the current
boundaries of approximately seven and four months remaining for standard-policy
certificates and 18 and 12 months remaining for extended-policy certificates. A
certificate that is expired or not yet valid is always in the red zone.

**Behavioral changes by zone:**

- **Green zone**: No action needed. Certificates are valid and not approaching
  expiry.
- **Yellow zone**: Warning logged. Certificates in this zone are automatically
  regenerated the next time MicroShift is manually started or restarted
  (preserving existing behavior). Entering the yellow zone while MicroShift is
  running does not itself stop or restart the service.
- **Red zone (changed)**: Behavior is now governed by the
  `certificates.forceRestartOnRedZone` configuration option:
  - **`forceRestartOnRedZone: true`** (default and legacy behavior): MicroShift
    cancels the run context via `WhenToRotateAtEarliest` /
    `context.WithDeadline` in `pkg/cmd/run.go`, causing systemd to restart the
    process.
  - **`forceRestartOnRedZone: false`**: Warning logged; MicroShift does _not_
    force a process exit. The cluster continues to run until certificates are
    actually invalid. Near-expiry is visible in logs, `certs status` output
    (zone, time until `NotAfter`, and whether `forceRestartOnRedZone` is
    enabled), and healthcheck. The administrator decides when to perform renewal
    in a maintenance window.

  > **Note:** The existing yellow-zone behavior where certificates are
  > automatically regenerated on manual service start (`certsToRegenerate`) is
  > _not_ affected by this configuration. The `forceRestartOnRedZone` setting
  > controls only the in-process red-zone deadline, not the "regenerate on
  > start" behavior.

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

#### Fleet Status Collection

1. Fleet automation runs `sudo microshift certs status -o [json|yaml]` on each
   device.
2. It checks the process exit code and the output `apiVersion` before consuming
   the document.
3. It records `generatedAt`, certificate identity, role, zone, and `notAfter`
   values together with the device identity supplied by the fleet system.
4. It alerts or schedules maintenance according to the aggregated certificate
   state. MicroShift does not contact or depend on a central fleet service.

#### Serving/Client Certificate Renewal

1. Administrator runs `microshift certs status` to assess certificate state.
2. Administrator identifies certificates in yellow or red zone.
3. Administrator plans a maintenance window.
4. Administrator runs `microshift certs renew --serving --dry-run` to validate
   the scope.
5. Administrator stops MicroShift: `systemctl stop microshift`.
6. Administrator runs `microshift certs renew --serving`.
7. Administrator starts MicroShift: `systemctl start microshift`.
8. Application owners reload or restart applications that cache affected
   certificates or CA bundles.
9. Workloads resume with renewed certificates. No kubeconfig redistribution is
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
8. Application owners reload or restart applications that cache certificates or
   CA bundles mounted into their pods, then verify connectivity.

#### Changing Certificate Validity

1. The administrator sets `certificates.servingValidity` and/or
   `certificates.caValidity` in `/etc/microshift/config.yaml`.
2. The administrator runs `sudo microshift show-config` to validate the file and
   inspect the effective durations. Existing certificates are not changed solely
   because their configured validity changed.
3. The administrator uses `microshift certs renew --serving` or `microshift
   certs renew --ca` during a maintenance window when the new validity should take
   effect.
4. Newly issued certificates use the configured duration. `certs status` reports
   their resulting `NotAfter` values and proportionally calculated zones.

### API Extensions

This enhancement does not introduce or modify Kubernetes API resources. It adds
the `certificates.forceRestartOnRedZone`, `certificates.servingValidity`, and
`certificates.caValidity` fields to the host-local MicroShift configuration
file. The versioned JSON documents are a CLI output contract, not Kubernetes API
resources.

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
microshift certs status [-o json|yaml]
microshift certs renew
microshift certs renew --serving [--dry-run] [-o json|yaml]
microshift certs renew --ca [--dry-run] [-o json|yaml]
```

#### JSON Rendering

Human-readable tables and JSON documents are rendered from the same typed status
or renewal result so their certificate sets and calculated dates cannot diverge.
JSON output uses deterministic item ordering by service/function and certificate
name. Human-readable messages are not used as machine-readable state; callers
use the versioned fields and enum values instead.

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

#### Renewal Transaction and Failure Recovery

Renewal provides transaction-like behavior across all affected files. The
command first writes certificates, private keys, bundles, and generated
kubeconfigs to a staging area on the same filesystem without changing active
material. It validates certificate/key pairs, parent-child chains, validity, and
generated bundles before committing the renewal.

Before replacing active files, the command creates a recoverable backup and a
durable transaction marker. A failure before commit leaves active material
unchanged. A failure or interruption during commit restores the backup; a later
`certs status` or `certs renew` invocation detects and recovers an incomplete
transaction before proceeding. The command reports success and the new expiry
dates only after post-commit validation succeeds. MicroShift remains stopped
throughout the operation, so running components cannot observe a partially
updated certificate set.

#### Configurable Certificate Validity

The configuration implementation replaces hard-coded validity values for managed
serving certificates and CAs with the resolved `servingValidity` and
`caValidity` values. The existing expiration alignment performed by `certSetup`
is retained.

The PKI inventory records certificate role and rotation policy explicitly. This
replaces `IsCertShortLived` duration-based classification, which would
misclassify a CA configured with a validity shorter than five years and cannot
represent existing long-lived non-CA certificates safely. CA and serving roles
select their respective policies directly. Existing client and peer entries
retain their current standard or extended behavior through explicit inventory
assignments. Both `certsToRegenerate` and `WhenToRotateAtEarliest` calculate
their thresholds as fractions of each certificate's `NotAfter - NotBefore`
validity, using the policy recorded in the inventory.

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

**Risk**: A write failure or interruption during renewal leaves certificate
files from different generations in active paths.

**Mitigation**: Renewal stages and validates the complete output before commit,
keeps a recoverable backup and transaction marker during replacement, rolls back
on failure, and recovers interrupted transactions before later status or renewal
operations.

---

**Risk**: Applications continue using cached certificates or CA bundles after
MicroShift has renewed the mounted material.

**Mitigation**: Dry-run and renewal output warn that affected applications must
reload their TLS material or restart. The CA workflow explicitly includes this
application-owner action and post-renewal connectivity verification.

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

### Alternative C: Make Forced Exit on Red-Zone Mandatory

Keep the existing behavior of forcing a process exit when certificates enter the
red zone without allowing administrators to disable it. This was rejected
because:

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

4. Should administrators be able to configure the spans of the green, yellow,
   and red zones, or should MicroShift define fixed proportional thresholds for
   all installations? If configurable, what validation prevents overlapping or
   unsafe zone boundaries?

## Test Plan

### Unit Tests

- PKI inventory construction and parent-child relationship tracking.
- Zone classification for standard and extended policies across default and
  custom certificate durations.
- Explicit role and rotation-policy assignment without duration heuristics,
  including CA policy normalization and preservation of long-lived client and
  peer behavior.
- Certificate configuration defaulting and validation, including zero, negative,
  malformed, and serving-validity-greater-than-CA-validity values.
- Renewal transaction fault injection before and during commit, including
  rollback and recovery of an interrupted transaction.
- Dry-run output generation.
- JSON serialization for status, planned renewal, completed renewal, and error
  documents, including schema version, enum values, deterministic ordering, and
  warning representation.
- Exhaustive validation of allowed `Error.code` values and required versus
  nullable fields.
- Validation of the two permitted renewal result combinations and rejection of
  inconsistent `status`, `dryRun`, or item `changed` values.
- Service-running detection guard.

### Integration Tests

- End-to-end `certs status` output validation against a running MicroShift
  instance.
- Validate `-o json|yaml` for `status`, serving and CA dry-runs, and completed
  serving and CA renewals. Each successful invocation produces one schema-valid
  JSON document on standard output with no human-readable text mixed into it.
- Verify table and JSON renderings contain the same certificates, calculated
  zones, impact, and expiry dates.
- Verify JSON-mode failures return non-zero, leave standard output empty, and
  produce a schema-valid error document on standard error.
- End-to-end `certs renew --serving` followed by service restart, verifying all
  renewed certificates are valid and workloads resume.
- End-to-end `certs renew --ca` followed by service restart, verifying CA chain
  is renewed, descendant certificates are re-issued, and kubeconfigs are
  regenerated.
- Dry-run produces accurate output matching what the actual renewal would do.
- Successful serving and CA renewal output includes the new expiry date for
  every renewed certificate.
- Renewal is refused while MicroShift is running.
- With `forceRestartOnRedZone: true`, a red-zone certificate preserves the
  existing forced-restart behavior; with `false`, it produces warnings without
  forcing a process exit.
- Entering the yellow zone while running does not restart MicroShift, and the
  certificate is regenerated on the next manual service start.
- A custom six-week `servingValidity` is reflected in newly issued serving
  certificates without placing them immediately in the yellow or red zone.
- A custom `caValidity` is reflected in newly issued CAs, and no descendant
  certificate expires after its signer.

### Scenario Tests

- Simulate certificate approaching red zone, verify warning behavior, execute
  planned renewal, verify recovery.
- Simulate CA renewal, verify kubeconfig regeneration, verify clients can
  authenticate with new kubeconfig.
- Verify an application that caches an old certificate or CA bundle is called
  out by renewal impact messaging and can recover by reloading or restarting.
- Collect `certs status -o json|yaml` from multiple devices and verify the
  documents can be aggregated by an external fleet-management system.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A This feature is targeted for GA directly.

### Tech Preview -> GA

- All CLI commands implemented and tested (`certs status`, `certs renew
--serving`, `certs renew --ca`, `--dry-run`, and `-o json|yaml`).
- Versioned JSON status, renewal, dry-run, and error contracts are documented
  and validated in CI.
- `forceRestartOnRedZone` implemented with a compatibility-preserving `true`
  default and a warn-only `false` mode.
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
- `certificates.forceRestartOnRedZone` defaults to `true`, so the existing
  red-zone forced-restart behavior is unchanged unless an administrator
  explicitly opts out.
- No certificate renewal is triggered by the upgrade itself.
- Existing certificates remain valid with their current expiry dates.
- Omitted lifetime settings resolve to the existing one-year serving and
  ten-year CA defaults. Setting a custom lifetime affects only certificates
  issued after the setting is applied.

### Downgrade

On downgrade to a MicroShift version without this enhancement:

- The `microshift certs` CLI subcommands are no longer available.
- The red-zone forced-restart behavior is unconditional. A previously configured
  `forceRestartOnRedZone: false` setting is not honored.
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
   each existing certificate, or use `-o json|yaml` when collecting status
   through fleet automation.
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
