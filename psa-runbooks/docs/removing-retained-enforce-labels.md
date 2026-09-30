# Removing retained `pod-security.kubernetes.io/enforce` labels

**Audience:** cluster administrators on a cluster upgraded into release `n` or
later. **Destination:** `openshift-docs`, and the release note for release `n`
must link here.

## Why this procedure exists

Before release `n`, the PSA label synchronization controller computed a
`pod-security.kubernetes.io/enforce` label for every Namespace it managed, based
on the SCCs reachable by that Namespace's ServiceAccounts, and kept it up to date
as those SCCs changed.

From release `n` that controller no longer runs, in any enforcement mode. The
labels it already wrote are **retained and frozen** at whatever value the last
pre-upgrade sync left. Nothing updates them, nothing removes them, and nothing
reports on them.

They are retained on purpose. Stripping them on upgrade would be a fleet-wide,
silent reduction in enforcement posture, and it would be irreversible — server-side
apply archives nothing, so the values could not be restored. Retention can be
undone by hand, which is what this procedure is.

Two situations lead here.

**The cluster-wide setting appears to have no effect.** Per-Namespace labels take
precedence over the cluster-wide default, so on an upgraded cluster where most
Namespaces carry a retained label, setting
`spec.podSecurityAdmission.enforcementMode: Privileged` changes the effective level of nothing that
already has a label. Those labels have to be removed individually.

**A workload is rejected after an SCC change that should have permitted it.** For
example, you grant a Namespace's ServiceAccount the `anyuid` SCC to run a
workload needing a fixed UID, and the workload is still rejected. Before release
`n` the controller would have relaxed the Namespace's label from `restricted` to
`baseline` in response. Now the label stays pinned at `restricted` and no
component reports the mismatch.

## Before you start

This procedure destroys a record. The labels, together with the
`metadata.managedFields` entry naming who set them and when, are the cluster's
evidence of the enforcement posture it held before release `n`. Deletion is
irreversible: no future release recomputes the value.

If this cluster attests to an enforcement level under FedRAMP, PCI or DISA STIG,
treat each removal as a change to an audited control and record it accordingly.

Do this per Namespace. Bulk removal is possible but is deliberately not given
here as a one-liner — the blast radius is the whole cluster's enforcement
posture, and the checks below do not survive being scripted away.

## Ordering

**Lower the cluster-wide default first, then remove labels.** Never the other way
around.

Between deleting a label and the kube-apiserver finishing its roll to a
`privileged` revision, a Namespace that just lost a `baseline` label is governed
by a cluster-wide default still set to `restricted` — stricter than what it had,
and capable of rejecting workloads that were fine a moment earlier.

Confirm the effective configuration has actually rolled before removing any
label. See `troubleshooting-psa-configuration.md`, or in short:

```bash
oc get kubeapiserver/cluster -o jsonpath='{.status.latestAvailableRevision}{"\n"}'
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'
```

Every `currentRevision` must equal `latestAvailableRevision`.

## Step 1 — confirm the label was written by the controller, not by a person

Ownership is recorded per label key in `metadata.managedFields`:

```bash
oc get ns $NAMESPACE -o json | jq '
  .metadata.managedFields[]
  | select(.fieldsV1."f:metadata".."f:pod-security.kubernetes.io/enforce"? != null)
  | {manager, operation, time}'
```

| Manager | Meaning | Action |
|---|---|---|
| `pod-security-admission-label-synchronization-controller` | written by the retired syncer | safe to remove |
| `cluster-policy-controller` | same, under the historical manager name | safe to remove |
| `oc`, `kubectl-label`, a GitOps controller, the CVO, anything else | somebody chose this value deliberately | **do not remove** |
| nothing returned | provenance lost — see below | **investigate first** |

Both syncer names count because the field manager was renamed; the
`forceHistoricalLabelsOwnership` path in the controller exists to handle exactly
that transition.

**No owner recorded is ambiguous, not safe.** Backup and restore tooling, etcd
restore and some migration paths drop or rewrite `managedFields`, so a
syncer-written label can lose its provenance and become indistinguishable from a
deliberate one. The evidence fails unsafe for deletion: the cost of guessing
wrong is silently relaxing a Namespace somebody intended to constrain. Establish
intent some other way — cluster documentation, the GitOps repository, the team
that owns the Namespace — before deleting anything.

## Step 2 — establish what the Namespace can actually meet

Nothing computes this any more, so ask the API server directly:

```bash
oc label --dry-run=server --overwrite ns/$NAMESPACE \
    pod-security.kubernetes.io/enforce=restricted
```

If that warns, repeat with `baseline`. If both warn, the Namespace needs
`privileged` in its current state.

This tells you what removing the label will expose the Namespace to once the
cluster-wide default governs it. It reports only on Pods that exist right now —
it says nothing about workloads scaled to zero, CronJobs that have not fired, or
anything not yet created.

## Step 3 — remove the label

```bash
oc label ns/$NAMESPACE pod-security.kubernetes.io/enforce-
```

The Namespace now falls to the cluster-wide default.

## If the Namespace needs a level other than the cluster-wide one

Do not remove the label — correct it:

```bash
oc label --overwrite ns/$NAMESPACE pod-security.kubernetes.io/enforce=baseline
```

You now own it. Nothing will update it again, including if the Namespace's SCCs
change later.

## Related

- `troubleshooting-psa-configuration.md` — confirming the effective configuration
- `disabling-psa-enforcement.md` — lowering the cluster-wide default
- `resolving-violating-namespaces.md` — fixing a Namespace rather than relabelling it
