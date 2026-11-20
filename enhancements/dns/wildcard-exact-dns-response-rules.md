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
last-updated: 2026-11-12
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
  for every query type; wildcard patterns replace exactly one leading
  label, so two distinct patterns never match the same name; conflicting
  rules are rejected at submission or reconcile time rather than silently
  resolved or silently answered incorrectly.
* Validate rules at submission (schema/admission and the validating
  webhook), so an invalid or conflicting rule is rejected before it is ever
  persisted, and again before the generated Corefile is applied, so an
  invalid rule cannot make CoreDNS unusable. Reconciliation independently
  repeats this validation and additionally handles cases submission cannot:
  cluster context that changes after a rule was accepted, and pre-apply
  validation failures against the generated Corefile. On either kind of
  failure found at reconcile time, keep serving the last-known-good
  Corefile and report a clear status condition, except that a rule newly
  conflicting with a protected name is excluded immediately.
* Preserve existing resolution behavior for any name not covered by a rule:
  cluster service/ingress discovery and configured `forwardPlugin` zones.
  A name a rule covers is answered by that rule for every query type (an
  `a` rule answers NODATA for types other than A), so the name's existence
  is consistent across types.
* Make the configuration and its effects survive an OpenShift upgrade without
  administrator intervention.

### Non-Goals

* A general-purpose custom DNS server or arbitrary record types/regex
  matching — wildcards are limited to replacing exactly one leading
  label (e.g. `*.example.com`); administrators specify names, not regular
  expressions.
* IPv6/AAAA record responses — that is NE-2118's scope; this enhancement
  intentionally does not duplicate or modify that work's AAAA/NOERROR
  behavior. An `a` rule's NODATA answer to an AAAA query for its own name
  is part of that rule's name-existence behavior, not an AAAA record
  response.
* RFE-6814's Azure-specific custom search-domain handling — a distinct
  request within the same RFE from the wildcard A/CNAME need addressed by
  this enhancement (see Motivation); tracked separately, not an acceptance
  criterion here.
* Azure's installer/custom-DNS path (OCPSTRAT-991) — unrelated mechanism, not
  evidence this feature already exists.
* Response generation for record types or response codes beyond A, CNAME, and
  exact-name NXDOMAIN, other than the NODATA answer an `a` rule gives for
  other query types.
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
Deployment, Service, serving-certificate resources, and
`ValidatingWebhookConfiguration`, and to read the
cluster configuration the webhook needs for its protected-domain checks —
see [Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling).

1. The administrator adds one entry to `spec.responseRules` on `DNS/default`
   naming a rule, a query name (exact or single-label wildcard), a TTL, and
   exactly one action (`a`, `cname`, or `nxdomain`).
