# Break glass: disabling Pod Security Admission enforcement

**Audience:** cluster administrators and support, under time pressure.
**Destination:** `openshift-docs` admin guide, plus a support KCS article.

> The enhancement marks this procedure as needing to be written out. This is the
> draft; the timings in particular need confirming against a real cluster before
> publication.

## When to use this

Workloads are being rejected at admission because the cluster's Pod Security
Admission enforcement level is higher than they can meet, and the priority is
restoring the workloads rather than fixing them.

## What it does and does not do

**It does:** set the cluster-wide `enforce` level to `privileged`, so that
Namespaces carrying no `pod-security.kubernetes.io/enforce` label of their own
admit any workload.

**It does not:**

- **Change any Namespace that has its own `enforce` label.** On a cluster
  upgraded from before release `n`, most Namespaces carry a retained label from
  the retired PSA label syncer, and those are unaffected. If the rejection is in
  such a Namespace, this procedure will not help — see
  `removing-retained-enforce-labels.md`.
- **Turn off the `PodSecurity` admission plugin.** It stays loaded. `warn` and
  `audit` stay pinned to `restricted`, so violations remain visible in metrics
  and in the `PodSecurityViolation` alert. This is intentional.
- **Affect running Pods.** PSA acts only on Pod *creation* and never evicts.
  Anything already running was already running.
- **Stop the `PodSecurityReadinessController`.** It keeps evaluating, which is
  what lets you tell when it is safe to re-enable.

## Procedure

```bash
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforcementMode":"Privileged"}}}'
```

Lowering enforcement is applied **unconditionally**. It is never gated on the
readiness evaluation, never blocked by violating Namespaces, and never blocked
by a stale or failed evaluation — the escape hatch has to work when the
evaluation machinery is exactly what has failed.

Removing the `spec.podSecurityAdmission` field entirely is equivalent to setting
`Privileged`.

## Confirming it has taken effect

The change is **not instantaneous**. It re-renders the kube-apiserver
configuration, cutting a new static pod revision that rolls the control plane one
node at a time. Until the last node has taken the new revision, some fraction of
Pod creations is still being rejected by the nodes that have not rolled yet.

```bash
oc get kubeapiserver/cluster -o jsonpath='{.status.latestAvailableRevision}{"\n"}'
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'
```

The change has fully landed only when every `currentRevision` equals
`latestAvailableRevision`. Expect minutes to tens of minutes on a three-node
control plane. **Do not conclude the patch did not work before this completes**
— that is the most common mistake made under pressure here.

Then confirm the effective level:

```bash
oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission.enforcementMode}{"\n"}'

oc get cm config -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults.enforce'
```

## On single-node OpenShift

There is one kube-apiserver, so there is no mixed window — but the rollout is a
**short API outage** while the static pod restarts, rather than a rolling
update. This applies to the disable path as much as the enable path, which is
worth knowing before reaching for it during an incident: the act of relieving the
problem briefly takes the API server away.

## Fallback: when `cluster-kube-apiserver-operator` is wedged

If the operator is not reconciling — crash-looping, degraded, or not rolling a
new revision — the API-level patch will not take effect. The fallback is to
override the rendered configuration directly:

```bash
oc patch kubeapiserver/cluster --type=merge -p '{
  "spec": {
    "unsupportedConfigOverrides": {
      "admission": {
        "pluginConfig": {
          "PodSecurity": {
            "configuration": {
              "defaults": { "enforce": "privileged" }
            }
          }
        }
      }
    }
  }
}'
```

`unsupportedConfigOverrides` is merged last and wins over everything, including
`spec.podSecurityAdmission.enforcementMode`.

**This is unsupported and has a cost.** It sets `Upgradeable=False` on the
`kube-apiserver` ClusterOperator with reason `UnsupportedConfigOverridesSet`, and
**the cluster cannot upgrade until it is removed**. It also silently defeats
`spec.podSecurityAdmission` for as long as it is set, which will confuse whoever
looks at this cluster next.

Remove it as soon as the underlying problem is fixed:

```bash
oc patch kubeapiserver/cluster --type=merge \
    -p '{"spec":{"unsupportedConfigOverrides":null}}'
```

## Afterwards

Disabling enforcement is a holding action. Before re-enabling:

1. Let the `PodSecurityReadinessController` complete a sweep — `warn` and `audit`
   stay at `restricted` while disabled, so violations keep accumulating in
   `pod_security_evaluations_total` and in the `PodSecurityViolation` alert.
2. Read `status.podSecurityAdmission.violatingNamespaces` and resolve what it lists. See
   `resolving-violating-namespaces.md`.
3. Re-request the level. The request is only applied once the evaluation is
   clean.

## Related

- `troubleshooting-psa-configuration.md` — when the patch appears to do nothing
- `removing-retained-enforce-labels.md` — when the rejection is in a labelled Namespace
- `resolving-violating-namespaces.md` — fixing the underlying violations
