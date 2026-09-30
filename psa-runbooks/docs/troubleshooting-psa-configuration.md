# Troubleshooting: PSA enforcement configuration appears to have no effect

**Audience:** cluster administrators and support. **Destination:**
`openshift-docs` admin guide, plus a support KCS article.

## Symptom

`spec.podSecurityAdmission.enforcementMode` on `apiserver/cluster` is set to a
level the cluster is plainly not applying, while `status` reports the change as
successful — or a change to `spec` appears to do nothing at all.

The configuration lives on the cluster-wide API server config resource, not on a
resource of its own:

```bash
oc get apiserver cluster -o jsonpath='{.spec.podSecurityAdmission}' | jq
oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission}' | jq
```

The causes are distinguished by reading the **effective** admission
configuration rather than any operator's `status`.

## Reading the effective configuration

`targetconfigcontroller` writes the merged kube-apiserver configuration into the
`config` ConfigMap in `openshift-kube-apiserver`, under the key `config.yaml`.
Despite the key's name the value is JSON — the merge encodes through
`UnstructuredJSONScheme` — so `jq` reads it directly:

```bash
oc get cm config -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults'
```

That ConfigMap is the **desired** configuration. It is revisioned: the installer
copies it to `config-<revision>`, and each control plane node reports the
revision it is actually running in `status.nodeStatuses[].currentRevision`. To
see what a particular kube-apiserver is enforcing right now, read the revision
that node is on:

```bash
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'

# e.g. master-0 is on currentRevision 12 -> read configmap config-12
oc get cm config-12 -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults'
```

Expect `warn` and `audit` to be `restricted` regardless of the enforcement level
— that is intentional and keeps violations observable. Only `enforce` tracks
`spec.podSecurityAdmission.enforcementMode`.

## Cause 1: the rollout has not finished

Every enforcement change cuts a new static pod revision, which rolls the control
plane one node at a time. Until it completes, different kube-apiservers enforce
different levels, and which one a request reaches is a load-balancer decision.

```bash
oc get kubeapiserver/cluster -o jsonpath='{.status.latestAvailableRevision}{"\n"}'
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'
```

While these differ, `cm/config` and `cm/config-<currentRevision>` disagree by
design and the cluster genuinely has two answers. Wait for every
`currentRevision` to equal `latestAvailableRevision`.

This applies to **lowering** enforcement too, including the break-glass path —
disabling is not instantaneous. Do not conclude the change did not take until the
roll is complete.

## Cause 2: `unsupportedConfigOverrides` is winning

`spec.unsupportedConfigOverrides` on `kubeapiservers.operator.openshift.io` is
the last layer in the configuration merge, so it silently defeats the API.

```bash
oc get kubeapiserver/cluster -o jsonpath='{.spec.unsupportedConfigOverrides}{"\n"}'
```

Look for `admission.pluginConfig.PodSecurity` in the output. This is common on
clusters that enabled PSA enforcement manually before the API existed, following
the earlier Pod Security Admission enhancement.

The ClusterOperator also reports `Upgradeable=False` with reason
`UnsupportedConfigOverridesSet` and a message enumerating the overridden leaf
paths:

```bash
oc get co/kube-apiserver -o jsonpath='{range .status.conditions[?(@.type=="Upgradeable")]}{.status}{"\t"}{.reason}{"\t"}{.message}{"\n"}{end}'
```

That condition is the reliable fleet-wide signal, but it does not say that
`spec.podSecurityAdmission` is being ignored — making that connection is the point of
this procedure.

**The only fix is to remove the override.** The cluster cannot upgrade until it
is gone.

## Cause 3: the request is blocked by violating Namespaces

`spec` records intent; `status` records what is in force. If the readiness
controller found Namespaces that would violate the requested standard, the
request is recorded but not applied.

```bash
oc get apiserver cluster \
    -o jsonpath='requested={.spec.podSecurityAdmission.enforcementMode} effective={.status.podSecurityAdmission.enforcementMode}{"\n"}'
oc get apiserver cluster \
    -o jsonpath='{range .status.podSecurityAdmission.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\n"}{end}'
```

`EnforcementBlocked=True` with reason `ViolatingNamespaces` is this case. See the
`PodSecurityEnforcementBlocked` runbook.

## Cause 4: the evaluation is stale or has never run

Raising enforcement is gated on a completed, recent evaluation. `Evaluated=False`
(reason `NeverRan`) or `StatusStale=True` both prevent it. Lowering enforcement
is never gated this way.

See the `PodSecurityReadinessEvaluationStale` runbook.

## Cause 5: the Namespace has its own label

The cluster-wide setting only governs Namespaces with **no**
`pod-security.kubernetes.io/enforce` label of their own. On a cluster upgraded
from before release `n`, most Namespaces carry a retained label and are
unaffected by any change to the cluster-wide default.

```bash
oc get ns $NAMESPACE -o jsonpath='{.metadata.labels}{"\n"}' | jq
```

This is the single most common reason the API appears inert on an upgraded
cluster. See `removing-retained-enforce-labels.md`.

## Cause 6: the field is not in the API at all

`apiserver/cluster` always exists on a standard cluster, but `podSecurityAdmission`
is a feature-gated field and is only in the schema where the gate is on. Check
whether the API server will even accept it:

```bash
oc get crd apiservers.config.openshift.io \
    -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.podSecurityAdmission}{"\n"}'
oc get featuregate cluster -o jsonpath='{.spec.featureSet}{"\n"}'
```

Empty output from the first command means the field does not exist in this
cluster's schema. A write that sets it is **silently pruned** rather than
rejected, which is why the setting can appear to have been accepted and then
vanish.

A field absent from the schema, a field present but unset, and a field set to
`Privileged` are all the same state: the cluster has opted out and the effective
level is `privileged`.

## Cause 7: the configuration was erased by a downgrade

If the cluster was downgraded to a release without the field and then returned,
`spec.podSecurityAdmission` is gone — pruned by the older schema, at the latest
on the first write to `apiserver/cluster` by anything. That write may have been
an unrelated change made days later, so the disappearance need not line up with
the downgrade in the audit log.

Nothing restores it. Re-apply the intended configuration:

```bash
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforcementMode":"Restricted"}}}'
```

Administrators performing a downgrade should record the value first:

```bash
oc get apiserver cluster -o jsonpath='{.spec.podSecurityAdmission}' > psa-config.json
```

## Clearing conditions left behind by a downgrade

After a downgrade from release `n` to `n-1`, condition types introduced in `n`
remain on `kubeapiservers.operator.openshift.io` with no controller writing them,
and are not garbage collected. Because the status controller unions every
`*Degraded` and `*Upgradeable` condition into the ClusterOperator, a condition
captured in the seconds before a downgrade can leave `kube-apiserver`
permanently `Degraded` or `Upgradeable=False` for a reason no running component
can explain.

The signature is a condition whose `lastTransitionTime` predates the downgrade
and whose type is unknown to the running release. Find the index:

```bash
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.podSecurityAdmission.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.lastTransitionTime}{"\n"}{end}' \
    | cat -n
```

Then remove it explicitly (indices are zero-based; `cat -n` above is one-based):

```bash
oc patch kubeapiserver/cluster --type=json --subresource=status \
    -p '[{"op":"remove","path":"/status/conditions/<index>"}]'
```

Remove one at a time and re-read between patches — indices shift.
