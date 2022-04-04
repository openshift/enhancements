---
title: shared-resource-validation
authors:
  - "@gabemontero"
reviewers:
  - "@deads2k"
  - "@jsafrane"
  - "@adambkaplan"
  - "@coreydaley"
  - "@soltysh"
  - Barnaby Court
  - Chris Snyder
approvers:
  - "@deads2k"
  - "@jsafrane"
  - "@adambkaplan"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - "@deads2k"
creation-date: 2022-03-08
last-updated: 2022-03-29
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/BUILD-406
see-also:
  - "/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md"
replaces:
  - "N/A"
superseded-by:
  - "N/A"
---

# Shared Resource Validation

## Summary

This Enhancement Proposal details improvements to existing validations currently performed, as well as the introduction
of new validations to be performed, on artifacts related to the Shared Resource CSI Driver and Operator component introduced 
as Tech Preview in 4.10.  As a quick reminder, the associated API for "Shared Resources", namely SharedSecrets and SharedConfigMaps, are 
cluster scoped objects whose referenced namespace scoped Secrets and Configmaps are mounted into Pod Spec'able objects
via use of the associated Shared Resource CSI Driver.

With the introduction of the Shared Resources API in 4.10, any restrictions around use of the new CRDs or the declaring
of Volumes using the new Driver in Pods, was done at the CSI Driver level.  That is often not the 
most efficient location to enforce such restrictions, as the entire Kubelet provisioning path for the Pod must be exercised 
for the error to be caught.  Also, the user does not learn of the error upon creation of the Pod Spec'able object.
Instead, the user must inspect Pod status and Events to learn of the problem.

This proposal intends to improve on the usability around error discovery by employing validations called during API object
creation and modification.

## Motivation

### Goals

- Ensure names of pre-supplied SharedSecrets and SharedConfigMaps by OCP components will not conflict with any SharedSecrets or SharedConfigMaps created by users.
- Whenever possible prevent unsupported use of read-write volumes, as well as any future restrictions around the CSI Volume API or Shared Resource API
- Provide pluggable means of validation of the associated Secrets and ConfigMaps of SharedSecrets and SharedConfigMaps.

And provide validation failures on API object creation/modification for each of these whenever possible.

### Non-Goals

- Shared Resource controller, CSI Driver, Operator, nor Webhooks will determine the "Readiness" of any underlying `Secret` 
or `ConfigMap` of a `SharedSecret` or `SharedConfigMap`.  Such determinations are the sole provenance of the Operators/Controllers/etc.
that create those items.

## Proposal

See upstream doc 
[https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook)
for explanation of `ValidationAdmissionWebhooks`

We will employ this plug point for inserting the code that performs any validations added.

### User Stories

As a developer using the Shared Resource CSI Driver
I want validation for `readOnly: true` to be set on volumes for the Shared Resource CSI Driver to occur prior
to the Pod Spec'able object getting persisted to etcd, so that pods are not stuck in "Creating" state waiting for a mount that will never succeed.
Rather, I learn of the problem on the API Object PUT/POST.

As an OpenShift operator maintainer or cluster administrator
I want to reserve the `openshift-` prefix for SharedSecrets and SharedConfigMaps
so that future OpenShift operators can create system-level shared resources and not have name conflicts 
with items created by OCP users.

As a developer, OpenShift operator maintainer, or cluster administrator
I want SharedSecrets and SharedConfigMaps pre-populated by OCP Cluster Operators to make some indication on those
SharedSecrets and SharedConfigMaps that the referenced Secrets and ConfigMaps are valid, functional, and ready for use.

As a developer using the Shared Resource CSI Driver I want any validations to be provisioned and lifecycled in as much
as possible the same fashion as the driver itself for consistency (i.e. via Cluster Storage Operator).

As a developer or administrator using the Shared Resource CSI Driver I want sufficient metrics on the validations' prevention of incorrect Pods,
SharedConfigMaps, and SharedSecrets, as a means of understanding the need for correcting misuse..

As a developer or administrator using the Shared Resource CSI Driver I want necessary debug of the validation mechanism to be captured by
OCP must-gather process.


### API Extensions

