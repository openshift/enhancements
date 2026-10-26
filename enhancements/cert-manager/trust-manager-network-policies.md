---
title: trust-manager-network-policies
authors:
  - "@arun717"
reviewers:
  - "@mytreya" ## reviewer for cert-manager component
  - "@Bharath B" ## reviewer for cert-manager component
approvers:
  - "@mytreya" ## approver for cert-manager component
  - "@Bharath B" ## approver for cert-manager component
api-approvers:
  - "@mytreya" ## approver for cert-manager component
  - "@Bharath B" ## approver for cert-manager component
creation-date: 2026-10-19
last-updated: 2026-10-23
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/CM-962
see-also:
  - "/enhancements/cert-manager/cert-manager-network-policies.md"
replaces:
  - NA
superseded-by:
  - NA
---

# Network Policies for the trust-manager Operand

## Summary

This enhancement adds operator-managed `NetworkPolicy` objects for the trust-manager operand. Trust-manager runs in the `cert-manager` namespace. The policies select only trust-manager pods, so other workloads in that namespace stay unaffected. They are created whenever a `TrustManager` custom resource exists and `spec.networkPolicy` is unset or `Enabled`. They do not depend on `CertManager.spec.defaultNetworkPolicy`. The rules default-deny the pod and allow only Kubernetes API egress, validating-webhook ingress, and user-workload-monitoring metrics ingress. Setting `spec.networkPolicy` to `Disabled` deletes those four policies and stops the controller from recreating them.

## Motivation

[cert-manager network policies](cert-manager-network-policies.md) secure the cert-manager controller, webhook, cainjector, and istio-csr. Those policies do not select trust-manager. Cert-manager operand policies match `app.kubernetes.io/instance: cert-manager` or `app: cert-manager` / `app: webhook`. Trust-manager pods are labeled `app: cert-manager-trust-manager` and `app.kubernetes.io/instance: cert-manager-trust-manager`.

Without its own policies, a trust-manager pod has unrestricted ingress and egress. Trust-manager distributes CA bundles inside the cluster. It does not call external issuers, so the policies are on unless an administrator turns them off. `TrustManager.spec.networkPolicy` is that switch. Unset and `Enabled` reconcile the four policies. `Disabled` removes them. The field does not accept extra egress rules. While the policies stay `Enabled`, an additional allow is a separate `NetworkPolicy` selecting the same pods. Istio-csr has no equivalent switch; its four policies stay for the life of the `IstioCSR` object.

### User Stories

- As a cluster administrator, I want trust-manager limited to the traffic it needs, so a compromised trust-manager pod cannot reach unrelated workloads or external networks.
- As a security engineer, I want a default-deny policy that selects only trust-manager pods, so other pods in the `cert-manager` namespace keep their current network behavior.
- As an SRE, I want user-workload monitoring to be able to scrape trust-manager metrics after the default-deny policy is applied.
- As a trust-manager user, I want Bundle admission and reconcile to keep working after the policies are applied, without setting `CertManager.spec.defaultNetworkPolicy`.
- As a cluster administrator, I want to set `TrustManager.spec.networkPolicy` to `Disabled` when my own policies replace this set, so the operator deletes its four policies and does not put them back.



### Goals

- Create four `NetworkPolicy` objects in the `cert-manager` namespace when a `TrustManager` resource named `cluster` exists and `spec.networkPolicy` is unset or `Enabled`.
- When `spec.networkPolicy` is `Disabled`, delete those four reserved names and do not recreate them. Leave every other `NetworkPolicy` in the namespace alone. Changing the field back to `Enabled`, or clearing it, creates the four policies again.
- Select only pods labeled `app: cert-manager-trust-manager`.
- Allow egress to the Kubernetes API server on TCP 6443.
- Allow ingress to the validating webhook on TCP 6443, with no source peer restriction, so hostNetwork `kube-apiserver` can call it.
- Allow ingress to metrics on TCP 9402 from `openshift-user-workload-monitoring` only.
- Reconcile drift on those objects back to the desired spec.
- Leave cert-manager and istio-csr policy behavior unchanged.



### Non-Goals

