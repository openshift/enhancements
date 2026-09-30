# Resolving violating Namespaces

**Audience:** cluster administrators. **Destination:** `openshift-docs` admin
guide.

A Namespace listed in `apiserver/cluster`'s `status.podSecurityAdmission.violatingNamespaces` contains
workloads that would be rejected if the requested Pod Security Standard were
enforced. Each entry carries a `reason` whose prefix identifies the class of
problem.

```bash
oc get apiserver cluster \
    -o jsonpath='{range .status.podSecurityAdmission.violatingNamespaces[*]}{.name}{"\t"}{.reason}{"\n"}{end}'
```

| Prefix | Section below |
|---|---|
| `PSAConfig` | [Namespace name starts with `openshift`](#namespace-name-starts-with-openshift) |
| `PSALabel` | [Workload uses user-based SCCs](#workload-uses-user-based-sccs) |

## Finding the specific violation

Reproduce what the readiness controller saw:

```bash
oc label --dry-run=server --overwrite ns/$NAMESPACE \
    pod-security.kubernetes.io/enforce=restricted
```

The warnings name the fields in the Pod spec that violate the standard. If
`restricted` warns, try `baseline`; if both warn, the Namespace needs
`privileged` in its current state.

Server-side dry run reports only on Pods that **exist right now**. It says
nothing about a Deployment scaled to zero, a CronJob that has not fired, or a
DaemonSet whose nodes are cordoned. Those will be caught at creation time
instead.

## PSA denials are not visible in `oc get pods`

When a controller creates the Pod, the rejection surfaces on the owning object,
because no Pod is ever created. For a Deployment it appears on its ReplicaSet;
for a CronJob, on its Job:

```bash
oc -n $NAMESPACE describe replicaset $NAME
oc -n $NAMESPACE get events --field-selector reason=FailedCreate
```

This is the most common source of confusion when debugging PSA. Reach for it
first.

## Namespace name starts with `openshift`

The `openshift` prefix is reserved for OpenShift. Such Namespaces are expected to
carry their own `pod-security.kubernetes.io/enforce` label, shipped in the
component's own manifest by the team that owns it.

Guides and scripts sometimes create Namespaces with this prefix; alternatively
the owning team did not set the required label, which should not happen and may
indicate an out-of-date component.

- **If the Namespace is user-created:** creating a Namespace with the
  `openshift-` prefix is not supported. Recreate it under a different name, or
  set the `enforce` label on it manually.
- **If the Namespace is owned by OpenShift:** check for updates. If the cluster
  is up to date, report a bug against the owning component.

### `openshift-operators`

`openshift-operators` — where OLM installs operator bundles by default — has no
`enforce` label from any source. It is governed entirely by the cluster-wide
default. On a cluster that opts in to `Restricted`, operator bundles needing more
than restricted will fail admission there.

Label the Namespace explicitly if you rely on it:

```bash
oc label ns/openshift-operators pod-security.kubernetes.io/enforce=baseline
```

Or install operators into a dedicated Namespace you control the labelling of.

## `pod-security.kubernetes.io/enforce` label has not been set

A Namespace with no `enforce` label is held to the cluster-wide default. From
release `n` nothing computes a per-Namespace label on your behalf — the PSA label
syncer is retired — so if a Namespace cannot meet the cluster-wide standard, you
must set its label yourself.

Assess first:

```bash
oc label --dry-run=server --overwrite ns/$NAMESPACE \
    pod-security.kubernetes.io/enforce=restricted

oc label --dry-run=server --overwrite ns/$NAMESPACE \
    pod-security.kubernetes.io/enforce=baseline
```

Then set it by dropping `--dry-run=server`:

```bash
oc label --overwrite ns/$NAMESPACE pod-security.kubernetes.io/enforce=baseline
```

A Namespace with its own `enforce` label is excluded from the readiness sweep
entirely, so it will stop appearing in `status.podSecurityAdmission.violatingNamespaces`. Nothing
maintains the label afterwards — it is yours now, including if the Namespace's
SCCs change later.

## Workload uses user-based SCCs

A workload can be admitted under the SCCs of the *user* who created it rather
than those of a ServiceAccount. This usually happens when a Pod is created
directly rather than through a Deployment with a properly configured
ServiceAccount.

This matters because the evaluation, and the retired syncer before it, only ever
considered SCCs reachable by a Namespace's **ServiceAccounts**. A workload
relying on its creator's SCCs looks compliant until that user is gone, or until
the workload is recreated by a controller.

Check the provenance annotation on the Pod:

```bash
oc -n $NAMESPACE get pods \
    -o custom-columns='NAME:.metadata.name,SCC:.metadata.annotations.openshift\.io/scc,SUBJECT:.metadata.annotations.security\.openshift\.io/validated-scc-subject-type'
```

`SUBJECT` of `user` is this case. The annotation is written at admission time, so
it is **absent on Pods created before the release that introduced it** — on an
upgraded cluster, absence tells you nothing. Fall back to checking
`openshift.io/scc` and whether the workload's ServiceAccount can actually reach
that SCC.

### Fix

Grant the Namespace's ServiceAccount the SCCs its workloads genuinely need,
identifiable from the `openshift.io/scc` annotation on the existing Pods:

```bash
oc adm policy add-scc-to-user <scc> -z <serviceaccount> -n $NAMESPACE
```

Then recreate the workload through a Deployment (or other controller) configured
to use that ServiceAccount.

This is enough for the next evaluation to stop reporting the Namespace as
violating, in any enforcement mode.

**It does not cause a PSA label to be written.** The syncer no longer runs, so
granting an SCC changes what the Namespace is *allowed* to run without changing
what PSA *enforces* on it. If the Namespace needs an effective level other than
the cluster-wide one, set the `enforce` label manually as above. On a cluster
that has opted in to `Baseline` or `Restricted`, that is the only mechanism.

## Related

- `disabling-psa-enforcement.md` — if you need enforcement off now
- `removing-retained-enforce-labels.md` — if a frozen label is the problem
- [Kubernetes Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
