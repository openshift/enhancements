---
title: OAuth-Proxy-Proxy-Support
authors:
  - "@deads2k"
reviewers:
  - "@s-urbaniak"
  - "@bparees"
  - "@enj"
  - "@sttts"
approvers:
  - "@sttts"
creation-date: 2019-09-18
last-updated: 2019-09-18
status: implementable
see-also:
replaces:
superseded-by:
---

# OAuth Proxy Proxy Support

We should create zero-config method of enabling proxy-support for the oauth-proxy to enable non-expert teams to be secure
when proxies are configured in a cluster.
The `oauth-proxy` is used to front many webservers to provide authentication and authorization protection without directly
building it into their binaries. This is common, easy, and used by people without a deep understand of how OpenShift is
wired together.  They are not likely to know how to reactively wire proxy configuration and ca-bundles into their 
side-cars.  Because it is re-used by directly embedding the `oauth-proxy` into a `PodSpec`, any changes to the way it is
invoked will cause adoption difficulties.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions

1. Can we expose proxy.config.openshift.io to system:authenticated by default.  Some cluster-admins may place basic-auth
 credentials here because we provide no alternative.  This may make the configuration values suddenly sensitive and there
 will be desire to hide that from workloads and other users.

## Summary

In order to function, the `oauth-proxy`
 1. Gets the oauth-server location using the kube-apiserver's internal IP: `https://172.30.0.1/.well-known/oauth-authorization-server`
 2. Uses the result to find the token endpoint.
 3. Hits the *external* token endpoint `https://oauth-openshift.apps.<cluster-dns-prefix>.openshift.com/oauth/token`.  

This flow requires hitting a potentially external router, which requires honoring proxy settings.  `/.well-known/oauth-authorization-server`
implements [RFC 8414](https://tools.ietf.org/html/rfc8414) which does not allow specifying multiple possible URLs.

## Motivation

See summary in first section.

### Goals

1. Zero-config proxy support that dynamically updates as proxy settings change.
2. Support for all proxy modes, including MITM.

### Non-Goals

1. Making generic code to rewrite proxy transport configuration.

## Proposal

The `oauth-proxy` can be written to use targeted list/watch against the proxy.config.openshift.io and 
`oc get -n openshift-config-managed configmaps/trusted-ca-bundle` to build a proxy transport.  Using standard informers with single item
watches allows RBAC to only expose GET for individual items.  It also means that an efficient observed update and `sync.Atomic` mechanism can be built
to ensure performant behavior.

This is an intricate thing to write, so we avoided it for every other proxy component, but this one is a component that is
re-used by numerous external teams by directly embedding it into a pod spec.  This changes the tradeoff, so it becomes worthwhile
to write this code.

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

1. If HTTP env vars are set, the dynamic proxy-support code must be disabled.  The user is in charge of what the `oauth-proxy`
 does in these cases.
2. The list/watch driving the informers must be focused to a single item, so the RBAC special case for single item watch
 takes effect.
3. To be performant, the proxy and TLS configuration should be cached in something like a `sync.Atomic`.


### Risks and Mitigations

This code is intricate to write, but luckily our monitoring stack already relies on this.  If their tests pass when they
update to use the feature, the feature works.

## Design Details

### Test Plan

We would get @s-urbaniak to update the monitoring stack to avoid setting the HTTP env vars and ensure that the proxy e2e test
still works.

### Graduation Criteria

### Upgrade / Downgrade Strategy

None, fully compatible.

### Version Skew Strategy

None, fully compatible.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

1. Using the internal service DNS name. The .well-known endpoint is public and conformant to a [RFC 8414](https://tools.ietf.org/html/rfc8414).
 It can only include a single URL, which means it must
 be public so that external clients like `oc`, the webconsole, and anything else can successfully use it.  Using an internal
 service name would also preclude the kube-apiserver from ever using an external oauth provider like keycloak.

2. Setting HTTP env vars.  We are doing this is to deliver a mostly working 4.2.0, but has undesireable, long-term characteristics.
 Setting env vars on the `oauth-proxy` container is possible, but it requires informing every team using the `oauth-proxy`
 that they need to change.  If they are set, we should honor them, but communicating the scope of this change is difficult; 
 we can do it for the few in our payload before we ship 4.2.0, but the fanout afterwards is hard.

3. User responsible for MITM proxy.  We don't want to attempt this in 4.2.0.
 MITM proxies require setting ca-bundles. These come from configmaps which the pod author of `oauth-proxy` also has to know to create.
 Based on the apiserver team's experience, wiring these ca-bundle requires changes to the `oauth-proxy` to react to changes
 to the configmaps (so you pick up changes) and bash glue to move the ca-bundle content to the correct location.
 Communicating these restrictions and capabilities is almost more work than making the `oauth-proxy` reactive with a custom transport.
 
4. One suggestion from the PR.
    isn't there another alternative (implementation alternative anyway) in which the oauth-proxy

    1. launches itself in an init mode which reads the proxy config (and CA) on startup (in its own gocode/main)
    2. writes the CA to /etc/pki/......
    3. execs itself w/ some new arg(not init mode) (because it needs to reinit to load the updated CAs in the TLS libs)
    4. watches the proxy config/CA for changes and on changes, kills itself so it goes to (1)?
    
    I don't think we'll pursue something like that. It sounds more fragile to do seamlessly inside of containers you do not own,
    with valid healthchecks you didn't write. This binary is used as part of a user-owned container spec.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
