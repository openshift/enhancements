---
title: trust-manager-network-policies
authors:
  - "@arun717"
reviewers:
  - "@mytreya" ## reviewer for cert-manager component
approvers:
  - "@mytreya" ## approver for cert-manager component
api-approvers:
  - "None"
creation-date: 2026-10-19
last-updated: 2026-10-19
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

This enhancement adds operator-managed `NetworkPolicy` objects for the trust-manager operand. Trust-manager runs in the `cert-manager` namespace. The policies select only trust-manager pods, so other workloads in that namespace stay unaffected. They are created whenever a `TrustManager` custom resource exists and do not depend on `CertManager.spec.defaultNetworkPolicy`. The rules default-deny the pod and allow only Kubernetes API egress, validating-webhook ingress, and user-workload-monitoring metrics ingress.

## Motivation

[cert-manager network policies](cert-manager-network-policies.md) secure the cert-manager controller, webhook, cainjector, and istio-csr. Those policies do not select trust-manager. Cert-manager operand policies match `app.kubernetes.io/instance: cert-manager` or `app: cert-manager` / `app: webhook`. Trust-manager pods are labeled `app: cert-manager-trust-manager` and `app.kubernetes.io/instance: cert-manager-trust-manager`.

Without its own policies, a trust-manager pod has unrestricted ingress and egress. Trust-manager distributes CA bundles inside the cluster. It does not call external issuers, so it can take the istio-csr model: always-on policies, no user-configurable egress.

### User Stories

- As a cluster administrator, I want trust-manager limited to the traffic it needs, so a compromised trust-manager pod cannot reach unrelated workloads or external networks.
- As a security engineer, I want a default-deny policy that selects only trust-manager pods, so other pods in the `cert-manager` namespace keep their current network behavior.
- As an SRE, I want user-workload monitoring to be able to scrape trust-manager metrics after the default-deny policy is applied.
- As a trust-manager user, I want Bundle admission and reconcile to keep working after the policies are applied, without setting `CertManager.spec.defaultNetworkPolicy`.



### Goals

- Create four `NetworkPolicy` objects in the `cert-manager` namespace when a `TrustManager` resource named `cluster` exists.
- Select only pods labeled `app: cert-manager-trust-manager`.
- Allow egress to the Kubernetes API server on TCP 6443.
- Allow ingress to the validating webhook on TCP 6443, with no source peer restriction, so hostNetwork `kube-apiserver` can call it.
- Allow ingress to metrics on TCP 9402 from `openshift-user-workload-monitoring` only.
- Reconcile drift on those objects back to the desired spec.
- Leave cert-manager and istio-csr policy behavior unchanged.



### Non-Goals

- No new `TrustManager` API fields. There is no `defaultNetworkPolicy` or `networkPolicies` field on `TrustManager`.
- Trust-manager policies are not folded into `CertManager.spec.networkPolicies` or `ComponentName`. That API is cert-manager-only.
- These policies do not read or follow `CertManager.spec.defaultNetworkPolicy`. Enabling or disabling cert-manager operand policies has no effect on trust-manager policies.
- No namespace-wide deny-all (`podSelector: {}`) in `cert-manager`.
- No AdminNetworkPolicy.
- No user-defined extra egress.
- No operator-namespace or OLM policies. Those remain in the cert-manager network policy enhancement.
- No ACME or DNS-01 egress. That stays on the cert-manager controller.
- No DNS allow rule. Steady-state trust-manager does not issue DNS queries.
- No allow rule for the readiness port. Kubelet probes are not subject to `NetworkPolicy` on OVN-Kubernetes.
- Deleting the policies when the `TrustManager` resource is deleted is out of scope. IstioCSR leaves its policies in place today, and trust-manager matches that behavior.



## Proposal

Follow the istio-csr path from the cert-manager network policy enhancement: static manifest assets, always reconciled, no user-facing network policy API.

When the Trust Manager controller sees the `TrustManager` resource, it applies four policies in the operand namespace `cert-manager`. The pod selector on every policy is `app: cert-manager-trust-manager`. Cert-manager policies use different labels, so the two sets do not select each other's pods even though they share the namespace.

The allow rules match traffic observed on a live trust-manager 0.20.3 pod:

- The process dials `kubernetes.default.svc` (`172.30.0.1:443`). OVN delivers that to the API server endpoints on TCP 6443. The egress rule allows TCP 6443, the same port the shipped cert-manager and istio-csr API egress policies use. A rule on 443 does not match the enforced port.
- The validating webhook listens on container port 6443. Its Service exposes port 443 and targets 6443. The ingress rule allows TCP 6443. The caller is hostNetwork `kube-apiserver`, which appears on the pod network as the control-plane node gateway, so the rule has no `from` peer.
- Metrics listen in plaintext on TCP 9402 (`--metrics-port=9402`, `secure=false`). Ingress is limited to pods in `openshift-user-workload-monitoring`.
- Readiness listens on TCP 6060. No policy allows it. The same probe port is left closed on istio-csr.
- The default CA package is a file mounted from the in-cluster ConfigMap `trust-manager-default-ca-package` (`/packages/cert-manager-package-openshift.json`). No cloud, metadata, or internet egress was observed.



