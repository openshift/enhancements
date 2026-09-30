# PSA enforcement config — runbooks and docs to file separately

Scratch directory holding material extracted from
`enhancements/authentication/pod-security-admission-config.md` so the EP can stay
at proposal length. **Nothing here belongs in the enhancements repo** — this
directory is untracked and should be moved out or deleted before the EP merges.

Each file is written against the enhancement as it currently stands
(`podSecurityAdmission` on `config.openshift.io/v1 APIServer`, PSA label syncer
retired, no cleanup and no
fossil detection shipping). If any of those decisions change in review, these
change with them.

## Destinations

| File | Goes to | Blocking on |
|---|---|---|
| `alerts/PodSecurityReadinessEvaluationStale.md` | `openshift/runbooks` → `alerts/cluster-kube-apiserver-operator/` | alert merging in `cluster-kube-apiserver-operator` |
| `alerts/PodSecurityEnforcementBlocked.md` | `openshift/runbooks` → `alerts/cluster-kube-apiserver-operator/` | same |
| `docs/troubleshooting-psa-configuration.md` | `openshift-docs` (admin) + support KCS | API merging |
| `docs/removing-retained-enforce-labels.md` | `openshift-docs` (admin), **and the release note must link it** | release `n` release note |
| `docs/disabling-psa-enforcement.md` | `openshift-docs` (admin) + support KCS | API merging |
| `docs/resolving-violating-namespaces.md` | `openshift-docs` (admin) | API merging |

Both alerts are `Warning` severity, so `openshift/runbooks` entries are
**required before the alerts can merge**. The `runbook_url` annotation on each
alert must be:

```
https://github.com/openshift/runbooks/blob/master/alerts/cluster-kube-apiserver-operator/<AlertName>.md
```

which matches the convention already used by `kube-apiserver-down.yaml`,
`cpu-utilization.yaml` and the SLO alerts in that repo.

## Before filing

- **Check the runbook template.** These use `Meaning` / `Impact` / `Diagnosis` /
  `Mitigation`, which is the structure the existing
  `cluster-kube-apiserver-operator` runbooks follow, but I did not have a clone
  of `openshift/runbooks` to diff against. Reconcile with that repo's
  `TEMPLATE.md` before opening a PR.
- **Metric and alert names are not yet implemented.** Everything referenced here
  is proposed in the EP, not shipped. Confirm the final names against the
  merged `PrometheusRule` before filing.
- `openshift-docs` uses AsciiDoc modules, not Markdown. These are drafted as
  Markdown for review; they need converting and splitting into
  concept/procedure/reference modules to match that repo's conventions.

## Still unwritten

The EP's Support Procedures section lists these as outstanding and none of them
is covered by the files here:

- symptoms and log lines for the `PodSecurityReadinessController` not running,
  crash-looping, or failing to evaluate;
- the exact PSA denial message emitted at admission;
- audit-log correlation via the `pod-security.kubernetes.io/enforce-policy`
  annotation;
- must-gather coverage for `apiserver/cluster`'s `podSecurityAdmission`, the effective admission
  configuration, and the violating-Namespace list.
