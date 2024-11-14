---
title: early-monitoring-config-validation
authors:
  - "@machine424"
reviewers:
  - "@openshift/openshift-team-monitoring"
  - TBD
approvers:
  - "@openshift/openshift-team-monitoring"
  - TBD
api-approvers:
  - TBD
creation-date: 2024-11-13
last-updated: 2024-11-13
tracking-link:
  - TBD
---

# Early Monitoring Config Validation

## Summary

Introduce early validation for changes to monitoring configurations hosted in the
`openshift-monitoring/cluster-monitoring-config` and
`openshift-user-workload-monitoring/user-workload-monitoring-config` ConfigMaps to provide
shorter feedback loops and enhance user experience.

## Motivation

CMO currently uses ConfigMaps to store configurations for the Platform and User Workload monitoring stacks. Due to the limitations of this approach, a migration to CRD based configs is planned ([OBSDA-212](https://issues.redhat.com/browse/OBSDA-212)). In the interim, enhancing the validation process for these ConfigMaps would be highly beneficial.

Insights show that in `2024`, there were more than `650` unique CMO failures related to parsing issues that lasted over `1h`, with some going unnoticed for over `215` days. The total duration of all failures exceeded `10` years.

### User Stories

As a user, if my configuration is invalid (malformed JSON/YAML, contains invalid, no longer supported, or duplicated fields), I do not want to have to check the operator’s status or logs or wait for an alert to be notified of the issue.

Such situations may lead me to suspect other issues incorrectly within the monitoring stack, causing me to solicit help from colleagues or support.

The existing signals take time to propagate and can easily be missed, resulting in a poor user experience. A shorter feedback loop, where invalid configurations are rejected when trying to push them into the Configmaps (as with CRDs), would be more user-friendly.

### Goals

- **Early Identification of Invalid Configurations**: Detect some invalid configurations (malformed YAML/JSON, unknown fields, duplicated fields, etc.) before CMO attempts to apply them.
- **Improved User Experience**: Empower users with more autonomy by enabling them to identify and correct errors earlier in the configuration process.

### Non-Goals

- The early validation will focus on detecting common errors and avoid computationally intensive deep checks that might impact performance or make the check itself fragile. This means it will not catch all issues that may only be detected when CMO tries to apply the config.
- This addition does not intend to replace or render obsolete the existing `UserWorkloadInvalidConfiguration`/`InvalidConfiguration` related signals in the operator status/logs/alerts.
- This proposal does not intend to prevent or postpone the planned transition to CRDs for enhanced validation capabilities. Instead, it will prepare the way for it, provide a preview of what will happen with CRDs, educate users about it, and ease the migration.
- Some ConfigMap changes may bypass the CMO validation logic if the CMO operator is down for some reason; these changes will not be validated (best-effort approach).
- ConfigMaps with invalid monitoring configurations deployed before the webhook is enabled (before upgrading to the version that enables the validation webhook on CMO) will not be flagged or adjusted. The webhook will only intervene on them during subsequent changes, if any.

## Proposal

Implement and expose a validation webhook in CMO. This webhook will intercept `CREATE` and `UPDATE` actions on the platform and UW monitoring ConfigMaps. It will attempt to fetch the configuration within the ConfigMap, unmarshal/parse it, identify potential errors (such as malformed JSON/YAML, unknown field names, or duplicated fields), and reject the request if such issues are found.

### Workflow Description

The webhook will be enabled by default.

The `ValidatingWebhookConfiguration` will ensure the webhook only intervenes on changes to the two ConfigMaps: `openshift-monitoring/cluster-monitoring-config` and `openshift-user-workload-monitoring/user-workload-monitoring-config`.

```yaml
matchConditions:
  - name: 'monitoringconfigmaps'
    expression: '(request.namespace == "openshift-monitoring" && request.name == "cluster-monitoring-config")
      || (request.namespace == "openshift-user-workload-monitoring" && request.name
      == "user-workload-monitoring-config")'
```

The webhook will attempt to unmarshal/parse the config within these ConfigMaps.
If the unmarshalling fails, the action on the ConfigMap will be denied.

For example, if an incorrect field (with a subtle typo) is to be set, the change will fail with:

```
$ kubectl edit configmap cluster-monitoring-config -n openshift-monitoring
error: configmaps "cluster-monitoring-config" could not be patched: admission webhook "monitoringconfigmaps.openshift.io" denied the request: failed to parse data at key \"config.yaml\": error unmarshaling JSON: while decoding JSON: json: unknown field \"telemeterCliennt\"
```

### API Extensions

The following `ValidatingWebhookConfiguration` will be added:

```
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  ...
  name: monitoringconfigmaps.openshift.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    service:
      name: cluster-monitoring-operator
      namespace: openshift-monitoring
      path: /validate-webhook/monitoringconfigmaps
      port: 8443
  failurePolicy: Ignore
  name: monitoringconfigmaps.openshift.io
  namespaceSelector:
    matchExpressions:
      - key: kubernetes.io/metadata.name
        operator: In
        values: ["openshift-monitoring","openshift-user-workload-monitoring"]
  matchConditions:
    - name: 'monitoringconfigmaps'
      expression: '(request.namespace == "openshift-monitoring" && request.name == "cluster-monitoring-config")
        || (request.namespace == "openshift-user-workload-monitoring" && request.name
        == "user-workload-monitoring-config")'
    - name: 'not-skipped'
      expression: '!has(object.metadata.labels)
        || !("monitoringconfigmaps.openshift.io/skip-validate-webhook" in object.metadata.labels)
        || object.metadata.labels["monitoringconfigmaps.openshift.io/skip-validate-webhook"] != "true"'
  rules:
  - apiGroups: [""]
    apiVersions: ["v1"]
    operations:
    - CREATE
    - UPDATE
    resources:
    - configmaps
    scope: Namespaced
  sideEffects: None
  timeoutSeconds: 5
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

To avoid any divergence (the validate webhook producing false positives), the webhook will be
running the same code (a subset of the checks) that CMO runs when laoding and applying the config.

CMO will expose the webhook at `:8443/validate-webhook/monitoringconfigmaps`.

### Risks and Mitigations

In the event that the validation webhook makes incorrect decisions (which we aim to keep rare, as the webhook will run the same code that CMO uses when applying the configuration), users will have the option to temporarily bypass the CMO webhook. This can be done by adding the label `monitoringconfigmaps.openshift.io/skip-validate-webhook: true` to the ConfigMaps.

Additionally, the webhook endpoint will not perform client authentication on `/validate-webhook/monitoringconfigmaps`. Another proposal will be initiated to discuss how to facilitate easier identification of requests from the apiserver for webhooks in OCP.

### Drawbacks

Some users who may have been relying on or exploiting the lack of pre-validation will need to adapt, as their invalid changes to the ConfigMaps will now be denied by the apiserver. This adaptation is necessary for the future transition to CRD-based configuration. Tightening the configurations now serves as a preparatory step for the upcoming CRD-based configuration effort.

## Open Questions [optional]

## Test Plan

Since the webhook will be enabled by default, all existing tests that create or update the ConfigMaps holding the monitoring configuration are considered tests for the webhook itself.

Additionally, unit and e2e tests will be added (or adjusted) to better highlight invalid configurations scenarios.

Invalid configuration related tests outside the CMO repository will also need to be adjusted accordingly.

## Graduation Criteria

The webhook is intended to go directly to `GA` and be enabled by default.

End users will be informed of this change via an entry in the release notes.

We'll wait for the first instance where the webhook needs to be skipped (via the `monitoringconfigmaps.openshift.io/skip-validate-webhook: true` label) to document the procedure, probably in a KCS article. We'll avoid mentioning the opt-out mechanism in the official documentation to prevent abuse, as we want users to tighten up their use of the monitoring ConfigMaps in preparation for the CRD-based configuration migration.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

Once CRD-based configuration is GA, configuration via ConfigMaps will no longer be allowed, and the webhook logic will be removed.

## Upgrade / Downgrade Strategy

Even after CMO is upgraded to a version with the webhook enabled, as long as the existing monitoring config ConfigMaps are not updated, they will not be flagged by the webhook.

A change in `4.17.z` will make CMO report `upgradeable=false` if the existing configs contain malformed JSON/YAML, invalid fields, no longer supported fields, or duplicated fields. We will ensure clusters reach that version before being able to upgrade to `4.18`. This will help avoid blocking implicit or unplanned changes to ConfigMaps with invalid configs during the upgrade.

Upgrades will be covered by existing upgrade tests.

In case of a rollback, the CVO-managed `monitoringconfigmaps.openshift.io` `ValidatingWebhookConfiguration` may need to be deleted to avoid the unnecessary `timeoutSeconds: 5` overhead on each change to the monitoring config ConfigMaps.

## Version Skew Strategy

The `matchConditions` fields of `ValidatingWebhookConfiguration` are used to limit the webhook to only the 2 monitoring config ConfigMaps and to implement the opt-out mechanism via the label.

`matchConditions` are considered stable in Kubernetes `v1.30`, which has been used since OCP `4.17`. This means that even in the case of a partial upgrade of the apiserver or a downgrade, having that resource around shouldn't cause any issues.

## Operational Aspects of API Extensions

The webhook is configured with `failurePolicy: Ignore`, making it best effort and avoiding having the single CMO replica as a single point of failure. Another protection is added by setting `timeoutSeconds: 5` in case CMO is overwhelmed.

`timeoutSeconds: 5` means that the webhook may add up to `5 seconds` to the two monitoring config ConfigMaps `CREATE` and `UPDATE` requests.

In reality, even for a scenario of `5` ConfigMap updates per second (which is likely an overestimate of actual usage), the `99th` percentile of the processing latency of the admission webhook is expected to be less than `5ms`.

Fewer failures due to `UserWorkloadInvalidConfiguration`/`InvalidConfiguration` should start to be seen for the `monitoring` cluster operator, as some invalid configs will be caught earlier via the webhook now.

## Support Procedures

The `apiserver_admission_webhook_*` metrics should provide insights into the status of the webhook from the apiserver's perspective. For example:

```
histogram_quantile(0.99, rate(apiserver_admission_webhook_admission_duration_seconds_bucket{name="monitoringconfigmaps.openshift.io"}[5m]))
```

This allows us to monitor the processing latency.

From the CMO perspective, increasing the operator's log level (setting it to `-v=9`) reveals logs such as:

```
I1113 10:58:29.103668       1 handler.go:153] cluster-monitoring-operator: POST "/validate-webhook/monitoringconfigmaps" satisfied by nonGoRestful
I1113 10:58:29.103702       1 pathrecorder.go:243] cluster-monitoring-operator: "/validate-webhook/monitoringconfigmaps" satisfied by exact match
I1113 10:58:29.104042       1 http.go:117] "received request" logger="admission" object="openshift-monitoring/cluster-monitoring-config" namespace="openshift-monitoring" name="cluster-monitoring-config" resource={"group":"","version":"v1","resource":"configmaps"} user="system:admin" requestID="b154b96a-6fe6-4abd-a827-c662d8211719"
I1113 10:58:29.104687       1 http.go:163] "wrote response" logger="admission" code=403 reason="Forbidden" message="failed to parse data at key \"config.yaml\": error unmarshaling JSON: while decoding JSON: json: unknown field \"telemeterCliennt\"" requestID="b154b96a-6fe6-4abd-a827-c662d8211719" allowed=false
I1113 10:58:29.104762       1 httplog.go:134] "HTTP" verb="POST" URI="/validate-webhook/monitoringconfigmaps?timeout=5s" latency="1.42784ms" userAgent="kube-apiserver-admission" audit-ID="2570bcda-55eb-44f6-b319-5e29d58ad3f0" srcIP="10.128.0.2:48220" resp=200
```

Cross-referencing these with the apiserver logs should provide detailed insights on a per-request basis.

## Alternatives

Wait for CRD based configs to be GA.

## Infrastructure Needed [optional]
