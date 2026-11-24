---
title: coredns-custom-ipv6-template-plugin
authors:
  - "@grzpiotrowski"
reviewers:
approvers:
api-approvers:
creation-date: 2026-01-28
last-updated: 2026-03-18
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/NE-2118
see-also:
replaces:
superseded-by:
---

# Enable Custom IPv6 Responses via CoreDNS Template Plug-in

## Summary

This enhancement adds a `template` field to the DNS operator API to configure
the CoreDNS template plugin. The primary use case is filtering AAAA queries in
IPv4-only clusters to reduce DNS latency by returning empty NOERROR responses,
causing clients to fall back to IPv4 (A record) queries immediately. The API
is designed with extensibility in mind for future expansion to custom response
generation, additional record types, classes, and response codes.

## Motivation

In IPv4-only clusters, applications may query for both A and AAAA
records. CoreDNS forwards unresolvable AAAA queries to upstream resolvers,
adding latency per query. This behavior occurs because stub resolvers like
glibc's [getaddrinfo()](https://man7.org/linux/man-pages/man3/getaddrinfo.3.html)
function query both IPv4 (A) and IPv6 (AAAA) records by default to remain
protocol-agnostic, even in single-stack IPv4 environments. The getaddrinfo()
function combines IPv4 and IPv6 functionality into a single interface, allowing
programs to eliminate IPv4-versus-IPv6 dependencies by defaulting to `AF_UNSPEC`
which permits both IPv4 and IPv6 addresses. Filtering AAAA queries at CoreDNS
eliminates this delay and reduces upstream DNS load.

