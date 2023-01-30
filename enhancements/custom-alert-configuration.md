---
title: custom-alert-configuration
authors:
  - "@atiratree"
reviewers:
  - "@deads2k"
  - "@soltysh"
  - "@fabiand"
  - "@acardace"
approvers:
  - "@deads2k"
api-approvers:
  - "@deads2k"
creation-date: 2023-01-26
last-updated: 2023-01-26
tracking-link:
  - "https://issues.redhat.com/browse/CNV-15064"
see-also:
  - "https://github.com/openshift/cluster-kube-controller-manager-operator/pull/583"
---


# Custom Alert Configuration

## Summary

There is a need for a mechanism for a certain use cases, that would allow
OpenShift components to opt out kubernetes objects from being picked up by
alerts. The impact of missing this feature is felt most by users of KubeVirt
(OpenShift Virtualization). This feature should not be available to users
because it could disrupt existing alerting/monitoring. Users should use
alert silences instead.

## Motivation

Running virtual machine consists of 3 objects: VirtualMachine,
VirtualMachineInstance and a pod that is running the VM (virt-launcher-).
KubeVirt has a [Live Migration][1] feature that uses a two pod mechanism to
move the virtual machine between nodes. For the time of the migration there are
2 pods that are running at the same time. Once the migration is complete
the original pod will be deleted. This can be achieved by setting
`evictionStrategy: LiveMigrate` on the VirtualMachineInstance spec.

The migration can be started at any time. One of the main features of the
migration is the support for node drain (see Node maintenance).
That means that they should be able to handle any API eviction requests,
but since the migration takes time they have to defer that eviction until the
migration is done.

This is implemented via a validating [webhook][2] that intercepts the eviction
requests and starts the migration.

KubeVirt creates a PDB for each live migratable VM (the `virt-launcher-*` Pod)
to assist with this task. It uses `spec.minAvailable` field and is set to the
number of active pods: 1 in normal operation and 2 when migration is in progress.
This means the PDB is always at limit. This has two main functions.

1. As we described. KubeVirt is doing a custom eviction, so it lets the eviction
  request continue to the eviction API where it fails because the PDB is at
  limit and has `disruptionsAllowed` set to 0. This blocks the node drain in
  the short term as it cannot evict the virtual machine pods. But once the
  migration completes after some time, it will stop the original pod, so the
  drain can progress in the end.

2. They want the pods to be protected even when the webhook is down for any
  reason. The API initiated eviction will always respect the PDB and let the VM
  pod live. In that case this would block the node drain.

There does not seem to be any other option how they would achieve this
(for example by tinkering with PDBs).

In OpenShift we are alerting with warning severity on any PDB that is at limit
([PodDisruptionBudgetAtLimit][3]). Which means we check if a PDB status is
`current_healthy == desired_health` (and `expected_pods > 0`). Normal workloads
should have at least one more pod than is desired to support eviction.
Reporting these alerts is important because it helps users to pinpoint the
workloads that are violating this and could block node drain.
This is important for node maintenance and [cluster upgrades][4].

KubeVirt on the other hand uses PDBs, but is going around the intended use case
of eviction. They do this in order to support their functionality, but the
eviction or PDBs were never designed with such use cases in mind.
KubeVirt use case could be explored in a KEP, but that is a different topic.

Users that wish to use Live Migration feature then trigger this alert. From
their perspective they are just using KubeVirt API as intended and this warning
is unnecessary. The warning is a way to tell people that there are not enough
replicas in their workload  and that the node drain might not work.
For KubeVirt that is not the case - the intended number of pods per VM is 1
and the eviction is handled in a custom way.

For users with live migratable VMs, this alert is a constant occurrence.
Apart from the alert being annoying, this diminishes the value of the alert,
and it could be very easily overlooked when other workloads run into the PDB
limit.

### User Stories

Components should be able to mark certain resources in the cluster that should
not trigger specific alerts. However, this should only be allowed with the
consent of the alert owner.


* As an OpenShift component owner I want to be able to have a mechanism to opt
  out of alerts, where it makes sense.

* As a KubeVirt user I do not want to see alerts for PDBs that are guarding
  virtual machines with `LiveMigrate` eviction strategy.

### Goals

- Introduce a generic CRD for alert exclusion that can be used by the
  ValidatingAdmissionPolicy.
