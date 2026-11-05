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
  - "@JoelSpeed"
creation-date: 2026-10-21
last-updated: 2026-11-04
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
reconcile. RFE-6814 independently describes an ARO use case for wildcard
`*.apps` answers and broader template rules, reinforcing the same wildcard
A/CNAME need from a second customer; RFE-6814's separate Azure-specific
custom search-domain request is out of scope here (see Non-Goals).

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
  rejected with a clear, actionable error — immediately at submission for
  structural mistakes, or via a status condition on the next reconcile for
  checks that need live cluster context — so that a mistake in my
  configuration cannot degrade or break CoreDNS for the whole cluster.

* As an SRE, I want queries that don't match any configured rule to follow
  their existing resolution path unchanged (service discovery, ingress names,
  configured forwarding), so that this feature cannot regress unrelated DNS
  behavior.

### Goals

* Let an administrator declare wildcard A, wildcard CNAME, and exact-name
  NXDOMAIN rules through the DNS Operator API, reusing the `DNSTemplatePlugin`
  feature gate and the CoreDNS `template` plugin mechanism from NE-2118.
* Define unambiguous precedence: an exact name always wins over a wildcard
  for every query type both rules' actions render; among wildcards, the
  most specific wins; overlapping/conflicting rules — including the one
  action pairing where an exact rule's action covers fewer query types than
  an overlapping wildcard rule's (an exact `a` rule vs. a wildcard `cname`
  rule) — are rejected at submission or reconcile time rather than silently
  resolved or silently answered incorrectly.
* Validate rules at submission (schema/admission and the validating
  webhook), so an invalid or conflicting rule is rejected before it is ever
  persisted, and again before the generated Corefile is applied, so an
  invalid rule cannot make CoreDNS unusable. Reconciliation independently
  repeats this validation and additionally handles cases submission cannot:
  cluster context that changes after a rule was accepted, and pre-apply
  validation failures against the generated Corefile. On either kind of
  failure found at reconcile time, keep serving the last-known-good
  Corefile and report a clear status condition.
* Preserve existing resolution behavior for anything not covered by a rule:
  cluster service/ingress discovery, configured `forwardPlugin` zones, and
  other query types for a name that only has a rule for one record type —
  or, where that cannot yet be guaranteed, reject the combination outright
  rather than silently answer it incorrectly (see the precedence Goal
  above).
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
* RFE-6814's Azure-specific custom search-domain handling — a distinct
  request within the same RFE from the wildcard A/CNAME need addressed by
  this enhancement (see Motivation); tracked separately, not an acceptance
  criterion here.
* Azure's installer/custom-DNS path (OCPSTRAT-991) — unrelated mechanism, not
  evidence this feature already exists.
* Response generation for record types or response codes beyond A, CNAME, and
  exact-name NXDOMAIN.
* Distinguishing, in the client-visible response, between a `cname` rule's
  target having no record of the queried type, not existing at all
  (NXDOMAIN), or failing to resolve (SERVFAIL) — the underlying CoreDNS
  `template` plugin's CNAME-chase collapses all three into the same
  `NOERROR` response containing only the synthesized `CNAME` (see
  Implementation Details); changing that would mean patching the vendored
  `template` plugin, which this enhancement does not do.

## Proposal

### Workflow Description

**cluster administrator** is a human user with permission to edit the
cluster-scoped `DNS/default` resource. No additional administrator
permission is required beyond permission to edit `DNS/default`:
`responseRules` is a field on an object administrators can already edit,
gated only by the existing `DNS/default` edit permission and the
`DNSTemplatePlugin` feature gate. The implementation itself does add new
permissions for the operator — to manage the validating webhook's
Deployment, Service, and serving-certificate resources, and to read the
cluster configuration the webhook needs for its protected-domain checks —
see [Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling).

1. The administrator adds one entry to `spec.responseRules` on `DNS/default`
   naming a rule, a query name (exact or single-segment wildcard), a TTL, and
   exactly one action (`a`, `cname`, or `nxdomain`).