2. On submission, schema/admission validation checks each rule's shape
   (name pattern, exactly-one-action, address/target format, TTL bounds, an
   exact `cname` rule's literal self-reference) and cross-rule checks
   decidable from `spec.responseRules` and `spec.servers` together
   (duplicate/conflicting rule detection, overlap with a configured
   `forwardPlugin` zone,
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
   stanzas' setup result is known synchronously, before anything is
   written anywhere.

   Setup success establishes only that each stanza parses and its `match`
   regex and Go templates compile; answer records are generated when a query
   executes the template. The operator therefore also renders each rule
   against representative names (boundary label lengths, the longest
   allowed name) and checks that the resulting A records, CNAME target, and
   SOA parse as valid records. Runtime cost is bounded: each `a` rule
   accepts at most 8 addresses, so an answer fits a 512-byte UDP response
   even for a maximum-length name; templates contain no loops or lookups;
   and the only recursive path, the CNAME chase, is bounded by the cycle
   checks below. The availability guarantee is limited to what these checks
   establish.
5. If validation returns an error, the operator does not write the
   candidate: it keeps serving the currently-applied ConfigMap unchanged and
   sets a status condition on `DNS/default` naming the invalid rule(s) and
   the validation error. Exception: a rule that now conflicts with a
   protected API or ingress name is a safety failure. The operator excludes
   that rule and applies the remaining configuration immediately, even when
   a prior ConfigMap exists, because keeping it would keep serving the
   prohibited override; other valid changes, such as forwarding updates,
   are applied in the same pass, and the excluded rule is named in the
   status condition. If there is no currently-applied ConfigMap to fall
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
// precedence over a wildcard rule for the same query. A wildcard replaces
// exactly one leading label, so distinct wildcard patterns never overlap.
// Queries that do not match any rule
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

On HCP, HyperShift's control-plane-operator runs the DNS Operator in the
hosted control plane namespace on the management cluster, managing the
guest cluster through a kubeconfig. The webhook runs in the guest cluster:
the operator creates its Deployment, Service, and
`ValidatingWebhookConfiguration` in the guest through that kubeconfig, the
same code path as on standalone clusters, and the hosted kube-apiserver
reaches the webhook Service through its konnectivity egress path, as it
does for any guest webhook. The webhook reads `Ingress/cluster` and
`Infrastructure/cluster` from the guest cluster, where `DNS/default` also
lives. Its pod runs on guest worker nodes; if no worker is available,
matching writes are rejected (fail closed) while removing all rules
remains possible.

HCP support needs two changes: the webhook's RBAC, ServiceAccount, and
NetworkPolicy manifests gain the `include.release.openshift.io/hypershift`
annotation, and the operator takes the webhook image from an environment
variable instead of reading its own Deployment, which does not exist in
the guest cluster. The variable is set by the payload manifest on
standalone clusters and by HyperShift's control-plane-operator on HCP, a
small change in the HyperShift repository. Until both changes land, when
`Infrastructure.status.controlPlaneTopology` is `External` the operator
does not create the webhook or serve any `spec.responseRules`, and sets a
status condition stating that response rules are not yet supported on
this topology, so rules are never served without admission validation.

#### Standalone Clusters

This is the primary target; no additional considerations.

#### Single-node Deployments or MicroShift

On single-node OpenShift, `spec.responseRules` behaves as on standalone
clusters; the webhook adds one Deployment. Applicability to MicroShift is
**unconfirmed**: MicroShift manages DNS through its own configuration and
Corefile path rather than the DNS Operator, so whether this `DNS/default`
API applies at all needs a MicroShift reviewer. The validating webhook's
applicability to MicroShift is a separate **unverified** question — whether MicroShift's API
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

* `a` → two stanzas:
  * `template IN A <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN A <addr>" ... ; fallthrough }`,
    with one `answer` line per configured address;
  * `template ANY ANY <zone> { match <name-regex> ; authority "<zone> <ttl> IN SOA ns.invalid. admin.invalid. 1 60 60 60 <ttl>" ; fallthrough }`,
    rendered after the `IN A` stanza, so every other query type for the
    name gets NOERROR with no answer (NODATA), negatively cached for
    `ttlSeconds` through the SOA minimum field, as for `nxdomain`.
* `cname` → `template ANY ANY <zone> { match <name-regex> ; answer "{{ .Name }} <ttl> IN CNAME <target>" ; fallthrough }`,
  so every query type for the name gets the CNAME, as RFC 1034 specifies.

  Every action therefore answers every query type for the names it
  matches, so a ruled name never falls through to upstream resolution for
  some types and not others. This keeps name existence consistent: a
  caching resolver never receives an upstream NXDOMAIN for one type of a
  ruled name and the synthetic answer for another, which RFC 2308 negative
  caching would apply to the whole name. The trade-off is that real
  records of other types published upstream for a ruled name (for example
  MX, TXT, or SRV) are hidden while the rule exists (see Risks). The
  `template` plugin behavior these stanzas rely on is taken from its source
  (`plugin/template/template.go`) and is verified by the tests in the Test
  Plan.

  A client `ANY`-type query against a ruled name matches the first stanza
  for that name, whatever that stanza's configured type.
  CoreDNS's `template` plugin treats a stanza's configured type and the
  incoming query's type as matching whenever *either* side is `ANY`
  (confirmed in `plugin/template/template.go`'s `match()`: the type check
  only rejects a stanza when both the stanza's type and the query's type
  are non-`ANY` and unequal). Consequently an `ANY` query against a ruled
  name is intercepted and answered by whichever stanza for that name is
  declared first in the Corefile (exact before wildcard, per the ordering
  rule below). For an `a` rule this returns the
  configured A record(s), the same as an `A` query would get. For a
  `cname` rule this returns the bare synthesized `CNAME` record with **no**
  address bundled in: the internal CNAME-chase described next only
  triggers when the query type is literally `A` or `AAAA`
  (`state.QType() == dns.TypeA || state.QType() == dns.TypeAAAA` in
  source), which excludes `ANY`. This is the intended behavior this
  enhancement exposes for `ANY`, not a gap to close — returning one
  representative record rather than enumerating every type matches modern
  authoritative-server guidance (RFC 8482) and needs no additional
  mechanism.

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
  (`plugin/template/template.go`, `plugin/pkg/upstream/upstream.go`).
  CoreDNS attempts to resolve the target and include the result in the
  response; whether a client sends its own follow-up query depends on the
  resolver and the response it receives.

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
  rule for that name if one exists, otherwise the one matching wildcard
  rule, per the same precedence [Goals](#goals) establish —
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

`nxdomain` matches the exact name only, but a resolver implementing
[RFC 8020](https://www.rfc-editor.org/rfc/rfc8020.html#section-2) may treat
descendants (for example `child.blocked.example.com`) as nonexistent after
caching the answer. This enhancement does not guarantee descendants keep
resolving; administrators who need that should not use `nxdomain` for the
parent name. A response action that preserves descendants is out of scope.

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

Because every action renders a stanza for every query type at its names
(see the stanza list above), rendering exact-name stanzas first gives
exact-over-wildcard precedence for every query type and every combination
of actions: an exact rule answers all queries for its name, and an
overlapping wildcard rule answers every other name it matches. Within an
`a` rule, the `IN A` stanza is rendered before its NODATA stanza.

All names and targets are canonicalized once, before any check: lowercased,
a single trailing dot removed, each label 1–63 octets and the whole name at
most 253 octets, with regex metacharacters escaped when the `match` regex
is rendered. Matching, protected-name, duplicate, forwarding-overlap, and
cycle checks all use this canonical form.

Validation is layered across schema/static admission, the validating
webhook, and operator reconciliation. Generated template-stanza validation
is an additional pre-apply check described in Workflow step 4.

* **Schema/CRD (submission time), per rule:** name pattern (exact or
  single-label wildcard), exactly-one-of `a`/`cname`/`nxdomain`,
  `nxdomain` rejected when the name is a wildcard, address/target format,
  at most 8 addresses per `a` rule, TTL bounds, and an exact-name `cname` rule's target rejected when it
  equals the rule's own query name — an exact rule's precedence never
  depends on any other rule, so this one case of self-reference is a fixed,
  unconditional error decidable from the rule alone.
* **Schema/CRD (submission time), cross-rule:** everything else decidable
  from `spec.responseRules` and `spec.servers` alone, since both are
  fields on the one `DNS/default` object being submitted and need no
  cluster context beyond the object itself: rejection of two rules with the
  same normalized selector, regardless of action or value; and rejection
  of a rule whose name overlaps a configured `forwardPlugin` zone
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
discussion above.

Two rules with the same normalized selector (the same exact name, or the
same wildcard pattern) are rejected regardless of action or value, so an
answer never depends on render order; multiple addresses belong in one
`a` rule. An exact-name rule and a wildcard rule that both match a name
are not a conflict: every action covers every query type at its names
and exact-name stanzas are rendered first, so the exact rule answers its
own name and the wildcard rule answers every other name it matches.

### Admission Webhook Lifecycle and Failure Handling

The DNS Operator owns the webhook at runtime, including the
`ValidatingWebhookConfiguration`; only the webhook's RBAC, ServiceAccount,
NetworkPolicy, and ServiceMonitor ship as payload manifests. When the
`DNSTemplatePlugin` gate is enabled, the operator creates a Service
annotated for a service-ca serving certificate, a ConfigMap annotated for
CA-bundle injection, and the webhook Deployment (the operator's own image,
taken from an environment variable, with `--response-webhook-only`, one
replica). It waits until the
certificate and CA bundle are populated and the Deployment has an available
replica, and only then registers the webhook (`failurePolicy: Fail`, 5s
timeout, `sideEffects: None`, matching CREATE and UPDATE of `DNS/default`
with non-empty `spec.responseRules`), so the fail-closed registration never
exists before its endpoint. With one replica, a restart of the webhook pod
rejects matching writes until it is available again; the replica count is
to be decided before GA.

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
  validation attempt within a reconcile either passes or rejects the
  rendered stanzas outright; passing establishes only what Workflow step 4
  describes. On a rejection, the operator keeps the last-known-good
  Corefile (except for a rule newly conflicting with a protected name,
  which is excluded immediately); if none exists yet, it excludes only the
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
* **Real records hidden by a rule.** A rule answers every query type for
  the names it matches, so records of other types published upstream for
  those names (for example MX, TXT, or SRV) are not served while the rule
  exists; for a wildcard rule this applies to every name the pattern
  matches. Mitigated by documenting this in user-facing docs and by keeping
  wildcard patterns narrow; administrators who need other record types at a
  name should publish that name in authoritative DNS instead.
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
  `min(ttlSeconds, spec.cache.negativeTTL)` for `nxdomain` and for the
  NODATA answers of `a` rules — the generated
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
  pass/fail result for the checks in Workflow step 4 synchronously, with no extra RBAC or classification
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

1. HCP support depends on a small HyperShift change that passes the webhook
   image to the DNS Operator (see Topology Considerations).

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
  `spec.responseRules`; an `a` rule renders an `IN A` stanza followed by a
  NODATA stanza whose SOA minimum equals `ttlSeconds`, so an MX or TXT query
  for the name gets NOERROR with no answer; a `cname` rule renders one `ANY`
  stanza, so every query type for the name gets the CNAME; an `ANY` query
  against an `a` rule and against a `cname` rule
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
  value, for both the default `ttlSeconds` and an explicitly-set one. Also
  includes tests that two rules with the same normalized selector are
  rejected regardless of action or value; that names differing only in case
  or a trailing dot are treated as the same name by every check; and that
  each rule's rendered answers for boundary-length names, multi-address A
  lists, CNAME targets, and SOAs parse as valid records.
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
  `spec.responseRules` and its effect. Includes, as a GA criterion, a
  downgrade-then-upgrade round trip between releases that contain the
  webhook removal logic: back up `spec.responseRules` before downgrading,
  confirm the downgraded operator serves CoreDNS without the rules and that
  the `ValidatingWebhookConfiguration` is removed before the webhook's
  Deployment and Service, then confirm reapplying the
  backed-up rules after upgrading restores the configured behavior and
  re-registers the webhook. Also includes precedence tests for an exact
  `cname` rule over a wildcard `a` rule and an exact `a` rule over a
  wildcard `cname` rule, confirming for A, AAAA, CNAME, MX, and TXT queries
  that the exact rule answers its own name and the wildcard answers every
  other matching name. Also includes caching-resolver tests through a
  recursive resolver in front of CoreDNS: AAAA then A, and TXT then A,
  against a ruled name, confirming the A answer is not withheld; and
  `blocked.example.com` then `child.blocked.example.com` for an `nxdomain`
  rule, recording whether the child is treated as nonexistent. Also
  includes a coexistence
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
  Corefile rather than looping or crashing); a protected API or ingress
  name changed to cover an accepted rule, confirming by querying CoreDNS
  that the override is no longer served and that a forwarding change made
  at the same time is applied; `dns-default` ConfigMap
  recreation under the same condition, confirming the operator excludes
  only the attributable rule, revalidates the reduced set (reconcile-time
  checks plus a fresh in-process validation pass), and applies it; and the
  same ConfigMap recreation with a validation error that maps to no single
  rule (injected into the validator by the test), confirming the operator
  falls back to a revalidated candidate with `spec.responseRules` omitted
  entirely rather than producing no Corefile at all. Errors in plugins
  outside the rendered stanzas are not visible to this validator and are
  covered by the whole-Corefile test above.
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
  the documented DNS retention/fallback guarantees. Also includes, on a
  hosted (HCP) cluster, that the webhook is created in the guest cluster
  and rejects a protected-domain conflict through the hosted
  kube-apiserver; and, before HCP support lands, that a cluster with an
  `External` control-plane topology serves no `spec.responseRules` and
  reports the unsupported condition.
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
2. The DNS Operator removes the `ValidatingWebhookConfiguration` once the
   DNS CRD schema no longer accepts `responseRules`, and waits until it is
   gone.
3. Confirm registration removal before removing the webhook's serving
   Deployment, Service, or required certificate resources.

Cleanup must not rely on an older DNS Operator release having newly
introduced cleanup logic. Likewise, omitting or feature-gate-excluding an
object from a payload is not assumed to delete that object.

Registration is operator-managed, so `release.openshift.io/delete` is not
involved. Cleanup is done by an operator release that contains this removal
logic; a downgrade to a release without it is not covered and must be
excluded from the supported lifecycle unless an exception is agreed. Dev
Preview and Tech Preview clusters use `NoUpgrade` feature sets and cannot
be upgraded, downgraded, or have the gate disabled, so transition
sequencing is a GA graduation criterion.

## Version Skew Strategy

The validating webhook introduces coordination between API-field
availability, admission registration, and the webhook serving components.
These resources are created in sequence by the operator and must not be
treated as an atomic update.

Before the API accepts non-empty `responseRules`, the registration must be
effective for matching requests. If the registered webhook is temporarily
unavailable, `failurePolicy: Fail` rejects those requests rather than
allowing unchecked rules. Merely observing the registration object is not
sufficient test evidence; admission behavior must be exercised.

The inverse ordering applies during removal: prevent acceptance of
unvalidated non-empty `responseRules` before unregistering the webhook,
then remove its serving components. The status-update and remove-all-rules
exclusions described above remain available throughout the transition.

**Lifecycle decision:** the DNS Operator is the registration owner (see
[Admission Webhook Lifecycle and Failure Handling](#admission-webhook-lifecycle-and-failure-handling)).
The webhook uses the operator's own image, so operator and webhook versions
differ only while the webhook Deployment rolls out. The transition-window
tests in the Test Plan exercise the sequence by delaying certificate or
Deployment readiness, and by keeping the CRD schema in place during removal.

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
  reconcile time: the operator keeps serving the last-known-good Corefile
  (except for a rule newly conflicting with a protected name, which is
  excluded immediately), falling back first to excluding only the attributable rule(s) and
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
