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
last-updated: 2021-07-14
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
see-also:
  - https://github.com/openshift/enhancements/pull/461
replaces:
superseded-by:
---

# Request Timeout Configuration

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document proposes the addition of several API fields into the ingress
controller API, allowing the cluster admin to adjust how long routers will hold
open connections under various circumstances.

## Motivation

Customers need the ability to adjust how long HAProxy holds connections open,
either by extending the timeout to accommodate slower backends or clients, or by
shortening the timeout, allowing connections to be closed more aggressively.

### Goals

Allow admins to configure the following HAProxy connection timeout parameters:
- timeout client
- timeout client-fin
- timeout server
- timeout server-fin
- timeout tunnel
- tcp-request inspect-delay

### Non-Goals

- Allow admins to configure every HAProxy timeout parameter
- Allow users to configure timeouts per route

## Proposal

Add the following configurable variables to the ingresscontroller API:
```yaml
spec:
  tuningOptions:
    clientTimeout: "30s"
    clientFinTimeout: "1s"
    serverTimeout: "30s"
    serverFinTimeout: "1s"
	tunnelTimeout: "1h"
    tlsInspectDelay: "10s"
```

All variables expect time values. If no units are specified, the router assumes
milliseconds.

### User Stories

#### User Story 1

My application starts processing requests from clients, but the connection is
getting closed before it can respond.

I set `spec.tuningOptions.serverTimeout` in the ingresscontroller API to a
higher value to accommodate the slow response from the server.

#### User Story 2

The router has many connections open because an application running on my
cluster doesn't close connections properly.

I set `spec.tuningOptions.serverTimeout` and
`spec.tuningOptions.serverFinTimeout` in the ingresscontroller API to a lower
value, forcing those connections to close sooner if my application stops
responding to them.

### Implementation Details/Notes/Constraints

#### Router Image Environment Variables

`timeout client`, `timeout client-fin`, `timeout server`, and `timeout
server-fin` `timeout tunnel` were all configurable in 3.X, and the router image
still supports configuring them from the environment variables
`ROUTER_DEFAULT_CLIENT_TIMEOUT`, `ROUTER_CLIENT_FIN_TIMEOUT`,
`ROUTER_DEFAULT_SERVER_TIMEOUT`, `ROUTER_DEFAULT_SERVER_FIN_TIMEOUT`, and
`ROUTER_DEFAULT_TUNNEL_TIMEOUT` respectively.

A new environment variable will be defined, `ROUTER_INSPECT_DELAY`, which will
control the `tcp-request inspect-delay` variable.

All the timeout environment variables expect values to follow the HAProxy time
format. The format is an integer optionally followed by a time unit. If no unit
is specified, the value represents the delay in milliseconds. The full
description of the time format can be found in [section 2.4. of the HAProxy 2.2
documentation](https://github.com/haproxy/haproxy/blob/v2.2.0/doc/configuration.txt).

#### Time Format Validation

The format specified in [section 2.4. of the HAProxy 2.2
documentation](https://github.com/haproxy/haproxy/blob/v2.2.0/doc/configuration.txt)
will be enforced when the ingresscontroller config is admitted using the
regular expression `[0-9]+(?:[um]?s|[mhd])?`. The regular expression should
accept strings containing one or more digits, optionally followed by one of the
following time units:
- us
- ms
- s
- m
- h
- d

In addition, HAProxy only allows a maximum timeout of 2,147,483,647ms (about 24
days, 20 hours). If a time greater than the maximum is specified for any of the
API fields added in this proposal, the timeout will be set to the maximum
allowed value, and an error message will be logged by the ingress operator.

### Risks and Mitigations

Setting timeout values in the ingress controller will only affect timeouts
caused by ingress. Other hops in the connection, such as the backend or the
cloud LB, may still cause connections to time out even if the fields added in
this proposal are set sufficiently high.

## Design Details

### Open Questions

None

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
	tunnelTimeout: "30m"
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
		- name: "ROUTER_DEFAULT_TUNNEL_TIMEOUT"
		  value: "30m"
        - name: "ROUTER_TLS_INSPECT_DELAY"
          value: "5s"
        ...
```

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

#### Upgrade Strategy

When upgrading from 4.8 or earlier to 4.9, the new API fields will remain
unset, causing the existing defaults to be used.

#### Downgrade Strategy

When downgrading from 4.9 to 4.8 or earlier, the specified timeout values will
be discarded, and the previous defaults will be used.

### Version Skew Strategy

In order to take full advantage of the new API fields, the ingress controller
API, the ingress operator, and the router image must all be upgraded.

If the ingress controller API is behind, none of the new fields can be set,
forcing the operator to use their default values.

If the ingress operator is behind, the operator will ignore the new API fields.
The new timeout variables will remain set to the fixed values used in 4.8,
which are the same as the new default values.

If the router image is behind, all environment variables will be set for router
pods, but `tlsInspectDelay`/`ROUTER_TLS_INSPECT_DELAY` will not be recognized
by the HAProxy config generator. This will allow `clientTimeout`,
`clientFinTimeout`, `serverTimeout`, and `serverFinTimeout` to be set, but
`tlsInspectDelay` will effectively remain at the default value (5s) regardless
of the value in `tlsInspectDelay`.

## Implementation History

`ROUTER_DEFAULT_CLIENT_TIMEOUT`, `ROUTER_CLIENT_FIN_TIMEOUT`,
`ROUTER_DEFAULT_SERVER_TIMEOUT`, `ROUTER_DEFAULT_SERVER_FIN_TIMEOUT`, and
`ROUTER_DEFAULT_TUNNEL_TIMEOUT` were previously configurable in 3.x

## Drawbacks

- Setting the timeout higher may cause some dead connections to be kept open
  for longer, and would add to the memory footprint of the router
- Setting the timeout too low can cause connection closure before the server or
  client has enough time to respond

## Alternatives

N/A
