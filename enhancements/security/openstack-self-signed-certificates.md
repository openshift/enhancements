---
title: openstack-ca-certficate-support
authors:
  - "@egarcia"
  - "@mandre"
reviewers:
approvers:
creation-date: 2020-1-27
last-updated: 2019-1-27
status: implemented
---

# OpenStack CA Certificate Support

This enhancement allows users to pass the CA certificates for their openstack cluster to the installer. The installer will then distribute that certificate to all services that authenticate with OpenStack, so that they are able to trust the ssl connection.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-installer-docs](https://github.com/openshift/installer/)

## Open Questions

### How should we pass the certificate to the installer?

There were some concerns about how to pass the certificate to the installer. Originally, we were thinking about passing it via the AdditionalTrustBundle property of the install-config.yaml, however some developers argue this is not the correct (i.e. not the most secure) method.
We could get the certificate from the ca-cert field of the clouds.yaml entry, or alternatively add a new option for OpenStack in install-config.yaml.

*Answer:* We have decided to deliver use the clouds.yaml to pass the
certificate to the installer. The clouds.yaml is a file containing the
auth credentials for your openstack cluster, and it contains the entry
`cacert` that accepts the path to your openstack cloud's CA
certificates. This logically made the most sense because it is already
how we expect users to pass auth information about their cloud to the
installer. It also helps us seperate the CA certificate from the
AdditionalTrustBundle, which allows us to be more conservative about
what we are giving services access to.

### Where should we store the user's CA cert in openshift, so services can easily access it?

We currently store the `additionalTrustBundle` in the `openshift-config/user-ca-bundle` configmap, and could store the openstack CA cert in a new key there. And while that would logically make sense, is that the best place to store it?

*Answer:* We think the best place to store the certificate is in the
`openshift-config/cloud-provider-config` configmap under the
`ca-bundle.pem` key. The main advantage of this is the effortless
distribution and synchronization of this configmap with cloud-provider
components, since they have code in library-go that synchronizes and
writes the contents of each key to disk from this configmap. Most of
the components that need the CA cert already have access to it, and
the content of the configmap is not risky for them to have access to.

### Can we assume the cert is trusted at a level system on the node that runs the installer?

According to OSP docs, yes. According to OCP developers, no. They recommend against it and suggest the installer should have a way to use a separate trust store when initiating connections. In this case, we’ll have to patch the installer to use the provided cert when initiating connections to OpenStack.

Answer: For now, we are going to stand with the prescedent set by past and current OpenShift documentation, and continue to instruct users to make sure the host that is running the installer trusts the CA certificates.

### Should CAPO patch be updated to read cert from a configmap rather than passed as arg?

We made an initial change to `cluster-api-provider-openstack` (CAPO) that added an argument in its api for CA certificates. This would bring us out of alignment with upstream, and is also static.

*Answer:* Yes. We should use the same source of certificates for every
component if possible. That way, if customers need to update the
certificate, there is only a single config map that they need to
change. We also don't want to diverge from upstream if
possible. Lastly, by fetching the CA cert from a configmap each time
authentication with openstack is required, it ideally will allow us to
update the configmap for CAPO dynamically.

## Summary

Using the `cacert` argument in the user's clouds.yaml, users can pass the location of their OpenStack cluster's CA certificate to the installer. An example of a clouds.yaml that uses this feature is as follows:

```yaml
# clouds.yaml
clouds:
  openstack:
    auth: ...
    cacert: /home/myhome/ca.crt
```

If the user passes a CA cert to the installer, and the installer is running for the openstack platform, it will attempt to read the certificate from disk, and check to make sure that its valid. Then, it will write the certificate to a key in the `openshift-config/cloud-provider-config` configmap under the `ca-bundle.pem` key. This is where the certificate will live in the OpenShift cluster.

A number of OpenShift services use `cloud-provider-openstack` to
communicate with the underlying OpenStack cloud, and therefore will
need to authenticate with OpenStack. Luckily, it already supports
being passed CA certificates. In the installer, we simply add a known
static path to the `ca-file` argument in the `cloud.conf` file. This
path is where it can expect to read the CA certificate from. The path
that we add is where `library-go` writes the contents of the entire
`openshift-config/cloud-provider-config` configmap to disk, so we know
our cert file will always be at the path,
`/etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem`.

Kubelet also reads the `cloud.conf`, so it will expect a certificate
to be at
`/etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem`,
the problem is that kubelet is run in systemd on the nodes. A new
optional arugment is added to the `machine-config-operator` (MCO) ,
`--cloud-provider-ca-file=` so we are able to pass the CA file to it
during startup. This coupled with tweaks to the bootkube script that
enable the optional argument when installing on openstack allow us to
pass the CA cert to the MCO for rendering on the bootstrap node. A
template renders the content of the CA cert onto all nodes managed by
the MCO. Lastly, once the control plane is up, the MCO will fetch the
latest value from
`openshift-config/cloud-provider-config:ca-bundle.pem` in order to
keep the cert on disk synchronized with the latest state.

The `cluster-image-registry-operator` (CIRO) and the `image-registry`
also need to communicate with OpenStack in order to use swift. CIRO is
allowed to read configmaps from the `openshift-config` namespace, so
it simple uses an openshift listener to get the latest certificate
from the `cloud-provider-config` configmap, and can create a custom
http transport that trusts the certificate for authenticating with
OpenStack.

The `machine-controllers` and more specifically,
`cluster-api-provider-openstack` (CAPO), will need to authenticate
with openstack any time it needs to manage the current machine
pool. In order to keep `openshift-config/cloud-provider-config` as the
central point that services draw the CA certificate from, CAPO is
given RBAC access to `GET` configmaps named `cloud-provider-config`
from the `openshift-config` namespace. This allows the kubernetes
client in CAPO to fetch the latest CA certificate, and craft a custom
http transport that trusts it when authenticating with OpenStack.

Lastly, when Kuryr is configured cluster-network-operator and Kuryr
itself need to communicate with OpenStack networking services as part
of their core functionality. Following the common design pattern, CNO
GETs the latest version of `openshift-config/cloud-provider-config`
and builds a custom HTTP transport with the CA cert when
authenticating with OpenStack. Then a ConfigMap with certificate is
created in `openshift-kuryr` in order to mount it for kuryr-controller
pods. Please note that in case of Kuryr due to usage of Python
OpenStack APIs, the passed CA certificate overwrites default CAs
instead of being appended to them.

In total, these changes enable the distribution, and trust of OpenStack CA certificates in an OpenShift cluster both during and after install.

## Motivation

Numerous customer complaints have resulted in this issue being highly escalated. This prevents a number of customers from installing OpenShift 4 on their OpenStack clusters.

### Goals

1. To enable the OpenShift 4.4 installer to succesfully deploy a cluster on an OpenStack cloud that uses self signed certificates for auth.
2. OpenShift 4.4 can succesfully and stabily run on OpenStack clusters that use self signed certificates for auth.
3. Backport to 4.3

## Proposal

### Risks and Mitigations

1. It is possible to give services authorization/credentials that they dont need and shouldnt have, and this may pose a security risk.

> To prevent this, we make sure that only services that need the CA cert get access to it, and the cert we pass them only has the CA cert in it.

### Test Plan

This feature should not affect any of the other platforms, so to ensure this we can take advantage of OpenShift's CI for those platforms. They should pass during a pull request, and their test pass rate should not change.

For OpenStack, we have access to a cluster configured specifically to test this. We will be testing our code changes in that cluster before merging to ensure that the installer runs. Then, we can use the OpenShift CI and QE in order to garuntee that the code changes will not affect customers in any harmful ways.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- QE testing
- Downstream Docs
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

This should not affect the Upgrade/Downgrade functionality.

## Implementation History

- Merged into Master
- On QE

## Drawbacks

1. This only fixes the problem for OpenStack
2. All services patches with the CA cert in OpenShift now have access to the `cloud.conf`

## Alternatives

- [X509-trust](https://github.com/openshift/enhancements/pull/115)
