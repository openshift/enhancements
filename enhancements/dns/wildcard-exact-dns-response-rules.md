---
title: wildcard-exact-dns-response-rules
authors:
  - "@aswinsuryan"
reviewers:
  - "@alebedev87, for CoreDNS template-plugin design and consistency with NE-2118"
  - "@JoelSpeed, for API conventions and CRD validation patterns"
  - "@Thealisyed, for caching/negative-caching and MicroShift considerations"
approvers:
  - "@candita"
api-approvers:
  - TBD
creation-date: 2026-10-21
last-updated: 2026-10-21
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/NE-2930
see-also:
  - "/enhancements/dns/plugins.md"
  - "https://github.com/openshift/enhancements/pull/1936"
replaces:
superseded-by:
---

# Wildcard and Exact-Name DNS Response Rules

## Summary

This enhancement adds a declarative, validated way for a cluster administrator
to make operator-managed CoreDNS answer a specific set of DNS names with a
custom A, CNAME, or NXDOMAIN response, without editing the generated Corefile,
setting the DNS Operator to `Unmanaged`, or operating a separate DNS server.
It reuses the `DNSTemplatePlugin` feature gate and builds on the CoreDNS
`template` plugin integration proposed in NE-2118, extending it to cover
wildcard A/CNAME answers and exact-name NXDOMAIN, which are explicitly out of
scope for NE-2118's initial IN/AAAA/NOERROR filtering use case.

## Motivation

Customers need to publish wildcard DNS records (most commonly for a custom
application domain) that resolve through the cluster's own DNS, but today the
only ways to do this are unsupported: hand-editing the Corefile (overwritten
on reconcile), setting the DNS Operator `Unmanaged` (opts out of operator
support), or running an entirely separate DNS server. This must also survive
upgrades, since the DNS Operator regenerates its configuration from the API
on every reconcile.

### User Stories

* As a cluster administrator, I want to configure a wildcard A record (e.g.
  `*.apps.customer.com`) with one or more IPv4 addresses, so that I can run an
  application that depends on wildcard DNS without leaving the DNS Operator
  managed or standing up external DNS infrastructure.

* As a cluster administrator, I want to configure a wildcard CNAME record for
  a selected name pattern, so that I can alias a customer domain to an
  existing target that is resolved normally.

* As a cluster administrator, I want to make a single, explicitly named host
  return NXDOMAIN, so that I can signal a name does not exist without that
  rule unintentionally applying to an entire subdomain.

* As a cluster administrator, I want invalid or conflicting rules to be
  rejected with a clear error at submission time, so that a mistake in my
  configuration cannot degrade or break CoreDNS for the whole cluster.

* As an SRE, I want queries that don't match any configured rule to follow
  their existing resolution path unchanged (service discovery, ingress names,
  configured forwarding), so that this feature cannot regress unrelated DNS
  behavior.

### Goals

* Let an administrator declare wildcard A, wildcard CNAME, and exact-name
  NXDOMAIN rules through the DNS Operator API, reusing the `DNSTemplatePlugin`
  feature gate and the CoreDNS `template` plugin mechanism from NE-2118.
* Define unambiguous precedence: an exact name always wins over a wildcard;
  among wildcards, the most specific wins; overlapping/conflicting rules are
  rejected at submission rather than silently resolved.
* Validate rules both at submission (schema/admission) and before the
  generated Corefile is applied, so an invalid rule cannot make CoreDNS
  unusable; on an invalid update, keep serving the last-known-good Corefile
  and report a clear status condition.
* Preserve existing resolution behavior for anything not covered by a rule:
  cluster service/ingress discovery, configured `forwardPlugin` zones, and
  other query types for a name that only has a rule for one record type.
* Make the configuration and its effects survive an OpenShift upgrade without
  administrator intervention.

### Non-Goals

* A general-purpose custom DNS server or arbitrary record types/regex
  matching — wildcards are limited to replacing exactly one leading name
  segment (e.g. `*.example.com`); administrators specify names, not regular
  expressions.
* IPv6/AAAA record responses — that is NE-2118's scope; this enhancement
  intentionally does not duplicate or modify that work's AAAA/NOERROR
  behavior.
* ARO's additional custom search-domain handling (RFE-6814) — tracked
  separately, not an acceptance criterion here.
* Azure's installer/custom-DNS path (OCPSTRAT-991) — unrelated mechanism, not
  evidence this feature already exists.
