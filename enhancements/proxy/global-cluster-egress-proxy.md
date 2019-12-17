---
title: global-cluster-egress-proxy
authors:
  - "@bparees"
  - "@danehans"
reviewers:
  - "@deads"
  - "@derekwaynecarr"
  - "@knobunc"
  - "@eparis"
approvers:
  - "@derekwaynecarr"
  - "@eparis"
  - "@knobunc"
creation-date: 2019-10-04
last-updated: 2019-11-24
status: implemented
see-also:
  - "https://github.com/openshift/enhancements/pull/22"
---

# Global Cluster Egress Proxy

## Release Signoff Checklist

- [ x ] Enhancement is `implementable`
- [ x ] Design details are appropriately documented from clear requirements
- [ x ] Test plan is defined
- [ x ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

Various OpenShift infrastructure components have a need to make requests
to services that reside off-cluster.  Customers may require traffic that
goes outside their network to go through a proxy.  Therefore OpenShift
infrastructure components that need to make requests to external services
may need to go through a proxy.

The goal of this proposal is to define a mechanism for publishing and consuming
configuration information related to the proxy to be used when making external
requests.  This includes the proxy connection information as well as any
additional certificate authorities that are required to validate the proxy's
certificate.  Configuration information also includes domains for which requests
should not go through the proxy.

This information is needed at install time, but also must be configurable by
an administer at runtime, with infrastructure components picking up the
new configuration.

## Motivation

Enabling users to successfully run clusters in environments where external
service requests must go through a proxy.

### Goals

Provide proxy configuration information to the installer so it is available
to components when pulling infrastructure images from external registries and
making other external requests.

Provide certificate authority information to the installer so it is
available to components that make external requests so they can make
successful TLS connections.

Provide a mechanism for admins to update the proxy configuration information
(hostnames, non-proxied hostnames, and certificate authorities) and provide
a mechanism for interested components to consume that information in a consistent
way.

Provide a mechanism for resources managed by the cluster-version operator (CVO)
to have proxy configuration injected into their operator since they cannot manage
their own configuration (e.g. environment variables) without the CVO resetting it.

Provide sanity checking of updates the proxy configuration to ensure they appear
valid, as invalid configuration can break critical control plane components and brick
the cluster.

### Non-Goals

First-class support/enablement of proxy utilization for user provided applications.

End-to-end management of proxy configuration for consuming components (components
that need to use the proxy will need to consume the configuration themselves and
monitor it for changes, with the exception of the CVO-managed resources as noted
under goals).

Providing a single source of CAs to be used by all components, though this work
heads us in that direction.  (Having the bundle include the service CA cert might
be a nice addition in the future, as well as enforcing that components must
consume the provided bundle and not use CAs from their own image or other sources,
but those things are at best tangential to providing a proxy configuration mechanism).

## Proposal

* Introduce a cluster-scoped proxy configuration resource.
* Introduce a canonical location for additional CAs which will be used by
  components talking to the proxy.
* Make it possible to provide this configuration at install time.
* Include specific no-proxy hostnames automatically to ensure internal cluster
  components can communicate.

### User Stories

#### Story 1

As an administrator of a network with strict traffic egress policies,
I want to install an openshift cluster that can successfully make
external requests for images and other external service interactions.

#### Story 2

As an administrator of an openshift cluster, I want to change the proxy
used by my cluster to talk to external services.  I want to make this
change in a single location.

#### Story 3

As an administrator of a network using a man-in-the-middle proxy which
is performing traffic decryption/re-encryption, I want to provide a
valid CA that can trust my proxy's certificate to openshift components
so they will trust my proxy.

### Implementation Details/Notes/Constraints [optional]

This enhancement introduces a cluster scoped proxy configuration resource.
The resource includes fields to:

* specify an `https` proxy URL.
* specify an `http` proxy URL.
* specify additional domains that should not be proxied, in addition to some system defined ones.
* specify a reference to a user defined configmap containing additional CAs that should be trusted when
  connecting to the proxy.
* specify endpoints that can be used to validate the proxy configuration is functional.

All of this information with the exception of the validation endpoints can be provided
at install time to ensure that a cluster can bootstrap successfully even if it needs to
reach external services via the proxy to do so.

The information can also be modified at runtime.  If it is modified at runtime, a controller
will confirm that the validation endpoints can be successfully reached using the new configuration
before accepting the new configuration.  Once accepted, the configuration is moved into the status
section of the proxy config resource.  Components should only consume the proxy configuration from
this location.

Similarly, any user provided CAs will only be copied into an "accepted" CAs
configmap after confirming the validation endpoints can be accessed using the new CAs.

Additional behaviors:

* configmaps labeled with `config.openshift.io/inject-trusted-cabundle: "true"` will have the current
  set of additional CAs injected into them by logic in the cluster network operator.
* deployments with the `config.openshift.io/inject-proxy: <container-name>` will get the current proxy
  environment variables injected (HTTP_PROXY, HTTPS_PROXY, NO_PROXY) by the cluster version operator.

Critical touch points for the administrator:

* edits cluster scoped proxy config resource (`spec` fields).
* provides a configmap of additional CAs.

Critical touch points for proxy configuration consumers:

* Operator consumes status fields from cluster scoped proxy config resource and updates its operand accordingly.
* *May* consume the "accepted CAs" configmap (`openshift-config-managed/trusted-ca-bundle`) to get CAs.
* *Should* create their own configmap with the `config.openshift.io/inject-trusted-cabundle: "true"` label and
  consume the CA bundle from there.
* Operator deployment may request proxy environment injection via the `config.openshift.io/inject-proxy: <container-name>`
  annotation since operators cannot control their own environment variables, but the operator is responsible for mounting
  a configmap to pick up the CAs if it needs them.

Automatic no-proxy behavior:

The network operator injects no-proxy entries for domains and hosts which are known to be backed by the cluster itself.  For example:

* `localhost` and `127.0.0.1`
* `.cluster.local`
* `.svc` and the service CIDR, `serviceNetwork` in the install-config (usually 172.30.0.0/16).
* The machine CIDR, `machineCIDR` in the install-config (usually 10.0.0.0/16).
* The cluster pod CIDR, `clusterNetwork` in the install-config (usually 10.128.0.0/14).
* `api-int.${CLUSTER_NAME}.${BASE_DOMAIN}` but not `api.${CLUSTER_NAME}.${BASE_DOMAIN}`.
* `etcd-0.${CLUSTER_NAME}.${BASE_DOMAIN}` and so on for each of the control-plane machines.

Additional entries like `169.254.169.254` and `.${REGION}.compute.internal` may be injected for clusters running on platforms where nodes require access to those entries to retrieve their metadata, because they are required for bootstrapping.

Known limitations/future enhancements:

* The no-proxy entries that are automatically added do not include the ingress domain generically, so
  requests that go to external routes will likely go through the proxy.  This may be undesirable, though
  it should not break as long as the proxy is able to call back to the cluster.  The administrator can add
  additional no-proxy entries.  It was deemed unacceptable to automatically no-proxy the entire ingress domain
  because the front end load balancing/routing for the cluster could reside outside the clusters network.
* Not all cloud service APIs are added to the no-proxy list.  This means some cloud API requests may
  go through the proxy, which may be useful for auditing or may be undesirable for efficiency.  It should
  not break things as long as the proxy is functional, and administrators can add additional no-proxy entries
  as they see fit.
* No validation endpoints can be provided at install time.  This is because there is no point in validating
  the proxy configuration during install.  If it is incorrect, the install will fail.  However it also
  puts administrators at risk of never providing validation endpoints to their proxy configuration, which
  means they can update their proxy configuration to something that does not work, in the future, if they do
  not add validation endpoints on day 2.  We should consider enhancing the install config to allow specification
  of validation endpoints so this step is not forgotten by administrators.  In addition install is a lengthy
  operation.  Validating the proxy configuration up front would allow us to "fail fast".
* Currently the no-proxy value in the proxy config is append-only.  There is no way for an administrator to
  remove one of the no-proxy domains that we add automatically.  This means we must be extremely cautious to
  not add no-proxy domains that might need to be proxied.
* Long term it should be possible for the additional CA bundle to be the *only* source of CAs for components.
  Today the additional CA bundle is a combination of user provided CAs plus the system CAs from the network
  operator image.  In the future adding the system trusts to the bundle should be a configurable optional so customers
  who want to explicitly control the trusted CAs can do so.
* In addition, due to 4.1->4.2 upgrade limitations, components must fallback to using their own CAs from their image in the event
  that the configmap does not have a bundle injected into it because the network operator is not upgraded yet.  This
  latter limitation can be removed in 4.3, meaning components can make the configmap key a required mount and expect
  that it will have sufficient content to supplant any system CAs in their own image.

See also: [proxy workflow](https://docs.google.com/document/d/1y0t0yEOSnKc4abxsjxEQjrFa1AP8iHcGyxlBpqGLO08/edit#

### Risks and Mitigations

The biggest risk with this feature is administrators accidentally bricking their cluster by providing
an invalid proxy configuration that leads to critical components becoming non-functional to the point
that even API changes to fix the configuration are not possible.

We attempt to mitigate that risk by providing the "validation endpoints" feature which tries to ensure
that the proxy configuration is valid before propagating it to components for consumption, but the
ability to truly validate the configuration and functionality of the proxy is very limited, so this
risk cannot be eliminated.

Similarly, implementation bugs in the components that consume the proxy could also result in them
being unable to reach critical services and failing, such as by mishandling of CAs or configuration
updates.

## Design Details

### Test Plan

We will introduce an e2e-platform-proxy CI job which will run our usual e2e suite, but in a cluster
configured to use a proxy.  This will provide a minimal level of coverage, but additional coverage
should be added to handle:

1) changes to the proxy configuration (can be tested by individual config consumers)
2) upgrade testing from 4.1->4.2 since this is the upgrade that introduces the proxy config logic
3) man in the middle proxies (as distinct from passthrough proxies) since they present additional
certificate challenges.

