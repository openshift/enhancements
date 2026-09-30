# PodSecurityEnforcementBlocked

## Meaning

An administrator has requested Pod Security Admission enforcement in
`apiserver/cluster`'s `spec.podSecurityAdmission.enforcementMode` (`Baseline` or
`Restricted`), but the
`PodSecurityReadinessController` found Namespaces that would violate that
standard, so the request has not been applied. The `EnforcementBlocked` condition
has been `True` with reason `ViolatingNamespaces` continuously for 24 hours.

The 24-hour `for` duration is deliberate: a short block is the normal, expected
state immediately after an administrator records their intent, and is not worth
alerting on. A block that persists for a day means nobody is acting on it.

## Impact

**This is a "requested but not delivered" alert, not an outage.** Nothing is
broken and nothing is being rejected that was not already being rejected.

- The cluster is running at `status.podSecurityAdmission.enforcementMode`, which
  is lower than what `spec.podSecurityAdmission.enforcementMode` asks for.
- The security posture the administrator asked for is **not** in effect, and the
  cluster may be under the impression that it is. If the cluster attests to an
  enforcement level under FedRAMP, PCI or DISA STIG, `spec` is not the field to
  evidence that with — `status.podSecurityAdmission.enforcementMode` is.
- The request stays on record. Resolving the violations causes the requested mode
  to take effect on the next sweep with no further action from the administrator.

## Diagnosis

Read what is blocking:

```bash
oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission}' | jq
```

Compare the requested and effective levels, and list the offenders:

```bash
oc get apiserver cluster \
    -o jsonpath='requested={.spec.podSecurityAdmission.enforcementMode} effective={.status.podSecurityAdmission.enforcementMode}{"\n"}'

oc get apiserver cluster \
    -o jsonpath='{range .status.podSecurityAdmission.violatingNamespaces[*]}{.name}{"\t"}{.reason}{"\n"}{end}'
```

The `reason` prefix identifies the class of problem:

| Prefix | Meaning |
|---|---|
| `PSALabel` | the Namespace's ServiceAccounts hold SCCs insufficient for the requested standard |
| `PSAConfig` | a misconfigured `openshift`-prefixed Namespace |

Confirm the block is genuine rather than a stale evaluation — if `StatusStale` is
also `True`, deal with `PodSecurityReadinessEvaluationStale` first, because the
violation list may be describing a cluster that no longer exists:

```bash
oc get apiserver cluster \
    -o jsonpath='{range .status.podSecurityAdmission.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\n"}{end}'
```

For a specific Namespace, reproduce what the controller saw. Use the level the
controller actually evaluated against, which is not always `restricted`:

```bash
LEVEL=$(oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission.evaluatedLevel}' | tr 'A-Z' 'a-z')

oc label --dry-run=server --overwrite ns/$NAMESPACE \
    pod-security.kubernetes.io/enforce=$LEVEL
```

The warnings name the fields in the Pod spec that violate the standard.

`status.podSecurityAdmission.evaluatedLevel` is `Baseline` when the administrator requested
`Baseline`, and `Restricted` otherwise. Dry-running at `restricted` on a cluster
requesting `Baseline` will report Namespaces that are not blocking anything.

## Mitigation

There are three legitimate outcomes. Pick one — leaving the alert firing
indefinitely is the state this alert exists to prevent.

**1. Fix the Namespaces.** Preferred. See
`docs/resolving-violating-namespaces.md`. In summary: grant the Namespace's
ServiceAccount the SCCs its workloads actually need, or adjust the workloads'
`securityContext` to meet the standard. The next sweep clears the condition and
the requested mode takes effect on its own.

**2. Label the Namespaces that cannot comply.** A Namespace with its own
`pod-security.kubernetes.io/enforce` label is enforced at that label's level
rather than the cluster-wide one, and is excluded from the readiness sweep
entirely:

```bash
oc label ns/$NAMESPACE pod-security.kubernetes.io/enforce=privileged
```

Note that nothing maintains this label afterwards — the PSA label syncer is
retired — so it is frozen until someone changes it.

**3. Accept the violations deliberately.** If the violating Namespaces are known
and tolerated, acknowledge them. The acknowledgement is tied to the specific set
of findings by a content hash, so violations discovered by a *later* sweep will
block again. A sweep that rediscovers the same set produces the same hash and
does not, and neither does an unrelated edit to this resource:

```bash
HASH=$(oc get apiserver cluster \
    -o jsonpath='{.status.podSecurityAdmission.knownViolationsHash}')
oc patch apiserver cluster --type=merge \
    -p "{\"spec\":{\"podSecurityAdmission\":{\"acknowledgeKnownViolations\":\"$HASH\"}}}"
```

This applies the requested enforcement level to the whole cluster, including the
Namespaces just acknowledged. **Workloads in those Namespaces will be rejected on
their next creation** — which may be weeks later, when a node drain or upgrade
recreates them. Prefer option 2 for Namespaces you intend to keep running.

**Or withdraw the request.** If enforcement is no longer wanted, set
`spec.podSecurityAdmission.enforcementMode: Privileged`. This is applied
unconditionally and is never gated on the evaluation. See `docs/disabling-psa-enforcement.md`.