* Response generation for record types or response codes beyond A, CNAME, and
  exact-name NXDOMAIN.

## Proposal

### Workflow Description

**cluster administrator** is a human user with permission to edit the
cluster-scoped `DNS/default` resource.

1. The administrator adds one entry to `spec.responseRules` on `DNS/default`
   naming a rule, a query name (exact or single-segment wildcard), a TTL, and
   exactly one action (`a`, `cname`, or `nxdomain`).
2. On submission, the API server validates the rule's shape: name pattern,
   exactly-one-action, address/target format, and TTL bounds.
3. The DNS Operator reconciles `DNS/default`, performing validation that
   requires cluster context: protected-zone rejection (`cluster.local`,
   reverse zones, API/ingress domains), duplicate/conflicting rule detection
   across the whole list, and precedence computation.
4. If valid, the operator renders the corresponding CoreDNS `template` plugin
   stanzas into the default server block of the generated Corefile and
   confirms the new Corefile loads before applying it.
5. If invalid at either stage, the operator leaves the previously applied
   Corefile in place and sets a status condition on `DNS/default` describing
   which rule is invalid and why.
6. CoreDNS answers matching queries per the configured rule; queries outside
   any rule continue through normal resolution (`kubernetes` plugin,
   `forwardPlugin` zones, upstream resolvers).
7. On upgrade, the operator regenerates the Corefile from the same
   `spec.responseRules` on the new release, with no administrator action
   required.

### API Extensions

This proposes a new, optional, feature-gated field on the existing DNS
Operator API (`operator.openshift.io/v1`, `DNSSpec`) rather than a new custom
resource:

```go
// responseRules is an optional list of rules that configure CoreDNS to
// return custom A, CNAME, or NXDOMAIN responses for specific DNS names.
// Rules are evaluated independently; an exact-name rule always takes
// precedence over a wildcard rule for the same query, and among wildcard
// rules the most specific match wins. Queries that do not match any rule
// continue through normal CoreDNS resolution.
//
// +optional
// +listType=map
// +listMapKey=name
// +kubebuilder:validation:MaxItems=100
// +openshift:enable:FeatureGate=DNSTemplatePlugin
ResponseRules []DNSResponseRule `json:"responseRules,omitempty"`

type DNSResponseRule struct {
    // name uniquely identifies this rule within the list.
    // +required
    Name string `json:"name"`

    // query selects the DNS name this rule answers: an exact name, or a
    // name with exactly one leading wildcard label (e.g. "*.example.com").
    // +required
    Query DNSResponseQuery `json:"query"`

    // ttlSeconds is the TTL, in seconds, included in synthesized answers.
    // +optional, default 60, range 30-3600
    TTLSeconds int32 `json:"ttlSeconds,omitempty"`

    // action specifies how matching queries are answered. Exactly one of
    // a, cname, or nxdomain must be set; nxdomain is only valid when
    // query.name is an exact name, not a wildcard.
    // +required
    Action DNSResponseAction `json:"action"`
}
```

