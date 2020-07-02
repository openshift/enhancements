---
title: infrastructure-apis-custom-trust
authors:
  - "@abhinavdahiya"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-06-30
last-updated: 2020-06-30
status: implementable
see-also:
  - "/enhancements/proxy/global-cluster-egress-proxy.md"  
---

# Custom Trust for Infrastructure APIs

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The users should be allowed to provide a custom trust bundle for the purpose of communicating with infrastructure APIs, which should be available to all the operators so that they can configure their SDKs. The operators will be able to pull the trust bundle from a defined ConfigMap or use the [`config.openshift.io/inject-trusted-cabundle`][egress-proxy-proposal] mechanism to mount a volume with trust already configured. The later is important because a majority of our operators communicate with infrastructure APIs and configuring the SDKs with custom trust is tedious and error prone similar to the PROXY setup.


## Motivation

OpenShift components interact quite a bit with the underlying infrastructure using APIs to provide it's users various features and a majority of our operators today interact with infrastructure APIs. For our commercial platforms like AWS, Azure, GCP's public cloud trusting the secured API endpoints is not a problem, but for other platforms like vSphere, OpenStack or even private clouds from AWS or Azure we need our operators to trust user-provided bundles to securely communicate to the APIs.

### Goals

1. Allow users to provide custom trust for APIs using openshift-installer.
2. Allow users to provide custom trust for APIs as a day-2 operation.
3. Provide a no hassle workflow for operators to consume the trust bundle on-par with [proxy][egress-proxy-proposal]

### Non-Goals

None at this time.

## Proposal

1. openshift-installer allows the users to provide custom trust bundle for infrastructure APIs using the `install-config.yaml`.
2. [Infrastructure][openshift-api-infrastructure] object allows user to provide custom trust in the form of a ConfigMap in `openshift-config` namespace.
3. A controller validates the user-provided ConfigMap and provides it as an API for consumption in the form of a ConfigMap in `openshift-config-managed` namespace.
4. The [proxy controller][cno-proxy-controller] uses the above ConfigMap another source for generating the `openshift-config-managed/trusted-ca-bundle` so that operators get the infrastructure APIs trust when using the [`config.openshift.io/inject-trusted-cabundle`][egress-proxy-proposal] mechanism.

### User Stories

#### AWS GovCloud testing using emulation

#### AWS C2S/SC2S

#### Azure Stack Hub

### Risks and Mitigations

None

## Design Details

### Can we configure the custom trust for SDKs easily?

There are 2 types of ways we would need to configure custom trust for infrastructure APIs,

- The operator itself is talking to the APIs so it needs to configure the SDK.
- The operator is managing a component that communicates with infrastructure APIs so it needs to configure that component to use the custom trust.

#### SDKs directly