Additionally, some users need custom DNS responses for specific domains
without maintaining external DNS infrastructure.
The CoreDNS [template plugin](https://coredns.io/plugins/template/) supports both use cases but lacks operator API integration.

### User Stories

* As a cluster administrator in an IPv4-only environment, I want to centrally
  configure AAAA query filtering for the entire cluster so that I can eliminate
  IPv6 lookup delays and reduce upstream DNS load without modifying individual
  pod configurations (avoiding the need to set `dnsConfig.options.no-aaaa` in
  every pod spec, which is tedious and error-prone).

* As a network engineer, I want to configure custom IPv6 responses for specific
  domains so that I can route traffic without external DNS infrastructure.

* As an SRE, I want operator conditions and metrics for template configuration
  so that I can monitor DNS optimization effectiveness and troubleshoot
  configuration issues.

### Goals

* Add `template` field to DNS operator API with extensible design for future
  expansion
* Enable AAAA filtering by returning empty NOERROR responses
* Support IN class, AAAA query type, and NOERROR response code initially
* Validate templates before applying to CoreDNS (both CRD schema validation via
  kubebuilder markers and semantic validation in the DNS operator)
* Provide clear status via DNS operator conditions for template configuration
* Design extensible API to support custom response generation in future releases
* Provide metrics for monitoring template effectiveness ([CoreDNS template plugin metrics](https://coredns.io/plugins/template/#metrics))

### Non-Goals

* Record types other than AAAA initially (future: A, CNAME, MX, TXT, etc.)
* Response codes other than NOERROR initially (NXDOMAIN and other codes may be
  considered in future iterations after validating DNS client behavior to ensure
  they don't disrupt search domain resolution or A record lookups)
* DNS classes other than IN initially (future: CH - Chaos class)
* Custom response generation (answer/authority/additional sections will be added
  in a future API version when custom response use cases are validated)
* Automatic AAAA filtering on single-stack IPv4 clusters (administrator must
  explicitly configure templates)
* Automatic exclusions for internal services in IPv6/dual-stack
  clusters (AAAA filtering is primarily intended for IPv4-only environments;
  administrator responsible for zone configuration; templates field is optional)
* User-configurable regex patterns for zone matching (operator-generated
  configurations may use regex internally, but this is not exposed in the
  user-facing API)

## Proposal

Add a `template` field to the DNS operator API to configure CoreDNS template
plugins. The operator validates configurations, generates Corefile entries, and
provides status conditions. The API uses discriminated unions for actions and
supports multiple actions per template, enabling processing pipelines.

### Workflow Description

1. Administrator configures template in DNS CR specifying zones, query type,
   query class, and action
2. CRD validates API schema (required fields, enum values, zone patterns).
   Semantic validation by the operator: valid DNS names, zone format validation.
3. Operator generates Corefile with template blocks and reloads CoreDNS
4. CoreDNS processes matching queries per template rules (AAAA filtering via
   empty NOERROR response)

### API Extensions

This enhancement modifies the DNS operator CRD (`dns.operator.openshift.io`)
to add a `template` field. The API uses typed enums for extensibility and
type safety.

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
	// ResponseCodeNOERROR indicates a successful DNS query with or without answer records.
	ResponseCodeNOERROR ResponseCode = "NOERROR"
)

// Zone is a DNS zone name. It must be either "." (catch-all for all domains)
// or a valid RFC 1123 subdomain.
// Valid RFC 1123 subdomains consist of lowercase alphanumeric characters, hyphens, and dots.
// Labels cannot start or end with hyphens and must be 1-63 characters each.
// Total length cannot exceed 253 characters.
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^(\.|[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)$`
type Zone string

// DNSTemplate defines a template for custom DNS query handling via the CoreDNS template plugin.
// DNSTemplate enables filtering or custom responses for DNS queries matching specific zones and query types.
type DNSTemplate struct {
	// zones specifies the DNS zones this template applies to.
	// Each zone must be a valid DNS name as defined in RFC 1123.
	// The special zone "." matches all domains (DNS root zone / catch-all).
	// Multiple zones can be specified to apply the same template actions to multiple domains.
	// At least 1 and at most 15 zones may be specified.
	//
	// Note: The root zone (".") includes the cluster domain (cluster.local). When using
	// the root zone in IPv6 or dual-stack clusters, ensure you want to filter AAAA queries
	// for all domains including internal services. Use specific zones to avoid unintended
	// filtering of internal IPv6 service addresses.
	//
	// Examples:
	// - ["."] matches all domains (catch-all for global AAAA filtering)
	// - ["example.com"] matches only example.com and its subdomains
	// - ["example.com", "test.com"] matches both domains and their subdomains
	//
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=15
	// +required
	Zones []Zone `json:"zones,omitempty"`

	// queryType specifies the DNS query type to match.
	// Valid values are "AAAA" (IPv6 address records).
	//
	// AAAA records are queried when clients request IPv6 addresses for a domain.
	// In IPv4-only environments, these queries fail and add latency. Filtering
	// AAAA queries causes clients to fall back to A record (IPv4) queries immediately.
	//
	// +required
	QueryType QueryType `json:"queryType,omitempty"`

	// queryClass specifies the DNS query class to match.
	// Valid values are "IN" (Internet class - RFC 1035).
	//
	// The Internet (IN) class is the standard class for DNS queries on the Internet.
	// This is the class used for typical domain name resolution.
	//
	// +required
	QueryClass QueryClass `json:"queryClass,omitempty"`

	// action defines how to handle queries matching this template's zones and query type.
	// The action builds a single DNS response by specifying the response code and may be
	// extended by additional fields in the future.
	//
	// +required
	Action DNSTemplateAction `json:"action,omitzero"`
}

// DNSTemplateAction defines how to construct a DNS response for queries matching the template.
type DNSTemplateAction struct {
	// rcode is the DNS response code to return.
	// Valid values are "NOERROR".
	//
	// The template returns a response with no answer records. For AAAA filtering,
	// this means IPv6 address queries return successfully but with no IPv6 addresses,
	// causing clients to fall back to IPv4 (A record) queries.
	//
	// +required
	Rcode ResponseCode `json:"rcode,omitempty"`
}

// DNSSpec is the specification of the desired behavior of the DNS.
type DNSSpec struct {
	// <snip>

	// template is an optional configuration for custom DNS query handling via the CoreDNS template plugin.
	// The template defines how to handle queries matching specific zones and query types.
	//
	// The template applies to all domains (custom domains from spec.servers and the cluster domain)
	// to ensure consistent DNS resolution across all paths.
	//
	// When this field is not set, no template plugin configuration is added to CoreDNS.
	//
	// +optional
	// +openshift:enable:FeatureGate=DNSTemplatePlugin
	Template DNSTemplate `json:"template,omitzero"`
}
```

**Example: AAAA Filtering**
```yaml
spec:
  template:
    zones: ["."]
    queryType: AAAA
    queryClass: IN
    action:
      rcode: NOERROR
```

This configuration filters all AAAA queries by returning empty NOERROR responses,
causing clients to fall back to IPv4 (A record) queries immediately.

This enhancement does not modify existing DNS resource behavior. The template
plugin only affects queries matching configured zones and query types.

### Important Limitations and Warnings

**IPv4-only clusters**: AAAA filtering is designed for single-stack IPv4 clusters
where IPv6 is not used. The templates field is optional.

**IPv6 and dual-stack clusters**: When using templates in IPv6 or dual-stack
environments, configure specific zones instead of the catch-all "." to avoid
unintentionally filtering internal IPv6 service addresses (e.g., cluster.local).

**Single Response Construction**: The template action defines ONE DNS response.
The initial implementation returns an empty response with the specified response
code (NOERROR) for AAAA filtering. Future API versions could possibly extend the action
to support populating answer/authority/additional sections for custom DNS responses.

### API Extensibility Design

The API uses discriminated unions for actions and typed enums for query
types/classes/response codes.

**Initial Implementation:**
- `DNSTemplateAction` struct with only `rcode` field
- `AAAA` query type (IPv6 address records)
- `IN` query class (Internet class)
- `NOERROR` response code (successful response with no answer records)

**Future Expansion Capabilities:**

The API is designed to support additional functionality in future releases
without breaking existing configurations:

- **Response generation**: Future API version will extend `DNSTemplateAction` with
  `answer`, `authority`, and `additional` fields to support custom DNS response
  generation using CoreDNS template syntax
- **New query types**: Extend `QueryType` enum to add `A`, `CNAME`, `MX`, `TXT`, etc.
- **New query classes**: Extend `QueryClass` enum to add `CH` (Chaos)
- **New response codes**: Extend `ResponseCode` enum to add `NXDOMAIN`, `SERVFAIL`,
  etc. after validating DNS client behavior

**Extension Strategy:**

1. **Adding new enum values**: New query types, classes, or response codes can be
   added to enums without breaking existing configurations
2. **Adding new action fields**: Future API version can extend `DNSTemplateAction`
   with optional fields for response generation (answer/authority/additional sections)
3. **Backward compatibility**: Existing AAAA filtering configurations will continue
   to work when new capabilities are added

This design provides a simple, focused API for the initial AAAA filtering use case
while maintaining a clear path for future expansion.

### Future Extensions

While the initial implementation supports only AAAA filtering, the API is designed
for backward-compatible extension to support custom response generation.

#### Future API Version: Custom Response Generation

Support for custom DNS responses will be added in a future API version by extending
`DNSTemplateAction` with additional fields:

```go
// DNSTemplateAction defines how to construct a DNS response for queries matching the template.
type DNSTemplateAction struct {
	// rcode is the DNS response code to return.
	// +required
	Rcode ResponseCode `json:"rcode,omitempty"`

	// FUTURE: answer is the template for the answer section.
	// Uses CoreDNS template syntax with variables: .Name, .Type, .Class
	// For AAAA records, format: "{{ .Name }} <TTL> IN AAAA <ipv6-address>"
	// Example: "{{ .Name }} 3600 IN AAAA 2001:db8::1"
	// +optional
	Answer *string `json:"answer,omitempty"`

	// FUTURE: authority is the template for the authority section.
	// Example: "{{ .Zone }} 3600 IN NS ns1.example.com"
	// +optional
	Authority *string `json:"authority,omitempty"`

	// FUTURE: additional is the template for the additional section.
	// Example: "ns1.example.com 3600 IN A 10.0.0.1"
	// +optional
	Additional *string `json:"additional,omitempty"`
}
```

**Example use case:** Static DNS mappings for specific domains without external DNS:

```yaml
# NOTE: Not supported in initial implementation - future API version
spec:
  template:
    zones: ["legacy.corp.example.com"]
    queryType: AAAA
    queryClass: IN
    action:
      rcode: NOERROR
      answer: "{{ .Name }} 3600 IN AAAA 2001:db8::100"
```

#### Other Future Capabilities

- **Additional query types:** A, CNAME, MX, TXT, SRV, etc.
- **Additional response codes:** NXDOMAIN, SERVFAIL (after validating client behavior)
- **Additional query classes:** CH (Chaos)
- **Match patterns:** User-configurable regex for advanced query matching
- **Multiple templates:** Support for multiple independent template configurations

These extensions will be additive and maintain backward compatibility with
initial AAAA filtering configurations.

### Template Injection and Plugin Ordering

**IMPORTANT:** The template defined in `spec.template` is injected into **ALL**
Corefile server blocks (both custom servers from `spec.servers` and the default
`.:5353` block).

**Reasoning for global application:**
1. Reduces upstream AAAA query load for all configured upstreams
2. Users define template once, it applies everywhere
3. Template only activates when zones match
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
bufsize → errors → log → health → ready → **template** → kubernetes → prometheus → forward → cache → reload

**Zone Matching:** The template can specify multiple zones. It will process
queries matching any of the configured zones. For example, a template with
`zones: ["example.com", "test.com"]` will match queries for both domains.

**Template Behavior**:
- Template constructs ONE DNS response per matching query
- Returns empty response with specified response code (AAAA filtering)
- Future: answer/authority/additional sections will populate DNS response sections
- No fallthrough in initial implementation - template returns response directly

### Topology Considerations

#### Hypershift / Hosted Control Planes
Templates propagate via standard DNS operator mechanism to hosted clusters.

#### Standalone Clusters / Single-node / MicroShift / OKE

Fully applicable. MicroShift runs CoreDNS and can use the DNS operator to
configure templates. Template evaluation overhead is minimal (~microseconds per query).

#### Dual-Stack and IPv6 Clusters
AAAA filtering is designed for single-stack IPv4 clusters. When using templates
in IPv6 or dual-stack environments, configure specific zones instead of the
catch-all "." to avoid filtering internal IPv6 service addresses.

### Implementation Details/Notes/Constraints

**DNS Operator Changes**:
1. Add API types to openshift/api (`DNSTemplate`, `DNSTemplateAction`, enums)
2. Implement template validation in DNS operator
3. Update Corefile generation to include template plugin blocks
4. Add operator conditions with template-specific reasons

**Corefile Generation**: The template is inserted into **all server blocks**
(both custom servers from `spec.servers` and the default `.:5353` block),
positioned after the `ready` plugin and before the `kubernetes` plugin. This
ensures AAAA filtering applies consistently across all DNS resolution paths
and reduces upstream load for all configured forwarders.

**Validation**:
- API schema: required fields, enum values
- CRD validation: enforce returnEmpty-only in initial implementation
- Semantic: valid DNS names for zones
- Future: answerTemplate syntax validation when GenerateResponse is implemented

**Feature Gate**: `DNSTemplatePlugin` (TechPreviewNoUpgrade initially)

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| IPv6/dual-stack service breakage | Documentation warns against using "." zone in IPv6 clusters; recommend specific zones |
| Misconfigured templates | Multi-level validation; clear error messages; admin-only configuration |
| Performance impact | Scoped to zones/types; graduation testing |
| Template injection | Structured actions (not free-form); length limits; admin-only configuration |
| Misunderstanding | TechPreviewNoUpgrade initially; documentation with examples/warnings |

### Drawbacks

* Increases operator complexity (validation, Corefile generation)
* Limited initially to AAAA/IN/NOERROR; requires future work for broader use cases
* Administrators must understand filtering implications in IPv6/dual-stack environments
* Additional testing burden for various cluster network topologies

## Design Details

### Test Plan

**Unit Tests**: 
- Validation: valid/invalid configurations (zones, query types, response codes)
- Corefile generation: template block format, zone handling, plugin ordering
- Zone validation: RFC 1123 compliance, pattern matching, duplicate detection

**Integration Tests**: 
- Operator workflow: apply/update/delete template configurations
- ConfigMap regeneration: verify Corefile updates correctly
- Operator conditions: verify appropriate status reporting
- Multiple zones: verify template applies to all configured zones

**E2E Tests** (labeled `[OCPFeatureGate:DNSTemplatePlugin]`):

For feature gate promotion from TechPreviewNoUpgrade to GA, tests must be
added to the openshift/origin repository as required by the graduation criteria.

Tests for initial implementation:
1. **AAAA filtering (basic and specific zones)**: Verify empty NOERROR responses
   for AAAA queries, A queries unaffected, metrics show template matches
2. **Template application to custom servers**: Verify template injected into
   spec.servers blocks, filtering works for custom upstreamResolvers
3. **Template updates**: Verify add/modify/delete propagates correctly to CoreDNS
4. **Multiple zones**: Verify template applies to all zones in zones array
5. **Error cases**: Invalid zones, invalid query types, invalid response codes
   rejected with clear error messages
6. **Feature gate**: Verify template ignored when DNSTemplatePlugin feature gate disabled
7. **Upgrade/downgrade**: Verify template persistence and compatibility across versions
8. **Caching interaction**: Verify filtered responses interact correctly with cache plugin

**Tests for future implementation** (when custom response generation is added):
- Custom responses: verify correct IPv6 address returned with specified TTL
- Answer template validation: verify invalid template syntax rejected
- Authority/additional sections: verify complete DNS response generation

**Performance/Scale Tests**: 
- Measure query latency with template enabled vs disabled
- Verify memory usage with large zone lists (10-15 zones)
- Load testing: 1000-10000 qps with template filtering enabled
- Verify upstream request reduction (via `coredns_forward_requests_total` metric)

### Graduation Criteria

#### Dev Preview -> Tech Preview
N/A - Starts in TechPreviewNoUpgrade.

#### Tech Preview -> GA
- Minimum 5 E2E tests, 95% pass rate on all platforms (AWS/Azure/GCP/bare metal)
- Performance testing confirms minimal latency impact
- User documentation in openshift-docs (examples, warnings for IPv6 clusters, troubleshooting)
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

**Operator Conditions**: The template configuration will use existing DNS operator
conditions (`Available`, `Degraded`, `Progressing`) with template-specific reasons:
- Template configuration invalid: `Degraded=True` with reason describing validation error
- Template successfully applied: `Available=True`
- Template configuration in progress: `Progressing=True`

**Metrics**:

CoreDNS template plugin provides built-in metrics:
- `coredns_template_matches_total{server,view,zone,class,type}` - Total template matches
- `coredns_template_template_failures_total{server,view,zone,class,type,section,template}` - Template execution failures

Additional relevant metrics:
- `coredns_dns_requests_total{type="AAAA"}` - Decreases for filtered zones
- `coredns_forward_requests_total` - Decreases with filtering (reduced upstream load)
- `coredns_cache_entries`, `coredns_cache_hits_total`, `coredns_cache_misses_total` - Cache behavior

Reference: https://coredns.io/plugins/template/#metrics

**Impact**: Minimal latency overhead, minimal memory footprint

**Failure Modes**:
| Failure | Symptom | Recovery |
|---------|---------|----------|
| Invalid config | `TemplateConfigurationValid=False` | Fix config per error message |
| Evaluation error | CoreDNS "template error" logs, SERVFAIL | Fix answerTemplate syntax |
| Reload failure | `TemplateConfigurationApplied=False` | Operator retries; review config if persistent |
| IPv6 service breakage | Service connectivity issues in IPv6 clusters | Use specific zones instead of "." |
| Performance | High latency, CPU usage | Reduce/consolidate templates |

### Support Procedures

**Debugging**:
- Check conditions: `oc get dns.operator.openshift.io default -o yaml`
- Review ConfigMap: `oc get configmap/dns-default -n openshift-dns -o yaml`
- Check CoreDNS logs: `oc logs -n openshift-dns -l dns.operator.openshift.io/daemonset-dns=default`

**Common Issues**:
- Invalid config: Check `TemplateConfigurationValid` condition message
- Not applied: Check `TemplateConfigurationApplied` condition
- IPv6 service issues: Use specific zones instead of "." in IPv6/dual-stack clusters
- Runtime errors: Check CoreDNS logs for "template error" or "SERVFAIL"

**Disabling**: Remove template via `oc edit` or `oc patch --type=json -p='[{"op": "remove", "path": "/spec/template"}]'`

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
