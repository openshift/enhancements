---
title: wildcard-exact-dns-response-rules
authors:
  - "@aswinsuryan"
reviewers:
  - "@melvinjoseph86, for DNS Operator behavior and response rule design"
  - "@bentito, for OpenShift DNS integration and networking considerations"
approvers:
  - "@Miciah"
api-approvers:
  - TBD
creation-date: 2026-10-21
last-updated: 2026-10-30
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

RFE-3603 describes customers blocked from deploying an application that
requires a wildcard A record because the DNS Operator owns the generated
Corefile. Today the only ways to publish a wildcard record (most commonly for
a custom application domain) through the cluster's own DNS are unsupported:
hand-editing the Corefile (overwritten on reconcile), setting the DNS
Operator `Unmanaged` (opts out of operator support), or running an entirely
separate DNS server. OCPSTRAT-456 additionally requires that this
configuration survive OpenShift upgrades without administrator intervention,
since the DNS Operator regenerates its configuration from the API on every
reconcile.

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
   reverse zones, and the **cluster's own** API/ingress domain — not an
   administrator's custom application domain, which is the motivating use
   case this enhancement exists to support; see the clarification in
   [Implementation Details](#implementation-detailsnotesconstraints)),
   rejection of rules whose name overlaps a configured `forwardPlugin` zone,
   rejection of a `cname` rule that targets its own query name or that forms
   a cycle with another `cname` rule, duplicate/conflicting rule detection
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
(field naming, discriminated-union/CEL validation pattern) is left to the
`openshift/api` PR, reviewed by API approvers in addition to this document,
per the enhancement template's guidance to avoid duplicating review effort.
This enhancement does not reuse NE-2118's `QueryType`, `QueryClass`, or
`ResponseCode` enums: `QueryType` and `ResponseCode` are each scoped to
NE-2118's single use case (`AAAA` only, `NOERROR` only) and do not cover the
values this enhancement needs (`A`/`CNAME`/`NXDOMAIN` actions spanning
multiple query types per rule), and `query.name` here has no query-type or
query-class concept at all.

This enhancement does **not** modify NE-2118's `DNSTemplate` field or its
behavior. It does not depend on [PR #1936](https://github.com/openshift/enhancements/pull/1936)
merging first; per the tracking epic, implementation may proceed
independently, coordinating on API coherence with NE-2118's authors and
reviewers directly.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations identified yet; the DNS Operator's existing HCP
behavior is assumed to extend unchanged. **Open question**, to confirm with
HCP reviewers.

#### Standalone Clusters

This is the primary target; no additional considerations.

#### Single-node Deployments or MicroShift

Fully applicable: per NE-2118's own topology considerations, MicroShift runs
CoreDNS and can use the DNS Operator to configure it, the same as any other
standalone/single-node cluster. `spec.responseRules` is expected to apply
normally on MicroShift with no additional exposure path needed.

#### OpenShift Kubernetes Engine

No impact identified; this is a DNS Operator/CoreDNS feature with no
dependency on OKE-excluded functionality.

### Implementation Details/Notes/Constraints

Each rule compiles to one or more CoreDNS `template` plugin stanzas in the
default server block:

* `a` → `template IN A <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN A <addr> ..." ; fallthrough }`,
  with one `answer` line per configured address, to support "one or multiple
  IPv4 addresses" per rule.
* `cname` → answers the record types a client actually sends for an aliased
  name, as well as a literal `CNAME`-type query:
  * `template IN A <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN CNAME <target>" ; fallthrough }`
  * `template IN AAAA <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN CNAME <target>" ; fallthrough }`
  * `template IN CNAME <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN CNAME <target>" ; fallthrough }`

  CoreDNS writes the `CNAME` record into the answer section and, because
  `fallthrough` is set, **CoreDNS itself** re-enters its plugin chain to
  resolve `<target>` through the normal A/AAAA resolution path
  (`kubernetes`, `forwardPlugin`, upstream) and bundles the result into the
  same response before replying to the client — this was verified
  empirically against CoreDNS 1.11.3. The client's resolver does not need
  to, and does not, issue a separate follow-up query itself; a standard
  `getaddrinfo()` call succeeds from this single response. If `<target>`
  has no record of the queried type (for example, an AAAA query against a
  target with only an A record), the response correctly contains the
  `CNAME` alone with no address, which is ordinary NODATA behavior, not a
  defect.

  **A self-referencing or cyclic `cname` rule crashes CoreDNS, and the
  operator must reject it.** The internal chase described above has no
  loop or depth protection: verified empirically, a rule whose target is
  its own query name (e.g. `foo.example.com` → CNAME `foo.example.com`),
  or two `cname` rules that form a cycle (rule A's target is rule B's query
  name and vice versa), caused CoreDNS 1.11.3 to loop internally,
  consuming memory until the process was OOM-killed — taking down DNS
  resolution for every query the instance was serving, not only queries
  related to the broken rule. The operator must reject such a rule at
  reconcile time (not merely document the risk), since this is a
  whole-server availability failure, not a contained one.
* `nxdomain` → `template ANY ANY <zone> { match <exact-name-regex> ; rcode NXDOMAIN ; authority "<zone> <ttl> IN SOA ..." ; fallthrough }`

`fallthrough` is required on every stanza, including `nxdomain`: CoreDNS
returns `SERVFAIL` — not a pass-through to normal resolution — for any query
that matches a template's zone but not its `match` regex when `fallthrough`
is absent. Since every stanza here shares the default server's zone with
every other query it serves, omitting `fallthrough` on the `nxdomain`
stanza would turn every other query served by the default block into
`SERVFAIL`. `fallthrough` is what confines the NXDOMAIN answer to the one
exact name the regex matches while every other query keeps its normal
resolution path.

**Stanza order determines precedence, and the operator must enforce it.**
When two `template` stanzas in the same zone both match a query, CoreDNS
answers from whichever stanza is declared first in the Corefile and never
evaluates the second — `fallthrough` only governs the path where a stanza's
regex does *not* match, not precedence between two stanzas that both match.
This was verified empirically: swapping the declaration order of an
overlapping exact-name and wildcard rule changed which answer CoreDNS
returned. Consequently, the operator **must render exact-name rules' `template`
stanzas before any wildcard rules' stanzas** in the generated Corefile,
regardless of the order rules appear in `spec.responseRules`, or the
exact-over-wildcard precedence stated in [Goals](#goals) is not actually
enforced — it would depend on administrator list order instead.

Validation is split into two stages:

* **Schema/CRD (submission time):** name pattern (exact or single-segment
  wildcard), exactly-one-of `a`/`cname`/`nxdomain`, `nxdomain` rejected when
  the name is a wildcard, address/target format, TTL bounds.
* **Operator (reconcile time):** protected-zone rejection using live cluster
  domain/API/ingress configuration (not expressible as static schema) —
  this protects the **cluster's own** `cluster.local` domain, reverse
  zones, and the cluster's own API/ingress domain as configured on the
  cluster (for example, the `Ingress` config object's `spec.domain`), so
  that a response rule cannot break the cluster's own control-plane or
  route resolution. It does **not** reject an administrator's custom
  application domain (e.g. `customer.com`) merely because it resembles an
  ingress-style name — RFE-3603's motivating case, a wildcard A rule for
  `*.apps.customer.com` on a domain the administrator owns, is a distinct
  domain from the cluster's own ingress domain and is expected to pass this
  check; rejection applies only when a rule's name actually falls under the
  cluster's own protected zone/domain, which would be a genuine conflict
  with existing cluster-managed records;
  rejection of rules whose name overlaps a configured `forwardPlugin` zone
  (CoreDNS routes each query to the server block with the longest matching
  zone, so such a rule would never reach the default block's `template`
  stanzas and would silently have no effect), rejection of a `cname` rule
  that targets its own query name or that forms a cycle with one or more
  other `cname` rules (see the empirical finding above — this is a
  whole-server crash, not a degraded-feature risk), cross-rule
  duplicate/conflict detection, precedence computation, and a dry-run/parse
  check of the rendered Corefile before it is applied.
  Cross-rule conflict detection explicitly covers `a`/`cname` overlap, but
  only between rules of **equal specificity**: two rules with the same exact
  name, or two wildcard rules with the same pattern, that specify different
  actions are rejected as a conflict (for example, two `*.example.com`
  rules, one `a` and one `cname`). An exact-name rule and a wildcard rule
  that both match the same query but specify different actions are not a
  conflict, because the operator enforces the stanza-ordering rule above:
  the exact-name rule's stanza is always rendered first and wins for its
  one name, and the wildcard rule's stanza, rendered after, continues to
  answer every other name it matches (for example, a `cname` rule for
  `app.example.com` alongside an `a` rule for `*.example.com` is valid:
  `app.example.com` gets the `cname` answer, every other name under
  `*.example.com` gets the `a` answer). This guarantee depends entirely on
  the operator's stanza ordering, not on any precedence mechanism intrinsic
  to CoreDNS itself.

### Risks and Mitigations

* **A malformed or conflicting rule degrading cluster DNS.** Mitigated by the
  two-stage validation above and by always keeping the last-known-good
  Corefile on any failure, surfaced via a status condition.
* **A self-referencing or cyclic `cname` rule crashing CoreDNS entirely.**
  Verified empirically against CoreDNS 1.11.3: a `cname` rule targeting its
  own query name, or two `cname` rules forming a cycle, causes CoreDNS's
  internal CNAME-chase to loop with no depth limit, consuming memory until
  the process is OOM-killed — an outage for every query the instance was
  serving, not a contained failure of the one bad rule. Mitigated by
  rejecting self-reference and cross-rule cycles at reconcile time, before
  the rule ever reaches the generated Corefile.
* **Unrelated queries turning into SERVFAIL or losing fallthrough.** Every
  generated stanza, including the exact-name NXDOMAIN case, includes
  `fallthrough`; without it, a template whose zone matches a query but whose
  regex does not would return `SERVFAIL` instead of continuing normal
  resolution, since these rules share the default server's zone with every
  other query it serves.
* **Stale answers after a rule is removed/changed.** Bounded by the
  configured TTL (30-3600s, default 60s); documented in user-facing docs as
  the maximum staleness window. Negative-caching behavior for `nxdomain`
  (how long a cached NXDOMAIN response persists in CoreDNS's `cache` plugin
  after the rule that produced it is removed) needs explicit documentation
  in this enhancement rather than left unspecified.

### Drawbacks

This adds a second, structurally different set of fields under the same
feature gate as NE-2118 (`spec.template`, singular/zone-scoped, vs.
`spec.responseRules`, a list of per-name rules). An alternative of unifying
both under one mechanism was considered; see Alternatives.

## Alternatives (Not Implemented)

* **Extend NE-2118's `DNSTemplate`/`DNSTemplateAction` directly** (as its own
  "Future Extensions" section sketches: raw `answer`/`authority`/`additional`
  Go-template-string fields) rather than adding a new `responseRules` list.
  Rejected: the tracking epic's design decisions settle on a structured
  `spec.responseRules` list on `DNS/default`, not an extension of
  `DNSTemplate`/`DNSTemplateAction`, because exposing raw CoreDNS template
  syntax to administrators conflicts with the decision that administrators
  specify DNS names, not templates/regular expressions.
* **A separate CRD** instead of a field on `DNS/default`. Rejected: no
  separate resource is needed, and `DNS/default` is already the established
  home for CoreDNS-affecting configuration (`servers`, `forwardPlugin`,
  `cache`).
* **Unmanaged DNS Operator / external DNS server.** These are exactly the
  unsupported workarounds this enhancement exists to replace.

## Open Questions [optional]

1. HCP/Hypershift-specific considerations are not yet captured.
2. Exact negative-caching/TTL behavior for `nxdomain` and for a removed rule
   needs to be specified in this enhancement's user-facing documentation.

## Test Plan

**Note:** *Section not required until targeted at a release.*

* Unit/pre-merge tests (NE-1728) for schema validation (malformed names,
  invalid addresses/targets, wildcard NXDOMAIN rejection, conflicting rules)
  and for Corefile rendering per rule type, including: an exact rule's
  `template` stanza is always rendered before an overlapping wildcard
  rule's stanza regardless of the order the rules appear in
  `spec.responseRules`; a `cname` rule renders `template` stanzas for `A`,
  `AAAA`, and `CNAME` query types; and a self-referencing or cyclic `cname`
  rule is rejected at reconcile time rather than reaching the Corefile.
* E2E tests (NE-1729) against a running cluster: positive resolution for `a`/
  `cname`/`nxdomain`, negative cases (rule rejected, last-good Corefile
  retained), update and removal of rules, and upgrade persistence of
  `spec.responseRules` and its effect. Includes a downgrade-then-upgrade
  round trip: back up `spec.responseRules` before downgrading, confirm the
  downgraded operator serves CoreDNS without the rules, then confirm
  reapplying the backed-up rules after upgrading restores the configured
  behavior. Also includes an explicit precedence test: an exact-name rule
  and an overlapping wildcard rule with different actions, confirming the
  exact-name answer wins for its one name and the wildcard answer is
  returned for every other matching name.
* CI wiring (NE-1730) to run both suites as part of standard presubmit/
  periodic jobs, consistent with other `DNSTemplatePlugin`-gated behavior.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Reuses the existing `DNSTemplatePlugin` feature gate. NE-2118's enhancement
proposal text states `TechPreviewNoUpgrade`, but the gate's actual
registration in `openshift/api#2765` (NE-2411, not yet merged) is
`enable(inDevPreviewNoUpgrade())` — `DevPreviewNoUpgrade` only. This document
adopts `DevPreviewNoUpgrade` as authoritative, matching the registered
feature gate; NE-2118's enhancement text needs a follow-up correction
tracked against that document rather than reconciled here.

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

On upgrade, the operator regenerates the Corefile from the same
`spec.responseRules` on the new release. No administrator action is required
to retain behavior across an upgrade.

Downgrade is not simply "the operator ignores the field": a downgraded
release's CRD schema may not include `responseRules` at all — the field is
only present in the `DNSTemplatePlugin`-gated CRD variants, not the
`Default` feature set's CRD (see `openshift/api#2765`). Under structural-
schema pruning, the field survives in etcd only until the next full write to
`DNS/default`; a write made against the downgraded schema drops it, and
CoreDNS then serves the last Corefile the downgraded operator generated,
without these rules. Administrators must back up `spec.responseRules` before
downgrading and reapply it after a subsequent upgrade; this is called out
explicitly in user docs as a behavior change on downgrade, and is exercised
by an explicit downgrade-then-upgrade round trip (see Test Plan).

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
  naming the offending rule and the reason. Removing the offending entry
  from `spec.responseRules` and resubmitting clears the condition on the
  next reconcile.
* To remove a working response rule, delete its entry from
  `spec.responseRules` and resubmit. Do not rely on disabling the
  `DNSTemplatePlugin` feature gate to remove rules: `spec.featureSet`
  cannot be changed once a cluster is on `DevPreviewNoUpgrade` or
  `TechPreviewNoUpgrade`, so disabling the gate is not a supported recovery
  path on those clusters.

## Infrastructure Needed [optional]

None beyond existing `openshift/api` and `cluster-dns-operator` CI.
