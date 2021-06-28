---
title: request-timeout-configuration
authors:
  - "@rfredette"
reviewers:
  - "@miciah"
  - "@danehans"
  - "@frobware"
  - "@sgreene570"
  - "@knobunc"
  - "@miheer"
  - "@candita"
  - "@alebedev87"
approvers:
  - "@miciah"
  - "@frobware"
  - "@knobunc"
creation-date: 2021-06-17
last-updated: 2021-06-28
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
see-also:
replaces:
superseded-by:
---

# Request Timeout Configuration

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document proposes the addition of several API fields into the ingress
controller API, allowing the cluster admin to adjust how long routers will hold
open connections that are awaiting a response.

## Motivation

Customers need the ability to adjust how long HAProxy holds connections open,
either by extending the timeout to accomodate slower backends or clients, or by
shortening the timeout, allowing connections to be closed more aggressively.

### Goals

Allow admins to configure various HAProxy connection timeout parameters

### Non-Goals

TODO

## Proposal

Add the following configurable variables to the ingresscontroller API:
```yaml
spec:
  tuningOptions:
    clientTimeout: "30s"
    clientFinTimeout: "1s"
    serverTimeout: "30s"
    serverFinTimeout: "1s"
    tlsInspectDelay: "10s"
```

All variables expect time values. If no units are specified, the router assumes
milliseconds.

### User Stories

TODO

### Implementation Details/Notes/Constraints

`timeout client`, `timeout client-fin`, `timeout server`, and `timeout
server-fin` were all configurable in 3.X, and the router image still supports
configuring them from the environment variables
`ROUTER_DEFAULT_CLIENT_TIMEOUT`, `ROUTER_CLIENT_FIN_TIMEOUT`,
`ROUTER_DEFAULT_SERVER_TIMEOUT`, and `ROUTER_DEFAULT_SERVER_FIN_TIMEOUT`,
respectively.

A new environment variable will be defined, `ROUTER_INSPECT_DELAY`, which will
control the `tcp-request inspect-delay` variable. If no unit is specified, the
value represents the delay in milliseconds, but time units can be specified,
following the time format outlined in [section 2.4. of the HAProxy 2.2
documentation](https://github.com/haproxy/haproxy/blob/v2.2.0/doc/configuration.txt).

### Risks and Mitigations

TODO

## Design Details

### Open Questions

- How do these timeout variables affect websocket connections (if at all)?

### Test Plan

#### Test 1
- set the following values:
```yaml
spec:
  tuningOptions:
    clientTimeout: "45s"
    clientFinTimeout: "3s"
    serverTimeout: "60s"
    serverFinTimeout: "4s"
    tlsInspectDelay: "5s"
```
- verify that the router deployment contains the following values:
```yaml
spec:
  template:
    spec:
      containers:
      - ...
        env:
        - name: "ROUTER_DEFAULT_CLIENT_TIMEOUT"
          value: "45s"
        - name: "ROUTER_CLIENT_FIN_TIMEOUT"
          value: "3s"
        - name: "ROUTER_DEFAULT_SERVER_TIMEOUT"
          value: "60s"
        - name: "ROUTER_DEFAULT_SERVER_FIN_TIMEOUT"
          value: "4s"
        - name: "ROUTER_TLS_INSPECT_DELAY"
          value: "5s"
        ...
```

### Graduation Criteria

N/A?

### Upgrade / Downgrade Strategy

#### Upgrade Strategy

When upgrading from 4.8 or earlier to 4.9, the new API fields will remain
unset, causing the existing defaults to be used.

#### Downgrade Strategy

TODO

### Version Skew Strategy

TODO

## Implementation History

`ROUTER_DEFAULT_CLIENT_TIMEOUT`, `ROUTER_CLIENT_FIN_TIMEOUT`,
`ROUTER_DEFAULT_SERVER_TIMEOUT`, and `ROUTER_DEFAULT_SERVER_FIN_TIMEOUT` were
previously configurable in 3.x

## Drawbacks

- Setting the timeout higher may cause some dead connections to be kept open
  for longer, and would add to the memory footprint of the router
- Setting the timeout too low can cause connection closure before the server or
  client has enough time to respond

## Alternatives

N/A