Quick note:  When discussing API, 'Shared Resource' on its own == SharedConfigMap / SharedSecret, and 'Reference' == the corresponding
Secret or ConfigMap.

A [ValidatingAdmissionWebhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook)
will be used to inspect all created Shared Resources, and make sure their names do not start
with the reserved prefix "openshift-" unless their Shared Resource name and Reference key are found in a 
well known list of OCP pre-populated Shared Resource.  That list will be in well known ConfigMaps that resides 
in the managed CSI Driver namespace.  Components / operators other than Shared Resources that want to create pre-populated
SharedSecrets or SharedConfigMaps should submit PRs against the Shared Resource CSI Driver Operator Repo to update the 
corresponding ConfigMap with the necessary information.  This is akin to how we create in-payload image streams via the PRs against the 
samples operator today.

There will be a "ocp-sharedsecret-list" and "ocp-sharedconfigmap-list" ConfigMap.  The key of an entry is the Shared Resource
name.  The value should be of the form "namespace:name" in order to capture.  These ConfigMaps will be managed by the
CVO/CSO infrastructure via there definition in the Shared Resourece CSI Driver Operator assets folder in the CSO repository.

A brief example:

```yaml
kind: ConfigMap 
apiVersion: v1
metadata:
  name: ocp-sharedsecret-list
  namespace: openshift-cluster-csi-drivers
data:
  openshift-etc-pki-entitlement: openshift-config-managed:etc-pki-entitlement
```

The "openshift-*" naming restriction is new as compared to the initial tech preview release of Shared Resources in 4.10.

A `ValidatingAdmissionWebhook` will be used to inspect 
all created Pods, and k8s Pod derived types (Deployments, DaemonSets, etc.) with CSI volume mounts referencing the Shared
Resource CSI Driver and make sure no read-write Shared Resource CSI Volumes are specified.  In other words, the volume attribute
'readOnly: true' setting must exist.  Note, the understanding is that there is no "duck typing" in vanilla k8s or OpenShift that we can leverage to
capture all "Pod Spec'able" API objects.