### Workflow Description

No administrator action turns the policies on. Installing the trust-manager operand is the trigger.

1. An administrator enables the Trust Manager feature gate and creates `TrustManager` `cluster`.
2. The operator deploys the trust-manager operand in the `cert-manager` namespace.
3. The Trust Manager controller applies the four `NetworkPolicy` objects in that namespace. It does this whether or not `CertManager.spec.defaultNetworkPolicy` is set.
4. Bundle create and update requests are admitted through the webhook on TCP 6443. The controller reconciles Bundles through the API on TCP 6443.
5. If a user-workload-monitoring ServiceMonitor targets `trust-manager-metrics` port 9402, that scrape is allowed. The operator does not create that ServiceMonitor.
6. If someone edits one of the four policies, the controller writes the desired spec back.
7. Deleting the `TrustManager` resource does not delete the policies.



### API Extensions

None. This enhancement does not add or change CRDs, webhooks, finalizers, or aggregated API servers. The validating webhook trust-manager already serves is unchanged. The policies only restrict which peers can reach it.

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

Implement the policies as static assets, on the same path as `bindata/networkpolicies/istio-csr-*.yaml` and `istioCSRNetworkPolicyAssets`. Reconcile them from the Trust Manager controller. Watch `NetworkPolicy` so drift is repaired. Unit tests cover create and spec equality.

All four objects live in namespace `cert-manager` and use this selector:

```yaml
podSelector:
  matchLabels:
    app: cert-manager-trust-manager
```

1. **Deny all trust-manager traffic**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: trust-manager-deny-all
      namespace: cert-manager
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
- **Risk:** A metrics scrape from outside `openshift-user-workload-monitoring` starts failing.
  - **Mitigation:** The operator does not ship a trust-manager ServiceMonitor, so platform monitoring is not scraping this port today. A Prometheus the administrator installed in another namespace will not match the metrics rule. That matches istio-csr. Document the allowed source namespace.
- **Risk:** An upgrade applies the policies to every existing `TrustManager` with no field to turn them off.
  - **Mitigation:** Supported trust-manager traffic is in the allow list. Trust-manager has no ACME or other external egress, which is why cert-manager policies stayed opt-in and these do not. Customers who added their own egress around the pod, or who scrape metrics from a namespace other than `openshift-user-workload-monitoring`, will lose that traffic.
- **Risk:** A namespace-wide deny would cut off cert-manager, cainjector, or webhook pods that share the namespace.
  - **Mitigation:** Every policy uses `app: cert-manager-trust-manager` only.

Security review is the cert-manager component reviewers on this enhancement. There is no new user-facing API for UX review.

### Drawbacks

Operators cannot disable the policies or add extra egress through the `TrustManager` API. A cluster that needs a scrape from a namespace other than `openshift-user-workload-monitoring` has to add its own `NetworkPolicy`. That is the same trade-off already accepted for istio-csr.

The policies are not removed when the operand is removed, so a later uninstall leaves four unused objects in the `cert-manager` namespace.

## Alternatives (Not Implemented)

- **Opt-in through `CertManager.spec.defaultNetworkPolicy`.** Rejected. That field gates cert-manager operand policies only, and it exists so existing cert-manager installs keep external-issuer egress. Trust-manager has no such egress. Tying the two together would leave trust-manager unrestricted whenever an administrator left cert-manager policies off.
- **New `TrustManager.spec.defaultNetworkPolicy` or `networkPolicies` fields.** Rejected. IstioCSR does not expose them. An opt-in field would ship trust-manager unrestricted by default.
- **Reuse the cert-manager policy objects by widening their pod selectors.** Rejected. Those objects allow DNS and webhook port 10250 for different pods. Widening the selector would apply the wrong ports to trust-manager, or trust-manager's ports to cert-manager.
- **Namespace-wide deny-all in `cert-manager`.** Rejected in the cert-manager network policy enhancement. A `podSelector: {}` policy would isolate every pod in the namespace, including cert-manager, webhook, and cainjector.
- **Allow the Service ports (API 443 and webhook Service 443) instead of 6443.** Rejected. Shipped operand policies allow the endpoint port 6443. The trust-manager socket table shows the process dialing 443, and the webhook Service fronting 443, but enforcement matches 6443.
- **Allow TCP 6060 for readiness.** Rejected. IstioCSR does not allow its probe port. OVN-Kubernetes does not enforce `NetworkPolicy` on kubelet probes.
- **Allow DNS to `openshift-dns` on TCP/UDP 5353.** Rejected for this operand. The pod's resolver is configured, and a live pod opened no DNS sockets. API access uses the ClusterIP from the environment. The default CA package is a local file.