- `TrustManager.spec.networkPolicy` accepts only `Enabled` and `Disabled`. It does not accept custom peers, ports, or extra egress. There is no `defaultNetworkPolicy` or `networkPolicies` list on `TrustManager`.
- Trust-manager policies are not folded into `CertManager.spec.networkPolicies` or `ComponentName`. That API is cert-manager-only.
- These policies do not read or follow `CertManager.spec.defaultNetworkPolicy`. Enabling or disabling cert-manager operand policies has no effect on trust-manager policies.
- No namespace-wide deny-all (`podSelector: {}`) in `cert-manager`.
- No AdminNetworkPolicy.
- No user-defined extra egress through the `TrustManager` API. An extra allow, including a metrics scrape from a namespace other than `openshift-user-workload-monitoring`, is a separate `NetworkPolicy` while `spec.networkPolicy` stays `Enabled`.
- No operator-namespace or OLM policies. Those remain in the cert-manager network policy enhancement.
- No ACME or DNS-01 egress. That stays on the cert-manager controller.
- No DNS allow rule. Steady-state trust-manager does not issue DNS queries.
- No allow rule for the readiness port. Kubelet probes are not subject to `NetworkPolicy` on OVN-Kubernetes.
- Deleting the policies when the `TrustManager` resource is deleted is out of scope. IstioCSR leaves its policies in place today, and trust-manager matches that behavior.



## Proposal

Follow the istio-csr asset path from the cert-manager network policy enhancement, and add `TrustManager.spec.networkPolicy` so an administrator can turn the policies off. Istio-csr reconciles its four policies for as long as the `IstioCSR` object exists and has no field to stop that. Trust-manager defaults to the same reconcile behavior. `Disabled` is the difference.

When the Trust Manager controller sees the `TrustManager` resource and `spec.networkPolicy` is unset or `Enabled`, it applies four policies in the operand namespace `cert-manager`. The pod selector on every policy is `app: cert-manager-trust-manager`. Cert-manager policies use different labels, so the two sets do not select each other's pods even though they share the namespace. When the field is `Disabled`, the controller deletes the four reserved names and does not recreate them. A watch on `NetworkPolicy` repairs drift only while the field is `Enabled`.

Those four names in `cert-manager` are reserved for the operator: `trust-manager-deny-all`, `trust-manager-allow-egress-to-api-server`, `trust-manager-allow-ingress-to-webhook`, and `trust-manager-allow-ingress-to-metrics`. Each object carries `trustmanager.operator.openshift.io/owned-by: trust-manager`. Kubernetes identity is `(namespace, name)`, so an extra administrator `NetworkPolicy` must use a different name. Allows combine, which is how extra scrapes or egress coexist with the operator set. There is no user-policy name prefix: `TrustManager` has no `networkPolicies[]` field, unlike `CertManager`. While the field is `Enabled`, apply of those four names is intended; the operator takes over an existing object of the same name. `Disabled` deletes those four names only.

The allow rules match traffic on the GA trust-manager operand **v0.25.0**, which the operator ships (`RELATED_IMAGE_CERT_MANAGER_TRUST_MANAGER`). Webhook TCP 6443, metrics TCP 9402, readiness TCP 6060, and API egress enforced as TCP 6443 are the same ports as the v0.25.0 Deployment and Services. v0.25.0 did not add listen or dial ports beyond that set:

- The process dials `kubernetes.default.svc` (`172.30.0.1:443`). OVN delivers that to the API server endpoints on TCP 6443. The egress rule allows TCP 6443, the same port the shipped cert-manager and istio-csr API egress policies use. A rule on 443 does not match the enforced port.
- The validating webhook listens on container port 6443. Its Service exposes port 443 and targets 6443. The ingress rule allows TCP 6443. The caller is hostNetwork `kube-apiserver`, which appears on the pod network as the control-plane node gateway, so the rule has no `from` peer.
- Metrics listen in plaintext on TCP 9402 (`--metrics-port=9402`, `secure=false`). Ingress is limited to pods in `openshift-user-workload-monitoring`.
- Readiness listens on TCP 6060. No policy allows it. The same probe port is left closed on istio-csr.
- The default CA package is a file mounted from the in-cluster ConfigMap `trust-manager-default-ca-package` (`/packages/cert-manager-package-openshift.json`). No cloud, metadata, or internet egress was observed.



