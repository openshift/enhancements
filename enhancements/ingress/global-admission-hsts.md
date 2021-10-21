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

creation-date: 2021-04-16

last-updated: 2021-09-23

status: implementable

see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=1512759
  - https://bugzilla.redhat.com/show_bug.cgi?id=1430035

replaces:
  - enhancements/ingress/global-options-enable-hsts.md

superseded-by:

---

# Global Admission Plugin for HTTP Strict Transport Security (HSTS)

This enhancement provides the capability to enforce HSTS policy requirements
for TLS routes.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In 3.x and 4.x customers can [provide a per-route annotation to enable HSTS](
https://docs.openshift.com/container-platform/4.4/networking/routes/route-configuration.html#nw-enabling-hsts_route-configuration).
For customers with many routes or regulatory compliance issues, the manual per-route annotation is seen as sub-optimal.

This enhancement extends the `Ingress.config.openshift.io` API and adds a new `route` admission plugin to the OpenShift API server
which together allow cluster administrators to enforce HSTS globally. This enhancement also provides a recommendation for batch route annotation configuration.

This enhancement supersedes a previous enhancement documented in
[global-options-enable-hsts.md](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/global-options-enable-hsts.md),
for reasons provided within this proposal.  Thanks to David Eads and Miciah Masters for their insights and contributions to the updated proposal.

## Motivation

HSTS ([RFC 6797](https://tools.ietf.org/html/rfc6797)) policy enforces the use
of HTTPS in client requests to the hosts that the policy covers, without having to make use of HTTP redirects.
HSTS provides user protection and is concerned with minimizing security threats based on
network traffic eavesdropping and man-in-the-middle attacks.  Using a response header
called `Strict-Transport-Security`, a HTTP response informs clients that a single host or entire domain
can be accessed only via HTTPS.

Administrators who are tasked with route management and/or regulatory compliance face a
number of issues with regard to enforcing HSTS.  For efficiency and protection against
configuration errors, they have requested to automatically and globally enable HSTS
on the basis of the cluster `Ingress` domains. However, because enabling HSTS automatically
and globally can cause disruption of service on a wide scale, they should also be able to
audit and change HSTS configuration without further outages.  Finally, they should be able
to consistently apply the same HSTS configuration and predict the outcome on any cluster.

### Goals

This proposal allows administrators to:
- Continue to enable HSTS per route (i.e., retain the existing feature functionality).
- Provide HSTS verification per domain.
- Enable administrators to audit HSTS configurations on any cluster.
- Enable users to predict HSTS enforcement for a route on any cluster by referring solely
  to that specific route's manifest.

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
- `max-age`: (required) delta time range in seconds during which the host is to be regarded as an HSTS host
  - If set to 0, it negates the effect, and the host is no longer regarded as an HSTS host, including any
    of the matching subdomains if `includeSubDomains` is specified
  - `max-age` is a time-to-live value, so if a client makes a request for the route's host
  and this period of time elapses before the client makes another request for the same host, the
  HSTS policy will expire on that client
- `preload`: (optional) if present, tells the client to include the route's host in its host preload list so that
  it never needs to do an initial load to get the HSTS header (note that this is not defined in RFC 6797
  and is therefore client browser implementation-dependent)
- `includeSubDomains`: (optional) if present, the HSTS Policy applies to any hosts with subdomains of the host's
  domain name.  The purpose is for web applications to protect Secure-flagged "domain cookies" as discussed in
  [RFC6797 Section 1.4.4](https://tools.ietf.org/html/rfc6797#section-14.4).
  - E.g., in terms of HSTS, hosts `abc.bar.foo.com` and `bar.foo.com` both match the superdomain `bar.foo.com`.
  - Thus, if the `route` with `host` of `bar.foo.com` specified a HSTS Policy with `includeSubDomains`, then:
    - the host `app.bar.foo.com` would inherit the HSTS Policy of `bar.foo.com`
    - the host `bar.foo.com` would inherit the HSTS Policy of `bar.foo.com`
    - the host `foo.com` would NOT inherit the HSTS Policy of `bar.foo.com`, but another route for `foo.com` could be
      created and annotated
    - the host `def.foo.com` would NOT inherit the HSTS Policy of `bar.foo.com`, but another route for `def.foo.com`
      could be created and annotated
  - It is worth noting, that in terms of the Openshift API, if a `route` with `host` of `bar.foo.com` allows a
    `WildcardPolicy` of `Subdomain`, then it exclusively serves all the hosts ending in `foo.com`. Another route could
    not be created that ends in `foo.com` unless it is in the same namespace.
  - Furthermore, if a `route` with `host` of `bar.foo.com` allows a `WildcardPolicy` of `Subdomain`, AND specifies a
    HSTS Policy with `includeSubDomains`, then there would be no way to add a HSTS Policy to `foo.com` or `def.foo.com`,
    unless they are in the same namespace.  For more information on this, see Design Details.

HSTS is currently implemented in the HAProxy template and applied to `edge` and
`reencrypt` routes that have the `haproxy.router.openshift.io/hsts_header` annotation:
```gotemplate
{{- /* hsts header in response: */}}
{{- /* Not fully compliant to RFC6797#6.1 yet: has to accept not conformant directives */}}
{{- $hstsOptionalTokenPattern := `(?:includeSubDomains|preload)` }}
{{- $hstsPattern := printf `(?i)(?:%[1]s\s*[;]\s*)*max-age\s*=\s*(?:\d+|"\d+")(?:\s*[;]\s*%[1]s)*`  $hstsOptionalTokenPattern -}}
...
{{- if matchValues (print $cfg.TLSTermination) "edge" "reencrypt" }}
    {{- with $hsts := firstMatch $hstsPattern (index $cfg.Annotations "haproxy.router.openshift.io/hsts_header") }}
  http-response set-header Strict-Transport-Security {{$hsts}}
    {{- end }}{{/* hsts header */}}
  {{- end }}{{/* is "edge" or "reencrypt" */}}
```

#### Provide HSTS verification per-domain
This proposal allows cluster administrators to configure HSTS verification on a per-domain basis with
the addition of a new openshift-api-server validating admission plugin for the router, called
`route.openshift.io/RequiredRouteAnnotations`.  If the administrator configures this plugin to enforce HSTS, then
any newly created route must be configured with a compliant HSTS Policy, which will be verified against the global
setting on the cluster `Ingress` configuration, called `ingresses.config.openshift.io/cluster`.

The administrator will interact with the `RequiredRouteAnnotations` plugin by configuring the
`Ingress.Spec.RequiredHSTSPolicies` field with one or more `RequiredHSTSPolicy` values.  `RequiredHSTSPolicy` is
a new type that will be added to the API `openshift/api/config/v1/types_ingress.go`, to capture the
configuration of the required HSTS Policy.  With `RequiredHSTSPolicy`, administrators can configure namespace
selectors and/or route domains to use for matching routes to HSTS policy parameters.  They can also configure the
HSTS maximum age, preload policy, and whether the HSTS policy should include subdomains of the configured route's host.

````go
type RequiredHSTSPolicy struct {
    // namespaceSelector specifies a label selector such that the policy applies only to those routes that
    // are in namespaces with labels that match the selector, and are in one of the DomainPatterns.
    // Defaults to the empty LabelSelector, which matches everything.
    // +optional
    NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

    // domainPatterns is a list of domains for which the desired HSTS annotations are required.
    // If domainPatterns is specified and a route is created with a spec.host matching one of the domains,
    // the route must specify the HSTS Policy components described in the matching RequiredHSTSPolicy.
    //
    // The use of wildcards is allowed like this: *.foo.com matches everything under foo.com.
    // foo.com only matches foo.com, so to cover foo.com and everything under it, you must specify *both*.
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:Required
    // +required
    DomainPatterns []string `json:"domainPatterns"`

    // maxAge is the delta time range in seconds during which hosts are regarded as HSTS hosts.
    // If set to 0, it negates the effect, and hosts are removed as HSTS hosts.
    // If set to 0 and includeSubdomains is specified, all subdomains of the host are also removed as HSTS hosts.
    // maxAge is a time-to-live value, and if this policy is not refreshed on a client, the HSTS
    // policy will eventually expire on that client.
    MaxAge MaxAgePolicy `json:"maxAge"`

    // preloadPolicy directs the client to include hosts in its host preload list so that
    // it never needs to do an initial load to get the HSTS header (note that this is not defined
    // in RFC 6797 and is therefore client implementation-dependent).
    // +optional
    PreloadPolicy PreloadPolicy `json:"preloadPolicy,omitempty"`

    // includeSubDomainsPolicy means the HSTS Policy should apply to any subdomains of the host's
    // domain name.  Thus, for the host bar.foo.com, if includeSubDomainsPolicy was set to RequireIncludeSubDomains:
    // - the host app.bar.foo.com would inherit the HSTS Policy of bar.foo.com
    // - the host bar.foo.com would inherit the HSTS Policy of bar.foo.com
    // - the host foo.com would NOT inherit the HSTS Policy of bar.foo.com
    // - the host def.foo.com would NOT inherit the HSTS Policy of bar.foo.com
    // +optional
    IncludeSubDomainsPolicy IncludeSubDomainsPolicy `json:"includeSubDomainsPolicy,omitempty"`
}

// MaxAgePolicy contains a numeric range for specifying a compliant HSTS max-age for the enclosing RequiredHSTSPolicy
type MaxAgePolicy struct {
    // The largest allowed value (in seconds) of the RequiredHSTSPolicy max-age
    // This value can be left unspecified, in which case no upper limit is enforced.
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=2147483647
    LargestMaxAge *int32 `json:"largestMaxAge,omitempty"`

    // The smallest allowed value (in seconds) of the RequiredHSTSPolicy max-age
    // Setting max-age=0 allows the deletion of an existing HSTS header from a host.  This is a necessary
    // tool for administrators to quickly correct mistakes.
    // This value can be left unspecified, in which case no lower limit is enforced.
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=2147483647   
    SmallestMaxAge *int32 `json:"smallestMaxAge,omitempty"`
}

// PreloadPolicy contains a value for specifying a compliant HSTS preload policy for the enclosing RequiredHSTSPolicy
// +kubebuilder:validation:Enum=RequirePreload;RequireNoPreload;NoOpinion
type PreloadPolicy string

const (
    // RequirePreloadPolicy means HSTS "preload" is required by the RequiredHSTSPolicy
    RequirePreloadPolicy PreloadPolicy = "RequirePreload"

    // RequireNoPreloadPolicy means HSTS "preload" is forbidden by the RequiredHSTSPolicy
    RequireNoPreloadPolicy PreloadPolicy = "RequireNoPreload"

    // NoOpinionPreloadPolicy means HSTS "preload" doesn't matter to the RequiredHSTSPolicy
    NoOpinionPreloadPolicy PreloadPolicy = "NoOpinion"
)

// IncludeSubDomainsPolicy contains a value for specifying a compliant HSTS includeSubdomains policy
// for the enclosing RequiredHSTSPolicy
// +kubebuilder:validation:Enum=RequireIncludeSubDomains;RequireNoIncludeSubDomains;NoOpinion
type IncludeSubDomainsPolicy string

const (
    // RequireIncludeSubDomains means HSTS "includeSubDomains" is required by the RequiredHSTSPolicy
    RequireIncludeSubDomains IncludeSubDomainsPolicy = "RequireIncludeSubDomains"

    // RequireNoIncludeSubDomains means HSTS "includeSubDomains" is forbidden by the RequiredHSTSPolicy
    RequireNoIncludeSubDomains IncludeSubDomainsPolicy = "RequireNoIncludeSubDomains"

    // NoOpinionIncludeSubDomains means HSTS "includeSubDomains" doesn't matter to the RequiredHSTSPolicy
    NoOpinionIncludeSubDomains IncludeSubDomainsPolicy = "NoOpinion"
)
````
A new type `RequiredHSTSPolicies` will be added to `IngressSpec` to contain any configured required HSTS Policies:
```go
type IngressSpec struct {
...
    // requiredHSTSPolicies specifies HSTS policies that are required to be set on newly created  or updated routes
	// matching the domainPattern/s and namespaceSelector/s that are specified in the policy.
	// Each requiredHSTSPolicy must have at least a domainPattern and a maxAge to validate a route HSTS Policy route
	// annotation, and affect route admission.
	//
	// A candidate route is checked for HSTS Policies if it has the HSTS Policy route annotation:
	// "haproxy.router.openshift.io/hsts_header"
	// E.g. haproxy.router.openshift.io/hsts_header: max-age=31536000;preload;includeSubDomains
	//
	// - For each candidate route, if it matches a requiredHSTSPolicy domainPattern and optional namespaceSelector,
	// then the maxAge, preloadPolicy, and includeSubdomainsPolicy must be valid to be admitted.  Otherwise, the route
	// is rejected.
	// - The first match, by domainPattern and optional namespaceSelector, in the ordering of the RequiredHSTSPolicies
	// determines the route's admission status.
	// - If the candidate route doesn't match any requiredHSTSPolicy domainPattern and optional namespaceSelector,
	// then it may use any HSTS Policy annotation.
	//
	// The HSTS policy configuration may be changed after routes have already been created. An update to a previously
	// admitted route may then fail if the updated route does not conform to the updated HSTS policy configuration.
	// However, changing the HSTS policy configuration will not cause a route that is already admitted to stop working.
	//
	// Note that if there are no RequiredHSTSPolicies, any HSTS Policy annotation on the route is valid.
	// +optional
	RequiredHSTSPolicies []RequiredHSTSPolicy `json:"requiredHSTSPolicies,omitempty"`
}
```
The `route.openshift.io/RequiredRouteAnnotations` route validating admission plugin would follow the current OpenShift
API server design pattern for admission plugins and additionally validate that routes matching
the `RequiredHSTSPolicy` `DomainPatterns` and `NamespaceSelector` (if any), are configured with the required HSTS
policy.  NOTE:  If there are no `RequiredHSTSPolicies`, any route annotation will be valid.

Some admission code is listed here:
````go
...
type requiredRouteAnnotations struct {
	*admission.Handler
	routeLister   routev1listers.RouteLister
	nsLister      corev1listers.NamespaceLister
	ingressLister configv1listers.IngressLister
	cachesToSync  []cache.InformerSynced
	cacheSyncLock cacheSync
}

// Ensure that the required OpenShift admission interfaces are implemented.
var _ = initializer.WantsExternalKubeInformerFactory(&requiredRouteAnnotations{})
var _ = admission.ValidationInterface(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftConfigInformers(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftRouteInformers(&requiredRouteAnnotations{})

var maxAgeRegExp = regexp.MustCompile(`max-age=(\d+)`)

// Validate ensures that routes specify required annotations, and returns nil if valid.
// The admission handler ensures this is only called for Create/Update operations.
func (o *requiredRouteAnnotations) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) (err error) {
	if a.GetResource().GroupResource() != grouproute.Resource("routes") {
		return nil
	}
	newObject, isRoute := a.GetObject().(*routeapi.Route)
	if !isRoute {
		return nil
	}

	// Determine if there are HSTS changes in this update
	if a.GetOperation() == admission.Update {
		wants, has := false, false
		var oldHSTS, newHSTS string

		newHSTS, wants = newObject.Annotations[hstsAnnotation]

		oldObject := a.GetOldObject().(*routeapi.Route)
		oldHSTS, has = oldObject.Annotations[hstsAnnotation]

		// Skip the validation if we're not making a change to HSTS at this time
		if wants == has && newHSTS == oldHSTS {
			return nil
		}
	}

	// Wait just once up to 20 seconds for all caches to sync
	if !o.waitForSyncedStore(ctx) {
		return admission.NewForbidden(a, errors.New(pluginName+": caches not synchronized"))
	}

	ingress, err := o.ingressLister.Get("cluster")
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	newRoute := a.GetObject().(*routeapi.Route)
	namespace, err := o.nsLister.Get(newRoute.Namespace)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	if err = isRouteHSTSAllowed(ingress, newRoute, namespace); err != nil {
		return admission.NewForbidden(a, err)
	}
	return nil
}

func (o *requiredRouteAnnotations) SetExternalKubeInformerFactory(kubeInformers informers.SharedInformerFactory) {
	o.nsLister = kubeInformers.Core().V1().Namespaces().Lister()
	o.cachesToSync = append(o.cachesToSync, kubeInformers.Core().V1().Namespaces().Informer().HasSynced)
}

// waitForSyncedStore calls cache.WaitForCacheSync, which will wait up to timeToWaitForCacheSync
// for the cachesToSync to synchronize.
func (o *requiredRouteAnnotations) waitForSyncedStore(ctx context.Context) bool {
	syncCtx, cancelFn := context.WithTimeout(ctx, timeToWaitForCacheSync)
	defer cancelFn()
	if !o.cacheSyncLock.hasSynced() {
		if !cache.WaitForCacheSync(syncCtx.Done(), o.cachesToSync...) {
			return false
		}
		o.cacheSyncLock.setSynced()
	}
	return true
}

func (o *requiredRouteAnnotations) ValidateInitialization() error {
	if o.ingressLister == nil {
		return fmt.Errorf(pluginName + " plugin needs an ingress lister")
	}
	if o.routeLister == nil {
		return fmt.Errorf(pluginName + " plugin needs a route lister")
	}
	if o.nsLister == nil {
		return fmt.Errorf(pluginName + " plugin needs a namespace lister")
	}
	if len(o.cachesToSync) < 3 {
		return fmt.Errorf(pluginName + " plugin missing informer synced functions")
	}
	return nil
}

func NewRequiredRouteAnnotations() *requiredRouteAnnotations {
	return &requiredRouteAnnotations{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}

func (o *requiredRouteAnnotations) SetOpenShiftRouteInformers(informers routeinformers.SharedInformerFactory) {
	o.cachesToSync = append(o.cachesToSync, informers.Route().V1().Routes().Informer().HasSynced)
	o.routeLister = informers.Route().V1().Routes().Lister()
}

func (o *requiredRouteAnnotations) SetOpenShiftConfigInformers(informers configinformers.SharedInformerFactory) {
	o.cachesToSync = append(o.cachesToSync, informers.Config().V1().Ingresses().Informer().HasSynced)
	o.ingressLister = informers.Config().V1().Ingresses().Lister()
}

// isRouteHSTSAllowed returns nil if the route is allowed.  Otherwise, returns details and a suggestion in the error
func isRouteHSTSAllowed(ingress *configv1.Ingress, newRoute *routeapi.Route, namespace *corev1.Namespace) error {
	// Invalid if a HSTS Policy is specified but this route is not TLS.  Just log a warning.
	if tls := newRoute.Spec.TLS; tls != nil {
		switch termination := tls.Termination; termination {
		case routeapi.TLSTerminationEdge, routeapi.TLSTerminationReencrypt:
		// Valid case
		default:
			// Non-tls routes will not get HSTS headers, but can still be valid
			klog.Warningf("HSTS Policy not added for %s, wrong termination type: %s", newRoute.Name, termination)
			return nil
		}
	}

	requirements := ingress.Spec.RequiredHSTSPolicies
	for _, requirement := range requirements {
		// Check if the required namespaceSelector (if any) and the domainPattern match
		if matches, err := requiredNamespaceDomainMatchesRoute(requirement, newRoute, namespace); err != nil {
			return err
		} else if !matches {
			// If one of either the namespaceSelector or domain didn't match, we will continue to look
			continue
		}

		routeHSTS, err := hstsConfigFromRoute(newRoute)
		if err != nil {
			return err
		}

		// If there is no annotation but there needs to be one, return error
		if routeHSTS != nil {
			if err = routeHSTS.meetsRequirements(requirement); err != nil {
				return err
			}
		}

		// Validation only checks the first matching required HSTS rule.
		return nil
	}

	// None of the requirements matched this route's domain/namespace, it is automatically allowed
	return nil
}

type hstsConfig struct {
	maxAge            int32
	preload           bool
	includeSubDomains bool
}

// Parse out the hstsConfig fields from the annotation
// Unrecognized fields are ignored
func hstsConfigFromRoute(route *routeapi.Route) (*hstsConfig, error) {
	var ret hstsConfig

	trimmed := strings.ToLower(strings.ReplaceAll(route.Annotations[hstsAnnotation], " ", ""))
	tokens := strings.Split(trimmed, ";")
	for _, token := range tokens {
		if strings.EqualFold(token, "includeSubDomains") {
			ret.includeSubDomains = true
		}
		if strings.EqualFold(token, "preload") {
			ret.preload = true
		}
		// unrecognized tokens are ignored
	}

	if match := maxAgeRegExp.FindStringSubmatch(trimmed); match != nil && len(match) > 1 {
		age, err := strconv.ParseInt(match[1], 10, 32)
		if err != nil {
			return nil, err
		}
		ret.maxAge = int32(age)
	} else {
		return nil, fmt.Errorf("max-age must be set in HSTS annotation")
	}

	return &ret, nil
}

// Make sure the given requirement meets the configured HSTS policy, validating:
// - range for maxAge (existence already established)
// - preloadPolicy
// - includeSubDomainsPolicy
func (c *hstsConfig) meetsRequirements(requirement configv1.RequiredHSTSPolicy) error {
	if requirement.MaxAge.LargestMaxAge != nil && c.maxAge > *requirement.MaxAge.LargestMaxAge {
		return fmt.Errorf("is greater than maximum age (%d)", *requirement.MaxAge.LargestMaxAge)
	}
	if requirement.MaxAge.SmallestMaxAge != nil && c.maxAge < *requirement.MaxAge.SmallestMaxAge {
		return fmt.Errorf("is less than minimum age (%d)", *requirement.MaxAge.SmallestMaxAge)
	}

	switch requirement.PreloadPolicy {
	case configv1.NoOpinionPreloadPolicy:
	// anything is allowed, do nothing
	case configv1.RequirePreloadPolicy:
		if !c.preload {
			return fmt.Errorf("preload must be specified")
		}
	case configv1.RequireNoPreloadPolicy:
		if c.preload {
			return fmt.Errorf("preload must not be specified")
		}
	}

	switch requirement.IncludeSubDomainsPolicy {
	case configv1.NoOpinionIncludeSubDomains:
	// anything is allowed, do nothing
	case configv1.RequireIncludeSubDomains:
		if !c.includeSubDomains {
			return fmt.Errorf("includeSubDomains must be specified")
		}
	case configv1.RequireNoIncludeSubDomains:
		if c.includeSubDomains {
			return fmt.Errorf("includeSubDomains must not be specified")
		}
	}

	return nil
}

// Check if the route matches the required domain/namespace in the HSTS Policy
func requiredNamespaceDomainMatchesRoute(requirement configv1.RequiredHSTSPolicy, route *routeapi.Route, namespace *corev1.Namespace) (bool, error) {
	matchesNamespace, err := matchesNamespaceSelector(requirement.NamespaceSelector, namespace)
	if err != nil {
		return false, err
	}

	routeDomains := []string{route.Spec.Host}
	for _, ingress := range route.Status.Ingress {
		routeDomains = append(routeDomains, ingress.Host)
	}
	matchesDom := matchesDomain(requirement.DomainPatterns, routeDomains)

	return matchesNamespace && matchesDom, nil
}

// Check all of the required domainMatcher patterns against all provided domains,
// first match returns true.  If none match, return false.
func matchesDomain(domainMatchers []string, domains []string) bool {
	for _, pattern := range domainMatchers {
		for _, candidate := range domains {
			matched, err := filepath.Match(pattern, candidate)
			if err != nil {
				klog.Warningf("Ignoring HSTS Policy domain match for %s, error parsing: %v", candidate, err)
				continue
			}
			if matched {
				return true
			}
		}
	}

	return false
}

func matchesNamespaceSelector(nsSelector *metav1.LabelSelector, namespace *corev1.Namespace) (bool, error) {
	if nsSelector == nil {
		return true, nil
	}
	selector, err := getParsedNamespaceSelector(nsSelector)
	if err != nil {
		klog.Warningf("Ignoring HSTS Policy namespace match for %s, error parsing: %v", namespace, err)
		return false, err
	}
	return selector.Matches(labels.Set(namespace.Labels)), nil
}

func getParsedNamespaceSelector(nsSelector *metav1.LabelSelector) (labels.Selector, error) {
	// TODO cache this result to save time
	return metav1.LabelSelectorAsSelector(nsSelector)
}
````

Formatting on the HSTS Annotation field on a `route` must include semi-colons between HSTS directives.  Any directives
other than `max-age`, `includeSubDomains`, and `preload` will be ignored.  Here is an example:
```yaml
haproxy.router.openshift.io/hsts_header: max-age=<numberofseconds>[;includeSubDomains][;preload]
```

The `RequiredHSTSPolicies` variable can be configured by the administrator in the `Ingress`, as shown in this example, where
there is one HSTS Policy, and it is for domain abc.com, expires after one year, includes subdomains, and requests preload:
```yaml
apiVersion: config.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  domain: apps.abc.com
  requiredHSTSPolicies:
  - domainPatterns:
    - abc.com
    maxAge:
      smallestMaxAge: 1
      largestMaxAge: 31536000
    preloadPolicy: "RequirePreload"
    includeSubDomainsPolicy: "RequireIncludeSubDomains"
```
To edit the `Ingress` config:
```bash
oc edit ingresses.config.openshift.io/cluster
```
#### Route Administration

To handle upgraded clusters with non-compliant HSTS routes, the best solution is to update
the manifests at the source and apply the updates.  However, in some situations, administrators
can update the routes directly via the API.

To apply HSTS to all routes in the cluster, use a command like this. (This command requires
an updated `oc` client that supports the `--all-namespaces` flag on the `oc annotate` verb):
```shell
$ oc annotate route --all --all-namespaces --overwrite=true "haproxy.router.openshift.io/hsts_header"="max-age=31536000"
```
To apply HSTS to all routes in a particular namespace, use a command like this:
```shell
$ oc annotate route --all -n my-namespace --overwrite=true "haproxy.router.openshift.io/hsts_header"="max-age=31536000"
```
To remove HSTS from all routes in a particular namespace, don't just remove the annotation.
Removing the annotation is not the same as setting the max-age to 0. The header will remain on the client until
its max-age times out, so setting the max-age to 0 is the correct way to remove the header.
To set the `max-age` to 0, use a command like this:
```shell
$ oc annotate route --all -n my-namespace --overwrite=true "haproxy.router.openshift.io/hsts_header"="max-age=0"
```

To apply more selective policy to running resources, cluster administrators can use this simple
command-line script, `batch_annotate.sh`, run with an input file of routes:
````shell
#!/bin/bash

# Usage: ./batch_annotate.sh filename.txt
# Where filename.txt lines are formatted: route_name annotation_key annotation_value namespace overwrite
# For example: console  haproxy.router.openshift.io/hsts_header  max-age=600 openshift-console true

CR=$'\n'
# Annotate routes with a given route name (but will not overwrite existing unless overwrite is true)
function annotate_route () {
  local route_name=${1}
  local annotation_key=${2}
  local annotation_value=${3}
  local namespace=${4}
  local overwrite=${5}
  echo "${CR}[cmd]: oc annotate route --overwrite=${overwrite} ${route_name} ${annotation_key}=${annotation_value} -n ${namespace}"
  oc annotate route "${route_name}" --overwrite="${overwrite}" "${annotation_key}"="${annotation_value}" -n "${namespace}""
}

function check_result () {
  local r=${1}
  if [[ $r -eq 0 ]]; then
    echo "Success"
  else
    echo "Failure: error code $r"
  fi
}

if [[ -z ${1} ]]; then
  echo "Usage: must enter filename"
  exit 1
else
  echo "Reading from ${1}..."
fi

while IFS= read -r line; do
  IFS=' ' read -r -a fields <<< "$line"
  annotate_route  "${fields[0]}" "${fields[1]}" "${fields[2]}" "${fields[3]}" "${fields[4]}"
  result=$?
  check_result $result
done < "${1}"

echo "Done"
````
Input file example (filename.txt) for the script:
```text
console  haproxy.router.openshift.io/hsts_header  max-age=900 openshift-console true
myBigRoute  haproxy.router.openshift.io/hsts_header  max-age=31536000;preload;includeSubDomains openshift-console true
```
Usage example:
```shell
$ ./batch_annotate.sh filename.txt
```

#### Testing
Accompanying unit and e2e test changes will be added to exercise the `RequiredHSTSPolicies` type.
The admission plugin guarantees the following:
* Routes with HSTS without a TLS termination policy of edge or re-encrypt will be accepted without checking the annotation
* Routes with hosts that match the `domainPatterns` of the HSTS will be validated if no `namespaceSelector` is in effect
* Routes with hosts that match the `domainPatterns` and `namespaceSelector` of the HSTS will be validated
* Routes that only match the `namespaceSelector` and not the `domainPatterns` will not be validated, any annotation is ok
* Routes that are validated will validate that the `maxAge` exists and falls within the range of the HSTS `maxAgePolicy`
* Routes that are validated will validate that the `preloadPolicy` and `includeSubDomainPolicy` match the HSTS Policy if they exist
* Routes that are validated will be rejected if they do not comply with the HSTS Policy that matches their `domainPatterns`/`namespaceSelector`

#### Audit HSTS configurations on any cluster
This proposal offers the cluster administrator a way to review the HSTS configurations guaranteed by the admission plugin.
For example, to review the maxAge set for required HSTS Policies:
```shell
$ oc get clusteroperator/ingress -n openshift-ingress-operator -o jsonpath='{range .spec.requiredHSTSPolicies[*]}{.spec.requiredHSTSPolicies.maxAgePolicy.largestMaxAge}{"\n"}{end}'
```
To review the HSTS annotations on all routes:
````shell
$ oc get route  --all-namespaces -o go-template='{{range .items}}{{if .metadata.annotations}}{{$a := index .metadata.annotations "haproxy.router.openshift.io/hsts_header"}}{{$n := .metadata.name}}{{with $a}}Name: {{$n}} HSTS: {{$a}}{{"\n"}}{{else}}{{""}}{{end}}{{end}}{{end}}'

Name: myBigRoute HSTS: max-age=31536000;preload;includeSubDomains
````
Note: a route may show an HSTS annotation, but not apply it, if it has not set TLS termination policy of edge or re-encrypt.

#### Predict HSTS enforcement on any cluster with a single configuration manifest
This proposal offers the cluster administrator a way to configure and enforce HSTS configurations on routes by configuring
the `Ingress` using templates that substitute only for the difference between clusters.  For example, for
a development cluster with domain `dev.abc.com` and production cluster with `prod.abc.com` domains, administrators can use a single
`Ingress` configuration template that substitutes the domain:

```yaml
apiVersion: config.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  domain: abc.com
  requiredHSTSPolicies:
  - domainPatterns:
    - ${DOMAIN}
    maxAge:
      smallestMaxAge: 1
      largestMaxAge: 31536000
    preloadPolicy: RequirePreload
    includeSubDomainsPolicy: RequireIncludeSubDomains
```
All routes that require HSTS will then use the same required Annotation on either cluster:
````yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=31536000;preload;includeSubDomains
...
spec:
  host: def.abc.com
  tls:
    termination: "reencrypt"
    ...
  wildcardPolicy: "Subdomain"
````
After applying the annotation, a route can be checked for the proper header using a curl command like this:
```shell
$ curl -sik https://foo.com | grep strict-transport-security
strict-transport-security: max-age=31536000
````

### User Stories

#### As a cluster administrator, I want to verify HSTS globally, for all TLS routes in domain `foo.com`
Update the `Ingress` configuration spec like the example below:
```yaml
spec:
  domain: abc.com
  requiredHSTSPolicies:
  - domainPatterns:
    - *.foo.com
    - foo.com
    maxAge:
      smallestMaxAge: 1
      largestMaxAge: 31536000
```
Also update the route for domain `foo.com` with the matching annotation:
````yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=31536000   
````
To audit the routes that match host `foo.com`, use a command like this:
```shell
oc get route -A -o go-template='{{range .items}}{{if .metadata.annotations}}{{$a := index .metadata.annotations "haproxy.router.openshift.io/hsts_header"}}{{$h := .spec.host}}{{if $a}}{{if $h}}{{if eq $h "foo.com" }}Host: {{ $h}} HSTS: {{$a}}{{"\n"}}{{end}}{{end}}{{end}}{{end}}{{end}}'
Host: foo.com HSTS: max-age=31536000
```
To audit the routes that match any host ending in `foo.com`, use a command like this (requires `jq`):
```shell
oc get routes -A -o json | jq -r '.items|.[]|select(.spec.host|endswith("foo.com"))|"Host: "+.spec.host+" HSTS: "+(.metadata.annotations["haproxy.router.openshift.io/hsts_header"]//"<none>")'
Host: foo.com HSTS: max-age=31536000;preload
Host: abc.foo.com HSTS: max-age=31536000;preload
```
#### As a cluster administrator, I want to verify HSTS for only the domain `kibana.foo.com`
Update the `Ingress` configuration spec like the example below:
```yaml
spec:
  domain: abc.com
  requiredHSTSPolicies:
  - domainPatterns:
    - kibana.foo.com
    maxAge:
      smallestMaxAge: 1
      largestMaxAge: 31536000
```
Also update the route for domain `kibana.foo.com` with the matching annotation:
````yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=31536000   
````
### Implementation Details/Notes/Constraints
Implementing this enhancement requires changes in the following repositories:
- openshift/api
- openshift/openshift-apiserver
- openshift/cluster-config-operator

### Risks and Mitigations
As previously mentioned, use of the `includeSubDomains` directive may cause problems unless the user
is aware of its encompassing implications.  As described in
[RFC 6797 Section 11.4](https://tools.ietf.org/html/rfc6797#section-11.4), without special mitigation, it is possible
for at least two complex problems to arise:
- it is possible for a HSTS host to offer unsecured services (HTTP) on alternate ports or different subdomains of the
  HSTS host.  For example: abc.foo.com is a webserver with paths on ports 8080 for HTTP and 443 for HTTPS.  If the HSTS
  Policy for a route foo.com was set to `includeSubDomains`, the policy will apply to abc.foo.com and the client will see
  errors when accessing abc.foo.com:8080
- if different web applications are offered on different subdomains of a HSTS host with `includeSubDomains`,
  there is no guarantee that the web applications access the superdomain directly, thus they may not receive the HSTS
  Policy at all.  For example, day.foo.com is an application that requires HSTS. If the HSTS Policy for a route foo.com
  was set to `includeSubDomains` and there is no separate policy for day.foo.com, and the client never interacts directly
  with foo.com, it will not receive or enforce the HSTS Policy.

To mitigate issues, RFC 6797 recommends to not use `includeSubDomains` in the case that HTTPS and HTTP
routes share the same domain, or to ensure HTTP based services are also offered via HTTPS for the same subdomain.
In the case that different web applications are offered on different subdomains of a HSTS host, each domain should be
configured separately instead of using `includeSubDomains` on a superdomain.

Additionally, the current implementation has a `preload` directive for the `Strict-Transport-Security`
header.  This is not an RFC 6797 directive and therefore its implementation may vary by
user agent.  No specifications for support are made here.

## Design Details
Design details are addressed throughout this document.  However, additional discussion is warranted, regarding the use
of `includeSubDomains` and how this interoperates with the `route`'s wildcard-policy for subdomains.  Though the host
admitter reduces the subdomain of `bar.foo.com` to `foo.com` and thus matches any
subdomains ending in `foo.com`, the HSTS Policy understands the subdomains of `bar.foo.com` to include `bar.foo.com`
and exclude `def.foo.com`.  There are several possible outcomes we could provide for when a `route` has a wildcard-policy
of `Subdomain`, and it also specifies a HSTS Policy with `includeSubDomains`:
1. It is rejected because of the possible confusion.
2. It is admitted.
3. It is admitted, but when a HSTS Policy with `includeSubDomains` is present, and the route has a wildcard-policy of
  `Subdomain`, the HAProxy template adds HSTS headers to any associated wildcard subdomains.  E.g. for host `bar.foo.com`,
  any routes ending in `foo.com` will receive the same HSTS header as `bar.foo.com`.
4. It is admitted, but we change the host admitter logic to allow routes that belong to a wildcard subdomain to be
  admitted, instead of marked `HostAlreadyClaimed` because they are a part of a wildcard.  This logic seems to belong in the
  [host admitter code](https://github.com/openshift/router/blob/master/pkg/router/controller/host_admitter.go#L172).

Outcome 1 is not acceptable because it would introduce breaking changes in an upgrade.  Option 2 is a possibility, and
proper documentation could also be supplied to describe the effect, as well as how to create the intended effect if the
outcome is not as intended.  Option 3 is a possibility but adds extra cycles to an already overburdened HAProxy
template processor.  Option 4 was discussed in team meetings, but it was decided that changing the host admitter should
not be a part of this feature unless HSTS would not work otherwise.

Documentation to explain the effects (especially for Option 2):

It should be noted, that in terms of the Openshift API, if a `route` with `host` of `www.foo.com` allows a
`WildcardPolicy` of `Subdomain`, then it exclusively serves all the hosts ending in `.foo.com`.  Therefore, another
`route` may NOT be admitted that ends in `.foo.com`, unless it is in the same `namespace` and doesn't try to set a
`Subdomain` wildcard policy itself.  (Exceptions can be made by setting the IngressOperator's
`routeAdmission.namespaceOwnership`.)  E.g. if you create this `route`:
```yaml
kind: Route
metadata:
  name: test1
  namespace: owner
spec:
   host: www.foo.com
   ...
   wildcardPolicy: Subdomain
```
Then if you try to create a separate `route` for `abc.foo.com` in another namespace, it will not be admitted.  It will produce
the status `HostAlreadyClaimed`:
```yaml
kind: Route
metadata:
  name: test2
  namespace: notowner   // WRONG - HostAlreadyClaimed
spec:
  host: abc.foo.com
```
However, you may create the `route` in the _same_ namespace (this is as-designed but not really as expected):
```yaml
kind: Route
metadata:
  name: test2
  namespace: owner  // OK
spec:
  host: abc.foo.com
```
Furthermore, if a `route` with `host` of `www.foo.com` has a `WildcardPolicy` of `Subdomain`, AND specifies a
HSTS Policy with `includeSubDomains`, then it places constraints on the host names of other `routes` when it comes to
HSTS Policy.  A HSTS Policy that has `includeSubDomains` for `www.foo.com` would apply to `routes` with `hosts` ending
in `www.foo.com`, such as `abc.www.foo.com`.  If you wanted to apply the HSTS Policy to `www.foo.com` and `abc.foo.com`,
you would need to create a route with a host `foo.com`, for the HSTS Policy to use `includeSubdomains`.  In this case,
you would not use `WildcardPolicy` of `Subdomain`, as `.com` would cover too many hosts.
```yaml
kind: Route
metadata:
  name: test1
  namespace: owner
annotations:
  haproxy.router.openshift.io/hsts_header: max-age=31536000;includeSubDomains
spec:
  host: foo.com
  ...
  wildcardPolicy: None
```
A more common use case is that you want a construct that will claim the subdomain `foo.com`, but also uses HSTS on the
`foo.com` subdomain.  In this case, create two `route`s:
```yaml
kind: Route
metadata:
  name: hstsRoute
  namespace: owner
annotations:
  haproxy.router.openshift.io/hsts_header: max-age=31536000;includeSubDomains
spec:
  host: foo.com
  ...
  wildcardPolicy: None
```
```yaml
kind: Route
metadata:
  name: claimSubdomainRoute
  namespace: owner
spec:
  host: www.foo.com
  ...
  wildcardPolicy: Subdomain
```
Finally, the best practice to use when there are both HTTP and HTTPS routes that share a hostname, is to make sure that
the HSTS annotation is not inadvertently applied to the HTTP route.  Do not use the `includeSubDomains` directive.

## Drawbacks
N/A
## Alternatives
An abandoned alternative was discussed in the superseded document in
enhancements/enhancements/ingress/global-options-enable-hsts.md

### Open Questions
Version Skew Strategy - not clear to me if that is required here.

### Test Plan
HAProxy and Router have unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional goals.

The operator has end-to-end tests; for this enhancement, add the following tests:

#### Enable global HSTS and validate a route
1. Create an Ingress config that enables global HSTS for a domain
2. Create a Route that annotates for HSTS in this domain and verify that it is admitted
3. Open a connection to this route using the domain and send a request
4. Verify that a response is received and that the headers include HSTS as configured in step 1

#### Enable global HSTS and invalidate a route
1. Create an Ingress config that enables global HSTS for a domain
2. Create a Route that does NOT annotate for HSTS in this domain and verify that it is NOT admitted

#### Audit HSTS configurations
1. Create an Ingress config that enables global HSTS for a domain
2. Query the API server for the new HSTS configuration and verify that it is there

### Graduation Criteria
N/A

### Upgrade / Downgrade Strategy

On upgrade, any existing per-route HSTS configuration remains in effect even for domains that are
otherwise configured for HSTS.  The recommendation will be for
administrators to take a survey of route annotations prior to applying a new HSTS Policy, then audit
and correct routes in the domain/s of the new HSTS Policy.

If a cluster administrator applied any of the new HSTS configuration options
in 4.9, then downgraded to 4.8, the HSTS configuration settings would no longer be validated
as a part of admission control. The administrator would be responsible
for removing any per-route HSTS configurations that were no longer applicable.

From a user perspective, after a downgrade an end-user's browser
may continue to access the configured routes via the previously configured HSTS Policy,
until the HSTS Policy expires.

### Version Skew Strategy
N/A
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