This is not trivial, as some Go SDKs require the user to configure the http.Client with appropiate transport like for example AWS and GCP, [for example](https://github.com/aws/aws-sdk-go/issues/2404#issuecomment-454196045). While there are Go SDKs like for Azure that do not provide any great way, [for example](https://github.com/Azure/azure-sdk-for-go/issues/5974).

Also since configuring the trust at best requires creating custom http transport, this is difficult and error prone to the same extent that made us move to using PROXY environment variable instead of managing transports for http clients with these specific values. Using universal system trust mechanism is more safer for the same reasons.

#### Components that communicate with APIs

When operators manage external components the operators are bound to API provided by the component to configure the trust and more often these components choose system trust's known locations as the API, a big example is the kube-controller-manager which does not provide any API to configure the trust for cloud APIs. In this scenario the operators would have to provide management of system trust in component's container at runtime so that cloud APIs are trusted and also other endpoints it might communicate with. This is also tedious and error prone and can sometimes be very difficult if the container images have in compatible permissions to execute certain commands like `update-system-trust`.

So all the problems of configuring the PROXY trust bundles are present in configuring trust for infrastructure APIs for **majority of our operators** and hence we should re-use the trust bundle distribution mechanism for this use case.

### Infrastructure API

```go
// InfrastructureSpec contains settings that apply to the cluster infrastructure.
type InfrastructureSpec struct {
	// trustedCA is a reference to a ConfigMap containing a CA certificate bundle.
	// The trustedCA field should only be consumed by a infrastructure API validator. The
	// validator is responsible for reading the certificate bundle from the required
	// key "ca-bundle.crt", and writing the merged trust bundle to a ConfigMap 
	// named "infrastructure-api-trusted-ca-bundle" in the "openshift-config-managed" namespace. 
	// Clients that expect to make connections to infrastructure APIs must use the generated 
	// bundle for all HTTPS requests, or use the `config.openshift.io/inject-trusted-cabundle` trust 
	// injection mechanism from the proxy controller to mount trust bundle that includes it.
	//
	// The namespace for the ConfigMap referenced by trustedCA is
	// "openshift-config". Here is an example ConfigMap (in yaml):
	//
	// apiVersion: v1
	// kind: ConfigMap
	// metadata:
	//  name: user-infra-api-ca-bundle
	//  namespace: openshift-config
	//  data:
	//    ca-bundle.crt: |
	//      -----BEGIN CERTIFICATE-----
	//      Custom CA certificate bundle.
	//      -----END CERTIFICATE-----
	//
	// +optional
	TrustedCA ConfigMapFileReference `json:"trustedCA,omitempty"`
```

### Cluster Network Operator: Proxy controller

Currently the proxy config controller in CNO(cluster-network-operator) reads the [trustedCA][proxy-object-trusted-ca] from `Proxy` config object and generates a ConfigMap as detailed [here][egress-proxy-proposal] so that each operator can use the `config.openshift.io/inject-trusted-cabundle` injection to receive a trust bundle.

The proxy config controller should,

1. Add Infrastructure `.spec.trustedCA` as another source of input in addition to the `.spec.trustedCA` from Proxy object in creating the `openshift-config-managed/trusted-ca-bundle`.
2. This makes it so that injection mechanism delivers the infrastructure trust bundle to all the operators using existing workflow.

### Operators

All the existing operators like [kube-controller-manger-operator][kcm-o-inject], [cloud-credential-operator][cco-inject], [cluster-ingress-operator][cio-inject], [cluster-registry-operator][cro-inject] already use the trust bundle injection to configure the trust in their containers and would need no change. But there are changes in two operators namely machine-api-operator and machine-config-operator to support the trust for infrastructure APIs.

#### Machine API Operator

The MAO(machine-api-operator) does not talk to infrastructure APIs itself, rather it manages cluster-api providers per platform that communicate with these APIs. MAO should use the same process

1. Create a ConfigMap in it's namespace that uses `config.openshift.io/inject-trusted-cabundle` to receive the trusted bundle, like [here][cro-inject].
2. Mount the ConfigMap to appropriate location in the provider containers like [here][cio-mount-trust].

#### Machine Config Operator

MCO(machine-config-operator) is one of the few operators that cannot use the inject mechanism because it configures the `kubelet` on the host and we wouldn't want to the override the trust for the entire machine but add specific trust bundle for `kubelet`.

There was previous work done to support a form of custom trust for [OpenStack][openstack-workaround-mco], but that is not widely implementable across other providers for various reasons mentioned [above][#Can-we-configure-the-custom-trust-for-SDKs-easily].

On bootstrap host it reads the `<root>/tls/cloud-ca-cert.pem` for the custom trust, while in cluster it reads `ca-bundle.pem` key from ConfigMap specified in `.spec.cloudConfig`.

Moving forward the MCO on bootstrap host will read the ConfigMap file `<root>/manifests/assets/manifests/infra-user-ca-bundle-config.yaml` for the trust bundle, and in cluster the operator will read the generated ConfigMap as described [above][#Infrastructure-API]. It should treat it similar to the Proxy trust bundle, i.e ensure it ends up in the list of machine trust [here][mco-additional-trust-list]

### Test Plan

One of the user stories is using this feature for testing AWS GovCloud using emulation. So that should provide appropriate CI for this feature.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

This adds an optional input into already existing workflow and therefore there should be no effect to upgrade/downgrade strategy.

### Version Skew Strategy

This adds an optional input into already existing workflow and therefore there should be no effect to upgrade/downgrade strategy.

## Implementation History

### OpenStack workaround

When OpenStack required custom trust bundle for user deployments, they added certificate key to the `.spec.cloudConfig` ConfigMap for example in Machine Config Operator [PR#1392][openstack-workaround-mco]. That workflow depends on a key in a config map that is meant to configure the `Kubernetes Core` bits interacting with cloud APIs like the kube-controller-manager. But that solution is not suitable for all our operators and also uses a less precise API. This new proposal is more widely implementable and with greater ease across platforms.

## Drawbacks

### Excess trust delivered to containers

There have been push back of getting the infrastructure trust to all HTTPS request instead of just specific request for the container/operators using Proxy inject workflow, but I think since majority of our operators communicate with infrastructure to provide various features and how we reached a point of consensus that most applications do not have easy way to configure custom trust in an easy way, I think the trade off is not very large in terms of any reduction in security as users will be in control of the trust bundle.

## Alternatives

### Overloading Proxy trustedCA

Since the current recommendation technically piggy-backs from the proxy trust delivery mechanism, we could have asked the users to set the infrastructure custom trust using the same API, but I think there are 2 benefits for going the separate route,

1. Increased confusion in usage as asking users to set trustedCA in Proxy object for infrastructure is likely to create un necessary questions. Providing users a separate mechanism to set each but combining the delivery in the backend creates simplification.
2. Separate API allows consumers to fetch the custom trust from a known location instead of complete trust inject providing users control when they choose to take on extra conplexity.

## Infrastructure Needed

None

[cco-inject]: https://github.com/openshift/cloud-credential-operator/blob/1f9a14c54d8f30fe3ecdc72a402c290dacace9f4/manifests/01-trusted-ca-configmap.yaml
[cio-inject]: https://github.com/openshift/cluster-ingress-operator/blob/07b4a561e2abb6bbaa6f56750706168092403319/manifests/01-trusted-ca-configmap.yaml
[cio-mount-trust]: https://github.com/openshift/cluster-ingress-operator/pull/334
[cno-proxy-controller]: https://github.com/openshift/cluster-network-operator/blob/2f04083382a5a824aeca0adfc2b5a58bd34cbb80/pkg/controller/proxyconfig/controller.go
[cro-inject]: https://github.com/openshift/cluster-image-registry-operator/blob/6855b9648c6456e4e8fdd7a5746c0d521054adad/manifests/04-ca-trusted.yaml
[egress-proxy-proposal]: https://github.com/openshift/enhancements/blob/8097f7a3af592c827e00b6e995566d669c97c6a3/enhancements/proxy/global-cluster-egress-proxy.md#proposal
[kmc-o-inject]: https://github.com/openshift/cluster-kube-controller-manager-operator/blob/bb2d93326fd90d274e35281f2e61ed9b2ee9e15e/bindata/v4.1.0/kube-controller-manager/trusted-ca-cm.yaml
[mco-additional-trust-list]: https://github.com/openshift/machine-config-operator/blob/c38f3123e4ce12ec288a38b2dd47f261dcd9f694/pkg/operator/sync.go#L226-L253
[openshift-api-infrastructure]: https://github.com/openshift/api/blob/bd35d0abb483082aba103946dec6d6917e4f0985/config/v1/types_infrastructure.go#L131
[openstack-workaround-mco]: https://github.com/openshift/machine-config-operator/commit/cfea3407615af8371be8ef05f33bc50da0609af0
[proxy-object-trusted-ca]: https://github.com/openshift/api/blob/de5b010b2b386867fc61c4f718080755f9707748/config/v1/types_proxy.go#L69