### Workflow Description

No administrator action turns the policies on. Installing the trust-manager operand is the trigger. Turning them off is an explicit `spec.networkPolicy: Disabled`.

1. An administrator enables the Trust Manager feature gate and creates `TrustManager` `cluster`. `spec.networkPolicy` is unset, which means `Enabled`.
2. The operator deploys the trust-manager operand in the `cert-manager` namespace.
3. The Trust Manager controller applies the four `NetworkPolicy` objects in that namespace. It does this whether or not `CertManager.spec.defaultNetworkPolicy` is set.
4. Bundle create and update requests are admitted through the webhook on TCP 6443. The controller reconciles Bundles through the API on TCP 6443.
5. If a user-workload-monitoring ServiceMonitor targets `trust-manager-metrics` port 9402, that scrape is allowed. The operator does not create that ServiceMonitor. A scrape from another namespace, while the field stays `Enabled`, is an additional `NetworkPolicy` selecting `app: cert-manager-trust-manager`. Allows combine, so that object does not require `Disabled`.
6. If someone edits one of the four policies while the field is `Enabled`, the controller writes the desired spec back.
7. If an administrator sets `spec.networkPolicy` to `Disabled`, the controller deletes the four reserved names (`trust-manager-deny-all`, `trust-manager-allow-egress-to-api-server`, `trust-manager-allow-ingress-to-webhook`, and `trust-manager-allow-ingress-to-metrics`). It does not delete other `NetworkPolicy` objects. It does not recreate the four names on later reconciles.
8. Setting the field back to `Enabled`, or clearing it, creates the four policies again.
9. Deleting the `TrustManager` resource does not delete the policies. If `Disabled` already removed them, they stay absent.



### API Extensions

One new field on the existing cluster-scoped `TrustManager` CRD. No new CRD, webhook, finalizer, or aggregated API server. The validating webhook trust-manager already serves is unchanged. The policies only restrict which peers can reach it, and only while this field is unset or `Enabled`.

`networkPolicy` sits on `TrustManagerSpec`, beside `controllerConfig`, because it controls objects the operator creates rather than the trust-manager process. Unset means `Enabled`. The field can change in either direction. It is not the cert-manager operand field: `CertManager.spec.defaultNetworkPolicy` defaults to off and cannot be changed from `"true"` back to `"false"`. This field defaults to on and can be set to `Disabled` after the policies exist.

```go
// networkPolicy controls the operator-managed NetworkPolicy objects for the
// trust-manager operand.
//
// This is not an opt-in. Policies are created whenever a TrustManager exists.
// Unset and "Enabled" both create and reconcile the four reserved names.
// "Disabled" is the only way to delete them and stop recreation.
// Extra administrator allows do not use this field; they are separate
// NetworkPolicy objects with other names.
//
// The field can change from Enabled to Disabled and back.
// CertManager.spec.defaultNetworkPolicy has no effect on this field.
//
// +kubebuilder:default:=Enabled
// +kubebuilder:validation:Enum:=Enabled;Disabled
// +kubebuilder:validation:Optional
// +optional
NetworkPolicy TrustManagerNetworkPolicy `json:"networkPolicy,omitempty"`

// TrustManagerNetworkPolicy is the management state of the operator-owned
// trust-manager NetworkPolicy objects.
// +kubebuilder:validation:Enum:=Enabled;Disabled
type TrustManagerNetworkPolicy string

const (
	// TrustManagerNetworkPolicyEnabled creates and reconciles the four policies.
	TrustManagerNetworkPolicyEnabled TrustManagerNetworkPolicy = "Enabled"
	// TrustManagerNetworkPolicyDisabled deletes the four policies and does not recreate them.
	TrustManagerNetworkPolicyDisabled TrustManagerNetworkPolicy = "Disabled"
)
```

### Topology Considerations



#### Hypershift / Hosted Control Planes

