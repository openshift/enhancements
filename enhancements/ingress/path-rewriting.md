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
For instance with the nginx controller adding the following annotation does
the trick:

`traefik.ingress.kubernetes.io/rewrite-target: /`
`nginx.ingress.kubernetes.io/rewrite-target: /$1`

We would like to support these annotations for Routes as well.
We don't need support of regex path matching as `Route.spec.path` is not using regex as well.

**Route-specific annotations**

| Variable	| Description | Environment variable used as default |
| --------- | ----------- | ------------------------------------ |
| router.openshift.io/rewrite-target | Sets the rewrite path of the request on the backend.| |

**Route-rewrite examples**

| Route.spec.path	| Request path | Rewrite target | Output |
| --------------- | ----------- | --------------- | ------ |
| /foo | /foo | / | / |
| /foo | /foo/ | / | / |
| /foo | /foo/bar | / | /bar |
| /foo | /foo/bar/ | / | /bar/ |
| /foo | /foo | /bar | /bar |
| /foo | /foo/ | /bar | /bar/ |
| /foo | /foo/bar | /baz | /baz/bar |
| /foo | /foo/bar/ | /baz | /baz/bar |
| /foo | /foo | /baz | /baz/bar |
| /foo/ | /foo | / | Application is not available (Error 404) |
| /foo/ | /foo/ | / | / |
| /foo/ | /foo/bar | / | /bar |

**Annotation example**

```
kind: Route
metadata:
  name: example
  annotations:
    haproxy.router.openshift.io/rewrite-target: /
```

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