- Create a ValidatingAdmissionPolicy and alert for
  kube-controller-manager-operator that acknowledges PDB labels.

### Non-Goals

## Proposal

### Workflow Description

To solve this we need a mechanism for customizing alerts.
We are proposing to introduce a
`alerts.openshift.io/AlertName: excluded` label to allow exclusion of a set of
specific objects from a specific alert. In the case of KubeVirt we would like
to exclude PodDisruptionBudgets owned by KubeVirt from
`PodDisruptionBudgetAtLimit` alert. So the following label would have to be
placed on these PDBs:
`alerts.openshift.io/PodDisruptionBudgetAtLimit: excluded`.
The owner of the alert would have to make sure to exclude these objects in the
alert query.

Nevertheless, only allowed actors should have the power to set these labels to
prevent misuse.

In the case of the `PodDisruptionBudgetAtLimit` alert, excluding other users'
PDBs from the alerting would have a detrimental effect on the cluster
stability. Users could set this on any PDB and admins would have no reliable
way of knowing whether something is blocking the node maintenance or an
upgrade.

We can use an alternative admission control based on [CEL][5] expressions that
does not have the maintenance and reliability issues of an admission webhook.
We can create [ValidatingAdmissionPolicy][6] for PDB objects to prevent a misuse
of these labels. The validation expression would test for a presence of a
specified label in the PDB object and would check for the Kubevirt user/SA
in the request to decide if it is allowed.

We will discuss introduction of a CRD in [API Extensions](#api-extensions)
to support customization of allowed users.


#### Variation [optional]


### API Extensions

Since we would like to parametrize users or service accounts, it is difficult
to use built-in objects like ConfigMap and Secret. This is because they do not
allow special characters like `:` in the data key which the service accounts
have. And it seems there is no way to lookup a value in a map at this moment.

To support this, we suggest a following CustomResourceDefinition:

```go

//
// AlertExclusionConfig describes the alert exclusion configuration
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AlertExclusionConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AlertExclusionConfigSpec `json:"spec"`
}

// AlertExclusionConfigSpec describes the alert exclusion configuration
type AlertExclusionConfigSpec struct {
    // subjects to exclude from the alert
    //
    // +optional
    Subjects map[string]string `json:"subjects,omitempty"
}

```

The owner of the alert would create the AlertExclusionConfig resource and
manage the subjects. In the case of KubeVirt, this would include determining
if and in what namespace KubeVirt is installed.

```yaml
kind: AlertExclusionConfig
apiVersion: config.openshift.io/v1
metadata:
  name: pod-disruption-budget-at-limit
spec:
  subjects:
    'system:serviceaccount:kubevirt:kubevirt-controller': 'enabled'
```

Then the alert owner would introduce a validating admission policy that would
allow changes to the `alerts.openshift.io/PodDisruptionBudgetAtLimit` label
only for these subjects.

```yaml
apiVersion: admissionregistration.k8s.io/v1alpha1
kind: ValidatingAdmissionPolicy
metadata:
  name: poddisruptionbudget-at-limit
spec:
  failurePolicy: Ignore
  paramKind:
    apiVersion: config.openshift.io/v1
    kind: AlertExclusionConfig
  matchConstraints:
    resourceRules:
    - apiGroups:   ["policy"]
      apiVersions: ["v1"]
      operations:  ["CREATE", "UPDATE"]
      resources:   ["poddisruptionbudgets"]
    objectSelector:
      matchExpressions:
        - key: "alerts.openshift.io/PodDisruptionBudgetAtLimit"
          operator: Exists
  validations:
    - expression: "(has(params.spec.subjects) && request.userInfo.username in params.spec.subjects &&  params.spec.subjects[request.userInfo.username] == 'enabled') ? true : !((has(object.metadata.labels) && 'alerts.openshift.io/PodDisruptionBudgetAtLimit' in object.metadata.labels) || (oldObject != null && has(oldObject.metadata.labels) && 'alerts.openshift.io/PodDisruptionBudgetAtLimit' in oldObject.metadata.labels))"
      message: "Setting alerts.openshift.io/PodDisruptionBudgetAtLimit label is not allowed."
      reason: Forbidden
```

And bind the parameters with:

```yaml
apiVersion: admissionregistration.k8s.io/v1alpha1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: poddisruptionbudget-at-limit
spec:
  policyName: poddisruptionbudget-at-limit
  paramRef:
    name: pod-disruption-budget-at-limit # AlertExclusionConfig
