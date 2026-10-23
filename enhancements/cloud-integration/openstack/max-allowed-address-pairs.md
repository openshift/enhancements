---
title: max-allowed-address-pairs
authors:
  - "@danchild"
reviewers:
  - "@mandre"
approvers:
  - "@stephenfin"
  - "@jcaamano"
api-approvers:
  - "@bpalmer"
creation-date: 2026-10-13
last-updated: 2026-10-13
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3523
see-also:
  - https://github.com/openshift/cloud-network-config-controller/pull/230
  - https://github.com/openshift/cluster-network-operator/pull/3058
---

# Configurable OpenStack Maximum Allowed Address Pairs

## Summary

Standalone OpenShift clusters on OpenStack currently use a default limit of 10 allowed address pairs per Neutron port when calculating Egress IP capacity. OpenStack administrators have the option to configure Neutron with a different limit; however, OpenShift cannot inherit a modified configuration because the value is
hardcoded in the Cloud Network Config Controller (CNCC). Because the neutron configuration cannot be automatically discovered through its API, we are proposing the addition of an optional `OpenStack.maxAllowedAddressPairs` field to the `Infrastructure` API, allowing administrators to mirror the OpenStack configuration in their OpenShift clusters. The Cluster Network Operator (CNO) would consume this field and pass it to the CNCC. 

## Motivation
A current OpenShift user requested this capability because it is blocking a migration to OVN.

### User Stories

- As an OpenStack cluster administrator, I want to pass a non-default `max_allowed_address_pairs` Neutron configuration to OpenShift to increase per-node Egress IP capacity.

### Goals

- Expose an optional OpenStack field in the `Infrastructure` API so users can configure the max allowed address pairs per Neutron port.
- Reject explicitly configured values less than 1 through API validation.
- Allow CNO to pass the configured value to the cloud-network-config-controller, which uses it in its existing Egress IP capacity calculation.
- Preserve current behavior when the field is omitted; the controller continues to use its existing default of 10.

### Non-Goals

- Discovering the configured Neutron limit automatically. Neutron does not currently provide an API for this setting.
- Changing the cloud-network-config-controller's capacity calculation, port mutation, or error behavior.
- Changing the default behavior for clusters that do not set the new field.
- Integrating this setting with OpenStack HyperShift hosted clusters; that work may be proposed separately.

## Proposal

Add an optional `maxAllowedAddressPairs` field to the OpenStack platform configuration in the cluster-scoped `Infrastructure` API. When set, the API accepts only positive values. CNO will read the configured value and pass it to the cloud-network-config-controller. The controller uses the value as the Neutron allowed-address-pair capacity when calculating Egress IP capacity for node ports.

The administrator must set the field to match the OpenStack cloud's Neutron configuration if different from the default. 
Omitting the field preserves the existing behavior, which defaults to 10.

### Workflow Description

**OpenStack administrator** is responsible for configuring the OpenStack cloud and its Neutron settings.

**OpenShift cluster administrator** is responsible for configuring the cluster's `Infrastructure` resource.