Trust-manager and its policies run in the hosted cluster, in the hosted `cert-manager` namespace. Egress targets the in-cluster API server on TCP 6443. Nothing in this enhancement runs in the management cluster.

#### Standalone Clusters

On standalone clusters the policies work as written. They select trust-manager pods in the `cert-manager` namespace and leave every other pod in that namespace alone.

#### Single-node Deployments or MicroShift

The policies are four small `NetworkPolicy` objects. They do not add containers or raise the trust-manager resource requests. On a single-node cluster the same pod selector and ports apply.

MicroShift does not ship this operator. If trust-manager is not installed, this enhancement has no effect. No MicroShift config file knob is added.

#### OpenShift Kubernetes Engine

The cert-manager operator is an optional operator. These policies use the stable Kubernetes `NetworkPolicy` API and do not depend on features excluded from OpenShift Kubernetes Engine. On a cluster where the operator is installed, behavior matches OpenShift Container Platform.

### Implementation Details/Notes/Constraints

Implement the policies as static assets, on the same path as `bindata/networkpolicies/istio-csr-*.yaml` and `istioCSRNetworkPolicyAssets`. Reconcile them from the Trust Manager controller when `spec.networkPolicy` is unset or `Enabled`. Watch `NetworkPolicy` so drift is repaired in that state. When the field is `Disabled`, delete the four reserved names and ignore later changes to those names. Unit tests cover create, spec equality, delete on `Disabled`, and recreate when the field returns to `Enabled`.

All four objects live in namespace `cert-manager`, use the reserved names below, and carry `trustmanager.operator.openshift.io/owned-by: trust-manager` on metadata (that label is in the static assets). Apply also merges the same default operand labels used on Deployment and Service: `app`, `app.kubernetes.io/name`, `app.kubernetes.io/instance`, `app.kubernetes.io/managed-by`, `app.kubernetes.io/part-of`, and `app.kubernetes.io/version` from the operand image env. Those labels are not repeated in the examples below so the snippets do not go stale when the image version changes. Extra keys from `spec.controllerConfig.labels` are merged. Operator-owned keys take precedence: a user value for `trustmanager.operator.openshift.io/owned-by` or for `app` / `app.kubernetes.io/*` is overwritten. Isolation is still `podSelector`; apply is not skipped when a user label conflicts.

Metadata labels are for ownership and watches. They are not the isolation rule. Isolation is only `spec.podSelector`, which stays:

```yaml
podSelector:
  matchLabels:
    app: cert-manager-trust-manager
```

Do not add the `app.kubernetes.io/*` set to `podSelector`. The Deployment already selects pods with `app: cert-manager-trust-manager`; the policies use that same match.

1. **Deny all trust-manager traffic**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: trust-manager-deny-all
      namespace: cert-manager
      labels:
        trustmanager.operator.openshift.io/owned-by: trust-manager
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-trust-manager
      policyTypes:
      - Ingress
      - Egress
    ```

2. **Allow egress to the API server**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: trust-manager-allow-egress-to-api-server
      namespace: cert-manager
      labels:
        trustmanager.operator.openshift.io/owned-by: trust-manager
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-trust-manager
      policyTypes:
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443
    ```

3. **Allow ingress to the validating webhook**

    The webhook Service port is 443. The container port, and the policy port, is 6443. There is no `from` selector.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: trust-manager-allow-ingress-to-webhook
      namespace: cert-manager
      labels:
        trustmanager.operator.openshift.io/owned-by: trust-manager
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-trust-manager
      policyTypes:
      - Ingress
      ingress:
      - ports:
        - protocol: TCP
          port: 6443
    ```

4. **Allow ingress to metrics from user-workload monitoring**

    Metrics are plaintext HTTP. The namespace selector uses `kubernetes.io/metadata.name`, matching the shipped istio-csr metrics policy.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: trust-manager-allow-ingress-to-metrics
      namespace: cert-manager
      labels:
        trustmanager.operator.openshift.io/owned-by: trust-manager
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-trust-manager
      policyTypes:
      - Ingress
      ingress:
      - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: openshift-user-workload-monitoring
        ports:
        - protocol: TCP
          port: 9402
    ```



