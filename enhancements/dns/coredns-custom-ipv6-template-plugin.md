---
title: coredns-custom-ipv6-template-plugin
authors:
  - "@grzpiotrowski"
reviewers:
approvers:
api-approvers:
creation-date: 2026-01-28
last-updated: 2026-01-28
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/NE-2118
see-also:
replaces:
superseded-by:
---

# Enable Custom IPv6 Responses via CoreDNS Template Plug-in

## Summary

This enhancement adds a `templates` field to the DNS operator API to configure
CoreDNS template plugins. The primary use case is filtering AAAA queries in
IPv4-only clusters to reduce DNS latency. The API supports AAAA filtering and
custom response generation, with an extensible design for future expansion to
additional record types, classes, and response codes.

## Motivation

In IPv4-only clusters, applications may query for both A and AAAA
records. CoreDNS forwards unresolvable AAAA queries to upstream resolvers,
adding latency per query. Filtering AAAA queries at CoreDNS eliminates
this delay and reduces upstream DNS load.

Additionally, some users need custom DNS responses for specific domains
without maintaining external DNS infrastructure. The CoreDNS template plugin
supports both use cases but lacks operator API integration.

### User Stories

* As a cluster administrator in an IPv4-only environment, I want to centrally
  configure AAAA query filtering for the entire cluster so that I can eliminate
  IPv6 lookup delays and reduce upstream DNS load without modifying individual
  pod configurations.

* As a network engineer, I want to configure custom IPv6 responses for specific
  domains so that I can route traffic without external DNS infrastructure.

* As an SRE, I want operator conditions and metrics for template configuration
  so that I can monitor DNS optimization effectiveness.

### Goals

* Add `templates` field to DNS operator API with extensible design for future
  expansion
* Enable AAAA filtering (primary use case) and custom response generation
* Support IN class, AAAA records, and NOERROR response initially
* Validate templates before applying to CoreDNS (both CRD schema validation via
  kubebuilder markers and validation in the DNS operator)
* Provide new Reasons in DNS operator conditions for template configuration status
* Protect dual-stack clusters with cluster.local exclusions: When a
  user configures AAAA filtering with zone "." on a dual-stack cluster, the
  operator automatically generates an exclusion template that allows cluster.local
  AAAA queries to reach the kubernetes plugin (preserving IPv6 service
  connectivity) while filtering external AAAA queries. Example: Template with
  `match "^(.*\.)?cluster\.local\.$"` and `fallthrough` is added before
  the user's filter template.

### Non-Goals

* Record types other than AAAA initially
* Response codes other than NOERROR initially
* DNS classes other than IN initially
* Automatic AAAA filtering on single-stack IPv4 clusters based on network
  topology detection (the operator will not automatically enable filtering)
* Regular expression-based zone matching

## Proposal

Add a `templates` field to the DNS operator API to configure CoreDNS template
plugins. The operator validates configurations, generates Corefile entries, and
provides status conditions. The API uses discriminated unions for extensibility.

### Workflow Description

1. Administrator configures template in DNS CR specifying zones and action
2. CRD validates API schema (required fields, enum values). Semantic validation
   by the operator: valid DNS names, no duplicate zone+queryType combinations,
   no reserved zones like cluster.local.
3. Operator detects dual-stack and auto-excludes cluster.local if needed
4. Operator generates Corefile with template blocks and reloads CoreDNS
5. CoreDNS processes matching queries per template rules (filter or custom
   response)

### API Extensions

This enhancement modifies the DNS operator CRD (`dns.operator.openshift.io`)
to add a `templates` field. The API uses typed enums and discriminated unions
for extensibility and type safety.

