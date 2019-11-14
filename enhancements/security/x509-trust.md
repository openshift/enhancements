---
title: x.509-trust
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@bparees"
  - "@dhansen"
  - "@sdodson"
approvers:
  - TBD
creation-date: 2019-11-14
last-updated: 2019-11-24
status: provisional
see-also:
  - "/enhancements/automated-service-ca-rotation.md"
  - "/enhancements/kube-apiserver/certificates.md"
  - "/enhancements/kube-apiserver/tls-config.md"
  - "/enhancements/proxy/global-cluster-egress-proxy.md"
---

# X.509 Trust

This enhancement provides a big-picture overview of X.509 trust management for cluster components.
This enhancement provides for both cluster-scoped defaults and per-component overrides.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Several cluster components may make TLS requests to external services.
For example, [the cluster-version operator][cluster-version-operator] may request available updates, [the machine-API operator][machine-api-operator] may call platform APIs to manage machines, [CRI-O][cri-o] may contact container image registries when downloading images for pods, etc.
Cluster components may also perform other X.509 verification besides TLS requests.
For example, user mail processing components may perform [S/MIME][smime] verification.
This enhancement provides a framework for managing X.509 trust for these situations.

Currently the default trust for CRI-O, the kubelet's cloud provider, and other node-namespace egress is the node's X.509 certificate authority (CA) store, which depends on the operating system ([Red Hat Enterprise Linux CoreOS (RHCOS)][rhcos], etc.) and any subsequent configuration ([the machine-config operator][machine-config-operator], etc.).
The default trust for containers like the machine-API operator is that container's CA store, which depends on the base layer ([Red Hat Universal Base Image (UBI)][ubi], etc.).
Many external components will have X.509 certificates signed by one of those default CAs, but this is not always the case.

There may also be internal communication between cluster components that relies on X.509 trust but lacks a custom trust configuration mechanism.
The framework from this enhancement can also be used to configure trust for that communication as well.

This enhancement provides a mechanism for configuring additional or alternative trust bundles for all cluster services that do not already have existing trust configuration mechanisms.

## Motivation

### Goals

Administrators will have a single location to configure a default additional trust bundle to be used by all cluster components.
They will also be able to specify whether this default is to be use in addition to, or instead of, the component's default trust bundle.

Administrators will have per-component configuration locations to adjust the cluster-scoped default.
As for the cluster-scoped default, they will be able to specify whether the per-component trust bundle is to be used in addition to, or instead of, the cluster-scoped default.

### Non-Goals

This enhancement does not address components with existing X.509 trust-distribution mechanisms.
Trust for those requests is covered by [the Kubernetes API-server Certificates enhancement](../kube-apiserver/certificates.md) and [the Automated Service CA Rotation enhancement](../automated-service-ca-rotation.md).

This enhancement does not address configuration for non-CA TLS settings like ciphers and allowed TLS versions.
[The ingress TLS configuration enhancement](../kube-apiserver/tls-config.md) covers those settings for cluster-hosted services.
Outgoing TLS probably needs similar configuration, but that configuration is decoupled from X.509 trust and should be addressed in a separate enhancement.

## Proposal

### Cluster-scoped Default Trust

There will be a ConfigMap (FIXME: or should this be a new CRD under openshift/api?) named `default-ca-bundle` in the `openshift-config` namespace.
The ConfigMap will have the following keys:

* `ca-bundle.crt`, containing a PEM-encoded X.509 certificate bundle.
* `composition`, configuring how the bundle in `ca-bundle.crt` is composed with the component's default trust store.
    Valid values are:
    * `union`, in which case components should use the union of the `ca-bundle.crt` and their default trust store.
        This is convenient for cluster administrators who need to extend the default trust store to support additional CAs, but who wish to delegate default trust store maintenance to the cluster components.
    * `override`, in which case components should use only `ca-bundle.crt` and ignore their default trust store.
        This is allows cluster admistrators to take complete control of the trust store for situations where delegating trust maintenance is not permissible.
        For example, a given component may not need to trust all 100+ CAs in [the default certificate store][mozilla-ca-certificates] if it only uses X.509 trust to verify TLS connections to a few services with stable CAs.
        Excluding unnecessary CAs from the trust store reduces the risk of being exploited via a compromised CA.

        This behavior is also the default when a component recieves an unrecognized `composition` value.

### Per-component Trust