For Builds, BuildRequests, and BuildConfigs, the existing [Aggregated OpenShfit API Server Build Admission plugin](https://github.com/openshift/openshift-apiserver/blob/master/pkg/build/apis/build/validation/validation.go)
could be updated to enforce this restriction before the build controller in the OCM gets involved in translating a Build, BuildConfig,
or BuildRequest into a Build Pod.  The Pod level ValidatingAdmissionWebhook could catch this as well, but catching the 
error case sooner will provide even better usability.  In particular `oc start-build` invocations would now see the error
immediately.

The 'readOnly: true' for Pods restriction exists in 4.10, but is only enforced at the CSI driver level.

On the associated `ValidatingWebhookConfiguration`, per guidance from David Eads, which should utilize the label selector
capabilities to help filter which Pods are addressed by our new `ValidatingAdmissionWebhook`.  Consider the following 
example:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
...
webhooks:
  - name: pod.csi.sharedresource.openshift.io
    failurePolicy: Ignore
    namespaceSelector:
      matchExpressions:
        - key: runlevel
          operator: NotIn
          values: ["0", "1"]
        - key: openshift.io/run-level
          operator: NotIn
          values: ["0", "1"]
        - key: csi.sharedresource.openshift.io/skip-validation
          operator: NotIn
          values: ["true"]
    rules:
      - operations: [ "CREATE" ]
        apiGroups: []
        apiVersions: []
        resources: [ "pods" ]
        scope: "*"
      - operations: [ "CREATE", "UPDATE" ]
        apiGroups: [ "apps" ]
        apiVersions: [ "v1", "v1beta1" ]
        resources: [ "deployments", "daemonset", ....]
        scope: "*"
      - operations: [ "CREATE, "UPDATE" ]
        apiGroups: [ "apps.openshift.io" ]
        apiVersions: [ "v1" ]
        resources: [ "deploymentconfigs" ]
        scope: "*"
    ...
```

Let's dive into the 2 different label
- The first `matchExpression` is the means of preventing the use of our new webhook for any Pods of the core OCP control pane.  Because of this
label selector we will filter out components like [the K8s API Server Operator Namespace manifest](https://github.com/openshift/cluster-kube-apiserver-operator/blob/d80b406ec4844328e04253f9ed5aec3f4a29834a/manifests/0000_20_kube-apiserver-operator_00_namespace.yaml#L11)
- The second `matchExpression` is the means of preventing the use of our new webhook for any Pods in other OCP namespaces we want to exclude.
Initially, we are thinking only the [CSI Drivers namespace](https://github.com/openshift/cluster-storage-operator/blob/2a3fbb68afd066d16582b09f9fd834c7fd03457e/manifests/02_csi_driver_operators_namespace.yaml) 
will be excluded (as that is where we will also host our webhook), so
to allow for the use of Shared Resources in OCP operators in the future for accessing "popular" Secrets and ConfigMaps (think the Global Proxy certs).


Next, the idea of validating the References of any future well-known, pre-populated Shared Resources
(like the entitlement secret) by other OCP CVO Operators was also raised during the review of the 4.10 blog post fof Shared Resources.  

The discussion landed around these points:
- A golang constant will declare the name of a k8s Condition that other components (i.e. OC Operators besides Shared Resources) will
set on any Shared Resource that they pre-populate into OCP clusters.
- REMINDER: the status structs of Shared Resources have an array of Conditions.  See 
[here](https://github.com/openshift/api/blob/dbc82f2a3bc8b07e39628d36aee19d5a62847e2c/sharedresource/v1alpha1/types_shared_configmap.go#L83-L88) and
[here](https://github.com/openshift/api/blob/dbc82f2a3bc8b07e39628d36aee19d5a62847e2c/sharedresource/v1alpha1/types_shared_secret.go#L81-L86)
- The components who create pre-populated Shared Resources should have a controller that performs whatever validations
they deem possible to verify the References, and those controllers are what will update the Status Condition arrays
for the pre-populated Shared Resources

With the initial implementation, 2 new constants will be defined along with the current status API, with godoc explaining
the intent of these conditions, and who manages them:
- `Available`: indicates that the Reference exists
- `Ready`: indicates that the Reference is ready to be consumed, or facilitate the purpose for which it was created (i.e. the associated user can access his entitled content with the entitlement secret)

Who sets the conditions:
- 'Available': the Shared Resource controller can set this as part of its existing watches on the Shared Resources (creation, update, and relist).  Updating Shared Resources on 
Reference delete events is *possible*, though the implementation details are trickier there, so this EP will not call for that (though we can revisit during implementation and its PR review),
in the name of "eventual consistency".  An OCP operator which is pre-populating Shared Resources for References it manages is allowed to set it as well if it likes, but it is not required.
- `Ready`: if the Reference is not available, the Shared Resource controller *could* by extension set Ready to false; however, we absolutely want to avoid the slippery slope of consumers 
thinking that the Shared Resource controller deals with the "readiness" of the underlying Reference of the Shared Resource; so this EP is putting the stake in the ground that only the 
contributing OCP operator maintains `Ready`.

At this time, neither the CSI Driver / Operator nor any Webhooks will react to external controllers/operators manipulating
the "validity" conditions.  See the alternatives section for potential considerations for doing so.  But human inspection,
controller reaction via watches, as well as metrics/alerts, are all possible off of this condition.

To further illustrate, consider this specific example:
- the Insights Operator starts creating (via their CVO manifest list, or Operator logic) a SharedSecret to encapsulate the entitlement Secret (namespace: openshift-config-managed, name: etc-pki-entitlement) they maintain
- Insights Operator may in fact be already performing some "validations" around that secret
- but if not, we work with them to add or augment what they have
- but with their validations, if Insights Operator discover something wrong with the entitlement secret, the Insights Operator updates the status of the SharedSecret they create with the appropriate conditions, setting true/false, reason, message on our well known "validity" condition, explaining something is wrong
- and if we get a support case that says "entitled builds don't work", we will ask them to check the conditions on the SharedSecret that Insights Operator manages, where
  the Reason/Message Insgiht Operator puts there helps the customer figure out what he needs to do to fix his entitlement credentials (maybe the customer has to log onto console.redhat.com and do something) and then his entitlement builds work

This is very similar to what CVO operators do with the well known conditions CVO defines for the ClusterOperator object in order 
to report their status.

### Implementation Details/Notes/Constraints [optional]

Generally, basic field checking of the API Object created or modified when the webhook is called should be sufficient
for the 'readOnly: true' and 'openshift-*' restriction enforcement.

No special handling of existing Shared Resources whose names start with 'openshift-*' and were created by customers when OCP was at a 
4.10.x level is required, since with "TechPreviewNoUpgrade", we have no upgrade to worry about.  A customer will have to 
create a new 4.11.x cluster, and if they attempt to create a Shared Resource whose name starts with "openshift-*", 
the 4.11.x webhook will prevent it, and they will simply have to pick a new name.

The webhook will handle the "openshift-*" naming restriction for creates of Shared Resources.
The webhook will read the "allow list" ConfigMaps on startup (the "allow list" ConfigMap should not change except on upgrade, as 
it is CSO Operator managed item) and so the webhook's validations should not involve any RPC calls, thus minimizing SLI
impact on the API Server.

Existing checks in the controller for the presence of "readOnly: true" check will be kept.  Analogous checks for 
who creates "openshift-*" Shared Resources will also exist in the controller.  This covers the case when the webhook
might be down.

### Risks and Mitigations

Since Shared Resources is Tech Preview 4.10, we can introduce validation controls that "break existing API usage" in later releases than 4.10.
In particular, any users who were creating SharedSecrets of their own that start with "openshift-".  But we have to 
have all such restrictions in place before moving from Tech Preview to GA.  The 4.10 blog post does give a warning
that we might restrict use of Shared Resources names starting with "openshift-*"

And again, the "readOnly: true" validation was already in effect in 4.10, but only enforced at the controller.

That said, even with Tech Preview, release notes are warranted if we go down the path of invalidating of 
Shared Resources that were "OK" in 4.10.  And user push back is at least conceivable on a case by case basis.

Any Pod level validation logic will get utilized *A LOT*, so it better work.  See the [Failure Modes](#failure-modes)
section for our rationale on being OK with taking this check on.

## Design Details


### Open Questions [optional]

- Metrics for some persistent volumes were found at [https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/#kube-controller-manager-metrics](https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/#kube-controller-manager-metrics)
but nothing was found for in-line CSI ephemeral volumes other than the alpha level feature [External Health Monitor](https://kubernetes-csi.github.io/docs/volume-health-monitor.html).  Are we missing anything?
- Are their any issues with preventing UPDATES to the workload types if they specify CSI volumes where 'readOnly: true' is not set?


### Test Plan

The existing Shared Resource CSI Driver e2e tests will be tweaked to look for read-write failures on object creation in addition
examining events and Pod status.  We'll see if we can force a mode where the webhook is disabled and the controller checks still work.

New e2e tests will be added to our suite to ensure any Shared* objects whose names start with
"openshift-" must match up with the list of allowed items in the designated ConfigMap in the OpenShift CSI Driver namespace.  
Again if tenable will have flavors for both when the webhook is present and when only the controller is present.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N / A

#### Tech Preview -> GA

Adding just this feature will not result in this feature moving from Tech Preview as introduced in 4.10 to GA.

We also need the storage team's work and SCC/Pod Policy and formalizing the approach for unprivileged users 
creating objects with use of the Shared Resource CSI Driver.

Nor is it the gating factor on the API moving past the v1alpha1 level introduced in 4.10.
Ideally, we want a period of usage in the field and feedback from users before making the call for API 
version promotion from v1alpha to v1beta or v1.

#### Removing a deprecated feature

N / A

### Upgrade / Downgrade Strategy

Technically, with still requiring TechPreviewNoUpgrade, we don't have to worry about upgrade / downgrade yet.

However, some thoughts on preparing ourselves for when we promote from tech preview to GA:
- As a cluster storage operator managed item, upgrades/downgrades follow the same pattern established there, both in how
the CSO is still managed by the CVO, and in how the specific CSI Driver Operators are managed by the CSO.
- The webhook configuration for plugging into the API Server as well as the deployment for the webhook itself are both
assets managed by the CSO managed Shared Resource CSI Driver Operator, with the deployment of the webhook getting 
rolled out as different versions of the CSO and Share Resource CSI Driver Operator are applied.


### Version Skew Strategy

No version skew issues anticipated.  For starters, for now at least, we are still TechPreviewNoUprade.  But rationale as 
to why there would not be any once achieving GA follows:

- The validation admission controls discussed here are pattern matching only, stateless checks across calls to it, so the belief is
upgrades/downgrades in version should roll out the associated Deployments should not affect this enhancement, and the webhooks behavior 
and what is allowed is thus gated by the version it is at.  
- Similarly, changes in the Webhook's configuration
should get applied by existing OCP CVO/CSO processing.  The only API for engaging the webhooks is the k8s webhook APIs themselves.
- The one time read of the ConfigMap allow list for Shared Resources whose name starts with "openshift-*" is impacted
in the sense that which Shared Resources the webhook allows could change.  However, the removal of any 4.N+1 Shared Resources
pre-populated by another OCP controller/operator should be handled by the CVO's management of that operator's manifest
(i.e. other OCP operators that want to provide pre-populated Shared Resources should do so in their manifest.)

And with duplicating checks in the controller relist or CSI Driver interface with the Kubelet, the desired validations
will be made even if for some reason and upgrade/downgrade of the webhooks are not started yet, or if the new webhooks are not
functional for some reason. The only difference is how the prevention of the mount is surfaced, as articulated in the goals of the EP.

### Operational Aspects of API Extensions

Carry over from 4.10 (looking to confirm this with 4.10 metrics): expect a low number (single digits) of SharedSecrets
and SharedConfigMaps.  Expect consumption of those Shares across many Pods etc. across many namespaces.

Also carry over: RBAC objects that grant permission for Pods' ServiceAccounts in those namespaces to use SharedSecrets / SharedConfigMaps 
in each of those namespaces are needed.  RBAC so that specific SharedSecrets / SharedConfigMaps can be discovered are also
most likely.

So the 4.10 impact for Shared Resources on the API Server storage i.e. 'etcd' was deemed "no problem".

#### New SLIs for the new admission validations to help cluster administrators and support

Separate metrics for
= SharedSecret creation/modification failures because of an openshift-* name.  
- SharedConfigMap creation/modification failures because of an openshift-* name.  
- SharedConfigMap/SharedSecret having the 'Available' or 'Ready' condition go to non-true.  
- Pod creation/modification success/failure count for Shared Resource CSI Volumes without 'readOnly: true'.  

We do not foresee any labelling required at this time for any of these.

The existing `webhook_admission_duration_seconds("<name of our new admission controller>", "validating", "put", "false")` metric
has been deemed sufficient for tracking any SLI impact for `ValidatingAdmissionWebhooks` that we add.

As we are validating at both the webhook and controller (in case webhook is down), we will have separately
named metrics (i.e. "..._webhook_total" and "..._controller_total") for the same logical items in the above list.
Given the controller and webhook will be in separate Pods and have separate metric service endpoints registering metrics
with prometheus, that is the implementation reality.  Any alerts defined can take into account both _webhook and _controller
metrics of the same logical significance.

Alerts:  using the `rate()` prometheus function, we can compare our new metrics above along with existing metrics from 4.10
wrt invalid names and failed/successful mounts.  We'll define alerts where, for example, if failures / successes over 1 minute > 0.1, fire.  A pseudo-code
example: `rate(read_only_failures[1m] / rate(successful_mount[1m] + read_only_failures[1m]) > 0.1`.  Debating info only to moderate priority for the alert.
We'll finalize during implementation and update EP accordingly.

Alert 2: if any Shared Resources have an invalid Reference.  Info level priority for user created Shared Resources.  Considering
moderate to higher priority for pre-populated Shared Resources from other OCP operators.  Or define additional keys to be
added to the allow list ConfigMap that specifies the priority desired by the component which is creating the pre-populated
Shared Resource.

#### Impact of existing API Server SLIs

The 'readOnly: true' and 'openshift-' checks involve simple pattern matching on the API objects the webhook(s) are configured
to validate, so the impacts on API throughput/scalability/availability are thought to be negligible.  In particular, 
no additional RPCs are envisioned while executing the validation.

The "handling" of 4.10 level Shared Resources created with names starting with "openshift-" will be done in the controller
and not in the ValidatingAdmissionWebhook.

The controller's examination of the Validity condition in its existing Shared Resource reconciliation loop should 
have minimal/negligible impact there.  And the controller's removal of data is a behavior whose path length is 
already established in 4.10, as we remove data if the service account has its 'use' permissions on the Shared Resource
revoked.

#### Measuring / Verifying impact on existing API Server SLIs

From existing metric discovery, what we will depend on:
- the EP template example in "Support Procedures" of using `webhook_admission_duration_seconds("<name of our new admission controller>", "validating", "put", "false")`, with >1s latency resulting in the `WebhookAdmissionLatencyHigh` alert firing, appears to be the valid choice addressing our new webhooks' impact on the API Object create/modify flows.  Is that correct?

Otherwise, items found from general k8s metric searches ....
- the `apiserver_request_duration_seconds` is a histogram metric that has labels around resource/subresource, scope, and verb
- the `kubelet_pod_start_duration_seconds_bucket` is a histogram metric that has labels around the create/update/sync operator types.

Unclear how to sort the second and third metrics by namespace so that we could compare with Shared Resource metrics that could 
convey "which namespaces are using Shared Resources and are slowing things down".  So they appear unuseful to us.

#### Failure Modes

1. Cluster Storage Operator / Shared Resource CSI Driver Operator failures could result in the ValidatingAdmissionWebhook not getting started
2. API Server failures could result in the ValidatingAdmissionWebhook not getting called
3. Unanticipated bugs in the ValidatingAdmissionWebhook result in incorrect validations

Each of those ValidatingAdmissionWebhook checks either already are or will be made in the Shared Resource Controller and CSI Driver.
So no violations of what we allow should occur, unless there are bugs across the entire path from webhook to controller to CSI driver for 
a given constraint.

Incorrectly preventing the create/update of Shared Resources at the moment appears to be relatively minor on the problem
scale for cluster administrators and support.  But the error returned wil be directly available to the user.  That, along
with the CLI invocation used, and any associated YAML for the object, should lead to straight forward diagnostics for 
support.

The enforcement of 'readOnly: true' in particular is worth discussing in this section.  It seems like very simple 
pattern matching / field checking, but unanticipated bugs when performing this check
in a ValidatingAdmissionWebhook could be impactful, as we are ultimately gating whether a Pod is allowed or not.  If 
there is a bug which results in valid Pods being rejected, that could have broad and major consequences.  As Adam Kaplan
reminded me, this also came up during the work on the KEP draft for CSI volume security.  But after conferring with David
Eads, a check of 
- are their volume mounts
- do any use the Shared Resource CSI Driver
- of those mounts, are any missing 'readOnly: true'
should be simple enough that code inspection, QE testing, and CI should suffice to provide sufficient confidence.

On to the last topic for this section, the OCP teams possibly involved with issues:
- API Server (webhook invocation, confusion around creating/modifying objects)
- Kubelet/node (incorrectly perhaps if valid Pods fail to come up, unless there is a problem beyond this EP's validations)
- Cluster Storage Operator (currently storage team, if a CSO problem leads to the webhook not coming up) 
- Shared Resource Operator/Driver themselves (currently build/jenkins team)


#### Support Procedures

##### Detect Failure modes and Possible Symptoms

- If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
- Cluster Storage Operator / CSI Shared Resource Operator will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
- The k8s metric `webhook_admission_duration_seconds("<name of our new admission controller>", "validating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.
- Otherwise, we should be able to fall back on must gather.

##### Disable the API extension (e.g. remove ValidatingWebhookConfiguration `<names of the our new webhook configs>`)

- What consequences does it have on the cluster health?

There are no consequences on cluster health for disabling the new ValidatingAdmissionWebhook(s).  They provide
for better user experience with respect to error reporting when incorrect settings are provided Shared Resource instances
or Pods (and their descendents) that specific CSI volumes using the Shared Resource CSI Driver.  But the controller / CSI Driver
can also catch those errors, though as the Goals/User Stories note, the user has to do more digging.
      

- What consequences does it have on existing, running workloads?

There are no consequences on existing, running workloads.  Only the Shared Resource Controller and CSI Driver impact
existing, running workloads that have Shared Resource CSI Volume mounts.

- What consequences does it have for newly created workloads?

For properly constructed Shared Resources and Pods with Shared Resource CSI Volume Mounts, incorrect specifications
will still be denied, but the errors will not manifest on Pod creation, but when Pod containers are trying to start.

## Implementation History

Initial submission of this EP

## Drawbacks

- Customers prove inexplicably resistant to reserving "openshift-" naming prefixes for Shared Resources (though they seem to have accepted similar restrictions for namespaces)

## Alternatives

First, currently see no reasons to use the older method (like in the early 3.0 days) of an admission plugin inside openshift-apiserver
for validation, like you see for older API like Builds, DeploymentConfigs, ImageStreams, and Templates, could be found.
There was some question during the review of this PR if validating Pods necessitated the use of an aggregated API server
admission plugin (because of availability reasons) for any Pod level validation, but this was brought up with David Eads in #forum-api-review and 
as long as we made sure the `ValidatingWebhookConfiguration` ensured that control plane pods were exempt, the pods
in the namespace hosting our webhook were exempt, and that our 
validations were not directly related to Pod security (it was determined that the SELinux labelling motivation for 
requiring 'readOnly: true' did *NOT* fall under that category), using the simpler `ValidatingAdmissionWebhooks` was OK.

Also, for failure recovery in case the webhook is down, we are/will already provide the same validations in the Controller/CSI Driver.  

Next, the use of a `MutatingAdmissionWebhook` was also considered briefly prior to the EP's creation, and was also
suggested during PR review, however, per guidance from Davide Eads, mutating admission webhooks make it difficult for 
Pod authors to understand what they are building and the behavior across different distributions is particularly painful.  
The only cases where each cluster or namespace will have a different value to embed (user UIDs vary by namespace in openshift instance)
truly necessitate the use of mutating webhooks.  But for cases where a Pod spec should simply specify something in particular, 
it is better to require a user to author it that way.

Then, with the new validity condition, both the webhook and CSI Driver / Operator could react to Shared Resource references
in the following ways:
- The webhook could look up the Shared Resource cited in the CSI Volume mount reference in a Pod and reject the Pod's
creation if the Shared Resource's Reference has been denoted as invalid.
- The CSI Driver / Operator could treat the validity condition in the same way it treats whether the Pod ServiceAccount has
'use' permissions for teh Shared Resource, and either prevent initial mount provisioning when contacted by the Kubelet, or 
remove the contents of the Shared Resource CSI Volume if the condition switches to "invalid" after the volume was provisioned
for the Kubelet.
However, during the review, simplicity was preferred.  We can certainly add such behavior later.  Also, short term inspection
of the condition is possible both by human users, as well as consumed by third party controllers/operators via watches on
the Shared Resource in question.

## Infrastructure Needed [optional]

There is a 'csi-snapshot-webhook' instance of the 'ValidatingWebhookConfiguration' type in the 'openshift-cluster-storage-operator'
namespace that actually employs a CVO based operator for its management.  Per confirmation with Jan/Storage team,
management of any new webhooks for Shared Resources should be managed by the Shared Resources CSI Driver Operator, where
that operator is managed by the Cluster Storage Operator.  Assets and manifests in those two repos will
be updated as needed so that there is permission to create the webhook related API Objects.

So we do not envision needing new repositories or a new operator for managing the webhook(s).

The current core repository of [https://github.com/openshift/csi-driver-shared-resource](https://github.com/openshift/csi-driver-shared-resource)
is a generic enough name that it could host the logic a new ValidatingAdmissionWebhook in addition to the Controller/CSI Driver and produce
the additional image to include in the OCP payload.  This way both the webhook and controller can share common validation and metric logic
(though there will ultimately be separate controller/webhook metrics).
