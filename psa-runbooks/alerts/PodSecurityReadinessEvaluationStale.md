# PodSecurityReadinessEvaluationStale

## Meaning

The `PodSecurityReadinessController` in `cluster-kube-apiserver-operator` has not
completed a Pod Security Admission evaluation sweep recently. The alert fires when
`pod_security_readiness_last_evaluation_timestamp_seconds` is older than twice the
evaluation interval (the interval is `checkInterval = 240 * time.Minute`, so the
threshold is 8 hours), sustained for 1 hour.

The controller sweeps every Namespace that carries no
`pod-security.kubernetes.io/enforce` label, dry-run applies `enforce: restricted`
to each, and records any that would be rejected in
`apiserver/cluster`'s `status.podSecurityAdmission.violatingNamespaces`. That list is what an
administrator relies on to decide whether raising enforcement is safe.

## Impact

**Nothing is rejected and nothing changes level because of this alert.** The
effective enforcement level is unchanged; running workloads are unaffected.

What is lost is the ability to safely *raise* enforcement:

- `status.podSecurityAdmission` on `apiserver/cluster` is stale. Its `violatingNamespaces` describes
  the cluster as it was at `status.lastEvaluationTime`, not as it is now.
- The Config Observer treats a stale evaluation the same as one that never ran,
  so a request for `Baseline` or `Restricted` in `spec.podSecurityAdmission.enforcementMode` **will
  not be applied** while this alert is firing.
- The controller deliberately does **not** lower enforcement in response to
  staleness. A cluster already at `Baseline` or `Restricted` stays there.

An empty `status.podSecurityAdmission.violatingNamespaces` must not be read as "no violations" while
this alert is firing — it may simply be old. The `StatusStale` condition exists
to make that distinction.

## Diagnosis

Confirm the staleness and read the conditions:

```bash
oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission.lastEvaluationTime}{"\n"}'
oc get apiserver cluster -o jsonpath='{range .status.podSecurityAdmission.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\n"}{end}'
```

Expect `StatusStale=True`. If `Evaluated=False` with reason `NeverRan`, the
controller has never completed a sweep on this cluster at all — treat that as the
more serious case.

Check whether the operator is healthy and whether the evaluation is failing
rather than simply slow:

```bash
oc get co/kube-apiserver
oc -n openshift-kube-apiserver-operator get pods
oc -n openshift-kube-apiserver-operator logs deploy/kube-apiserver-operator \
    | grep -i PodSecurityReadiness | tail -50
```

A rising error counter points at a failing sweep rather than a stopped one:

```
rate(pod_security_readiness_evaluation_errors_total[1h])
```

Repeated failure also surfaces as `Degraded=True` on the `kube-apiserver`
ClusterOperator with reason `PodSecurityReadinessEvaluationFailing`.

Likely causes, most common first:

1. **The operator is not running or is crash-looping.** Check pod status and
   restart counts above.
2. **The sweep cannot finish inside the interval.** The controller uses a client
   throttled to `QPS = 2` / `Burst = 2` and issues roughly one request per
   Namespace plus a Pod LIST per violating Namespace. On a cluster with many
   thousands of Namespaces a sweep can take hours. Compare Namespace count
   against the elapsed time:
   ```bash
   oc get ns --no-headers | wc -l
   ```
3. **RBAC lost after an upgrade.** The controller needs to list Namespaces and
   Pods cluster-wide and to issue dry-run Namespace applies. Look for
   `forbidden` in the operator log.
4. **Leader election wedged.** Check the operator lease:
   ```bash
   oc -n openshift-kube-apiserver-operator get lease
   ```

## Mitigation

There is no supported way to trigger a sweep on demand. Restarting the operator
causes the controller factory to run one sync immediately on start:

```bash
oc -n openshift-kube-apiserver-operator delete pod -l app=kube-apiserver-operator
```

Then confirm `status.lastEvaluationTime` advances and `StatusStale` clears. On a
large cluster allow at least one full interval before concluding it has not
worked.

If the cause is RBAC, the operator's ClusterRole is reconciled by the CVO —
confirm `oc get co/kube-apiserver` is not reporting a manifest it cannot apply,
and check for manual edits to the operator's RBAC.

If the cause is sweep duration on a very large cluster, this is a known
limitation rather than a fault; the evaluation cost and the absence of pagination
are tracked as an open question in the enhancement. Enforcement cannot be raised
until a sweep completes, which is the intended failure mode.

**Do not work around this by setting `unsupportedConfigOverrides` on
`kubeapiservers.operator.openshift.io` to force an enforcement level.** Doing so
bypasses the interlock this alert protects, and it sets
`Upgradeable=False` on the ClusterOperator until removed.