```go
// QueryType represents DNS query types supported by templates.
// +kubebuilder:validation:Enum=AAAA
type QueryType string

const (
	// QueryTypeAAAA represents IPv6 address records (AAAA).
	QueryTypeAAAA QueryType = "AAAA"
	// Future expansion: A, CNAME, etc.
)

// QueryClass represents DNS query classes supported by templates.
// +kubebuilder:validation:Enum=IN
type QueryClass string

const (
	// QueryClassIN represents the Internet class.
	QueryClassIN QueryClass = "IN"
	// Future expansion: CH (Chaos), etc.
)

// ResponseCode represents DNS response codes.
// +kubebuilder:validation:Enum=NOERROR
type ResponseCode string

const (
	// ResponseCodeNOERROR indicates successful query with or without answers.
	ResponseCodeNOERROR ResponseCode = "NOERROR"
)

// Template defines a template for custom DNS query handling.
type Template struct {
	// name is a required unique identifier for this template.
	// Must be a valid DNS subdomain as defined in RFC 1123.
	// Used for validation, error messages, and ensuring deterministic ordering
	// when templates have equal zone specificity. NOT translated to Corefile
	// (CoreDNS template plugin has no concept of template names).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// zones specifies the DNS zones this template applies to.
	// Each zone must be a valid DNS name as defined in RFC 1123.
	// The special zone "." matches all domains (catch-all).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Zones []string `json:"zones"`

	// queryType specifies the DNS query type to match.
	// Only AAAA is supported in the initial implementation.
	// Required field - cannot be omitted. To match ANY query type, this would
	// need to be supported explicitly in a future API version.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=AAAA
	QueryType QueryType `json:"queryType"`

	// queryClass specifies the DNS query class to match.
	// Only IN is supported in the initial implementation.
	// Required field - cannot be omitted. To match ANY query class, this would
	// need to be supported explicitly in a future API version.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=IN
	QueryClass QueryClass `json:"queryClass"`

	// action defines what the template should do with matching queries.
	// +kubebuilder:validation:Required
	Action TemplateAction `json:"action"`
}

// TemplateAction defines the action taken by the template.
// This is a discriminated union - exactly one action type must be specified.
//
// CoreDNS template plugin supports: answer, authority, additional stanzas, and
// rcode directive. The initial implementation focuses on the returnEmpty which
// sets rcode only
// Future expansion could add generateResponse action and support for authority/additional sections if needed.
// +union
// +kubebuilder:validation:XValidation:rule="(has(self.returnEmpty) && !has(self.generateResponse)) || (!has(self.returnEmpty) && has(self.generateResponse))",message="exactly one action type must be specified"
type TemplateAction struct {
	// returnEmpty returns an empty response with the specified RCODE.
	// This is useful for filtering queries (e.g., AAAA filtering in IPv4-only clusters).
	// When set, no answer/authority/additional sections are included.
	// Maps to CoreDNS template plugin: rcode directive only.
	// +optional
	// +unionDiscriminator
	ReturnEmpty *ReturnEmptyAction `json:"returnEmpty,omitempty"`

	// generateResponse generates a custom DNS response with an answer section.
	// (Note: future extension, field shown here for design purpose only)
	// This is useful for static DNS mappings (e.g., custom IPv6 responses for
	// specific domains without external DNS infrastructure).
	// Maps to CoreDNS template plugin: answer and rcode directives.
	// +optional
	GenerateResponse *GenerateResponseAction `json:"generateResponse,omitempty"`
}

// ReturnEmptyAction configures returning empty responses for filtering.
type ReturnEmptyAction struct {
	// rcode is the DNS response code to return.
	// NOERROR indicates success with no answer records (standard for AAAA filtering).
	// Only NOERROR is supported in the initial implementation
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NOERROR
	// +kubebuilder:default=NOERROR
	Rcode ResponseCode `json:"rcode"`
}

// GenerateResponseAction configures custom response generation.
type GenerateResponseAction struct {
	// answerTemplate is the template for generating the answer section.
	// Uses CoreDNS template syntax with available variables:
	// - .Name: the query name (e.g., "example.com.")
	// - .Type: the query type (e.g., "AAAA")
	// - .Class: the query class (e.g., "IN")
	//
	// For AAAA records, format: "{{ .Name }} <TTL> IN AAAA <ipv6-address>"
	// Example: "{{ .Name }} 3600 IN AAAA 2001:db8::1"
	//
	// The template must produce valid DNS answer format matching the query type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	AnswerTemplate string `json:"answerTemplate"`

	// rcode is the DNS response code to return with the generated answer.
	// Only NOERROR is supported for generated responses in the initial implementation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NOERROR
	// +kubebuilder:default=NOERROR
	Rcode ResponseCode `json:"rcode"`
}

// DNSSpec is the specification of the desired behavior of the DNS.
type DNSSpec struct {
	// <snip>

	// templates is an optional list of template configurations for custom DNS
	// query handling.
	// Each template defines how to handle queries matching specific zones and
	// query types.
	//
	// Templates are injected into ALL Corefile server blocks (both custom
	// servers from spec.servers and the default .:5353 block). This ensures
	// consistent behavior across all DNS resolution paths.
	//
	// Templates are evaluated in order of zone specificity (most specific first).
	// The kubernetes plugin always processes cluster.local queries before templates.
	//
	// IMPORTANT: AAAA filtering in dual-stack clusters requires careful configuration.
	// The operator automatically excludes cluster.local from broad filters to prevent
	// breaking IPv6 service connectivity.
	//
	// +optional
	Templates []Template `json:"templates,omitempty"`
}
```