2. On submission, schema/admission validation checks each rule's shape
   (name pattern, exactly-one-action, address/target format, TTL bounds, an
   exact `cname` rule's literal self-reference) and cross-rule checks
   decidable from `spec.responseRules` and `spec.servers` together
   (duplicate/conflicting rule detection, an exact `a` rule overlapping a
   wildcard `cname` rule, overlap with a configured `forwardPlugin` zone,
   and overlap with the cluster's fixed `cluster.local` domain or a reverse
   zone) — see [Implementation Details](#implementation-detailsnotesconstraints)
   for the full breakdown.

   An operator-managed validating admission webhook then checks the
   submitted rules against the cluster's own protected API/ingress domain
   as actually configured on the cluster — not an administrator's custom
   application domain, which is the motivating use case this enhancement
   exists to support — and performs precedence-aware `cname` self-reference
   and cycle detection across the complete submitted rule set. Both checks
   need something static schema/CEL validation cannot provide: the first
   needs state beyond the submitted object (a separate cluster resource);
   the second needs only `spec.responseRules` itself, but requires
   following the chain of rules that actually win precedence at each step,
   a computation CEL cannot express (see
   [Implementation Details](#implementation-detailsnotesconstraints)). A
   rejected update is not persisted; the admission error identifies the
   offending rule and the conflicting domain or rule.
3. The DNS Operator independently repeats the webhook's semantic
   validation (protected-domain and self-reference/cycle checks) before
   rendering, using the same matching, precedence, conflict, and
   cycle-detection logic as the webhook rather than trusting that
   admission already caught everything. It also reacts on its own to
   relevant API/ingress configuration changes: a rule accepted at
   submission can become invalid later if the cluster's protected-domain
   configuration changes afterward, and reconciliation — not admission —
   is what catches that case, following the retention/fallback and status
   behavior described in step 5. It also computes precedence for rendering
   (step 4).
4. If valid, the operator renders the corresponding CoreDNS `template`
   plugin stanzas for `spec.responseRules` into the default server block of
   the candidate Corefile, then validates just those rendered stanzas —
   not the full candidate Corefile — before writing the candidate to the
   `dns-default` ConfigMap. Validation runs synchronously, inside the
   operator's own process, using CoreDNS's own `template` plugin parsing
   and setup code. This code is not already vendored into the operator
   today; this enhancement adds it, and its `github.com/coredns/caddy`
   directive-parsing dependency, as new operator dependencies. A CI check
   keeps that vendored copy aligned with the `template` plugin actually
   shipped in `r.CoreDNSImage` for the same release, failing the build on
   drift rather than assuming the two stay in sync.

   Scoping validation to only the stanzas this enhancement renders is a
   deliberately narrower guarantee than validating the whole Corefile: it
   catches a rule whose stanza is syntactically or semantically invalid
   (for example, a `match` regex that fails to compile), but it says
   nothing about the rest of the candidate Corefile (`kubernetes`,
   `forwardPlugin`, `reload`, `spec.template`, and so on) — nothing in the
   operator validates those today either, so this is not a regression. It
   also avoids side effects that setting up those other plugins would
   otherwise introduce during validation (for example, the `forward`
   plugin's setup reads TLS CA files from the operand's local filesystem,
   and the `reload` plugin's setup mutates process-global state), none of
   which the operator should trigger just to check rendered rule stanzas.
   A setup error comes back to the operator as an ordinary Go `error`
   within the same reconcile that rendered the candidate — there is no
   process boundary, scheduling, or polling involved, so the rendered
   stanzas are known valid or invalid synchronously, before anything is
   written anywhere.
5. If validation returns an error, the operator does not write the
   candidate: it keeps serving the currently-applied ConfigMap unchanged and
   sets a status condition on `DNS/default` naming the invalid rule(s) and
   the validation error. If there is no currently-applied ConfigMap to fall
   back to — initial creation, or the ConfigMap was deleted/recreated while
   `spec.responseRules` holds a rule that reconcile-time validation (step
   3) would now reject because cluster context changed since it was
   accepted — the operator's fallback depends on whether the failure can be
   attributed to specific rules:
   * If it can — a named reconcile-time rejection, or a validation error
     whose message maps back to a specific rule's rendered stanza, which is
     the common case since validation is scoped to the stanzas
     `spec.responseRules` itself renders — the operator renders a reduced
     candidate with only those rule(s) excluded and runs it back through
     the **full** validation pipeline (reconcile-time conflict/cycle checks
     across the remaining rules, then the same in-process validation)
     before applying it. It is applied only once that revalidation passes;
     excluding a rule is never assumed sufficient on its own, since the
     remaining rules can still conflict or cycle among themselves.
   * If it cannot — a validation error that does not map back to any single
     rule's rendered stanza — the operator instead falls back to a
     candidate with `spec.responseRules` omitted entirely, revalidated the
     same way, and applies that if it passes: serving DNS without the
     custom rules is still better than serving none.
   * If even that minimal candidate fails, the cause is undetermined —
     since no `spec.responseRules` stanzas remain to validate, this could
     be a problem elsewhere in the Corefile, but could equally be an error
     in the validator itself — and is surfaced through the operator's
     existing degraded-status reporting rather than a
     `spec.responseRules`-specific fallback or a diagnosis this enhancement
     isn't positioned to make.

   In every case, the status condition names which rule(s), if any, were
   excluded and why.
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
reviewers directly. When both fields are configured on the same cluster and
can render `template` stanzas for the same query type in the same default
server block, the operator must render all `spec.responseRules`-derived
stanzas before `spec.template`'s stanza (see the stanza-ordering rule in
[Implementation Details](#implementation-detailsnotesconstraints)): a
per-name `spec.responseRules` rule is more specific than a zone/type-wide
`spec.template`, so it must get the first chance to answer; `spec.template`
is then reached only for queries none of `spec.responseRules`'s stanzas
match, the same as any other downstream resolution.

This enhancement also adds a new API extension beyond the field itself: a
cluster-scoped validating admission webhook, registered for `CREATE`/
`UPDATE` requests against `DNS/default` whose proposed `spec.responseRules`
is non-empty. It does not modify the behavior of any resource this team
does not own: the webhook and operator obtain the protected ingress domain
from `Ingress/cluster`'s `spec.domain`, and obtain the API endpoint
hostnames by parsing `Infrastructure/cluster`'s `status.apiServerURL` and
`status.apiServerInternalURI` (`config.openshift.io/v1`) — these are two
distinct sources feeding the same shared protected-name validation logic,
not one combined "API/ingress domain" field; `apiServerInternalURI` is the
actual JSON field name for what the Go type calls
`APIServerInternalURL` (`openshift/api`'s own naming is inconsistent
here). The implementation only reads these objects and does not write to
them or change their behavior, so no additional reviewer is required on
that basis. See
[Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)
for its full design, and
[Operational Aspects of API Extensions](#operational-aspects-of-api-extensions)
for its operational impact.

### Topology Considerations

#### Hypershift / Hosted Control Planes

**Open question, sharpened by the validating webhook, to confirm with HCP
reviewers.** `DNS/default` is a guest-cluster resource, so the webhook must
validate submissions against the guest cluster's API server, while the DNS
Operator itself (like other HCP-aware operators) typically runs in the
management cluster: HCP's established split is "operator in management,
webhook in guest." Where the webhook Deployment/Service actually runs,
which cluster's `Ingress`/`Infrastructure` config it reads for the
protected ingress-domain and API-hostname checks, and whether
per-guest-cluster webhook registration is required, are not yet resolved
here. Beyond the webhook's own placement, no other unique
consideration has been identified; the DNS Operator's existing HCP behavior
for everything else is assumed to extend unchanged.

#### Standalone Clusters

This is the primary target; no additional considerations.

#### Single-node Deployments or MicroShift

The `spec.responseRules` field itself is fully applicable: per NE-2118's
own topology considerations, MicroShift runs CoreDNS and can use the DNS
Operator to configure it, the same as any other standalone/single-node
cluster, with no additional exposure path needed. The validating webhook's
applicability to MicroShift is **unverified** — whether MicroShift's API
server supports `ValidatingWebhookConfiguration` registration the same way
standalone OpenShift does, and the resource/availability cost of an
additional webhook Deployment on a single-node footprint, has not been
confirmed against MicroShift's own constraints and needs a MicroShift
reviewer before Tech Preview.

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

  This is a deliberate scope limit, not full RFC 1034 CNAME redirection: a
  query of a different type against a `cname`-ruled name (for example MX or
  TXT, but not `ANY` — see below) is not intercepted by this rule and
  continues through normal resolution, which typically returns
  NXDOMAIN/NODATA since the aliased name is not otherwise published.
  Administrators who need CNAME semantics for every query type should
  publish the alias in authoritative DNS instead of using this mechanism.

  Both `a` and `cname` stanzas are also matched by a client `ANY`-type
  query against the ruled name, not only by their own configured type.
  CoreDNS's `template` plugin treats a stanza's configured type and the
  incoming query's type as matching whenever *either* side is `ANY`
  (confirmed in `plugin/template/template.go`'s `match()`: the type check
  only rejects a stanza when both the stanza's type and the query's type
  are non-`ANY` and unequal). Consequently an `ANY` query against a ruled
  name is intercepted and answered by whichever stanza for that name is
  declared first in the Corefile (exact before wildcard, per the ordering
  rule below) — it does not fall through to normal resolution the way a
  genuinely different type (MX, TXT) does. For an `a` rule this returns the
  configured A record(s), the same as an `A` query would get. For a
  `cname` rule this returns the bare synthesized `CNAME` record with **no**
  address bundled in: the internal CNAME-chase described next only
  triggers when the query type is literally `A` or `AAAA`
  (`state.QType() == dns.TypeA || state.QType() == dns.TypeAAAA` in
  source), which excludes `ANY`. This is the intended behavior this
  enhancement exposes for `ANY`, not a gap to close — returning one
  representative record rather than enumerating every type matches modern
  authoritative-server guidance (RFC 8482) and needs no additional
  mechanism. Elsewhere in this document, "other query types are
  unaffected" means query types other than `A`, `AAAA`, `CNAME`, and
  `ANY`; `ANY` is explicitly covered here, not an unaffected type.

  CoreDNS writes the `CNAME` record into the answer section, and the
  `template` plugin's own built-in CNAME-following behavior — independent
  of `fallthrough`, which only governs the path where a stanza's regex does
  *not* match — then makes **CoreDNS itself** re-enter the full server's
  plugin chain to resolve `<target>` through the normal A/AAAA resolution
  path (`kubernetes`, `forwardPlugin`, upstream) and bundle the result into
  the same response before replying to the client. This is unconditional
  plugin behavior, not something the rule's Corefile stanza opts into:
  every `template` stanza automatically re-resolves a matched answer's
  target whenever the query is `A`/`AAAA` and the templated answer is a
  `CNAME`, confirmed against the CoreDNS `template` plugin source
  (`plugin/template/template.go`, `plugin/pkg/upstream/upstream.go`). The
  client's resolver does not need to, and does not, issue a separate
  follow-up query itself.

  This single response, however, cannot tell the client *why* `<target>`
  contributed nothing. The chase copies only the target response's
  `Answer` and `Truncated` fields into the reply — `msg.Truncated =
  up.Truncated; msg.Answer = append(msg.Answer, up.Answer...)` in
  `template.go` — never its response code or authority section, and the
  reply's own response code stays whatever the matched `cname` stanza is
  configured with, always `NOERROR` here. Consequently, a target that has
  no record of the queried type (true NODATA, e.g. an AAAA query against a
  target with only an A record), a target that does not exist at all
  (NXDOMAIN), a target whose own resolution fails (SERVFAIL), and an
  internal lookup error from the chase itself, all produce the identical
  client-visible response: `NOERROR` containing only the synthesized
  `CNAME`, with no address and no indication of which of these occurred.
  This is accepted behavior, not special-cased: distinguishing these cases
  in the client-visible response would require patching the vendored
  `template` plugin to propagate the target response's response code and
  authority section, which this enhancement does not do (see Non-Goals). A
  `getaddrinfo()` call against the ruled name succeeds from this single
  response only when `<target>` actually has a record of the queried type;
  administrators should not rely on the response to tell a genuinely
  missing record apart from a broken or nonexistent target.

  **A self-referencing or cyclic `cname` rule crashes CoreDNS, and the
  operator must reject it.** The internal chase described above has no
  loop or depth protection: verified empirically, a rule whose target is
  its own query name (e.g. `foo.example.com` → CNAME `foo.example.com`),
  or two `cname` rules that form a cycle (rule A's target is rule B's query
  name and vice versa), caused CoreDNS to loop internally, consuming
  memory until the process was OOM-killed — taking down DNS resolution for
  every query the instance was serving, not only queries related to the
  broken rule. The same failure can occur when a **wildcard** `cname`
  rule's target falls within the rule's own match pattern (e.g.
  `*.example.com` → CNAME `alias.example.com`: `alias.example.com` itself
  matches `*.example.com`), since the internal chase re-enters the same
  stanza and loops exactly as the literal self-reference case does — but
  only when that stanza is actually what answers the re-entered query.

  Detection must therefore follow the rule that actually wins at runtime
  for each step of the chase, not every rule whose raw pattern happens to
  overlap. If a higher-precedence rule overrides the wildcard's target
  before the chain repeats, resolution terminates normally and is not a
  cycle: given `*.example.com` → CNAME `alias.example.com` and a separate
  **exact** rule `alias.example.com` → CNAME `target.other.test`, the
  exact rule's stanza is rendered before the wildcard's (per the
  stanza-ordering rule below) and answers both the original query for
  `alias.example.com` and the chase's internal re-entry for it — the
  wildcard's own stanza is never reached again for that name, so this
  configuration has a terminating path and is not a cycle, even though the
  wildcard's target falls within its own pattern. The operator must walk
  each rule's target through the chain of **winning** rules — the exact
  rule for that name if one exists, otherwise the most specific matching
  wildcard rule, per the same precedence [Goals](#goals) establish —
  rather than through every rule whose pattern merely overlaps. A
  configuration is rejected only when that winning-rule chain revisits a
  stanza already seen earlier in the chain (a genuine, unresolvable
  cycle); it is not rejected merely because some other, lower-precedence
  rule's pattern also happens to match a name somewhere in the chain. A
  step with no winning `cname` rule at all (the name falls through to
  normal resolution, or only to an `a`/`nxdomain` rule) ends the chain
  there, not a cycle. Literal self-reference, cross-rule cycles, and an
  unintercepted wildcard self-match are rejected before persistence by
  admission validation (not merely document the risk), since this is a
  whole-server availability failure, not a contained one. Cycle detection
  follows the rule that actually wins precedence at each step; a
  terminating higher-precedence exact-name override, as above, remains
  valid. The operator repeats these checks defensively during
  reconciliation.
* `nxdomain` → `template ANY ANY <zone> { match <exact-name-regex> ; rcode NXDOMAIN ; authority "<zone> <ttl> IN SOA ns.invalid. admin.invalid. 1 60 60 60 <ttl>" ; fallthrough }`

**Negative-caching duration for `nxdomain` is the rule's own `ttlSeconds`,
not a separate value.** The SOA record's last field (the "minimum" field)
is what CoreDNS's `cache` plugin and RFC 2308-compliant downstream
resolvers use as the negative-caching TTL for an NXDOMAIN response; it is
rendered here as the rule's `ttlSeconds` rather than a fixed value, so an
administrator's configured TTL governs how long the NXDOMAIN answer is
cached, consistently with how it governs a positive answer's TTL for `a`
and `cname` rules. CoreDNS's own `cache` plugin (`spec.cache.negativeTTL`,
cluster-wide, defaulting to 30s today) still applies on top of this: the
actual negative-caching duration observed by a client is
`min(ttlSeconds, spec.cache.negativeTTL)`. This also bounds staleness after
a rule is removed or changed: a CoreDNS instance (or downstream resolver)
that cached the NXDOMAIN answer continues to serve it for up to that same
`min(ttlSeconds, spec.cache.negativeTTL)` window before re-querying and
picking up the regenerated Corefile's behavior — the same bound already
documented below for `a`/`cname` rules, with no separate mechanism needed.

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

**Exact-name precedence only holds for query types the exact rule's own
action actually renders, and one action pairing cannot be made to work
around that today.** An exact rule's action determines which stanza types
it renders (`a` renders only `IN A`; `cname` renders `IN A`/`IN AAAA`/`IN
CNAME`; `nxdomain` renders `ANY ANY`, which matches every type). When an
exact rule's action covers fewer types than an overlapping wildcard rule's
action, a query of a type the exact rule does not render has no exact-rule
stanza to win the type-scoped ordering above, and falls straight to the
wildcard rule's stanza for that type even though the query is for the
exact name. For example, an exact `a` rule for `app.example.com` renders
only `IN A`; an overlapping wildcard `cname` rule for `*.example.com`
renders `IN AAAA` and `IN CNAME` stanzas that also match
`app.example.com`, so an AAAA or CNAME query for `app.example.com` would
be answered by the wildcard rule instead of being left to normal
resolution as the exact rule's scope implies.

This gap does not extend to an `ANY` query against the exact name: since
`ANY` satisfies every stanza's type check regardless of the stanza's own
configured type (see the `ANY`-query discussion above), the
stanza-ordering rule alone decides the answer for `ANY`, and the exact
rule's stanza — rendered first — wins. The unresolved gap is specifically
about concrete types (here, `AAAA` and `CNAME`) that the wildcard's action
renders but the exact rule's narrower action does not.

This cannot be closed by adding a "guard" stanza — an empty, answer-less
`template` stanza for the exact name placed ahead of the wildcard's
stanza, relying on `fallthrough` to pass the query on to normal
resolution. The CoreDNS `template` plugin's `fallthrough` option only
applies on the path where a stanza's `match` regex does **not** match;
when the regex matches — as it would for the guard's own exact-name match
— the plugin always returns the response built from that stanza's
`answer`/`authority`/`additional`/`rcode` directives, defaulting to an
**empty `NOERROR`** when none are set, and never proceeds to the next
plugin regardless of `fallthrough`
([CoreDNS template plugin docs](https://coredns.io/plugins/template/)).
A guard stanza would therefore turn every AAAA/CNAME query against the
exact name into an empty, answerless `NOERROR` — not a pass-through to
normal resolution, and worse than the unguarded wildcard answer it was
meant to replace.

`a` and `cname` are the only two actions where this gap can occur:
`cname`'s rendered types (`A`, `AAAA`, `CNAME`) are a superset of `a`'s
(`A`), `nxdomain`'s `ANY ANY` stanza already matches every type, and two
rules sharing the same action always render the same type set. So the gap
is limited to one pairing — an exact `a` rule overlapping a wildcard
`cname` rule — and, absent a real precedence mechanism for it, **this
pairing must be rejected at admission**: an exact `a` rule whose name is
also matched by an overlapping wildcard `cname` rule (or vice versa) is
invalid, rejected by the API server at submission with a message naming
both rules and the unresolved query types, rather than ever being accepted
and surfaced later as a status condition. This is listed as an open
question below; closing it will likely require either a future CoreDNS
`template` capability to match-without-answering, or restructuring how
these two actions render so the gap cannot occur, rather than a change to
the generated Corefile alone.

Validation is layered across schema/static admission, the validating
webhook, and operator reconciliation. Generated template-stanza validation
is an additional pre-apply check described in Workflow step 4.

* **Schema/CRD (submission time), per rule:** name pattern (exact or
  single-segment wildcard), exactly-one-of `a`/`cname`/`nxdomain`,
  `nxdomain` rejected when the name is a wildcard, address/target format,
  TTL bounds, and an exact-name `cname` rule's target rejected when it
  equals the rule's own query name — an exact rule's precedence never
  depends on any other rule, so this one case of self-reference is a fixed,
  unconditional error decidable from the rule alone.
* **Schema/CRD (submission time), cross-rule:** everything else decidable
  from `spec.responseRules` and `spec.servers` alone, since both are
  fields on the one `DNS/default` object being submitted and need no
  cluster context beyond the object itself: cross-rule duplicate/conflict
  detection between rules of equal specificity that specify different
  actions; rejection of an exact `a` rule overlapping a wildcard `cname`
  rule for the same name (the one action pairing where the exact rule's
  action covers fewer query types than the overlapping wildcard's, with no
  available mechanism to reserve the uncovered types — see below); and
  rejection of a rule whose name overlaps a configured `forwardPlugin` zone
  (`spec.servers[].zones` — CoreDNS routes each query to the server block
  with the longest matching zone, so such a rule would never reach the
  default block's `template` stanzas and would silently have no effect),
  or falls under the cluster's fixed `cluster.local` domain or a reverse
  zone (`in-addr.arpa.`/`ip6.arpa.`) — unlike the cluster's actual
  ingress/API domain below, these are fixed values, not cluster-specific
  configuration, so they need no external state either.
* **Validating webhook (submission time):** reject conflicts with the
  cluster's own protected ingress domain and API endpoint hostnames as
  actually configured on the cluster — state on separate objects, not
  visible to schema/CEL validation on `DNS/default` alone — and reject
  `cname` cycles using the effective exact-name/wildcard precedence: a
  rule that forms a cycle with one or more other `cname` rules, or a
  wildcard rule whose target falls within its own pattern and is not
  intercepted by a higher-precedence rule (see the empirical finding
  above). The webhook and operator obtain the protected ingress domain
  from `Ingress/cluster`'s `spec.domain`, and obtain the API endpoint
  hostnames by parsing `Infrastructure/cluster`'s `status.apiServerURL`
  and `status.apiServerInternalURI` (note: the JSON field is
  `apiServerInternalURI`, not `apiServerInternalURL` — the Go struct field
  is named `APIServerInternalURL`, but `openshift/api`'s own JSON tag for
  it is `apiServerInternalURI`). These two sources are independent and
  feed the same shared protected-name validation logic used by both the
  webhook and reconciliation; the implementation includes the read
  permissions for both objects, watches both for changes relevant to
  already-admitted rules, and defines explicit handling for either field
  being absent (not every topology populates both) or holding a bare IP
  address rather than a hostname (the field is a URI, and a URI's
  authority component may be an IP literal). Unlike the ingress-domain and
  API-hostname checks, cycle detection needs no state beyond
  `spec.responseRules` itself, but it requires following the chain of
  rules that actually win precedence at each step, a computation
  schema/CEL validation cannot express — the webhook is what makes running
  this same Go logic at submission time possible. Protection applies to
  the cluster's own protected ingress domain and API hostnames, not an
  administrator's custom application domain merely because its name
  resembles an ingress-style name — RFE-3603's motivating case, a wildcard
  A rule for `*.apps.customer.com` on a domain the administrator owns, is
  a distinct domain from the cluster's own ingress domain and is expected
  to pass this check. The webhook does not run CoreDNS or validate the
  full Corefile; see
  [Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)
  for its deployment, failure-mode, and fail-closed behavior.
* **Operator (reconcile time):** repeats the webhook's semantic checks
  (protected-domain and self-reference/cycle detection, sharing the same
  matching, precedence, conflict, and cycle-detection logic as the
  webhook) before applying generated configuration, and whenever relevant
  cluster configuration changes — admission success does not replace
  reconciliation validation, since a rule accepted at submission can become
  invalid later if the cluster's protected-domain configuration changes
  afterward. Rejected stored rules are reported through status and handled
  using the retention/fallback behavior in Workflow step 5. The pre-apply
  validation of the generated `spec.responseRules` `template` stanzas
  themselves is described in step 4 of the Workflow Description above.

Precedence computation — determining which stanza renders first when an
exact and a wildcard rule both match a query — is not a rejection, and
happens once the rule set is already known valid; see the stanza-ordering
discussion below.
  Cross-rule conflict detection explicitly covers `a`/`cname` overlap, but
  only between rules of **equal specificity**: two rules with the same exact
  name, or two wildcard rules with the same pattern, that specify different
  actions are rejected as a conflict (for example, two `*.example.com`
  rules, one `a` and one `cname`). An exact-name rule and a wildcard rule of
  **different** specificity that both match the same query are not
  automatically a conflict: when the exact rule's action renders every type
  the wildcard rule's action renders (for example, a `cname` rule for
  `app.example.com` alongside an `a` rule for `*.example.com`: `cname`'s
  type set is a superset of `a`'s), the stanza-ordering rule above is
  sufficient — `app.example.com` gets the `cname` answer for every type
  `cname` covers, and every other name under `*.example.com` gets the `a`
  answer. The one pairing where the exact rule's action covers **fewer**
  types than the overlapping wildcard rule's action — an exact `a` rule
  overlapping a wildcard `cname` rule — has no such guarantee available
  today (see the discussion above) and is rejected as a conflict rather
  than silently answered incorrectly for the uncovered types.

### Admission Webhook Lifecycle and Failure Handling

The DNS Operator manages a dedicated validating webhook Deployment,
Service, and serving-certificate lifecycle. The owner of the
`ValidatingWebhookConfiguration` and its installation/removal sequencing
must be selected as part of the lifecycle decision described under
Version Skew Strategy; ownership is not assumed to follow from ownership
of the serving Deployment.

The webhook reads `Ingress/cluster`'s `spec.domain` and the API endpoint
hostnames derived from `Infrastructure/cluster`'s `status.apiServerURL`
and `status.apiServerInternalURI`. The operator watches both configuration
objects for changes relevant to already-admitted rules. Required read
permissions, supported topology, and availability are covered by
installation, upgrade, and recovery tests.

Registration is narrowly scoped to CREATE and UPDATE requests for the
main `DNS/default` resource whose proposed `spec.responseRules` is
non-empty. API-server-side request matching excludes status updates,
deletion of `DNS/default`, and updates that remove all `responseRules`.
These exclusions must not depend on the webhook server being reachable.
Thus removing all rules remains possible during a webhook outage; updates
that retain rules remain subject to validation. Changes to forwarding
configuration are also validated when `responseRules` remain present.

The webhook is non-mutating, declares `sideEffects: None`, and supports
server-side dry-run. It performs no DNS lookups, writes no cluster
resources, and does not depend on CoreDNS readiness to start.

Matching requests fail closed: use `failurePolicy: Fail` with an explicit,
bounded request timeout. If required cluster configuration cannot be
read, the webhook returns an error rather than assuming no protected zone
exists. Invalid-rule errors and validation-unavailable errors must be
distinguishable to the administrator. While validation is unavailable,
otherwise-valid matching updates may also be rejected; this is the
availability tradeoff of the submission-time guarantee.

The webhook endpoint and certificate trust are established before its
fail-closed registration is activated. Its Deployment, Service, and
certificate resources are outside its own admission scope, so their
recovery is not blocked by this webhook. Operator/webhook upgrade
sequencing must preserve validation availability and compatibility for
supported API versions. Relevant configuration changes after admission
are handled by operator reconciliation, not assumed to have been
prevented by admission.

### Risks and Mitigations

* **A malformed or conflicting rule degrading cluster DNS.** Mitigated by
  the layered schema/webhook/reconciliation validation above, plus a
  synchronous, in-process validation of the `template` stanzas
  `spec.responseRules` renders, using CoreDNS's own
  `template` plugin code, newly vendored into the operator and kept aligned
  with `r.CoreDNSImage` by a dedicated CI check — before the candidate is
  ever written to the `dns-default` ConfigMap. A setup error is a
  definitive signal: there is no scheduling, image pull, or asynchronous
  readiness step that could produce an ambiguous result, so every
  validation attempt within a reconcile either confirms the rendered
  stanzas or rejects them outright. On a rejection, the operator keeps the
  last-known-good Corefile; if none exists yet, it excludes only the
  specific rule(s) the failure is attributable to and revalidates the
  reduced set from scratch, or, if the failure isn't attributable to a
  specific rule, falls back to a revalidated candidate with
  `spec.responseRules` omitted entirely. All of this is surfaced via a
  status condition naming what was excluded and why (see Workflow
  Description steps 4-5).
* **A self-referencing or cyclic `cname` rule crashing CoreDNS entirely.**
  Verified empirically: a `cname` rule targeting its own query name, two
  `cname` rules forming a cycle, or a wildcard `cname` rule whose target
  falls within its own match pattern, all cause CoreDNS's internal
  CNAME-chase to loop with no depth limit, consuming memory until the
  process is OOM-killed — an outage for every query the instance was
  serving, not a contained failure of the one bad rule. Literal
  self-reference, cross-rule cycles, and an unintercepted wildcard
  self-match are rejected before persistence by admission validation.
  Cycle detection follows the rule that actually wins precedence at each
  step; a terminating higher-precedence exact-name override remains valid.
  The operator repeats these checks defensively during reconciliation.
* **Exact-name precedence silently not applying to a query type the exact
  rule's action doesn't render.** The only pairing where this can happen is
  an exact `a` rule overlapping a wildcard `cname` rule (`cname`'s type set
  is a superset of `a`'s in every other direction, and `nxdomain`'s `ANY
  ANY` stanza already covers every type). There is no CoreDNS `template`
  mechanism to make a stanza match a name without producing a response —
  `fallthrough` only applies when the regex does *not* match, so an
  answer-less stanza for the exact name would return an empty `NOERROR`
  rather than passing through to normal resolution. Mitigated by rejecting
  this one pairing at admission until a real precedence mechanism for
  it exists (see [Implementation Details](#implementation-detailsnotesconstraints)
  and [Open Questions](#open-questions-optional)).
* **The validating webhook being unavailable blocks an otherwise-valid
  `DNS/default` update.** Because the webhook fails closed
  (`failurePolicy: Fail`), an outage rejects any matching update (one that
  adds, changes, or retains `spec.responseRules` entries), not only an
  invalid one — this is an inherent tradeoff of a submission-time
  guarantee, not a bug. Mitigated three ways: the webhook's registration
  narrowly excludes status updates and updates that remove all
  `responseRules` entries, and that exclusion is evaluated by the API
  server itself rather than by calling the webhook, so administrators
  always retain a way to revert to no custom rules during an outage; a
  validation-unavailable error is returned distinctly from an invalid-rule
  error, so the failure mode is diagnosable rather than looking like a
  rejected rule; and the webhook's own availability/latency are tracked as
  SLIs (see Operational Aspects) so an outage is visible before it is
  reported as a mystery.
* **A `spec.template` (NE-2118) stanza and a `spec.responseRules` stanza
  both matching the same query type in the default server block.**
  Mitigated by rendering all `spec.responseRules`-derived stanzas (exact
  rules, then wildcard rules) before `spec.template`'s stanza: a per-name
  administrator rule is more specific than a zone/type-wide template and
  must get the first chance to answer. `spec.template` is then reached
  only for queries none of `spec.responseRules`'s stanzas match.
* **Unrelated queries turning into SERVFAIL or losing fallthrough.** Every
  generated stanza, including the exact-name NXDOMAIN case, includes
  `fallthrough`; without it, a template whose zone matches a query but whose
  regex does not would return `SERVFAIL` instead of continuing normal
  resolution, since these rules share the default server's zone with every
  other query it serves.
* **Stale answers after a rule is removed/changed.** Bounded by
  `min(ttlSeconds, spec.cache.positiveTTL)` for `a`/`cname` and by
  `min(ttlSeconds, spec.cache.negativeTTL)` for `nxdomain` — the generated
  Corefile's `cache` directive (`cache {{.PositiveTTL}} { denial 9984
  {{.NegativeTTL}} }`) sets no `MINTTL` argument on either the success or
  denial cap, so each falls back to the `cache` plugin's own documented
  default minimum TTL of 5 seconds. That floor is below `ttlSeconds`'s own
  enforced minimum of 30s, so it never raises the effective TTL above
  `ttlSeconds` for any rule this enhancement can produce; only the caps
  above can lower it further. `ttlSeconds` itself is bounded 30-3600s,
  default 60s. Documented in user-facing docs as the maximum
  staleness window; see the `nxdomain` negative-caching discussion above
  for why the SOA minimum field, not a separate value, is what sets its
  side of this bound.

### Drawbacks

This adds a second, structurally different set of fields under the same
feature gate as NE-2118 (`spec.template`, singular/zone-scoped, vs.
`spec.responseRules`, a list of per-name rules). An alternative of unifying
both under one mechanism was considered; see Alternatives.

This also adds a new validating webhook with its own availability
requirement: because it fails closed, a webhook outage can block an
otherwise-valid `DNS/default` update that retains response rules (adding,
changing, or leaving `spec.responseRules` non-empty), not just an invalid
one. The documented recovery path during such an outage is removing all
`responseRules` entries, which the webhook's registration explicitly
excludes from matching (see
[Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)),
so an administrator is never blocked from reverting to no custom rules at
all, even while the webhook itself is unavailable.

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
* **A short-lived, Service-less canary Pod** running the real
  `r.CoreDNSImage` with the candidate Corefile mounted in place of the real
  one, polled on its `/ready` endpoint, to validate a candidate before
  writing it to the `dns-default` ConfigMap. This was an earlier design for
  the same requirement implemented in this enhancement
  (see Workflow Description steps 4-5). Rejected in favor of the in-process
  validation of the `spec.responseRules`-rendered `template` stanzas
  described there: a canary Pod's outcome depends on cluster scheduling,
  image pull, and the operand's own asynchronous readiness — none of which
  says anything about whether a rule's rendered stanza is valid — which
  required a three-way inconclusive/confirmed-invalid/confirmed-valid
  classifier, bounded retries with backoff, and a separate status condition
  just to avoid treating transient infrastructure noise as a rule failure.
  The in-process call has no such dependency: it runs in the same process,
  against only the stanzas this enhancement renders, with no Pod,
  scheduling, or image pull involved, so it returns a definitive
  valid/invalid result synchronously, with no extra RBAC or classification
  logic to maintain.
* **A `ValidatingAdmissionPolicy` with a `paramRef`** to the cluster's
  `Ingress` and `Infrastructure` config objects, instead of a dedicated
  validating webhook, for the protected ingress-domain and API-hostname
  checks. Rejected for this enhancement: a
  `ValidatingAdmissionPolicy` expresses its policy in CEL against the
  parameter resource, which still cannot perform the precedence-aware
  `cname` cycle detection (a graph-traversal computation over the whole
  `spec.responseRules` list) that the same admission layer also needs to
  perform, so that check would still require a webhook regardless; running
  both checks together in one operator-managed webhook, sharing the same
  Go validation logic as reconciliation, was simpler than splitting them
  across two different admission mechanisms.
* **Unmanaged DNS Operator / external DNS server.** These are exactly the
  unsupported workarounds this enhancement exists to replace.

## Open Questions [optional]

1. HCP/Hypershift-specific considerations are not yet captured.
2. An exact `a` rule overlapping a wildcard `cname` rule for the same name
   is rejected at admission (see
   [Implementation Details](#implementation-detailsnotesconstraints))
   because the CoreDNS `template` plugin has no way to make a stanza match
   a name without producing a response, so AAAA/CNAME queries against the
   exact name cannot be reserved for normal resolution the way the A query
   is. A real precedence mechanism for this pairing — for example,
   restructuring how `a` and `cname` rules render, or a future CoreDNS
   `template` capability — is not yet designed.

## Test Plan

**Note:** *Section not required until targeted at a release.*

* Unit/pre-merge tests (NE-1728) for schema validation (malformed names,
  invalid addresses/targets, wildcard NXDOMAIN rejection, conflicting rules,
  a rule overlapping a configured `forwardPlugin` zone, and a rule falling
  under `cluster.local` or a reverse zone — all rejected by the API server
  at submission, not merely reported later) and for Corefile rendering per
  rule type, including: an exact rule's
  `template` stanza is always rendered before an overlapping wildcard
  rule's stanza regardless of the order the rules appear in
  `spec.responseRules`; an exact `a` rule overlapping a wildcard `cname`
  rule for the same name is rejected at admission (schema/CEL) rather than
  ever reaching reconcile time or the Corefile; a `cname` rule renders
  `template` stanzas for `A`, `AAAA`, and `CNAME` query types only, with a
  query of a different type (e.g. MX, TXT) against the same name left
  unaffected; an `ANY` query against an `a` rule and against a `cname` rule
  is answered by the first-declared matching stanza rather than falling
  through — an A-record answer and a bare `CNAME` with no chased address,
  respectively; literal self-reference, cross-rule cycles, and an
  unintercepted wildcard self-match are rejected before persistence by
  admission validation, with cycle detection following the rule that
  actually wins precedence at each step, and the operator repeating these
  checks defensively during reconciliation; a wildcard `cname` rule whose
  target falls within its own pattern is accepted, not rejected, when a
  higher-precedence exact rule overrides that exact target name, since the
  chain then terminates through the exact rule rather than looping; a
  rendered rule stanza that is syntactically invalid (for example, a
  `template` `match` regex that fails to compile) is caught by the
  in-process validation call before it ever reaches the `dns-default`
  ConfigMap, returning an error within the same function call rather than
  requiring any external process or polling; a reduced candidate (with an
  attributable invalid rule excluded) that still fails revalidation because
  the remaining rules independently conflict is rejected rather than
  applied; validation is scoped to only the stanzas `spec.responseRules`
  renders, asserted directly (a Corefile with a deliberately invalid
  `kubernetes`/`forwardPlugin`/`reload` stanza elsewhere still validates
  successfully, since those stanzas are never part of the validated input);
  repeated validation calls across reconciles produce the same result for
  the same input and leave no state behind that affects a later call; and,
  when both `spec.template` and `spec.responseRules` are configured,
  `spec.responseRules`-derived stanzas are rendered before `spec.template`'s
  stanza. Also includes a dedicated test that an `nxdomain` rule's rendered
  SOA record's minimum field equals the rule's `ttlSeconds`, not a fixed
  value, for both the default `ttlSeconds` and an explicitly-set one.
* A pre-merge test (NE-1728) that the complete rendered Corefile — every
  server block CoreDNS actually runs, including `kubernetes`, `forwardPlugin`,
  `cache`, `reload`, and any configured `spec.template`/`spec.responseRules`
  stanzas — parses successfully end to end, using fixture Corefiles
  representative of real cluster configurations (including a `forwardPlugin`
  configured with TLS upstreams, using test-fixture CA files, so the
  `forward` plugin's own `Setup()` is exercised too). This is independent of
  the narrower, in-process `spec.responseRules`-stanza validation described
  above: it is CI coverage that the generator as a whole keeps producing a
  Corefile CoreDNS can start with, not a per-reconcile runtime guarantee,
  and it does not require the operator itself to have a mechanism for
  supplying the operand's real CA files at reconcile time.
* E2E tests (NE-1729) against a running cluster: positive resolution for `a`/
  `cname`/`nxdomain`, negative cases (rule rejected, last-good Corefile
  retained), update and removal of rules, and upgrade persistence of
  `spec.responseRules` and its effect. Includes a downgrade-then-upgrade
  round trip: back up `spec.responseRules` before downgrading, confirm the
  downgraded operator serves CoreDNS without the rules and that the
  `ValidatingWebhookConfiguration` is removed immediately on downgrade (not
  merely the webhook's Deployment and Service), then confirm reapplying the
  backed-up rules after upgrading restores the configured behavior and
  re-registers the webhook. Also includes an explicit precedence test: a
  `cname` exact-name
  rule and an overlapping `a` wildcard rule (the pairing where precedence
  is actually guaranteed, since `cname`'s rendered types are a superset of
  `a`'s), confirming the exact-name answer wins for its one name across
  every type `cname` covers and the wildcard answer is returned for every
  other matching name; and a rejection test confirming an exact `a` rule
  overlapping a wildcard `cname` rule is refused at admission rather than
  producing an incorrect or empty answer. Also includes a coexistence
  test with `spec.template` (NE-2118) configured alongside
  `spec.responseRules`, confirming `spec.responseRules` answers take
  precedence for the query types and names they cover. Also includes a
  `cname` target-failure matrix against the target release's CoreDNS
  build: successful resolution, true NODATA (target has no record of the
  queried type), target NXDOMAIN, target SERVFAIL, and an upstream lookup
  failure, confirming the client-visible response in each of the latter
  four cases is the documented `NOERROR`-plus-bare-`CNAME` shape. Also
  includes explicit recovery tests: an operator restart while
  `spec.responseRules` holds a rule that reconcile-time validation would
  now reject (confirming the operator keeps serving its last-known-good
  Corefile rather than looping or crashing); `dns-default` ConfigMap
  recreation under the same condition, confirming the operator excludes
  only the attributable rule, revalidates the reduced set (reconcile-time
  checks plus a fresh in-process validation pass), and applies it; and the
  same ConfigMap recreation with validation failing for a reason not
  attributable to any single rule (for example, a `Setup()` error outside
  the `spec.responseRules`-derived stanzas), confirming the operator falls
  back to a revalidated candidate with `spec.responseRules` omitted
  entirely rather than producing no Corefile at all.
* Admission webhook integration/E2E tests against a running cluster: an
  ingress-domain conflict (sourced from `Ingress/cluster`'s `spec.domain`)
  and an API-hostname conflict (sourced from `Infrastructure/cluster`'s
  `status.apiServerURL`/`status.apiServerInternalURI`) are tested
  independently of each other, each rejected by the API before persistence
  with the stored `DNS/default` spec unchanged, including the case where
  only one of the two sources conflicts and the other does not; an
  external-API-hostname-only conflict and an internal-API-hostname-only
  conflict are also tested independently, since `apiServerURL` and
  `apiServerInternalURI` can differ; a rule matching a bare IP address in
  either API-hostname field, and a rule submitted while one of the two
  fields is absent, are both handled per the documented behavior rather
  than panicking or silently skipping the check; non-conflicting
  administrator application domains remain accepted; `cname` cycles are
  rejected before persistence, and terminating exact-name overrides remain
  accepted; forwarding configuration changes are checked against retained
  `responseRules`; missing/unreadable required cluster configuration,
  webhook timeout, endpoint outage, and certificate/trust failures reject
  matching requests with validation-unavailable errors rather than
  accepting unchecked rules; status updates and removal of all
  `responseRules` remain possible while the webhook is unavailable, and
  partial edits that retain rules do not bypass it; dry-run evaluates the
  same policy without writing cluster resources; and bootstrap, certificate
  rotation, supported topologies, operator/webhook upgrades, and recovery
  all preserve the documented admission behavior. Also includes a test that
  a change to either `Ingress/cluster` or `Infrastructure/cluster` made
  after a rule was already admitted triggers reconciliation and preserves
  the documented DNS retention/fallback guarantees.
* Transition-window tests exercise matching `DNS/default` writes during
  initial enablement, upgrade, and any supported removal/downgrade path,
  not only before/after snapshots:
  * The API must never persist new or updated non-empty `responseRules`
    without the required admission validation being effective.
  * Before enabling acceptance of the field, demonstrate that an actual
    protected-domain conflict is rejected; do not rely only on checking
    that the `ValidatingWebhookConfiguration` object exists.
  * With registration active but the serving endpoint unavailable,
    matching requests fail closed rather than bypassing validation.
  * During removal, admission protection remains effective for as long
    as non-empty `responseRules` can still be accepted. Registration is
    removed only after those writes can no longer be accepted unchecked.
  * Confirm registration removal before its serving resources disappear.
  * Status updates and removal of all `responseRules` remain possible
    throughout the transition under the documented matching exclusions.
  * Exercise configuration backup and restore for supported transitions
    whose target schema does not preserve the field.
* CI wiring (NE-1730) to run all of the above as part of standard
  presubmit/periodic jobs, consistent with other `DNSTemplatePlugin`-gated
  behavior, plus a new presubmit check that fails the build if the
  `template` plugin version vendored into the operator for validation
  diverges from the CoreDNS version referenced by `r.CoreDNSImage` for the
  same release.

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
- Pre-merge and e2e test coverage per the Test Plan above, in CI, including
  the validating webhook's own install, upgrade, and fail-closed recovery
  tests.
- User documentation with a configuration example and limitations
  (tracked downstream in OSDOCS-11148).

### Tech Preview -> GA

- Upgrade/downgrade tested explicitly, including that downgrade removes the
  `ValidatingWebhookConfiguration` (see Upgrade / Downgrade Strategy).
- Sufficient operator/SRE feedback on status-condition clarity for invalid
  rules, and on the validating webhook's availability/latency SLIs under
  real cluster conditions.
- User-facing documentation in openshift-docs.

### Removing a deprecated feature

Not applicable: this enhancement adds a new, optional, feature-gated field
and does not deprecate or remove any existing DNS Operator behavior.

## Upgrade / Downgrade Strategy

On upgrade, the operator regenerates the Corefile from the same
`spec.responseRules` on the new release. No administrator action is required
to retain behavior across an upgrade.

Downgrade behavior must account for both the `responseRules` API field and
the validating webhook's registration. A target release's schema may not
preserve `responseRules`. Administrators must back up the configuration
before any supported transition that can remove the field, and the
corresponding restore behavior must be documented and tested.

The required teardown order is:

1. Prevent acceptance of unvalidated non-empty `responseRules`. While the
   API still accepts the field, equivalent admission protection must
   remain effective; reconciliation alone is not a substitute.
2. Remove the `ValidatingWebhookConfiguration` through an explicitly
   identified cleanup component or supported procedure.
3. Confirm registration removal before removing the webhook's serving
   Deployment, Service, or required certificate resources.

Cleanup must not rely on an older DNS Operator release having newly
introduced cleanup logic. Likewise, omitting or feature-gate-excluding an
object from a payload is not assumed to delete that object.

Payload-managed registration is a candidate, not a confirmed cleanup
mechanism. If CVO performs deletion, the design must identify the explicit
deletion request, the component or release that supplies it, and the
ordering relative to API-field availability and webhook teardown. The
documented `release.openshift.io/delete` mechanism must not be confused
with automatic pruning of an excluded manifest.

Automatic downgrade cleanup is not claimed until the responsible component
or procedure, supported source/target transitions, and ordering have been
confirmed and covered by transition-window tests. Any transition that
cannot preserve the admission contract must be explicitly excluded from
the supported feature lifecycle unless an exception is agreed.

## Version Skew Strategy

The validating webhook introduces coordination between API-field
availability, admission registration, and the webhook serving components.
These resources must not be treated as an atomic update merely because
they are delivered in the same release payload.

Before the API accepts non-empty `responseRules`, the registration must be
effective for matching requests. If the registered webhook is temporarily
unavailable, `failurePolicy: Fail` rejects those requests rather than
allowing unchecked rules. Merely observing the registration object is not
sufficient test evidence; admission behavior must be exercised.

The inverse ordering applies during removal: prevent acceptance of
unvalidated non-empty `responseRules` before unregistering the webhook,
then remove its serving components. The status-update and remove-all-rules
exclusions described above remain available throughout the transition.

**Blocking lifecycle decision:** before this design is considered
complete, identify the registration owner and the concrete installation,
feature-enablement, upgrade, and removal sequence. This includes the
ordering relative to the API schema/feature-gate changes and compatibility
between operator and webhook versions during rollout.

Shipping the registration as a payload manifest alongside the gated API
manifest is one candidate. It must be confirmed to enforce the required
ordering; co-location in the payload is not itself proof. The agreed
mechanism and its transition-window test coverage must replace this open
decision before the feature's lifecycle guarantees are declared complete.

## Operational Aspects of API Extensions

* SLI: `DNS/default` status condition reflecting whether `responseRules`
  reconciled successfully (healthy) or a specific rule is invalid/rejected
  (degraded), analogous to existing DNS Operator conditions; validating
  webhook availability and admission request latency, since a webhook
  outage or slowness is now directly in the path of accepting a
  `spec.responseRules` write (see
  [Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)).
* Impact on existing SLIs: expected rule counts are small (capped at 100
  per the proposed `MaxItems`); the webhook's impact on API request
  latency for `DNS/default` writes has not been measured and should be
  established during implementation rather than assumed negligible.
* Failure mode: there are two distinct paths, not one. An invalid or
  conflicting submitted rule is rejected by schema/admission or the
  validating webhook before it is ever persisted, so it never reaches
  reconciliation or CoreDNS at all. A rule that was valid when accepted but
  whose generated Corefile stanza later fails validation, or whose
  protected-domain context changes after acceptance, is caught at
  reconcile time: the operator keeps serving the last-known-good Corefile,
  falling back first to excluding only the attributable rule(s) and
  revalidating, then to omitting `spec.responseRules` entirely and
  revalidating, before ever treating this as a problem outside this
  enhancement's scope. CoreDNS availability is not impacted by a single bad
  rule either way.

## Support Procedures

* An admission-time rejection (schema/CEL or the validating webhook) and a
  reconciliation-time status condition are distinct and must not be
  confused: a rejected submission is never written to `DNS/default` at
  all — the administrator sees the rejection immediately in the response
  to their own `apply`/`edit`, there is nothing new to look up afterward,
  and it does not itself create a new rejected-rule status entry. A
  `DNS/default` status condition, by contrast, only ever reflects a rule
  that was accepted at submission but later failed reconcile-time
  validation (a generated-stanza error, or a protected-domain context
  change after acceptance) — it is visible via the status condition,
  naming the offending rule and the reason, and removing the offending
  entry from `spec.responseRules` and resubmitting clears it on the next
  reconcile.
* If the validating webhook itself is unavailable, a matching request
  (one that would add, change, or retain `spec.responseRules` entries) is
  rejected with a validation-unavailable error distinct from an
  invalid-rule rejection; removing all `spec.responseRules` entries remains
  possible during the outage (see
  [Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)),
  and is the documented recovery path if an administrator needs to make an
  unrelated `DNS/default` change while the webhook is down.
* To remove a working response rule, delete its entry from
  `spec.responseRules` and resubmit. Do not rely on disabling the
  `DNSTemplatePlugin` feature gate to remove rules: `spec.featureSet`
  cannot be changed once a cluster is on `DevPreviewNoUpgrade` or
  `TechPreviewNoUpgrade`, so disabling the gate is not a supported recovery
  path on those clusters.

## Infrastructure Needed [optional]

Beyond existing `openshift/api` and `cluster-dns-operator` CI: the
validating webhook's own Deployment and Service, serving-certificate
issuance and rotation (consistent with how other OpenShift operators
manage webhook certificates), its `ValidatingWebhookConfiguration`
registration, the operator permissions needed to manage those resources
and to read the cluster configuration the webhook's protected-domain
checks depend on (see
[Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)),
and CI coverage for the webhook's own install/upgrade/recovery and
admission-integration test suites (see Test Plan).