Our QE team is covering some of these items, but ultimately automated coverage must exist for all
of them.

### Graduation Criteria

Being delivered as GA in 4.2.

### Upgrade / Downgrade Strategy

This feature is being implemented as parts of existing components, not as a new
component itself, so the upgrade is handled by those components.  That said, testing
has already turned up one specific dependency during upgrade:

Configmaps labeled for CA injection will not have CAs injected into them until the
network operator is upgraded to 4.2+.  Since the network operator is one the last
components to upgrade, this means other components upgrading to 4.2 may create
labeled configmaps and wait for injection to occur, thus blocking the upgrade
waiting on an event that will never happen because the network operator isn't
yet upgraded and cannot be upgraded until the earlier components finish their
upgrade.

The mitigation for this is that no component in 4.2 should be dependent on the
configmap injection occurring.  Once we reach 4.3 it should be acceptable to
require the configmap injection to occur.

### Version Skew Strategy

See the 4.1->4.2 discussion above for details about version skew challenges and mitigation.

## Implementation History

v4.2.0: initial implementation GA.

## Drawbacks

This feature requires touchpoints across many many components, all of which are impacted
as the design/implementation evolves.

## Alternatives

Make every component define its own configuration mechanism for proxy support and require
admins to modify all of them and keep them in sync.

## Infrastructure Needed [optional]

* CI environments with configured proxies that we can direct the clusters under test to use.