**Example: AAAA Filtering**
```yaml
spec:
  templates:
    - name: filter-aaaa
      zones: ["."]
      queryType: AAAA
      queryClass: IN
      action:
        returnEmpty:
          rcode: NOERROR
```

**Example: Custom IPv6 Response**
```yaml
spec:
  templates:
    - name: legacy-ipv6
      zones: ["legacy.corp.example.com"]
      queryType: AAAA
      queryClass: IN
      action:
        generateResponse:
          answerTemplate: "{{ .Name }} 3600 IN AAAA 2001:db8::100"
          rcode: NOERROR
```

This enhancement does not modify existing DNS resource behavior. The template
plugin only affects queries matching configured zones and query types.

### Important Limitations and Warnings

**Dual-Stack Clusters**: AAAA filtering is designed for single-stack IPv4
clusters. In dual-stack clusters, the operator automatically excludes
cluster.local from filtering to preserve IPv6 service connectivity.

**Dual-stack detection:** The DNS operator detects dual-stack configuration by
reading the `Network.config.openshift.io/cluster` resource and checking for IPv6
CIDRs in `spec.clusterNetwork[].cidr` or `spec.serviceNetwork[]`. If any IPv6
CIDR is present (e.g., `fd01::/48`), the cluster is considered dual-stack.

**Corefile translation:** When zone `.` is configured with AAAA filtering in a
dual-stack cluster, the operator automatically generates an exclusion template
BEFORE the user's template:

```
template IN AAAA . {
    match "^(.*\.)?cluster\.local\.$"
    fallthrough
}
template IN AAAA . {
    rcode NOERROR
}
```

This ensures cluster.local AAAA queries fall through to the kubernetes plugin
while external AAAA queries are filtered.

**Template Validation**: Invalid answerTemplate syntax causes SERVFAIL
responses. Test configurations in non-production environments.

**Note**: Templates number limit to be considered.

### API Extensibility Design

The API uses discriminated unions for actions and typed enums for query types/classes/response codes. This enables future expansion:

- **New actions**: Add fields to `TemplateAction` for future CoreDNS template
  plugin capabilities
- **New query types**: Add values to `QueryType` enum (e.g., `A`, `CNAME`, `MX`).
- **New query classes**: Add values to `QueryClass` enum (e.g., `CH`).
- **New response codes**: Add values to `ResponseCode` enum (e.g., `NXDOMAIN`,
  `SERVFAIL`).

Structured action types enable validation and prevent arbitrary template syntax injection. Existing configurations remain compatible when new enum values are added.

### Template Ordering and Precedence

**IMPORTANT:** Templates defined in `spec.templates` are injected into **ALL**
Corefile server blocks (both custom servers from `spec.servers` and the default
`.:5353` block).

**Reasoning for global application:**
1. Reduces upstream AAAA query load for all configured upstreams
2. Users define templates once, they apply everywhere
3. Templates only activate when zones match
4. All DNS resolution paths benefit from AAAA filtering or custom responses

**Example:** If a custom server is configured for `example.com` with upstream
`10.0.0.1`, and a template filters AAAA queries for zone `.`, the template will
be injected into both server blocks:

```
# Custom server - template injected here
example.com:5353 {
    template IN AAAA . {
        rcode NOERROR
    }
    forward . 10.0.0.1
}

# Default server
.:5353 {
    template IN AAAA . {
        rcode NOERROR
    }
    kubernetes cluster.local ...
    forward . /etc/resolv.conf
}
```

**Plugin Order Within Each Server Block:**
bufsize → errors → log → health → ready → **templates (ordered by zone
specificity)** → kubernetes → prometheus → forward → cache → reload

**Template Ordering Within Plugin:**
Templates are ordered by zone specificity (most specific first):
- `app.corp.example.com` (most specific)
- `corp.example.com` (less specific)
- `.` (catch-all, least specific)