Each component will define its own well-known ConfigMap(s) in its own namespace, with the same `ca-bundle.crt` and `composition` keys as [the cluster-scoped ConfigMap](#cluster-scoped-default-trust).
For example, the registry operator uses [`trusted-ca`][registry-configmap-name] in [the `openshift-image-registry` namespace][registry-configmap-namespace].
Components that have distinct trust domains may define multiple ConfigMaps for each domain.
For example, there may be one ConfigMap for proxy/egress TLS, and another ConfigMap for [S/MIME][smime] verification.
To populate that trust bundle, the cluster administrator can set labels on the ConfigMap(s).
For example, [the `config.openshift.io/inject-proxy-cabundle` label](../proxy/global-cluster-egress-proxy.md#implementation-detailsnotesconstraints-optional) asks for the current proxy trust bundle.

The cluster-version operator (CVO) [merges ConfigMaps][cluster-version-operator-EnsureConfigMap] by [clobbering any manifest-defined data][cluster-version-operator-mergeMap], [labels, and annotations][cluster-version-operator-EnsureObjectMeta].
Data keys, labels, and annotations not defined in the manifest are ignored.
This means that components which decide to provide their per-component ConfigMaps via the CVO should not set labels like `config.openshift.io/inject-default-cabundle` unless they also set [the `release.openshift.io/create-only` annotation to `true`][cluster-version-operator-create-only], to allow cluster administrators to manipulate the per-component ConfigMap without the CVO stomping on their updates.
For example, the registry operator [sets `create-only`][registry-configmap-create-only].

#### Universal Base Image Containers

FIXME: UBI-specific docs for this [RHEL behavior](rhel-cert-injection) that show that you don't need to install some non-UBI RHEL stuff to make it work.

Components can mount `ca-bundle.crt` from their ConfigMap(s) under `/usr/share/pki/ca-trust-source/anchors/`.
Components are then responsible for running `update-ca-trust` (FIXME: do we need `extract` or not?) at start.
They must also run `update-ca-trust` periodically thereafter to pull in any [updates from the mounted ConfigMap][mounted-configmap-updates].

Components that need a single trust bundle injected can bypass `update-ca-trust` and mount their `ca-bundle.crt` directly into `/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem` (FIXME: RHEL/UBI docs confirming this as a supported workflow).
For example, the registry operator [does this][registry-configmap-mount].

FIXME: We need to grow a way to handle `override` `composition`.

### Node Trust

[Red Hat Enterprise Linux CoreOS (RHCOS)][rhcos], and nodes more broadly, have a separate trust-management framework getting hashed out [here][machine-config-operator-trust].

### Container Image Registry Trust

FIXME: fill in how this works

### User Stories

#### Cloud-provider trust

Several cloud components such as [machine-API][machine-api-operator] providers may call platform APIs to manage cluster infrastructure.
Some cloud-providers may provide those services over HTTPS with X.509 certificates signed by CAs that are not included in default trust stores (e.g. [OpenShift behind a custom CA][openshift-custom-ca]).
By including the cloud provider's CA in the [cluster-scoped default trust bundle](#cluster-scoped-default-trust) or per-component overrides, a cluster administrator could enable cluster-managed infrastructure on those platforms.

#### Transparent proxies

FIXME

### Implementation Details/Notes/Constraints

The network operator supports copying trust bundles between ConfigMaps based on labels on the target ConfigMaps.

* [The `config.openshift.io/inject-default-cabundle` label](../proxy/global-cluster-egress-proxy.md#implementation-detailsnotesconstraints-optional) asks for the current [cluster-scoped default trust bundle](#cluster-scoped-default-trust).
* [The `config.openshift.io/inject-proxy-cabundle` label](../proxy/global-cluster-egress-proxy.md#implementation-detailsnotesconstraints-optional) asks for the current proxy trust bundle.
    The network operator will fall back to [the cluster-scoped default trust bundle](#cluster-scoped-default-trust) if no proxy-specific trust bundle has been configured.

If multiple labels are set `true`, the network operator will provide the union of the requested trust bundles.

If any input ConfigMap has the `union` `composition`, the network operator will set that composition on the target ConfigMap.
If no input ConfigMap has `composition` set, the network operator will clear `composition` on the target ConfigMap.
Otherwise, the network operator will set the `override` `composition` on the target ConfigMap.

All components involved in X.509 verification should allow trust configuration by some mechanism, defaulting to the mechanism described in this enhancement if they have no more-specific mechanism.
If a component's only X.509 verification is for outgoing TLS connections, it can include only the proxy-specific trust bundle and does not need to directly request the cluster-scoped default trust bundle.

### Risks and Mitigations

Administrators could break some cluster components by setting `override` `composition` for a trust bundle which does not include an important CA (e.g. a proxy CA).
This is mitigated by the Kubernetes API server and etcd having existing X.509 trust-distribution mechanisms, so the administrator is unlikey to break the core cluster enough to prevent recovery.

Administrators could misconfigure their cluster by setting an unknown `composition` (e.g. typoing `onion` instead of `union`).
The `override` default protects from a accidental promiscuity, but accidental `override` behavior might result in the unsufficient-trust situation discussed in the previous paragraph.
We could protect against these typos by using a custom resource definition, but only ConfigMaps can be mounted into containers, so that protection would be incomplete.

## Design Details

### Test Plan

FIXME

### Graduation Criteria

FIXME

#### Removing a deprecated feature

Currently cluster components set `config.openshift.io/inject-trusted-cabundle` to receive the _proxy_ bundle, not [the cluster-scoped default trust bundle](#cluster-scoped-default-trust).
And currently nothing populates `default-ca-bundle` (the installer uses [`user-ca-bundle`][installer-configmap-name]).
So a hard cut to the approach described in this enhancement would remove configured additional trust from proxy-consuming components.
This enhancement deprecates the `config.openshift.io/inject-trusted-cabundle` label in favor of the [new labels](#implementation-detailsnotesconstraints).
The expected migration path is:

1. The network operator learns about the new labels.
    `config.openshift.io/inject-trusted-cabundle` is treated as a synonym for `config.openshift.io/inject-proxy-cabundle`.
2. Existing proxy consumers migrate to `config.openshift.io/inject-proxy-cabundle`.
3. The network operator adds an alert on any ConfigMaps with the `config.openshift.io/inject-trusted-cabundle` label, to notify cluster administrators about the deprecation.
    This doesn't have to be an alert, it could happen through [the insights operator][insights-operator] instead.
4. After a suitable deprecation period, the `config.openshift.io/inject-trusted-cabundle` handling is removed from the network operator.

### Version Skew Strategy

FIXME

## Implementation History

None yet :).

## Drawbacks

Attempting to fix this with a single architecture is nice for consistency, but risks trying to wedge everyone into the same bucket.
Some components like [build tooling](#container-image-registry-trust) have established systems for trust distribution, and we will either leave them separate (which may be annoying for cluster adminstrators) or try to port them to the generic system (which may be annoying for their maintainers and existing users).

## Alternatives

Let every cluster component figure out their own approach for this.

[cluster-version-operator]: https://github.com/openshift/cluster-version-operator/
[cluster-version-operator-create-only]: https://github.com/openshift/cluster-version-operator/blob/751c6d0c872e05f218f01d2a9f20293b4dfcca88/docs/dev/operators.md#what-if-i-only-want-the-cvo-to-create-my-resource-but-never-update-it
[cluster-version-operator-EnsureConfigMap]: https://github.com/openshift/cluster-version-operator/blob/751c6d0c872e05f218f01d2a9f20293b4dfcca88/lib/resourcemerge/core.go#L10-L16
[cluster-version-operator-EnsureObjectMeta]: https://github.com/openshift/cluster-version-operator/blob/751c6d0c872e05f218f01d2a9f20293b4dfcca88/lib/resourcemerge/meta.go#L8-L16
[cluster-version-operator-mergeMap]: https://github.com/openshift/cluster-version-operator/blob/751c6d0c872e05f218f01d2a9f20293b4dfcca88/lib/resourcemerge/meta.go#L28-L41
[cri-o]: https://github.com/cri-o/cri-o/
[insights-operator]: https://github.com/openshift/insights-operator
[installer-configmap-name]: https://github.com/openshift/installer/blob/de10297f9158c16f36471c91a3d48be2fb2938e1/pkg/asset/manifests/additionaltrustbundleconfig.go#L26
[machine-api-operator]: https://github.com/openshift/machine-api-operator/
[machine-config-operator]: https://github.com/openshift/machine-config-operator/
[machine-config-operator-trust]: https://github.com/openshift/machine-config-operator/issues/528
[mounted-configmap-updates]: https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically
[mozilla-ca-certificates]: https://ccadb-public.secure.force.com/mozilla/IncludedCACertificateReport
[openshift-custom-ca]: https://bugzilla.redhat.com/show_bug.cgi?id=1608888#c0
[registry-configmap-create-only]: https://github.com/openshift/cluster-image-registry-operator/blob/75e8e851700add9c847190fb228d2e702b2af2e8/manifests/04-ca-trusted.yaml#L6
[registry-configmap-mount]: https://github.com/openshift/cluster-image-registry-operator/blob/75e8e851700add9c847190fb228d2e702b2af2e8/manifests/07-operator.yaml#L87-L97
[registry-configmap-name]: https://github.com/openshift/cluster-image-registry-operator/blob/75e8e851700add9c847190fb228d2e702b2af2e8/manifests/07-operator.yaml#L92-L93
[registry-configmap-namespace]: https://github.com/openshift/cluster-image-registry-operator/blob/75e8e851700add9c847190fb228d2e702b2af2e8/manifests/07-operator.yaml#L6
[rhcos]: https://docs.openshift.com/container-platform/4.2/architecture/architecture-rhcos.html
[rhel-cert-injection]: https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/security_hardening/using-shared-system-certificates_security-hardening#adding-new-certificates_using-shared-system-certificates
[smime]: https://tools.ietf.org/html/rfc5751
[ubi]: https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/building_running_and_managing_containers/using_red_hat_universal_base_images_standard_minimal_and_runtimes
