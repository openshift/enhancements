---
title: oauth-proxy-shared-configuration
authors:
  - "@deads2k"
reviewers:
  - "@standa"
approvers:
  - "@sttts"
creation-date: 2020-05-26
last-updated: 2020-05-26
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - https://github.com/openshift/enhancements/pull/22  
replaces:
superseded-by:
---

# oauth-proxy Shared Configuration

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

The [oauth-proxy](https://github.com/openshift/oauth-proxy) is a commonly used proxy which allows a naive application to 
make use of in-cluster authentication and authorization to protect itself on an openshift platform without changing
the application itself.
This effectively makes the application appear as an extension to our platform, examples include: logging kibana, 
monitoring grafana, etc.
There are a few configuration values that are considered as related to platform configuration or security that the oauth-proxy
could auto-detect.

## Motivation

If a customer wants to set a value on all oauth-proxy instances such as proxy configuration or `--cookie-expire` duration,
there is no unified way to set that across all instances of the oauth-proxy.
Having cluster-admins know every application using the oauth-proxy and figuring out how to configure them all seems 
abusive.

### Goals

1. Make it easy to describe configuration for all oauth-proxy binaries.

### Non-Goals


## Proposal

We will create an oauthproxies.config.openshift.io to ensure that read access can be widely granted without any risk of
accidentally allowing reading of more private data (versus attaching to oauth.config.openshift.io for instance).
Its .spec stanza contain typed fields for certain flags.

The following flags for oauth-proxy are good targets for being consistent for all oauth-proxy instances in the cluster.
If the oauth-proxy does not have one of these flags explicitly set, it will attempt to read the value from oauthproxies.config.openshift.io.
1. -cookie-expire
2. -cookie-refresh
3. logout-url (not yet present).  If created, it will have a spec and a status.
   If the spec is empty, the status will be filled in based on the console's logout URL.
4. http proxy settings.  This will have a spec and a status.
   If the values in proxies.config.openshift.io .status do not have credentials in them, we can automatically copy them
   to the https proxy status settings for oauthproxies.
   If they do have credentials in them, we will not set the value by default.
   The spec will always have precedence.
 
### Risks and Mitigations

1. HTTP_PROXY env vars can include basic auth credentials.
   We will only auto-copy HTTP_PROXY if there are no basic auth credentials.

## Design Details

### Test Plan

### Upgrade / Downgrade Strategy

Image streams have to be correct.

### Version Skew Strategy

oauth-proxy users can only rely on this feature in the openshift release it is delivered in.
If they need to work on a prior level of openshift, they will have to set the values on the oauth-proxy binary.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.
