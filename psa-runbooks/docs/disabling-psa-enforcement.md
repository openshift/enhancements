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

## Procedure

```bash
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforceLevel":"Privileged"}}}'
```

Every change to `spec.podSecurityAdmission.enforceLevel` is applied as written,
in both directions. Nothing on the cluster evaluates the request or can withhold
it, so this escape hatch has no machinery of its own that can be broken when you
need it.

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

Then confirm the effective level. The API is write-only — there is no `status`
echoing the applied level back — so the kube-apiserver's own configuration is
the only place to read it:

```bash
oc get cm config -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults.enforce'
```

That ConfigMap is the *desired* configuration. To confirm what a given master is
enforcing right now, read `config-<currentRevision>` for that node instead; see
`troubleshooting-psa-configuration.md`.

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
`spec.podSecurityAdmission.enforceLevel`.

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

Disabling enforcement is a holding action, and nothing on the cluster will tell
you when it is safe to undo. Before re-enabling:

1. Watch the audit signal. `warn` and `audit` stay pinned at `restricted` while
   enforcement is disabled, so every would-be violation keeps being counted in
   `pod_security_evaluations_total{decision="deny",mode="audit"}` and keeps
   firing the pre-existing `PodSecurityViolation` alert. A flat-zero deny rate
   sustained across a period long enough to cover your slowest CronJob is the
   closest thing to an all-clear you get.
2. Run the dry-run sweep in `resolving-violating-namespaces.md` to find out
   *which* Namespaces. Nothing names them for you — the
   `PodSecurityReadinessController` that used to report this was retired in
   release `n`.
3. Resolve them, then re-request the level.

**Nothing stops you re-enabling before step 2 or 3.** The requested level is
applied as written, with nothing evaluating the cluster first. If you re-enable
over known violations you will get the same rejections that made you disable
it, and during the control-plane rollout you will get them inconsistently —
the same Pod creation succeeding or failing depending on which master it
reaches.

## Related

- `troubleshooting-psa-configuration.md` — when the patch appears to do nothing
- `removing-retained-enforce-labels.md` — when the rejection is in a labelled Namespace
- `resolving-violating-namespaces.md` — fixing the underlying violations