### Risks and Mitigations

- **Risk:** The policies block traffic trust-manager needs, so Bundle admission or reconcile fails.
  - **Mitigation:** The allow list is the traffic observed on a running operand: API egress and webhook ingress. Readiness probes stay available because OVN-Kubernetes does not apply `NetworkPolicy` to kubelet probes. The test plan requires a Bundle create after the policies exist.
- **Risk:** `trust-manager-allow-egress-to-api-server` allows TCP 6443 to any destination. The rule sets the port and has no `to` peer, so a trust-manager pod can open TCP 6443 to addresses other than the API server.
  - **Mitigation:** API server addresses are cluster-specific and are typically the host-network addresses of the control-plane nodes. A static manifest cannot select those peers with a pod or namespace selector, and an `ipBlock` would have to be rewritten for each cluster. The shipped istio-csr policy `istio-csr-allow-egress-to-api-server` makes the same tradeoff: egress TCP 6443 with no destination peer. Every other port stays closed by `trust-manager-deny-all`.
- **Risk:** A metrics scrape from outside `openshift-user-workload-monitoring` starts failing.
  - **Mitigation:** The operator does not ship a trust-manager ServiceMonitor, so platform monitoring is not scraping this port today. A Prometheus in another namespace does not match the metrics rule. While `spec.networkPolicy` stays `Enabled`, an additional `NetworkPolicy` selecting `app: cert-manager-trust-manager` can allow that namespace. Setting the field to `Disabled` removes the operator rules instead, and the administrator's own policies then have to allow the scrape.
- **Risk:** An upgrade applies the policies to every existing `TrustManager`, because the new field defaults to `Enabled`.
  - **Mitigation:** Supported trust-manager traffic is in the allow list. An administrator who already has a stricter policy set sets `spec.networkPolicy` to `Disabled`. The controller then deletes the four policies. Trust-manager has no ACME or other external egress, which is why the default is `Enabled` rather than the cert-manager opt-in.
- **Risk:** A namespace-wide deny would cut off cert-manager, cainjector, or webhook pods that share the namespace.
  - **Mitigation:** Every policy uses `app: cert-manager-trust-manager` only.

Security review is the cert-manager component reviewers on this enhancement. `spec.networkPolicy` is a new user-facing field and needs API review. It does not take custom rules, so the only new choice is whether the operator set is present.

### Drawbacks

`spec.networkPolicy` does not accept extra egress or a different metrics namespace. While the field stays `Enabled`, a scrape from a namespace other than `openshift-user-workload-monitoring` is still an additional `NetworkPolicy`. Allows combine, so that case does not require `Disabled`.

`Disabled` removes all four operator rules, including API egress and webhook ingress. Bundle admission and reconcile then depend on policies the administrator supplies. Istio-csr does not offer this switch. Trust-manager does, because these policies are new and an administrator may already have a stricter set that cannot coexist with the operator allows. A second `NetworkPolicy` can add allows. It cannot remove the operator's webhook allow, which has no source restriction, or the API egress allow for TCP 6443 to any destination. Editing those objects is reverted while the field is `Enabled`.

The policies are not removed when the operand is removed, so a later uninstall leaves four unused objects in the `cert-manager` namespace unless the field was `Disabled` first.

## Alternatives (Not Implemented)

