---
title: route-rate-limit-http-429
authors:
  - "@sanjaytripathi97"
reviewers:
  -
  -
  -
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-08-31
last-updated: 2026-08-31
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/RFE-2924
see-also:
  - https://github.com/openshift/router/pull/823
replaces:
superseded-by:
---

# Return HTTP 429 When Route HTTP Rate Limit Is Exceeded

## Summary

When a client exceeds the HTTP request rate configured by the existing
`haproxy.router.openshift.io/rate-limit-connections.rate-http` Route annotation,
the OpenShift HAProxy router currently rejects the connection at the TCP layer.
Clients observe an empty reply or connection reset instead of a standard HTTP
status code. This enhancement proposes returning **HTTP 429 Too Many Requests**
when the HTTP request rate limit is exceeded on HTTP-mode Route backends.

## Motivation

[RFE-2924](https://issues.redhat.com/browse/RFE-2924) requests that rate-limited
HTTP clients receive a proper HTTP status code instead of a silent connection
drop. Customers report that applications and portals cannot display meaningful
error messages when limits are reached.

Today, when `haproxy.router.openshift.io/rate-limit-connections` and
`haproxy.router.openshift.io/rate-limit-connections.rate-http` are set on a
Route, the router generates HAProxy configuration similar to:

```text
tcp-request content reject if { src_http_req_rate ge <N> }
```

That directive rejects the connection before an HTTP response can be sent.
Clients using tools such as `curl` see `Empty reply from server` rather than
`HTTP/1.1 429 Too Many Requests`.

[RFC 6585 section 4](https://www.rfc-editor.org/rfc/rfc6585#section-4) defines
the 429 status code for rate limiting. Returning 429 allows clients and
middleware to detect rate limiting and react appropriately (for example,
backoff or user-facing error messages).

A prototype implementation exists in
[openshift/router#823](https://github.com/openshift/router/pull/823).

### User Stories

* As an application developer, I want HTTP clients to receive HTTP 429 when
  they exceed the configured per-route HTTP request rate limit, so that my
  application or API gateway can display a meaningful error message.
* As a platform administrator, I want rate-limited HTTP requests to return a
  standard status code, so that monitoring and client libraries can
  distinguish rate limiting from network failures.
* As a support engineer, I want predictable HTTP responses when route rate
  limits are triggered, so that customer issues are easier to diagnose.

### Goals

1. When `haproxy.router.openshift.io/rate-limit-connections.rate-http` is
   exceeded on an HTTP-mode Route backend, return **HTTP 429**.
2. Preserve existing TCP-level rate limiting behavior for
   `haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp` and
   `haproxy.router.openshift.io/rate-limit-connections.rate-tcp`.
3. Add automated test coverage for the generated HAProxy configuration.

### Non-Goals

1. Introducing a new Route annotation key.
2. Allowing users to configure a custom HTTP status code (for example, 503).
3. Returning an HTTP status for TCP connection limits (`concurrent-tcp`,
   `rate-tcp`), where no HTTP response is possible.
4. Adding `rate-http` support to passthrough (`be_tcp`) backends.
5. Coordinating rate limits across multiple router replicas.

## Proposal

Change the HAProxy configuration generated for HTTP-mode Route backends
(plain HTTP, edge-terminated TLS, and re-encrypt) so that exceeding the
`rate-http` threshold uses:

```text
http-request deny deny_status 429 if { src_http_req_rate ge <N> }
```

instead of:

```text
tcp-request content reject if { src_http_req_rate ge <N> }
```

The stick-table and tracking configuration remain unchanged:

```text
stick-table type ip size 100k expire 30s store conn_cur,conn_rate(3s),http_req_rate(10s)
tcp-request content track-sc2 src
```

`concurrent-tcp` and `rate-tcp` continue to use `tcp-request content reject`.

### Workflow Description

1. A cluster administrator or developer creates or updates a Route with:

   ```yaml
   metadata:
     annotations:
       haproxy.router.openshift.io/rate-limit-connections: "true"
       haproxy.router.openshift.io/rate-limit-connections.rate-http: "40"
   ```

2. The router watches the Route and regenerates `haproxy.config`.
3. A client sends HTTP requests to the Route hostname.
4. While the per-source HTTP request rate is below the configured threshold,
   requests are forwarded normally.
5. When the per-source HTTP request rate meets or exceeds the threshold, the
   router responds with **HTTP 429** and does not forward the request to the
   backend.

### API Extensions

None for the proposed implementation.

This enhancement does **not** add new Route annotations or API fields. It
changes the behavior of the existing, documented
`haproxy.router.openshift.io/rate-limit-connections.rate-http` annotation.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. The change is in the router operand HAProxy template
and applies to guest cluster IngressControllers in the same way as standalone
clusters.

#### Standalone Clusters

Relevant. This is the primary deployment model for the change.

#### Single-node Deployments or MicroShift

The change applies wherever the HAProxy-based router is used. No additional
resource consumption is expected beyond the existing stick-table rate limiting.

#### OpenShift Kubernetes Engine

No OKE-specific exclusions are anticipated. Route rate-limit annotations are
available wherever the HAProxy router is supported.

### Implementation Details/Notes/Constraints

* **Repository:** `openshift/router`
* **File:** `images/router/haproxy/conf/haproxy-config.template`
* **Scope:** HTTP-mode backends only (`mode http`). Passthrough backends do not
  evaluate HTTP request rates and are unchanged.
* **Tests:** Add `TestConfigTemplate` cases in `pkg/router/router_test.go` to
  verify the generated `http-request deny deny_status 429` directive.
* **Documentation:** Update `openshift/openshift-docs` route annotation
  documentation to state that exceeding `rate-http` returns HTTP 429.

Prototype PR: https://github.com/openshift/router/pull/823

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Behavior change for existing users relying on silent drops | Document the change in release notes and route annotation docs; discuss whether a feature gate is required |
| Clients that do not handle 429 | 429 is the standard status for rate limiting per RFC 6585 |
| Ordering of HAProxy rules | Place `http-request deny` after stick-table tracking and alongside existing `http-request` rules in the template |

### Drawbacks

* Clients that previously interpreted a connection drop as a network error will
  now receive HTTP 429. This is the intended improvement but is still a behavior
  change.
* TCP connection limits continue to drop connections without an HTTP status;
  only HTTP request rate limiting is improved.

## Alternatives (Not Implemented)

### Alternative 1: New Route API field

Add a first-class Route API field (for example,
`spec.rateLimit.httpResponseCode`) instead of changing annotation behavior.

**Not selected initially** because the existing `rate-http` annotation already
documents HTTP request rate limiting. The immediate user need is to make that
limit return a proper HTTP status. A new API field could be considered in a
follow-up if reviewers prefer that approach for long-term configuration.

### Alternative 2: Keep `tcp-request content reject`

**Rejected.** This is the current behavior and does not satisfy RFE-2924.

### Alternative 3: Custom error page via `errorfile 429`

**Deferred.** Returning `deny_status 429` is sufficient for the status code.
Custom response bodies can be a follow-up enhancement.

## Open Questions

1. Is changing the behavior of the existing `rate-http` annotation acceptable,
   or should this be implemented as a new Route API field?
2. Is a feature gate required before enabling this behavior change by default?
3. What OpenShift release should target this change (for example, 4.20 / 5.0)?

## Test Plan

* **Unit tests:** Extend `TestConfigTemplate` in `pkg/router/router_test.go`
  to assert `http-request deny deny_status 429` for edge and insecure backends
  when `rate-limit-connections` and `rate-http` annotations are set.
* **Manual test:**
  1. Create a Route with `rate-limit-connections=true` and a low `rate-http`
     value (for example, `5`).
  2. Send requests exceeding the limit with `curl -v`.
  3. Verify the response status is `429` and not an empty reply.
  4. Verify `concurrent-tcp` and `rate-tcp` limits still reject at the TCP
     layer without an HTTP status.

## Graduation Criteria

**Note:** Section not required until targeted at a release.

If implemented as a behavior change to an existing annotation with no feature
gate:

* Unit tests pass in `openshift/router` CI.
* Manual verification on a test cluster.
* Documentation updated in `openshift/openshift-docs`.

If a feature gate is required per OpenShift policy:

* Feature gate added in `openshift/api` and wired through the ingress/router
  stack before graduation to Default.

## Upgrade / Downgrade Strategy

No user action is required on upgrade. Routes that already use
`rate-limit-connections.rate-http` will automatically receive HTTP 429
responses instead of silent TCP rejects after the router image is updated.

Downgrade restores the previous silent-drop behavior.

## Version Skew Strategy

During a rolling router deployment, old and new router pods may behave
differently for rate-limited requests until all pods are updated. This is
acceptable for the duration of the rollout.

## Operational Aspects of API Extensions

Not applicable. No API extensions are proposed.

## Support Procedures

* **Symptom:** Clients receive HTTP 429 from a Route.
* **Check:** Inspect Route annotations for
  `haproxy.router.openshift.io/rate-limit-connections.rate-http`.
* **Resolution:** Reduce request rate, raise the limit, or remove the annotation
  if rate limiting is not desired.

## Infrastructure Needed

None.
