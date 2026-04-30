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
without maintaining external DNS infrastructure.
The CoreDNS [template plugin](https://coredns.io/plugins/template/) supports both use cases but lacks operator API integration.

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
* Enable AAAA filtering via `returnEmpty` action
* Support IN class, AAAA records, and NOERROR response initially
* Validate templates before applying to CoreDNS (both CRD schema validation via
  kubebuilder markers and validation in the DNS operator)
* Provide new Reasons in DNS operator conditions for template configuration status
* Design extensible API to support custom response generation in future releases

### Non-Goals

* Record types other than AAAA initially
* Response codes other than NOERROR initially
* DNS classes other than IN initially
* Custom response generation via `generateResponse` action
* Automatic AAAA filtering on single-stack IPv4 clusters (administrator must
  explicitly configure templates)
* Automatic exclusions for internal services in IPv6/dual-stack
  clusters (AAAA filtering is primarily intended for IPv4-only environments;
  administrator responsible for zone configuration; templates field is optional)
* User-configurable regex patterns for zone matching

## Proposal

Add a `templates` field to the DNS operator API to configure CoreDNS template
plugins. The operator validates configurations, generates Corefile entries, and
provides status conditions. The API uses discriminated unions for extensibility.

### Workflow Description

1. Administrator configures template in DNS CR specifying zones and action
2. CRD validates API schema (required fields, enum values). Semantic validation
   by the operator: valid DNS names, no duplicate zone+queryType combinations.
   Initial implementation validates only returnEmpty action.
3. Operator generates Corefile with template blocks and reloads CoreDNS
4. CoreDNS processes matching queries per template rules (AAAA filtering via
   empty response)

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
// The initial implementation supports ONLY the returnEmpty action for AAAA
// filtering. The discriminated union design enables future expansion to support
// generateResponse for custom DNS responses without breaking existing configurations.
// +union
// +kubebuilder:validation:XValidation:rule="has(self.returnEmpty)",message="only returnEmpty action is supported in the initial implementation"
type TemplateAction struct {
	// returnEmpty returns an empty response with the specified RCODE.
	// This is useful for filtering queries (e.g., AAAA filtering in IPv4-only clusters).
	// When set, no answer/authority/additional sections are included.
	// Maps to CoreDNS template plugin: rcode directive only.
	// +optional
	// +unionDiscriminator
	ReturnEmpty *ReturnEmptyAction `json:"returnEmpty,omitempty"`
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
	//
	// AAA filtering is intended for IPv4-only clusters. In IPv6 or
	// dual-stack clusters, use specific zones instead of "." to avoid filtering
	// internal IPv6 service addresses.
	//
	// +optional
	Templates []Template `json:"templates,omitempty"`
}
```

**Example: AAAA Filtering (Initial Implementation)**
```yaml
spec:
  templates:
    - zones: ["."]
      queryType: AAAA
      queryClass: IN
      action:
        returnEmpty:
          rcode: NOERROR
```

This enhancement does not modify existing DNS resource behavior. The template
plugin only affects queries matching configured zones and query types.

### Important Limitations and Warnings

**IPv4-only clusters**: AAAA filtering is designed for single-stack IPv4 clusters
where IPv6 is not used. The templates field is optional.

**IPv6 and dual-stack clusters**: When using templates in IPv6 or dual-stack
environments, configure specific zones instead of the catch-all "." to avoid
unintentionally filtering internal IPv6 service addresses (e.g., cluster.local).

**Note**: Templates number limit to be considered.

### API Extensibility Design

The API uses discriminated unions for actions and typed enums for query
types/classes/response codes.

**Initial Implementation:**
- `ReturnEmpty` action for AAAA filtering
- `AAAA` query type
- `IN` query class
- `NOERROR` response code
- CRD validation enforces returnEmpty-only

**Future Expansion Capabilities:**
- **New actions**: Add `GenerateResponse` for custom DNS responses; add support
  for authority/additional sections
- **New query types**: Add `A`, `CNAME`, `MX` to QueryType enum
- **New query classes**: Add `CH` (Chaos) to QueryClass enum
- **New response codes**: Add `NXDOMAIN`, `SERVFAIL` to ResponseCode enum after
  validating DNS client behavior

Structured action types enable validation and prevent arbitrary template syntax
injection. Existing configurations remain compatible when new enum values/actions
are added.

### Future Extensions

While the initial implementation supports only AAAA filtering via the `returnEmpty`
action, the API is designed for backward-compatible extension.

**Custom Response Generation (Future):**

```go
// GenerateResponseAction configures custom response generation.
// NOTE: Not implemented in initial release. This type is defined for API
// forward compatibility. Operator validation will reject configurations using
// this action until support is added in a future release.
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
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NOERROR
	// +kubebuilder:default=NOERROR
	Rcode ResponseCode `json:"rcode"`
}
```

**Example use case:** Static DNS mappings for specific domains without external DNS:
```yaml
# NOTE: Not supported in initial implementation
spec:
  templates:
    - zones: ["legacy.corp.example.com"]
      queryType: AAAA
      queryClass: IN
      action:
        generateResponse:
          answerTemplate: "{{ .Name }} 3600 IN AAAA 2001:db8::100"
          rcode: NOERROR