Within the same specificity level, templates are ordered alphabetically by name
for deterministic behavior.

**Zone Matching:** Templates only process queries matching their configured zones.
A template with zone `example.com` in the `.:5353` server block will only match
queries for `*.example.com` that reach that block.

**Cluster Protection**: cluster.local exclusions with fallthrough are
auto-generated before user templates when zone "." is configured in dual-stack
clusters.

**Template Behavior**:
- `returnEmpty`: Query processing stops (no fallthrough)
- Auto-exclusions: Use fallthrough to allow kubernetes plugin processing
- `generateResponse`: Response returned (no fallthrough)

### Topology Considerations

#### Hypershift / Hosted Control Planes
Templates propagate via standard DNS operator mechanism to hosted clusters.

#### Standalone Clusters / Single-node / MicroShift / OKE
Fully applicable. Template evaluation overhead is minimal (~microseconds).

#### Dual-Stack Clusters
AAAA filtering is designed for single-stack IPv4 clusters. In dual-stack:
- Operator auto-detects IPv6 CIDRs in Network.config.openshift.io
- Auto-generates cluster.local exclusions for zone "." templates
- Sets `AAAAFilterDualStackWarning` condition
- Recommends using specific zones instead of "." to avoid auto-exclusions

### Implementation Details/Notes/Constraints

**DNS Operator Changes**:
1. Add API types to openshift/api (`DNSTemplate`, action types, enums)
2. Implement `validateTemplateSettings()` in controller_dns_configmap.go
3. Update `corefileTemplate` to generate template plugin blocks
4. Add conditions: `TemplateConfigurationValid`, `TemplateConfigurationApplied`,
   `AAAAFilterDualStackWarning`

**Corefile Generation**: Templates are inserted into **all server blocks**
(both custom servers from `spec.servers` and the default `.:5353` block),
positioned after the `ready` plugin and before the `kubernetes` plugin. This
ensures AAAA filtering or custom response generation applies consistently across
all DNS resolution paths and reduces upstream load for all configured forwarders.

Templates are ordered by zone specificity (most specific first) when generating
the Corefile.

In dual-stack clusters with zone ".", the operator auto-generates an exclusion
template before user templates:
```
template IN AAAA . {
    match "^(.*\.)?cluster\.local\.$"
    fallthrough
}
template IN AAAA . {
    rcode NOERROR
}
```

**Validation**:
- API schema: name format, required fields, enum values, zone/template limits
- Semantic: valid DNS names, no reserved zones (cluster.local), no duplicate
  template names, no duplicate zone+queryType combinations (to prevent conflicts)
- Template ordering: templates ordered by zone specificity when generating Corefile
- Dual-stack safety: detect IPv6 CIDRs in Network.config.openshift.io/cluster,
  auto-exclude cluster domains, set warning condition
- Syntax: basic answerTemplate validation for generateResponse actions

**Feature Gate**: `DNSTemplatePlugin` (TechPreviewNoUpgrade initially)

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Dual-stack IPv6 breakage | Auto-detect and exclude cluster.local|
| Misconfigured templates | Multi-level validation; protected cluster domains; clear error messages |
| Performance impact | Scoped to zones/types; graduation testing |
| Template injection | Structured actions (not free-form); length limits; admin-only configuration |
| SERVFAIL from regex mismatch | Do NOT use `match` directive; use unconditional `answer`/`rcode` stanzas |
| Misunderstanding | TechPreviewNoUpgrade initially; documentation with examples/warnings |

### Drawbacks

* Increases operator complexity (validation, Corefile generation, dual-stack protection)
* Limited initially to AAAA/IN/NOERROR; requires future work for broader use cases
* Administrators must understand filtering vs custom responses and dual-stack implications
* Additional testing burden for dual-stack and template combinations

## Design Details

### Test Plan

**Unit Tests**: Validation (valid/invalid configs, duplicates, reserved zones),
Corefile generation (single/multiple templates, ordering, dual-stack exclusions)

**Integration Tests**: Operator workflow (apply/update/delete templates,
ConfigMap regeneration, conditions), dual-stack detection

**E2E Tests** (labeled `[OCPFeatureGate:DNSTemplatePlugin]`):

For feature gate promotion from TechPreviewNoUpgrade to GA, tests must be
added to the openshift/origin repository as required by the graduation criteria.

