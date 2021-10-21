---
title: global-options-enable-hsts 
authors:
  - "@cholman"

reviewers:
  - "@danehans" 
  - "@frobware"
  - "@knobunc"
  - "@Miciah"
  - "@miheer"
  - "@rfredette"
  - "@sgreene570"

approvers:
  - "@frobware"
  - "@knobunc"

creation-date: 2021-03-08

last-updated: 2021-06-28

status: superseded

see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=1512759
  - https://bugzilla.redhat.com/show_bug.cgi?id=1430035

replaces:

superseded-by:
  - enhancements/ingress/global-admission-hsts.md

---

# Global Options to Enable HTTP Strict Transport Security (HSTS)

This enhancement provides the capability to enable global automatic HSTS
as a default for all TLS routes.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In 3.x and 4.x customers can [provide a per-route annotation to enable HSTS](
https://docs.openshift.com/container-platform/4.4/networking/routes/route-configuration.html#nw-enabling-hsts_route-configuration).  For
customers with many routes or regulatory compliance issues, the manual per-route annotation is
problematic.

This enhancement extends the `IngressController` API to allow cluster administrators to
enable HSTS globally, without having to add an annotation to each route.

## Motivation

HSTS ([RFC 6797](https://tools.ietf.org/html/rfc6797)) policy enforces the use
of HTTPS in client requests to the host, without having to use an HTTP redirect.
HSTS provides user protection and is concerned with minimizing security threats based on network
traffic eavesdropping and man-in-the-middle attacks.  Using a response header
called `Strict-Transport-Security`, the HTTP response informs clients that a website
can only be accessed via HTTPS.  

Administrators who are tasked with route management and/or regulatory compliance
issues want an option to automatically and globally enable HSTS. They also want
to separately apply HSTS globally to specific routers that target applications,
while retaining the ability to opt-out of HSTS on a per-route basis.

### Goals

This proposal allows administrators to:
- Continue to enable HSTS per-route (retain the existing feature functionality)
- Enable HSTS globally, for all TLS routes
- Enable HSTS per-domain
- Disable HSTS per-route when it is enabled globally

### Non-Goals

- HSTS cannot be applied to non-TLS routes, even if HSTS is requested for all routes globally.  As
mentioned in [RFC 6797 Section 8.1](https://tools.ietf.org/html/rfc6797#section-8.1), if an HTTP
response is received over insecure transport, clients must ignore any present STS headers.

## Proposal

Each goal is addressed in the subsections below.

### Continue to enable HSTS per-route (retain the existing feature functionality)
The `Strict-Transport-Security` response header is currently applied per-route via an
annotation called `haproxy.router.openshift.io/hsts_header`.  This annotation has a
required directive `max-age`, and optional directives `includeSubDomains` and `preload`,
with the following definitions:
- `max-age`: delta time range in seconds during which the host is to be regarded as an HSTS host
  - If set to 0, it negates the effect and the host is no longer regarded as an HSTS host
  - `max-age` is a time-to-live value, so if a client makes a request for the route's host
  and this period of time elapses before the client makes another request for the same host, the
  HSTS policy will expire on that client
- `includeSubDomains`: if present, applies to any subdomains of the host's domain name, when the
route admission policy allows wildcards.  If wildcard routes aren't allowed, this directive has no
effect.
- `preload`: if present, tells the client to include the route's host in its host preload list so that
it never needs to do an initial load to get the HSTS header (note that this is not defined in RFC 6797
and is therefore client implementation-dependent)

HSTS is currently implemented in the HAProxy template and applied to `edge` and
`reencrypt` routes that have the `haproxy.router.openshift.io/hsts_header` annotation:
```gotemplate
{{- /* hsts header in response: */}}
 {{- $hstsOptionalTokenPattern := `(?:includeSubDomains|preload)` }}
 {{- $hstsPattern := printf `(?:%[1]s[;])*max-age=(?:\d+|"\d+")(?:[;]%[1]s)*`  $hstsOptionalTokenPattern -}}
...
{{- if matchValues (print $cfg.TLSTermination) "edge" "reencrypt" }}
    {{- with $hsts := firstMatch $hstsPattern (index $cfg.Annotations "haproxy.router.openshift.io/hsts_header") }}
  http-response set-header Strict-Transport-Security {{$hsts}}
    {{- end }}{{/* hsts header */}}
  {{- end }}{{/* is "edge" or "reencrypt" */}}
```

#### Enable HSTS globally, for all TLS routes
To add the header to all HTTP responses from an `edge` or `reencrypt` route, add a new router environment
variable called `ROUTER_DEFAULT_HSTS`.

Add a new type `IngressControllerHSTSHeaderPolicy` to the API in
`openshift/api/blob/master/operator/v1/types_ingress.go`, to capture the configuration of the HSTS Policy.
```go
// These are strings instead of enums so that they can be passed in easily from the HAProxy template
const (
    AllHSTSHeaderPolicyScope         = "All"
    LimitedHSTSHeaderPolicyScope     = "Limited"
)

// HSTSDirectiveType is an optional directive that directs HSTS Policy configuration.
//+kubebuilder:validation:Enum=preload;includeSubDomains
type HSTSDirectiveType string
const(
    // PreloadHSTSDirective directs the client to include hosts in its host preload list so that
    // it never needs to do an initial load to get the HSTS header (note that this is not defined
    // in RFC 6797 and is therefore client implementation-dependent).
    PreloadHSTSDirective             HSTSDirectiveType = "preload"

    // IncludeSubDomainsHSTSDirective means the HSTS Policy applies to any subdomains of the host's
    // domain name, when the route admission policy allows wildcards.  If wildcard routes aren't allowed,
    // this directive has no effect.
    IncludeSubDomainsHSTSDirective   HSTSDirectiveType = "includeSubDomains"
)

type IngressControllerHSTSHeaderPolicy {
	// maxAgeSeconds is the delta time range in seconds during which hosts are regarded as HSTS hosts.
	// If set to 0, it negates the effect and hosts are no longer regarded as HSTS hosts.
	// maxAgeSeconds is a time-to-live value, and if this policy is not refreshed on a client, the HSTS
	// policy will eventually expire on that client.
	// +required
    MaxAgeSeconds       int32 `json:"maxAgeSeconds"`

    // HSTSDirectives is a list of optional HSTS directives that direct HSTS Policy configuration.
    // +kubebuilder:validation:UniqueItems=true
    // +optional
    HSTSDirectives      []HSTSDirectiveType `json:"hstsDirectives,omitempty"`

    // scope indicates whether this header policy is enabled for a limited set of routes that have hosts
    // with domains in the list specified in inScopeDomains (limited scope) or for all TLS routes
    // (all scope).  If HSTS is desired for all routes, set the scope field to "All". If HSTS is desired
    // for only a limited set of routes, set the scope field to "Limited" and add a list of subdomains
    // to the InScopeDomains field.  If scope is omitted, no HSTS header policy will be added.
    // +optional
    Scope               string `json:"scope,omitempty"`

    // inScopeDomains is a list of router domains that require HSTS, when not all routes require HSTS.
    // If scope = All then this field is ignored.  If scope = Limited, HSTS policy is limited to hosts with
    // domains in the inScopeDomains list.
    // +optional
    InScopeDomains      []string `json:"inScopeDomains,omitempty"`
}
```

Validation and formatting in the `IngressController`
would change the fields into a semi-colon delimited string with formatting:
```yaml
max-age=<numberofseconds>[;includeSubDomains][;preload]`
```
`ROUTER_DEFAULT_HSTS` is detected in the HAProxy configuration file processing, and can be overridden
by any individual route's annotation.  The global `hstsHeaderPolicy` does not override an existing route
annotation for `haproxy.router.openshift.io/hsts_header`.

The `hstsHeaderPolicy` variable can be configured by the administrator in the `IngressController`, as shown in this example, where
the HSTS Policy scope is for all routes, expires after one year, includes subdomains, and requests preload:
```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpHeaders:
    hstsHeaderPolicy:
      scope: All
      maxAgeSeconds: 31536000
      hstsDirectives:
      - includeSubDomains
      - preload
```
An alternative option is to use a `hstsHeader` as a single text string so that it matches the existing annotation form.
Validation in the ingress controller would be different.  I chose to keep it as distinct fields in the `hstsHeaderPolicy`
for user experience ease.  If we decide not to go that way, the same HSTS Policy would look like this:
```yaml
    hstsHeaderPolicy:
      scope:  All
      hstsHeader: "max-age=31536000;includeSubDomains;preload"
```

The `spec.httpHeaders.hstsHeaderPolicy` is formatted and copied to the router environment variable `ROUTER_DEFAULT_HSTS` when the
`IngressOperator` creates the router.  Another environment variables holds the scope `ROUTER_HSTS_SCOPE`.  Changes are needed in `pkg/operator/controller/ingress/deployment.go`:
```go
const (
	WildcardRouteAdmissionPolicy = "ROUTER_ALLOW_WILDCARD_ROUTES"
	...
	RouterHSTSHeaderPolicy       = "ROUTER_DEFAULT_HSTS"
	RouterHSTSHeaderScope        = "ROUTER_HSTS_SCOPE"
    ...
)
...
func desiredRouterDeployment(ci *operatorv1.IngressController, ingressControllerImage string, ingressConfig *configv1.Ingress, apiConfig *configv1.APIServer, networkConfig *configv1.Network, proxyNeeded bool) (*appsv1.Deployment, error) {
...
    if ci.Spec.HTTPHeaders != nil && ci.Spec.HTTPHeaderPolicy != nil && len(ci.Spec.HSTSHeaderPolicy.MaxAge) != 0 {
        header := "max-age=" + ci.Spec.HTTPHeaders.HSTSHeaderPolicy.MaxAgeSeconds
        for _, directive := range ci.Spec.HTTPHeaders.HSTSHeaderPolicy.HSTSDirectives {
            switch directive {
                case  PreloadHSTSDirective:
                    header = header +  ";" + PreloadHSTSDirective
                case IncludeSubDomainsHSTSDirective:
                    header = header + ";" + IncludeSubDomainsHSTSDirective
            }
        }
        env = append(env, corev1.EnvVar{Name: RouterHSTSHeaderPolicy, Value: header})
        switch ci.Spec.HTTPHeaders.HSTSHeaderPolicy.Scope {
            case LimitedHSTSHeaderPolicyScope: // more details on this later in the EP doc
                env = append(env, corev1.EnvVar{Name: RouterHSTSHeaderScope, Value: LimitedHSTSHeaderPolicyScope})
            case AllHSTSHeaderPolicyScope:
                env = append(env, corev1.EnvVar{Name: RouterHSTSHeaderScope, Value: AllHSTSHeaderPolicyScope})
        }
        }
    }
...
```

We don't do any other header validation, but if needed it could go in here:
https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/controller/ingress/controller.go#L441

Accompanying unit and e2e test changes are added to exercise the new header.

A revised HAProxy template is added which will use a new helper method called `hstsHeaderValue`, defined later.
With the addition of processing for `ROUTER_DEFAULT_HSTS`, the code template snippet above becomes:
```gotemplate
{{- $router_global_hsts := env "ROUTER_DEFAULT_HSTS" "" }}
...
{{- if matchValues (print $cfg.TLSTermination) "edge" "reencrypt" }}
  {{- /* Get the HSTS header value using the default and the annotations in the cfg */ }}
  {{- with $hsts := hstsHeaderValue $router_global_hsts "" $cfg "" }}
    http-response set-header Strict-Transport-Security {{$hsts}}
  {{- end }}{{/* hstsHeaderValue */}}
{{- end }}{{/* is "edge" or "reencrypt" */}}
```

#### Enable HSTS per-domain
Some of the proposal for this goal is in the section above.  In order to allow the HAProxy template to add HSTS Headers to
a limited list of domains, it will use the previously mentioned field `InScopeDomains` in the `IngressControllerHSTSHeaderPolicy`
type.

The details on the processing for limited scope in `desiredRouterDeployment` are completed here.
Add one more environment variable, `ROUTER_HSTS_LIST`, which will contain the list of domains in `InScopeDomains`:
```go
	const RouterHSTSHeaderList        = "ROUTER_HSTS_LIST"

       if scope ==  LimitedHSTSHeaderPolicyScope && len(ci.Spec.HTTPHeaers.HSTSHeaderPolicy.InScopeDomains) > 0 {
            env = append(env, corev1.EnvVar{Name: RouterHSTSHeaderScope, Value: LimitedHSTSHeaderPolicyScope})
            env = append(env, corev1.EnvVar{Name: RouterHSTSHeaderList, Value: strings.Join(ci.Spec.HTTPHeaders.HSTSHeaderPolicy.InScopeDomains, ";")})
       }
```
As shown in this example, the HSTS Policy scope is for a limited list of two domains, expires after one year, includes
subdomains, and requests preload:
```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpHeaders:
    hstsHeaderPolicy:
      scope: Limited
      inScopeDomains:
        - domainA
        - domainB
      maxAgeSeconds: 31536000
      hstsDirectives:
        - includeSubDomains
        - preload
```

With the addition of processing for `ROUTER_HSTS_SCOPE` of `limited`, and `ROUTER_HSTS_LIST`, the HAProxy template code
snippet above will gain a few new variables:
```gotemplate
{{- $router_global_hsts := env "ROUTER_DEFAULT_HSTS" "" }}
{{- $router_hsts_scope := env "ROUTER_HSTS_SCOPE" "" }}
{{- $router_hsts_list := env "ROUTER_HSTS_LIST" }}
...
{{- if matchValues (print $cfg.TLSTermination) "edge" "reencrypt" }}
  {{- /* Get the HSTS header value using the default and the annotations in the cfg */ }}
    {{- with $hsts := hstsHeaderValue $router_global_hsts $router_hsts_scope $cfg $router_hsts_list }}
      http-response set-header Strict-Transport-Security {{$hsts}}
    {{- end }}{{/* hstsHeaderValue */}}
{{- end }}{{/* is "edge" or "reencrypt" */}}
```

#### Disable HSTS per-route when it is enabled globally
As mentioned previously, the `Strict-Transport-Security` annotation called
`haproxy.router.openshift.io/hsts_header` has a required directive `max-age` that determines
its lifespan.  A per-route override on globally enabled HSTS can be indicated by setting the `max-age`
value in the route's annotation to zero, as shown in this example:
```yaml
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=0
```

The HAProxy template remains the same, and the promised new function `hstsHeaderValue` will be added:

```go
// If the override annotation string is set, return it - the annotation overrides the default
// Otherwise, if the scope is limited and the domain is in the list - return the default
// Finally, if the scope is all - return the default
// Otherwise, return the empty string
func hstsHeaderValue(default_header string, hsts_scope string, config ServiceAliasConfig, hsts_list string) string {
    wantLimited := false

	if override, ok := config.Annotations["haproxy.router.openshift.io/hsts_header"]; ok {
        return override
    } else if hsts_scope == LimitedHSTSHeaderPolicyScope {
        hsts_array := Strings.split(hsts_list, ";")
        for domain := range hsts_array {
            if strings.HasSuffix(config.Host, domain) {
                wantLimited = true
                break
            }
        }
        if wantLimited {
            return default_header
        }
    } else if hsts_scope == AllHSTSHeaderPolicyScope {
        return default_header
    }
    return ""
}
```

### User Stories

#### As a cluster administrator, I want to enable HSTS globally, for all TLS routes
Update the `IngressController` deployment spec, for example, setting `scope` to `All`, and configuring the rest of the
HSTS Policy.
```yaml
spec:
  httpHeaders:
    hstsHeaderPolicy:
      scope: All
      maxAgeSeconds: 31536000
      hstsDirectives:
        - includeSubDomains
        - preload
```
#### As a cluster administrator, I want to enable HSTS for one domain but not others
Update the `IngressController` deployment spec, setting `scope` to `Limited`, adding that domain to
`inScopeDomains`, and configuring the rest of the HSTS Policy.
```yaml
spec:
  httpHeaders:
    hstsHeaderPolicy:
      scope: Limited
      inScopeDomains:
        - a.b.com
      maxAgeSeconds: 31536000
      hstsDirectives:
        - includeSubDomains
        - preload
```

#### As a cluster administrator, I want to disable HSTS per-route when it is enabled globally
Add a route annotation to the route, setting the directive `max-age=0`, e.g.:
```yaml
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=0
```

### Implementation Details/Notes/Constraints
Implementing this enhancement requires changes in the following repositories:

- openshift/router
- openshift/cluster-ingress-operator

### Risks and Mitigations
- As previously mentioned, use of the `includeSubDomains` directive may cause problems unless the user
is aware of its encompassing implications.  To mitigate the risk of unexpected side effects, an analysis
of the behavior of `includeSubDomains` will be performed and documented.
- The current implementation has a `preload` directive for the `Strict-Transport-Security`
header.  This is not an RFC 6797 directive and therefore its implementation may vary by
user agent.  It can be implemented, but support may vary by user agent implementation and therefore no specifications
are made here.
- The direction of overrides on global versus per-route HSTS annotations will be documented,
because it may not be clear to the user that a global setting cannot override a per-route setting.
Otherwise, it would not be possible to use a per-route setting.

## Design Details
## Drawbacks
N/A
## Alternatives
N/A

### Open Questions
Version Skew Strategy - not clear to me if that is required here.

### Test Plan
HAProxy and Router have unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional four goals and the new `ROUTER_DEFAULT_HSTS` environment variable propagation
from the `IngressController` to the router.

The operator has end-to-end tests; for this enhancement, add the following tests:

#### Enable global HSTS
1. Create an IngressController that enables global HSTS
2. Open a connection to this route and send a request
3. Verify that a response is received and that the headers include HSTS as configured in step 1

#### Enable Limited scope
1. Create an IngressController that enables Limited scope HSTS
2. Open a connection to a route with a domain outside the scope and send a request
3. Verify that a response is received and that the headers do not include HSTS
4. Open a connection to a route with a domain inside the scope and send a request
5. Verify that a response is received and that the headers include HSTS

#### Override global HSTS on an individual route
1. Create an IngressController that enables global HSTS
2. Create a route that disables global HSTS with an annotation
3. Open a connection to this route and send a request
4. Verify that a response is received and that the headers do not include HSTS as configured in step 2

### Graduation Criteria
N/A

### Upgrade / Downgrade Strategy
On upgrade, any previous per-route HSTS configuration remains in effect.

If a cluster administrator applied any of the new HSTS configuration options
in 4.8, then downgraded to 4.7, the HSTS configuration settings would be ignored
except for the per-route HSTS configurations.  The administrator would be responsible
for removing any per-route HSTS configurations that were no longer applicable.

From a user perspective, after a downgrade an end-user's browser
may continue to access the configured routes via the previously configured HSTS Policy,
until the HSTS Policy expires.

### Version Skew Strategy
N/A (TBD)

## Implementation History
N/A
### Placeholder for lint
N/A
#### Dev Preview -> Tech Preview
N/A
#### Tech Preview -> QA
N/A
#### Tech Preview -> GA
N/A
#### Removing a deprecated feature
N/A
