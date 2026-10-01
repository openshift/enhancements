# PSA enforcement config — runbooks and docs to file separately

Scratch directory holding material extracted from
`enhancements/authentication/pod-security-admission-config.md` so the EP can stay
at proposal length. **Nothing here belongs in the enhancements repo** — this
directory is untracked and should be moved out or deleted before the EP merges.

Each file is written against the enhancement as it currently stands
(`podSecurityAdmission` on `config.openshift.io/v1 APIServer`, the requested
level applied unconditionally with no interlock and no acknowledgement step,
**write-only** — no `status`, no conditions, no new metric and no new alert —
PSA label syncer retired, `PodSecurityReadinessController` retired, no cleanup
and no fossil detection shipping). If any of those decisions change in review,
these change with them.

## Destinations

| File | Goes to | Blocking on |
|---|---|---|
| `docs/troubleshooting-psa-configuration.md` | `openshift-docs` (admin) + support KCS | API merging |
| `docs/removing-retained-enforce-labels.md` | `openshift-docs` (admin), **and the release note must link it** | release `n` release note |
| `docs/disabling-psa-enforcement.md` | `openshift-docs` (admin) + support KCS | API merging |
| `docs/resolving-violating-namespaces.md` | `openshift-docs` (admin) | API merging |

**There is no longer an `alerts/` directory, and nothing here is blocked on
`openshift/runbooks`.** Two alert runbooks were drafted and have both been
deleted: `PodSecurityEnforcementBlocked.md` went with the interlock, since there
is no longer a state in which enforcement is withheld, and
`PodSecurityReadinessEvaluationStale.md` went with the
`PodSecurityReadinessController`. The enhancement ships no new alert and no new
metric, so no `runbook_url` annotation has to be satisfied.

The one standing PSA alert is the pre-existing `PodSecurityViolation`, driven by
`pod_security_evaluations_total{decision="deny",mode="audit"}`. It is not
introduced here and already has whatever runbook it has; the docs below point at
it rather than replacing it.

## Before filing

- **Check the runbook template.** These use `Meaning` / `Impact` / `Diagnosis` /
  `Mitigation`, which is the structure the existing
  `cluster-kube-apiserver-operator` runbooks follow, but I did not have a clone
  of `openshift/runbooks` to diff against. Reconcile with that repo's
  `TEMPLATE.md` before opening a PR.
- **API field names are not yet implemented.** Everything referenced here is
  proposed in the EP, not shipped. Confirm the final spelling of
  `spec.podSecurityAdmission` against merged `openshift/api` before filing.
- `openshift-docs` uses AsciiDoc modules, not Markdown. These are drafted as
  Markdown for review; they need converting and splitting into
  concept/procedure/reference modules to match that repo's conventions.

## Still unwritten

The EP's Support Procedures section lists these as outstanding and none of them
is covered by the files here:

- the exact PSA denial message emitted at admission;
- audit-log correlation via the `pod-security.kubernetes.io/enforce-policy`
  annotation;
- must-gather coverage for `apiserver/cluster`'s `podSecurityAdmission`, the
  effective admission configuration read from the revisioned `config-<revision>`
  ConfigMap, and the dry-run sweep that identifies violating Namespaces. The
  sweep is the pressing one: with no `status`, no conditions, no readiness
  controller and no `MinimallySufficientPodSecurityStandard` annotation, a
  support bundle from a release `n` cluster otherwise contains nothing at all
  that locates a violation.