1. AAAA filtering (basic and specific zones): verify empty NOERROR responses,
   A queries unaffected, cluster.local works, metrics show reduction
2. Custom responses: verify correct IPv6 address returned with TTL
3. Multiple templates: verify zone precedence and independent operation
4. Dual-stack protection: verify cluster.local AAAA works, IPv6 service
   connectivity preserved, warning condition set, external filtering works
5. Template application to custom servers: verify templates injected into
   spec.servers blocks, filtering works for custom upstreamResolvers
6. Template updates: verify add/modify/delete propagates correctly
7. Error cases: invalid zones, reserved zones, duplicate zone+queryType
   combinations rejected with clear errors
8. Feature gate: verify templates ignored when disabled
9. Upgrade/downgrade: verify template persistence and compatibility

**Performance/Scale Tests**: Measure latency/memory/throughput with 0-20
templates and large zone lists (1000-10000 qps loads)

### Graduation Criteria

#### Dev Preview -> Tech Preview
N/A - Starts in TechPreviewNoUpgrade.

#### Tech Preview -> GA
- Minimum 5 E2E tests, 95% pass rate on all platforms (AWS/Azure/GCP/bare metal)
- Dual-stack testing verifies IPv6 service connectivity preserved
- Performance testing confirms minimal latency impact
- User documentation in openshift-docs (examples, warnings, troubleshooting)
- Tech Preview feedback incorporated, known issues resolved
- Operator conditions provide clear status/errors

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

**Upgrade**: Template configurations preserved when feature gate enabled.

**Downgrade**: `templates` field ignored; CoreDNS reverts to previous config
(no manual cleanup required).

### Version Skew Strategy

No new version skew concerns. Hypershift configurations propagate via standard
DNS operator reconciliation.

### Operational Aspects of API Extensions

**New Conditions**: `TemplateConfigurationValid`, `TemplateConfigurationApplied`,
`AAAAFilterDualStackWarning`

**Metrics**: `coredns_dns_requests_total{type="AAAA"}` (decreases for filtered zones),
`coredns_forward_requests_total` (decreases with filtering)

**Impact**: Max 20 templates, <5% latency for matching queries, <1MB memory per template

**Failure Modes**:
| Failure | Symptom | Recovery |
|---------|---------|----------|
| Invalid config | `TemplateConfigurationValid=False` | Fix config per error message |
| Evaluation error | CoreDNS "template error" logs, SERVFAIL | Fix answerTemplate syntax |
| Reload failure | `TemplateConfigurationApplied=False` | Operator retries; review config if persistent |
| Dual-stack breakage | Service issues, warning condition | Auto-exclusions prevent; remove zone "." if fails |
| Performance | High latency, CPU usage | Reduce/consolidate templates |

### Support Procedures

**Debugging**:
- Check conditions: `oc get dns.operator.openshift.io default -o yaml`
- Review ConfigMap: `oc get configmap/dns-default -n openshift-dns -o yaml`
- Check CoreDNS logs: `oc logs -n openshift-dns -l dns.operator.openshift.io/daemonset-dns=default`

**Common Issues**:
- Invalid config: Check `TemplateConfigurationValid` condition message
- Not applied: Check `TemplateConfigurationApplied` condition
- Dual-stack warning: Review `AAAAFilterDualStackWarning` and verify auto-exclusions in ConfigMap
- Runtime errors: Check CoreDNS logs for "template error" or "SERVFAIL"

**Disabling**: Remove templates via `oc edit` or `oc patch --type=json -p='[{"op": "remove", "path": "/spec/templates"}]'`

## Implementation History

N/A

## Alternatives (Not Implemented)

| Alternative | Why Not Chosen |
|-------------|----------------|
| Single-purpose AAAA filtering API | Would require separate APIs for future use cases; less extensible |
| Free-form template syntax | Unsafe, no validation, injection vulnerabilities, unclear API docs |
| External DNS servers | Operational overhead, doesn't integrate with operator model |
| Direct Corefile editing | Bypasses validation, operator overwrites changes, fragile |
| CoreDNS rewrite plugin | Designed for query rewriting, not response generation/filtering |
| Application-level config | Requires modifying all apps/images, not feasible at scale |
| Node-level DNS filtering | Affects node resolution, more invasive, harder to manage |

## Infrastructure Needed [optional]

N/A