This document models the API at a high level only; the detailed schema
(field naming, discriminated-union/CEL validation pattern, enum reuse from
NE-2118's `QueryType`/`QueryClass`/`ResponseCode` types) is left to the
`openshift/api` PR, reviewed by API approvers in addition to this document,
per the enhancement template's guidance to avoid duplicating review effort.

This enhancement does **not** modify NE-2118's `DNSTemplate` field or its
behavior.

### Relationship to NE-2118

NE-2118 ([PR #1936](https://github.com/openshift/enhancements/pull/1936),
not yet merged) added the `DNSTemplatePlugin` feature gate and a `template`
field scoped to IN/AAAA/NOERROR, applied to **all** CoreDNS server blocks.
This enhancement reuses the same feature gate and the same underlying
CoreDNS `template` plugin mechanism, but makes two deliberate departures from
decisions already made in NE-2118's review, called out explicitly here so
they can be discussed rather than discovered:

* **Server scope.** NE-2118 applies its template configuration to all server
  blocks, reasoning that AAAA filtering is a blanket, cluster-wide concern.
  This enhancement instead applies response rules to the **default server
  block only**. Wildcard/exact response rules are narrower in purpose than
  AAAA filtering: they answer a specific, administrator-chosen customer
  domain and must not silently override or interact with custom
  `forwardPlugin` zones an administrator has configured for a different
  purpose.
* **NXDOMAIN.** NE-2118 originally considered and then explicitly removed
  NXDOMAIN from scope, pending "a strong reason" to add it. The strong reason
  for this enhancement is RFE-3603/RFE-6814's requirement to make a specific,
  explicitly named host return NXDOMAIN. The risk NE-2118's reviewers raised
  (NXDOMAIN on a name a client interprets as "stop trying other record
  types/search domains") is mitigated here by restricting NXDOMAIN to exact
  names only — never a wildcard — so it cannot unintentionally affect queries
  beyond the single named host.

This enhancement does not depend on PR #1936 merging first; per the tracking
epic, implementation may proceed independently, coordinating on API coherence
with NE-2118's authors and reviewers directly.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations identified yet; the DNS Operator's existing HCP
behavior is assumed to extend unchanged. **Open question**, to confirm with
HCP reviewers.

#### Standalone Clusters

This is the primary target; no additional considerations.

#### Single-node Deployments or MicroShift

NE-2118's review raised whether MicroShift runs CoreDNS directly, outside
DNS Operator management. If so, `spec.responseRules` would have no effect on
MicroShift clusters unless separately exposed through MicroShift's own
configuration surface. **Open question**, carried over from NE-2118.

#### OpenShift Kubernetes Engine

No impact identified; this is a DNS Operator/CoreDNS feature with no
dependency on OKE-excluded functionality.

### Implementation Details/Notes/Constraints

Each rule compiles to one or more CoreDNS `template` plugin stanzas in the
default server block:

* `a` → `template IN A <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN A <addr> ..." ; fallthrough }`
* `cname` → `template IN CNAME <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN CNAME <target>" ; fallthrough }`
* `nxdomain` → `template ANY ANY <zone> { match <exact-name-regex> ; rcode NXDOMAIN ; authority "<zone> <ttl> IN SOA ..." }`

`fallthrough` on the `a`/`cname` stanzas is what satisfies "queries outside a
configured rule, and other query types for a ruled name, keep their existing
resolution path."

Validation is split into two stages:

* **Schema/CRD (submission time):** name pattern (exact or single-segment
  wildcard), exactly-one-of `a`/`cname`/`nxdomain`, `nxdomain` rejected when
  the name is a wildcard, address/target format, TTL bounds.
* **Operator (reconcile time):** protected-zone rejection using live cluster
  domain/API/ingress configuration (not expressible as static schema),
  cross-rule duplicate/conflict detection, precedence computation, and a
  dry-run/parse check of the rendered Corefile before it is applied.

### Risks and Mitigations

* **A malformed or conflicting rule degrading cluster DNS.** Mitigated by the
  two-stage validation above and by always keeping the last-known-good
  Corefile on any failure, surfaced via a status condition.
* **Unrelated queries turning into SERVFAIL or losing fallthrough.** Every
  generated stanza includes `fallthrough` except the exact-name NXDOMAIN
  case, which only ever matches the one exact name it was written for.
* **Stale answers after a rule is removed/changed.** Bounded by the
  configured TTL (30-3600s, default 60s); documented in user-facing docs as
  the maximum staleness window. Negative-caching behavior for `nxdomain`
  needs explicit documentation; this mirrors an open, unresolved question
  from NE-2118's review (how long cached filtered responses persist after a
  template is removed) and should be answered here rather than deferred
  again.
* **Divergence from NE-2118's established patterns** (server scope,
  NXDOMAIN) causing reviewer friction. Addressed directly in the
  [Relationship to NE-2118](#relationship-to-ne-2118) section above rather
  than left for reviewers to discover.

### Drawbacks

This adds a second, structurally different set of fields under the same
feature gate as NE-2118 (`spec.template`, singular/zone-scoped, vs.
`spec.responseRules`, a list of per-name rules). An alternative of unifying
both under one mechanism was considered; see Alternatives.

## Alternatives (Not Implemented)

* **Extend NE-2118's `DNSTemplate`/`DNSTemplateAction` directly** (as its own
  "Future Extensions" section sketches: raw `answer`/`authority`/`additional`
  Go-template-string fields) rather than adding a new `responseRules` list.
  Not chosen as the primary proposal because exposing raw CoreDNS template
  syntax to administrators conflicts with this epic's design decision that
  administrators specify DNS names, not templates/regular expressions; kept
  as an open discussion point since it would avoid introducing a second field
  shape under the same feature gate.
* **A separate CRD** instead of a field on `DNS/default`. Rejected: no
  separate resource is needed, and `DNS/default` is already the established
  home for CoreDNS-affecting configuration (`servers`, `forwardPlugin`,
  `cache`).
* **Unmanaged DNS Operator / external DNS server.** These are exactly the
  unsupported workarounds this enhancement exists to replace.

## Open Questions [optional]

1. Should this extend NE-2118's `DNSTemplateAction` in place, or remain a
   separate `responseRules` field? See Alternatives.
2. HCP/Hypershift-specific considerations are not yet captured.
3. MicroShift CoreDNS management model — does `responseRules` need a
   MicroShift-specific exposure path?
4. Exact negative-caching/TTL behavior for `nxdomain` and for a removed rule
   needs to be specified, not left as an open question inherited from
   NE-2118.
5. Per-rule or per-query-type cross-rule duplicate detection semantics (e.g.
   an `a` rule and a `cname` rule both matching the same wildcard) need a
   precise, testable definition.

## Test Plan

**Note:** *Section not required until targeted at a release.*

* Unit/pre-merge tests (NE-1728) for schema validation (malformed names,
  invalid addresses/targets, wildcard NXDOMAIN rejection, conflicting rules)
  and for Corefile rendering per rule type.
* E2E tests (NE-1729) against a running cluster: positive resolution for `a`/
  `cname`/`nxdomain`, negative cases (rule rejected, last-good Corefile
  retained), update and removal of rules, and upgrade persistence of
  `spec.responseRules` and its effect.
* CI wiring (NE-1730) to run both suites as part of standard presubmit/
  periodic jobs, consistent with other `DNSTemplatePlugin`-gated behavior.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Reuses the existing `DNSTemplatePlugin` feature gate. Its current merged API
(NE-2411/PR #2765, also not yet merged) states `DevPreviewNoUpgrade`, while
NE-2118's enhancement proposal states `TechPreviewNoUpgrade` for the same
gate — this document does not resolve that discrepancy; the implementers of
NE-2930/NE-2931/NE-2932 should settle it explicitly with API approvers before
graduation criteria are finalized here.

### Dev Preview -> Tech Preview

- End-to-end wildcard A, wildcard CNAME, and exact NXDOMAIN rules function on
  a real cluster.
- Pre-merge and e2e test coverage per the Test Plan above, in CI.
- User documentation with a configuration example and limitations
  (tracked downstream in OSDOCS-11148).

### Tech Preview -> GA

- Upgrade/downgrade tested explicitly.
- Sufficient operator/SRE feedback on status-condition clarity for invalid
  rules.
- User-facing documentation in openshift-docs.

### Removing a deprecated feature

Not applicable: this enhancement adds a new, optional, feature-gated field
and does not deprecate or remove any existing DNS Operator behavior.

## Upgrade / Downgrade Strategy

`spec.responseRules` persists on `DNS/default` like any other field; on
upgrade, the operator regenerates the Corefile from the same spec on the new
release. No administrator action is required to retain behavior across an
upgrade. Downgrade: if the feature gate is not present/enabled on the
downgraded version, the field is ignored and the operator falls back to a
Corefile without these rules; this should be called out explicitly in user
docs as a behavior change on downgrade.

## Version Skew Strategy

N/A — this is a single-component (DNS Operator/CoreDNS) change with no
node-level or kubelet coordination.

## Operational Aspects of API Extensions

* SLI: `DNS/default` status condition reflecting whether `responseRules`
  reconciled successfully (healthy) or a specific rule is invalid/rejected
  (degraded), analogous to existing DNS Operator conditions.
* Impact on existing SLIs: expected rule counts are small (capped at 100 per
  the proposed `MaxItems`); no meaningful impact on API throughput.
* Failure mode: an invalid update never reaches CoreDNS — the operator keeps
  the last-known-good Corefile, so CoreDNS availability is not impacted by a
  bad rule.

## Support Procedures

* A rejected/invalid rule is visible via the `DNS/default` status condition,
  naming the offending rule and the reason.
* Removing the offending entry from `spec.responseRules` and resubmitting
  clears the condition on the next reconcile.
* Disabling the `DNSTemplatePlugin` feature gate removes all `template`
  plugin stanzas (both NE-2118's and this enhancement's) from the generated
  Corefile; any application depending on a wildcard record configured here
  will stop resolving through CoreDNS until the gate is re-enabled.

## Infrastructure Needed [optional]

None beyond existing `openshift/api` and `cluster-dns-operator` CI.