1. The OpenStack administrator configures Neutron's `max_allowed_address_pairs` limit. This is outside the control of OpenShift.
2. The cluster administrator sets the configured value on the cluster `Infrastructure` resource:

   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Infrastructure
   metadata:
     name: cluster
   spec:
     platformSpec:
       type: OpenStack
       openstack:
         maxAllowedAddressPairs: 20
   ```

3. The OpenShift API validates that an explicitly supplied value is greater than zero. Values of zero or less are rejected and are not persisted.
4. CNO observes the configured `Infrastructure` value and passes it to the cloud-network-config-controller.
5. The cloud-network-config-controller uses the configured limit in its existing calculation.

If the field is omitted, CNO does not provide an override and the controller retains its current default of 10. If the OpenStack administrator changes Neutron's limit, the cluster administrator must update the `Infrastructure` value to keep the two configurations in sync.


### API Extensions

This enhancement adds a field to the existing `Infrastructure.config.openshift.io/v1` API.

The proposed field is `spec.platformSpec.openstack.maxAllowedAddressPairs`, an optional integer. When set, its minimum value is 1.


### Topology Considerations

#### Hypershift / Hosted Control Planes

OpenStack HyperShift hosted clusters are out of scope for this proposal. The current HyperShift
reconciler creates the hosted cluster's `Infrastructure.spec.platformSpec.openstack` as an empty
object, and the HostedCluster API does not currently expose an Infrastructure spec field for this
value. Making the field user-configurable for hosted clusters would require a separate HyperShift
API and controller integration. That integration can be addressed in a later enhancement.

#### Standalone Clusters

This feature applies to standalone OpenShift clusters on OpenStack. The cluster administrator sets the value on the cluster `Infrastructure` resource, and CNO passes it to the cloud-network-config-controller running for that cluster.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO):** The feature uses the existing CNO and cloud-network-config-controller components and adds no new pods or per-node agents. The same API validation and OpenStack capacity behavior apply; no additional SNO-specific resource overhead is expected.

**MicroShift:** This feature does not apply to MicroShift. 

#### OpenShift Kubernetes Engine

This enhancement is specific to the OpenShift `Infrastructure` API and OpenShift networking components. It does not change upstream Kubernetes APIs.

### Implementation Details/Notes/Constraints

The intended API shape is:

```go
// maxAllowedAddressPairs is the maximum number of allowed address pairs
// configured in Neutron. When set, the value must be at least 1.
// +kubebuilder:validation:Minimum=1
// +optional
MaxAllowedAddressPairs int32 `json:"maxAllowedAddressPairs,omitempty"`
```

The field is optional. An omitted value is not materialized in the API object, and the controller's existing default of 10 remains in effect. Explicit values must be at least 1; the API does not attempt to determine whether the value matches the cloud's actual Neutron configuration.

The controller-side work is tracked in:

- [cloud-network-config-controller PR #230](https://github.com/openshift/cloud-network-config-controller/pull/230), which makes the capacity limit configurable and preserves the default of 10 when no override is supplied. The PR is merged.
- [cluster-network-operator PR #3058](https://github.com/openshift/cluster-network-operator/pull/3058), which adds downstream configuration plumbing. It was authored before this proposal; the CNO integration for this enhancement would read the value from the `Infrastructure`, removing the need for a config map.

The following behavior is intentionally not changed by this API enhancement: how the controller accounts for existing address pairs, how it reports a configured limit that is too small for a port's existing state, and how the controller handles Neutron-side failures. The OpenStack administrator must keep Neutron and OpenShift in sync.

### Risks and Mitigations

- **The OpenShift value does not match the Neutron setting.** If OpenShift is configured with a value higher than Neutron's actual limit, Neutron may reject later port updates. If the value is lower, OpenShift may understate available capacity. Mitigate with field documentation and operational guidance requiring administrators to match the value to their Neutron configuration.


### Drawbacks

- Administrators must maintain the same limit in two places: Neutron configuration and the OpenShift `Infrastructure` resource. A mismatch can cause capacity to be misreported or port updates to fail.
- Since Neutron does not expose this setting through an API, OpenShift cannot validate the configured value against the actual cloud setting.

## Alternatives (Not Implemented)

- **Use a user-managed ConfigMap as the public configuration interface in the CNO.** Allow OCP admins to apply a simple config map containing the same `maxallowedAddressPairs` configuration, performing validation which would be a first of its kind.
- **Discover the Neutron setting automatically.** Neutron does not expose the configured value through an API, so automatic discovery is not available. Also, exposing the setting would be undesirable.

## Open Questions

None

## Test Plan

- **API validation tests:** Verify that omission is accepted without materializing a value, positive values are accepted, and zero and negative values are rejected. Verify update and removal behavior for the optional field. The API repository contains tests for these cases.
- **CNO integration tests:** Verify that CNO reads the value from the standalone cluster's OpenStack `Infrastructure` resource and passes it through the self-hosted controller configuration.
- **Controller tests:** Retain the controller unit tests for custom values, default behavior, existing address-pair accounting, and a configured limit below existing port usage.
- **End-to-end testing:** On OpenStack, verify that a value above 10 allows the controller to calculate Egress IP capacity using the configured limit, and verify that clusters omitting the field retain the current default behavior. Include a test where the configured value matches the cloud's Neutron limit.
- Test the standalone OpenStack path in CI/QE. Hosted-cluster testing is out of scope and belongs to a future HyperShift integration enhancement.

## Graduation Criteria

### Dev Preview -> Tech Preview

Not Applicable

### Tech Preview -> GA

Not Applicable

### Removing a deprecated feature

No deprecated feature is removed by this enhancement.

## Upgrade / Downgrade Strategy

When the field is omitted, clusters retain existing behavior and use the controller default of 10. No user action is required for those clusters.

A downgrade to a version that does not recognize or consume the field cannot be relied upon to
preserve a configured value greater than 10. Such a downgrade may return the cluster to the existing
default behavior. Document this limitation before enabling the feature on upgradeable clusters; do
not claim downgrade support for a non-default configured limit until verified.

## Version Skew Strategy

The API value is consumed by CNO and passed to the cloud-network-config-controller. During
preview, the no-upgrade feature-gate policy avoids supported upgrades while the feature is in use.
Before graduation to an upgradeable feature, test the supported CNO/controller version skew to
ensure that a configured value is consistently propagated and that versions without support retain
the documented default behavior when the field is absent.

## Operational Aspects of API Extensions

This enhancement adds one optional field to an existing API object. Schema validation is performed by the API and rejects explicitly configured values below 1. The field does not increase API object counts or add a per-request cloud call.

The operational effect is limited to the existing CNO/cloud-network-config-controller configuration flow. Changing the field causes CNO to propagate the updated value according to its normal reconciliation behavior. Controller availability and Neutron API availability remain governed by the existing components; no new availability dependency is introduced for unrelated API requests.

A positive value is not proof that the same value is configured in Neutron. If the values differ, the controller may overestimate or underestimate capacity, and Neutron may reject operations that exceed its actual limit. Existing controller errors and status/logging are the primary signals for these failures; this enhancement does not add new metrics or alerts.

### Failure Modes

- **Invalid API value:** The API rejects zero or negative values. The `Infrastructure` resource is not updated, so the last valid configuration remains in effect.
- **Value differs from Neutron:** The API accepts a positive integer but cannot compare it with Neutron. The controller continues to use the declared value; later port operations can fail if the declared value exceeds the cloud limit.
- **Controller cannot apply or use the value:** The existing CNO and cloud-network-config-controller health and error reporting apply. Cluster network administration should inspect the relevant operator status and controller logs.
- **Configured value below existing port usage:** The controller reports the capacity calculation error according to the behavior in PR #230. Existing address pairs are not removed by this enhancement.

The API extension itself has no webhook failure mode. If API schema validation is unavailable, API request availability follows the existing `Infrastructure` API serving path; this change adds no separate runtime service.

## Support Procedures

To inspect the configured value and verify whether it is omitted or explicitly set:

```bash
oc get infrastructure cluster -o yaml
```

Confirm that `spec.platformSpec.type` is `OpenStack` and compare `spec.platformSpec.openstack.maxAllowedAddressPairs` with the OpenStack administrator's Neutron configuration. If the field is absent, OpenShift retains the existing default of 10.

For controller errors or capacity that differs from expectation:

1. Check the network operator and cloud-network-config-controller health using the cluster's normal operator and pod diagnostics.
2. Inspect the cloud-network-config-controller logs in `openshift-cloud-network-config-controller` for capacity calculation or Neutron port errors.
3. Compare the configured field with Neutron's actual `max_allowed_address_pairs` setting and with address pairs already present on the affected port.
4. Correct the `Infrastructure` value or the Neutron setting so that they align. Lowering the configured value below existing port usage does not remove address pairs; use the existing controller error to identify the affected port and resolve the mismatch through the normal OpenStack administration process.

Disabling the feature gate or removing the field is not a mitigation for a cloud that requires a value greater than 10: the controller would return to its existing default behavior. Do not remove a non-default setting until the cluster and cloud can safely operate with the default.

## Infrastructure Needed

None