## Test Plan

- **Unit tests:** Creating `TrustManager` `cluster` creates the four policies in `cert-manager` with the selector and ports above. A mutated policy is reconciled back to the desired spec. `CertManager.spec.defaultNetworkPolicy` is not consulted.
- **Cluster test, admission and reconcile:** With the policies applied, create a `Bundle` and confirm the validating webhook admits it and the target ConfigMap or Secret is written.
- **Cluster test, metrics:** The operator does not ship a trust-manager ServiceMonitor. Create one the way the cert-manager e2e tests do (`test/e2e/testdata/observe/servicemonitor.yaml` in cert-manager-operator), aimed at Service `trust-manager-metrics` port 9402 with scheme HTTP. Confirm user-workload monitoring can scrape it, and confirm a client outside that namespace cannot.
- **Negative check:** A pod in `cert-manager` that is not trust-manager is not selected by these policies. Cert-manager operand connectivity does not change when trust-manager policies exist and `defaultNetworkPolicy` is unset.



## Graduation Criteria

Trust-manager is a tech-preview operand today. These policies ship with that operand. They are not a separate feature gate, and they are not gated by `CertManager.spec.defaultNetworkPolicy`.

- The four policies are implemented and applied whenever `TrustManager` `cluster` exists.
- Unit tests for apply and spec equality are passing.
- The Bundle and metrics cluster checks in the Test Plan are passing.
- Documentation states that trust-manager network policies are automatic, select only trust-manager pods, and do not follow `CertManager.spec.defaultNetworkPolicy`.



### Dev Preview -> Tech Preview

Not a separate graduation. The policies are part of the existing tech-preview Trust Manager operand.

### Tech Preview -> GA

Trust-manager's own graduation owns the GA decision. Before that graduation, the cluster checks in the Test Plan must be passing with the policies applied. User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) must state that the policies are automatic.

### Removing a deprecated feature

Not applicable. This enhancement does not remove a feature.

## Upgrade / Downgrade Strategy

- **Upgrade:** On upgrade to an operator that includes this enhancement, every existing `TrustManager` resource gains the four policies on the next reconcile. No CR edit is required. Supported traffic (API, webhook, in-cluster Bundle reconcile, user-workload-monitoring scrapes) keeps working. Scrapes from any other namespace, and any extra egress an administrator attached to the pod, stop working. There is no field to opt out.
- **Downgrade:** An older operator ignores the policies and does not delete them. They remain in the `cert-manager` namespace and keep enforcing until an administrator deletes them.

Micro upgrades within a minor stream and the next minor upgrade do not require an intermediate step beyond the operator's normal upgrade. The policies are additive objects. Applying them does not roll the trust-manager pod.

## Version Skew Strategy

During an upgrade, a new operator can create the policies while an older trust-manager pod is still running. The allowed ports are the ports that pod already listens on, so the older pod keeps API and webhook connectivity. A new trust-manager pod does not require a new kubelet or CNI. No node component changes.

## Operational Aspects of API Extensions

Not applicable. This enhancement adds no API extensions.

The existing trust-manager validating webhook is unchanged. If the webhook pod cannot be reached on TCP 6443, `Bundle` create and update fail admission. That is existing webhook behavior. These policies exist so the apiserver path to that port stays open while every other ingress stays closed.

## Support Procedures

1. Confirm the operand is installed: `oc get trustmanager cluster` and `oc get deploy -n cert-manager trust-manager`.
2. Confirm `CertManager.spec.defaultNetworkPolicy` is irrelevant: trust-manager policies can exist while that field is empty or `"false"`.
3. List the policies: `oc get networkpolicy -n cert-manager`. Expect `trust-manager-deny-all`, `trust-manager-allow-egress-to-api-server`, `trust-manager-allow-ingress-to-webhook`, and `trust-manager-allow-ingress-to-metrics`. Each must select `app: cert-manager-trust-manager`.
4. If Bundle admission fails, check `kube-apiserver` logs for errors calling `trust.cert-manager.io`. Confirm the webhook policy allows TCP 6443 and not only Service port 443.
5. If reconcile fails with API timeouts, confirm the egress policy allows TCP 6443.
6. If metrics are missing, confirm the ServiceMonitor uses scheme HTTP against port 9402 and runs in `openshift-user-workload-monitoring`. A scraper in any other namespace is denied on purpose.
7. If a policy was edited by hand, wait for the controller to restore it. That restore is expected.
8. The policies have no disable flag. Removing enforcement means deleting the four `NetworkPolicy` objects. The controller will recreate them while the `TrustManager` resource exists. Deleting the `TrustManager` resource does not remove the policies.

