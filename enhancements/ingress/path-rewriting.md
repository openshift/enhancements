---
title: path-rewriting-support
authors:
  - "@l0rd"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-04-17
last-updated: 2020-04-17
status: provisional
---

# Routes Path Rewriting Support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Some Ingress controller implementations support doing path rewriting. This is
used to modify the path in the HTTP request as it gets proxied:

`http://example.com/path` ---> `http://example.com/`

This is a proposal to support path rewriting for Routes as well.

## Motivation

To use non wildcard SSL certificates we want to direct traffic based on the path
rather then hostname (e.g. `http://example.com/app` instead of
`http://app.example.com/`).

We want to remove the path from the HTTP requests (`http://example.com/path` -->
`http://example.com/`) because the upstream services are serving on /.

There is an open issue about that: <https://github.com/openshift/origin/issues/20474>

## Proposal

### User Stories

[CodeReady Workspaces](https://developers.redhat.com/products/codeready-workspaces/overview)
is a controller that dynamically creates developers workspaces (web IDEs). A
workspace is a web applications with N microservices exposing N routes (N
depends on the workspace but is usually ~5).

When a developer starts his workspace, N routes are created. When he stops it
the N routes are deleted.

**Wildcard certificates problems**

By default workspaces routes have URLs with random subdomains like
https://<random-part>.example.com. We create a lots of these routes and
that's not an issue if the customer is allowed to use a wildcard TLS
certificate (https://*.example.com).

But not all customers can use wildcard certs. For that reason we now support
fixed domain scenario using URLs with a fixed hostname and a variable path:
https://example.com/<random-part>/. We have been able to implement this
single-host option because upstream Ingress controllers support path rewriting.
For instance with the traefik and nginx controllers adding the following annotations does
the trick:

`traefik.ingress.kubernetes.io/rewrite-target: /`
`nginx.ingress.kubernetes.io/rewrite-target: /$1`

We would like to support these annotations for Routes as well.
We don't need support of regex path matching as `Route.spec.path` is not using regex as well.

**Rewrite Target documentation**
Documentation proposal at https://github.com/openshift/openshift-docs/pull/22021

**Annotation example**

```
kind: Route
metadata:
  name: example
  annotations:
    haproxy.router.openshift.io/rewrite-target: /
```

## Design Details

### Test Plan

Considered the multiple supported examples of rewrite-path we are going 
to add some unit test to verify them. We are NOT going to include any particular 
e2e test.

## Implementation History

- 2020-05-12 Draft PRs submitted on https://github.com/openshift/router/pull/129 and 
https://github.com/openshift/openshift-docs#22021

## Alternatives

We can fix this on the CRW side: we can deploy a reverse proxy per every CRW
installation (https://crw.example.com) and use it to route requests coming from
users browsers.

However that means shifting a responsibility that is an OpenShift responsibility
(creating endpoints) to CRW (and that's not its purpose). That also means missing
an opportunity to help those OpenShift customers that, as we do, are requesting
path rewriting.

I think that supporting path rewriting on OpenShift Route controller is a better
option. If OpenShift teams do not have the bandwidth to work on it we (CRW team)
can work on the proposal and implementation.