```

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

**Template Behavior**:
- `returnEmpty`: Query processing stops (no fallthrough)
- `generateResponse` (future): Response returned (no fallthrough)

### Topology Considerations

#### Hypershift / Hosted Control Planes
Templates propagate via standard DNS operator mechanism to hosted clusters.

#### Standalone Clusters / Single-node / MicroShift / OKE
Fully applicable. Template evaluation overhead is minimal (~microseconds).

#### Dual-Stack and IPv6 Clusters
AAAA filtering is designed for single-stack IPv4 clusters. When using templates
in IPv6 or dual-stack environments, configure specific zones instead of the
catch-all "." to avoid filtering internal IPv6 service addresses.

### Implementation Details/Notes/Constraints

**DNS Operator Changes**:
1. Add API types to openshift/api (`DNSTemplate`, action types, enums)
2. Implement `validateTemplateSettings()` in controller_dns_configmap.go
3. Update `corefileTemplate` to generate template plugin blocks
4. Add conditions: `TemplateConfigurationValid`, `TemplateConfigurationApplied`

**Corefile Generation**: Templates are inserted into **all server blocks**
(both custom servers from `spec.servers` and the default `.:5353` block),
positioned after the `ready` plugin and before the `kubernetes` plugin. This
ensures AAAA filtering or custom response generation applies consistently across
all DNS resolution paths and reduces upstream load for all configured forwarders.

Templates are ordered by zone specificity (most specific first) when generating
the Corefile.

**Validation**:
- API schema: required fields, enum values, zone/template limits
- CRD validation: enforce returnEmpty-only in initial implementation
- Semantic: valid DNS names, no duplicate zone+queryType combinations (to prevent conflicts)
- Template ordering: templates ordered by zone specificity when generating Corefile
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

**Unit Tests**: Validation (valid/invalid configs, duplicates),
Corefile generation (single/multiple templates, ordering)

**Integration Tests**: Operator workflow (apply/update/delete templates,
ConfigMap regeneration, conditions)

**E2E Tests** (labeled `[OCPFeatureGate:DNSTemplatePlugin]`):

For feature gate promotion from TechPreviewNoUpgrade to GA, tests must be
added to the openshift/origin repository as required by the graduation criteria.

Initial implementation tests:
1. AAAA filtering (basic and specific zones): verify empty NOERROR responses,
   A queries unaffected, metrics show reduction
2. Multiple templates: verify zone precedence and independent operation
3. Template application to custom servers: verify templates injected into
   spec.servers blocks, filtering works for custom upstreamResolvers
4. Template updates: verify add/modify/delete propagates correctly
5. Error cases: invalid zones, duplicate zone+queryType combinations rejected
   with clear errors
6. Feature gate: verify templates ignored when disabled
7. Upgrade/downgrade: verify template persistence and compatibility

Future tests (when GenerateResponse is added):
- Custom responses: verify correct IPv6 address returned with TTL
- answerTemplate validation: verify invalid templates rejected

**Performance/Scale Tests**: Measure latency/memory/throughput with 0-20
templates and large zone lists (1000-10000 qps loads)

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

**New Conditions**: `TemplateConfigurationValid`, `TemplateConfigurationApplied`

**Metrics**: `coredns_dns_requests_total{type="AAAA"}` (decreases for filtered zones),
`coredns_forward_requests_total` (decreases with filtering)

**Impact**: Max 20 templates, <5% latency for matching queries, <1MB memory per template

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