- **Opt-in through `CertManager.spec.defaultNetworkPolicy`.** Rejected. That field gates cert-manager operand policies only, and it exists so existing cert-manager installs keep external-issuer egress. Once it is `"true"` it cannot be set back to `"false"`. Trust-manager has no such egress. Tying the two together would leave trust-manager unrestricted whenever an administrator left cert-manager policies off, and would make the trust-manager policies impossible to turn off later.
- **Opt-in `TrustManager.spec.defaultNetworkPolicy`, default off, locked once `"true"`.** Rejected. That copies the cert-manager operand shape. An opt-in field would ship trust-manager unrestricted until an administrator sets it. The accepted field defaults to `Enabled` and can be set to `Disabled` after the policies exist.
- **Always on, with no `TrustManager` field, matching istio-csr.** Rejected after review. Istio-csr reconciles its four policies for the life of the `IstioCSR` object. Deleting or editing them does not stick, and deleting the `IstioCSR` object leaves the policies in place. The same behavior would leave a trust-manager administrator who already has a stricter policy set with no way to stop the controller from putting the four objects back.
- **Reuse the cert-manager policy objects by widening their pod selectors.** Rejected. Those objects allow DNS and webhook port 10250 for different pods. Widening the selector would apply the wrong ports to trust-manager, or trust-manager's ports to cert-manager.
- **Namespace-wide deny-all in `cert-manager`.** Rejected in the cert-manager network policy enhancement. A `podSelector: {}` policy would isolate every pod in the namespace, including cert-manager, webhook, and cainjector.
- **Allow the Service ports (API 443 and webhook Service 443) instead of 6443.** Rejected. Shipped operand policies allow the endpoint port 6443. The trust-manager socket table shows the process dialing 443, and the webhook Service fronting 443, but enforcement matches 6443.
- **Allow TCP 6060 for readiness.** Rejected. IstioCSR does not allow its probe port. OVN-Kubernetes does not enforce `NetworkPolicy` on kubelet probes.
- **Allow DNS to `openshift-dns` on TCP/UDP 5353.** Rejected for this operand. The pod's resolver is configured, and a live pod opened no DNS sockets. API access uses the ClusterIP from the environment. The default CA package is a local file.



## Test Plan

- **Unit tests:** Creating `TrustManager` `cluster` with `spec.networkPolicy` unset or `Enabled` creates the four reserved policies in `cert-manager` with the selector, ports, and `owned-by` label above. A mutated policy is reconciled back to the desired spec. `CertManager.spec.defaultNetworkPolicy` is not consulted. Setting `spec.networkPolicy` to `Disabled` deletes the four reserved names and a later reconcile does not recreate them. Another `NetworkPolicy` in the namespace is not deleted. Setting the field back to `Enabled` creates the four policies again. A `spec.controllerConfig.labels` value for `trustmanager.operator.openshift.io/owned-by` or `app` does not replace the operator-owned labels.
- **Cluster test, admission and reconcile:** With the policies applied, create a `Bundle` and confirm the validating webhook admits it and the target ConfigMap or Secret is written. With `spec.networkPolicy: Disabled`, confirm the four policies are gone.
- **Cluster test, metrics:** The operator does not ship a trust-manager ServiceMonitor. Create one the way the cert-manager e2e tests do (`test/e2e/testdata/observe/servicemonitor.yaml` in cert-manager-operator), aimed at Service `trust-manager-metrics` port 9402 with scheme HTTP. Confirm user-workload monitoring can scrape it, and confirm a client outside that namespace cannot.
- **Negative check:** A pod in `cert-manager` that is not trust-manager is not selected by these policies. Cert-manager operand connectivity does not change when trust-manager policies exist and `defaultNetworkPolicy` is unset.



## Graduation Criteria

Trust-manager is a tech-preview operand today. These policies ship with that operand. They are not a separate feature gate, and they are not gated by `CertManager.spec.defaultNetworkPolicy`. Unset `spec.networkPolicy` means `Enabled`.

- The four policies are implemented and applied whenever `TrustManager` `cluster` exists and `spec.networkPolicy` is unset or `Enabled`.
- `spec.networkPolicy: Disabled` deletes the four policies and keeps them deleted until the field is `Enabled` again.
- Unit tests for apply, spec equality, and the `Disabled` path are passing.
- The Bundle and metrics cluster checks in the Test Plan are passing.
- Documentation states that trust-manager network policies are on by default, select only trust-manager pods, do not follow `CertManager.spec.defaultNetworkPolicy`, and are removed by setting `TrustManager.spec.networkPolicy` to `Disabled`.



### Dev Preview -> Tech Preview

Not a separate graduation. The policies are part of the existing tech-preview Trust Manager operand.

### Tech Preview -> GA

Trust-manager's own graduation owns the GA decision. Before that graduation, the cluster checks in the Test Plan must be passing with the policies applied and with `spec.networkPolicy: Disabled`. User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) must state that the policies are on by default and how to disable them.