```

Now, we allow only `system:serviceaccount:kubevirt:kubevirt-controller` user
to change `alerts.openshift.io/PodDisruptionBudgetAtLimit` label on all PDBs.

We can then safely allow an alert query to exclude PDBs with these labels.

```text
max by(namespace, poddisruptionbudget) (kube_poddisruptionbudget_status_current_healthy == kube_poddisruptionbudget_status_desired_healthy and on (namespace, poddisruptionbudget) kube_poddisruptionbudget_status_expected_pods > 0 unless on (namespace, poddisruptionbudget) kube_poddisruptionbudget_labels{label_alerts_openshift_io_pod_disruption_budget_at_limit="excluded"} == 1)
```


### Implementation Details/Notes/Constraints [optional]

We could do without a parametrized `ValidatingAdmissionPolicy` and hardcode the
service account(s), but this would be inflexible to the KubeVirt deployment and
extending the allowed subjects in the future.


### Risks and Mitigations

In case of misconfigured or missing `ValidatingAdmissionPolicy` it would be
possible to disrupt `PodDisruptionBudgetAtLimit` alert for any user who can
create PDBs. This should be mitigated by KCM-o ownership of the policy and
by testing.

### Drawbacks

ValidatingAdmissionPolicy is still in alpha.


## Design Details

### Open Questions [optional]

1. Are there any security implications with storing service accounts names in a
  CRD and comparing these agains `request.userInfo.username` in the
   `ValidatingAdmissionPolicy` ?

### Test Plan

- e2e test for kube-controller-manager operator should test a presence of
  the alert and `ValidatingAdmissionPolicy` and that it guards modification
  of PDBs labels. And correctly excludes labeled PDBs from
  `PodDisruptionBudgetAtLimit` alert.

### Graduation Criteria

This should not be a user facing API.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy


### Version Skew Strategy

### Operational Aspects of API Extensions


#### Failure Modes


#### Support Procedures


## Implementation History

- 2023-01-30: Initial enhancement proposal


## Alternatives

### Use a validating admission webhook instead

We could guard a PDB with validating admission webhook to prevent an unhandled
labels change misuse. Based on the user info from AdmissionRequest we would
decide if they can modify the label. We could check for either hardcoded
user/SA that does the request or for KubeVirt alternatively check:

- if the PDB is owned by the VM
- do SubjectAccessReview if the user has access to the VM
- check RBAC rights for some KubeVirt resource that would identify a controller

These could be potential issue with this alternative:
- less reliable than Validating Admission Policy
- would increase network latency for a core object
- maintenance burden ( would be probably owned by
  kube-controller-manager-operator)

### Introduce a new metric

Mainly, KubeVirt should be the only one allowed to disable this alert. We could
achieve this by introducing a custom metric called for example
`ocp_kubevirt_vmi_owned_pdbs`. KubeVirt would track its own special PDBs that
do not conform to the standard use case. We would then subtract this from the
rest of the PDBs at limit to stop alerting on the KubeVirt ones.

The metric should have at least a namespace and poddisruptionbudget name.

The alert query could look like this:

```text
max by (namespace, poddisruptionbudget) (kube_poddisruptionbudget_status_current_healthy >= kube_poddisruptionbudget_status_desired_healthy and on (namespace, poddisruptionbudget) (kube_poddisruptionbudget_status_expected_pods > 0) unless on(namespace, poddisruptionbudget) (ocp_kubevirt_vmi_owned_pdbs))
```

Kubevirt would maintain/own `ocp_kubevirt_vmi_owned_pdbs` metric and ensure
only the valid PDBs are ignored. This would make it more difficult for users
to insert their own PDBs into this ignore list.

The disadvantage is that it is not scalable for more components. The metric
describes only KubeVirt use case.

### Silencing the alert

It is possible to partially silence alerts that are firing either manually via
an [openshift console][7] or via an [alertmanager API][8].

It can filter out alerts according to known prometheus labels. Here we check
for the poddisruptionbudget label with a `kubevirt-disruption-budget-.*`  value.

```shell
curl -s -X POST -H "Authorization: Bearer ${TOKEN}" -H 'Accept: application/json' -H 'Content-Type: application/json' "${ALERTMANANER_URL}/api/v2/silences" -d '{
	"status": {
		"state": "active"
},
	"comment": "kubevirt PDBs",
	"createdBy": "kube:admin",
	"startsAt": "2023-01-12T20:37:30.995Z"
	"endsAt": "2024-01-12T22:32:07.000Z",
	"matchers": [
  	{
    	"isRegex": false,
        "isEqual": true,
    	"name": "alertname",
    	"value": "PodDisruptionBudgetAtLimit"
  	},
  	{
    	"isEqual": true,
    	"isRegex": true,
    	"name": "poddisruptionbudget",
    	"value": "kubevirt-disruption-budget-.*"
  	},
  	{
    	"isEqual": true,
    	"isRegex": false,
    	"name": "severity",
    	"value": "warning"
  	}
	],
  },