### Removing a deprecated feature

Not applicable. This enhancement does not remove a feature.

## Upgrade / Downgrade Strategy

- **Upgrade:** On upgrade to an operator that includes this enhancement, every existing `TrustManager` resource gains the four policies on the next reconcile, because `spec.networkPolicy` is unset and unset means `Enabled`. No CR edit is required to keep the policies. Supported traffic (API, webhook, in-cluster Bundle reconcile, user-workload-monitoring scrapes) keeps working. An administrator who needs the operator set gone sets `spec.networkPolicy` to `Disabled`. The controller deletes the four policies. Scrapes from any other namespace keep working only if that administrator adds an allow while the field stays `Enabled`, or covers that traffic in their own policies after `Disabled`.
- **Downgrade:** An older operator ignores `spec.networkPolicy` and does not delete the policies. They remain in the `cert-manager` namespace and keep enforcing until an administrator deletes them. A `TrustManager` that was `Disabled` before downgrade has no policies for the older operator to recreate, because the older operator does not manage them.

Micro upgrades within a minor stream and the next minor upgrade do not require an intermediate step beyond the operator's normal upgrade. The policies are additive objects. Applying them does not roll the trust-manager pod.

## Version Skew Strategy

During an upgrade, a new operator can create the policies while an older trust-manager pod is still running. The allowed ports are the ports that pod already listens on, so the older pod keeps API and webhook connectivity. A new trust-manager pod does not require a new kubelet or CNI. No node component changes.

## Operational Aspects of API Extensions

Not applicable beyond the `networkPolicy` field above. This enhancement adds no new CRD, webhook, or finalizer.

The existing trust-manager validating webhook is unchanged. If the webhook pod cannot be reached on TCP 6443, `Bundle` create and update fail admission. That is existing webhook behavior. These policies exist so the apiserver path to that port stays open while every other ingress stays closed.

## Support Procedures

1. Confirm the operand is installed: `oc get trustmanager cluster` and `oc get deploy -n cert-manager trust-manager`.
2. Confirm `CertManager.spec.defaultNetworkPolicy` is irrelevant: trust-manager policies follow `TrustManager.spec.networkPolicy`, not that field. They can exist while `CertManager.spec.defaultNetworkPolicy` is empty or `"false"`.
3. Read `TrustManager.spec.networkPolicy`. Unset or `Enabled` means the four policies should exist. `Disabled` means they should be absent.
4. List the policies: `oc get networkpolicy -n cert-manager`. When the field is unset or `Enabled`, expect the four reserved names `trust-manager-deny-all`, `trust-manager-allow-egress-to-api-server`, `trust-manager-allow-ingress-to-webhook`, and `trust-manager-allow-ingress-to-metrics`. Each must select `app: cert-manager-trust-manager` and carry `trustmanager.operator.openshift.io/owned-by: trust-manager`.
5. If Bundle admission fails, check `kube-apiserver` logs for errors calling `trust.cert-manager.io`. Confirm the webhook policy allows TCP 6443 and not only Service port 443. If `spec.networkPolicy` is `Disabled`, the operator is not keeping that path open.
6. If reconcile fails with API timeouts, confirm the egress policy allows TCP 6443, or that `spec.networkPolicy` is not `Disabled` without a replacement allow.
7. If metrics are missing, confirm the ServiceMonitor uses scheme HTTP against port 9402 and runs in `openshift-user-workload-monitoring`. A scraper in any other namespace is denied while `spec.networkPolicy` is `Enabled`. Allow it with a separate `NetworkPolicy`, or set the field to `Disabled` and apply the administrator's own rules.
8. If a policy was edited by hand while `spec.networkPolicy` is `Enabled`, wait for the controller to restore it. That restore is expected.
9. To turn the policies off, set `TrustManager.spec.networkPolicy` to `Disabled`. The controller deletes the four reserved names and does not recreate them. Deleting the objects by hand while the field is `Enabled` does not stick. Deleting the `TrustManager` resource does not remove the policies. Extra administrator policies must not reuse those names.