'
```

This would work in a sense that it would stop notifications for this alert and
would be silenced in the UI.

There might be following problems with this approach:
- In the UI it shows the alert is firing and being silenced at the same time.
  It might prompt users to action when seeing the alert is silenced. When you
  discover the alert via an API it just shows this alert is firing, and you
  need to match this to silences to get the whole picture.
- The silence is meant as a temporary solution and there needs to be `endsAt`
  date and time defined. It is also possible to expire the silence with one
  click in the UI. We would need to manage the silence to be sure it is always
  present and not expired.
- As mentioned above this is not a standard solution, and we are not shipping
  any silences with openshift AFAIK. We would probably need a controller that
  would reconcile the silence through alertmanager API.
- It is not precise enough. That is we would be filtering according to the
  known prometheus labels, which is a poddisruptionbudget name in this case.
  Users could hijack this functionality and create their own PDBs in their
  namespace with a kubevirt-disruption-budget prefix that would get captured
  by this silence.

### Silencing the alert with inhibition rules

Would conceptually be similar to silences.
KubeVirt would have to fire an alert (info) for each PDB that it would want to
exclude. Then we would need to introduce an [inhibition rule][9] in
AlertManager configuration that would mute any PDB alert that have a matching
KubeVirt alert.

This approach has similar issues to the silences:
- KubeVirt would have to manage an info alert
- the inhibition rules would need to be either shipped together with
  AlertManager or injected into the AlertManager configuration when KubeVirt is
  present
- muted alert would still show up in the console, but would not trigger
  notifications
- harder for users to cancel the mute compared to silences

### Managing alert with AlertRelabelConfig

We could define a relabel config that would specifically [manage][10]
`PodDisruptionBudgetAtLimit` alert and target Kubevirt PDBs. 

This could be achieved with a following config:

```yaml
apiVersion: monitoring.openshift.io/v1alpha1
kind: AlertRelabelConfig
metadata:
  name: kubevirt-relabel
spec:
  configs:
  - sourceLabels: [alertname,poddisruptionbudget] 
    regex: "PodDisruptionBudgetAtLimit;kubevirt-disruption-budget-.*"
    targetLabel: poddisruptionbudget
    action: LabelDrop
```

- it would be easier to ship with KubeVirt
- would only stop the notifications when alert fires
- would be more opaque why the alert does not fire when compared to silences 
- alert would show up in a OpenShift console as firing without any modification

## Infrastructure Needed [optional]


[1]: https://kubevirt.io/user-guide/operations/live_migration/
[2]: https://github.com/kubevirt/kubevirt/blob/7e66c0b8b1af54e66a9b0b174887898de5b8ff57/pkg/virt-operator/resource/generate/components/webhooks.go#L305
[3]: https://github.com/openshift/cluster-kube-controller-manager-operator/blob/152572d0606b4de816d06ea08b293e866467c94f/manifests/0000_90_kube-controller-manager-operator_05_alerts.yaml#L25
[4]: https://bugzilla.redhat.com/show_bug.cgi?id=1762888
[5]: https://github.com/google/cel-spec/blob/master/doc/langdef.md
[6]: https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/
[7]: https://docs.openshift.com/container-platform/4.11/monitoring/managing-alerts.html#managing-silences_managing-alerts
[8]: https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml
[9]: https://prometheus.io/docs/alerting/latest/configuration/#inhibit_rule
[10]: https://docs.openshift.com/container-platform/4.12/monitoring/managing-alerts.html#managing-core-platform-alerting-rules_managing-alerts
