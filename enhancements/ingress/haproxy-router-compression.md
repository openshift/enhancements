---
title: expose-compression-variables-in-haproxy
authors:
- "@candita"

reviewers:
- "@frobware"
- "@knobunc"
- "@Miciah"
- "@miheer"
- "@rfredette"
- "@deads2k"

approvers:
- "@Miciah"
- "@frobware"

creation-date: 2021-08-27

last-updated: 2021-08-27

status: implementable

see-also:

replaces:

superseded-by:

---

# Expose Compression Variables in HAProxy

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal extends the IngressController API to allow the cluster administrator
to configure HTTP traffic compression via HAProxy variables.  HTTP traffic compression increases the
performance of a website by reducing the size of data that is passed in HTTP transactions.  

## Motivation

Customers with large amounts of compressible routed traffic need the ability to gzip-compress
their ingress workloads, as described in feature request [NE-542](https://issues.redhat.com/browse/NE-542).
HAProxy in OpenShift 3.x exposed compression variables, so lack of access to the variables in OpenShift 4.x
is a migration blocker for customers that were using them in OpenShift 3.x.

The ability to configure compression was available in 3.x, but it applied only to the HAProxy
`defaults` configuration section, and supported only the `gzip` compression algorithm.  This proposal
reintroduces and then enhances the previous implementation in these ways:
- allow the `frontend`, and `backend` sections in addition to the `defaults` section, which would provide for more
  fine-grain configuration at the route level.
- allow a choice of `gzip` or `raw-deflate` compression algorithms.

### Goals

Enable the configuration of HTTP compression global defaults via the `IngressControllerSpec`.

Enable the configuration of HTTP compression for route backends/frontends via route annotations.

Enable HTTP compression on selected MIME types, be they wildcard (e.g. text/*, application/*, etc.) or
specific (e.g. text/html, text/plain, text/css, application/json, image/svg+xml, font/eot, etc.).  Not all MIME
types benefit from compression, but there is a short list found [here](https://letstalkaboutwebperf.com/en/gzip-brotli-server-config/)
that will be implemented as a part of this proposal.

By exposing ROUTER_ENABLE_COMPRESSION, any HTTP request with the header "Accept-Encoding: gzip" or
"Accept-Encoding: raw-deflate" with a MIME type that matches one of the configured ROUTER_COMPRESSION_MIME types
will have HTTP compression applied to its payload.

### Non-Goals

HAProxy provides for the choice of compression algorithm (e.g. `identity`, `gzip`, `deflate`,
and `raw-deflate`) but this proposal selectively covers only the `gzip` and `raw-deflate` algorithms.
The `identity` algorithm is for debugging and doesn't apply any change on data.  The `deflate` algorithm
is a variation on `gzip` that is not supported on all browsers and lacks universal value.

HAProxy allows compression to be applied to the `listen` section as well as the `defaults`, `backend`, and `frontend`
sections of the configuration file.  A `listen` section is used to define a combined `frontend` and `backend`.
Since the current OpenShift HAProxy template only has a `listen` definition for the `StatsPort`, and
doesn't allow user configuration of a `listen` section, it is not considered in this proposal.

HAProxy provides a `compression offload` configuration option that makes HAProxy remove the "Accept-Encoding"
header to prevent backend servers from compressing responses.  This forces all response compression to happen within
HAProxy, jeopardizing its performance.  This configuration option is not recommended, even by HAProxy guides, so it
is omitted from this proposal.

A performance tuning variable, `maxcomprate`, sets the maximum per-process input compression rate to the specified
number of kilobytes per second.  It works in conjunction with `tune.comp.maxlevel` to rate-limit the compression
level to save data bandwidth. Another, `maxcompcpuusage`, does the same to save limit CPU used on compression.
Neither variable is currently exposed, and both are omitted from this proposal.  Other per-session performance tuning
variables like `maxconnrate`, which set the maximum per-process number of connections per second, and `maxsslrate`,
which sets the maximum per-process number of SSL sessions per second, are not exposed either.

## Proposal

Currently, the haproxy.cfg contains the lines below in the `defaults` section, but the mechanism for setting these
environment variables is not exposed.
```go
defaults
...
{{- if isTrue (env "ROUTER_ENABLE_COMPRESSION") }}
  compression algo gzip
  compression type {{ env "ROUTER_COMPRESSION_MIME" "text/html text/plain text/css" }}
  {{- end }}
```

### Global HTTP Compression via IngressControllerSpec and environment variables

To expose the two compression directives in the `defaults` section, the ROUTER_ENABLE_COMPRESSION environment variable
will be replaced with ROUTER_COMPRESSION_ALGORITHM, and these changes will be made:
```go
{{- /* compressionAlgoPattern matches valid options for HTTP compression */}}
{{- $compressionAlgoPattern := "gzip|raw-deflate" -}}

{{- /* compressionMimeTypes stores valid options for HTTP compression MIME Types */}}
{{- $compressionMimeTypes := "text/html text/css text/plain text/xml text/x-component text/javascript \
application/x-javascript application/javascript application/json application/manifest+json application/vnd.api+json \
application/xml application/xhtml+xml application/rss+xml application/atom+xml application/vnd.ms-fontobject \
application/x-font-ttf application/x-font-opentype application/x-font-truetype \
image/svg+xml image/x-icon image/vnd.microsoft.icon font/ttf font/eot font/otf font/opentype" -}}
...
defaults
...
{{- with  $compressionAlg := firstMatch $compressionAlgoPattern {{ env "ROUTER_COMPRESSION_ALGORITHM"}} -}}
compression algo {{ $compressionAlg }}
{{- with $compressionMimeTypes := allMatches $compressionMimeTypes {{ env "ROUTER_COMPRESSION_MIME"}} -}}
compression type {{ $compressMimeTypes }}
{{- end }}
{{- end -}}
```
The new function `allMatches` will be added to the `template_helper.go` code:
```go
// Compare two white-space delimited strings and return the latter if all its substrings
// exist in former
func allMatches(defined string, supplied string) string {
	log.V(7).Info("allMatches called", "supplied", supplied, "defined", defined)
	numFound := 0
	suppliedRange := strings.Fields(supplied)
	definedRange := strings.Fields(defined)
	for _, s := range suppliedRange {
		for _, d := range definedRange {
            if d == s { // optimize this
                log.V(7).Info("allMatches validated", "supplied", s)
                numFound++
                break
            }
        }
	}
	if numFound == len(suppliedRange){
        return supplied
    }
	return ""
}
```

The proposed change to IngressControllerSpec exposes the HTTP compression configuration variables at the default level:
```go
type IngressControllerSpec struct {
...
    // httpCompression defines policy for HTTP traffic compression
    // 
    // The default value is no compression
    //
    // +optional
    HTTPCompression *HTTPCompressionPolicy `json:"httpCompression,omitempty"`
}
...
// httpCompressionPolicy defines the compression algorithm and MIME types that need to
// have compression applied.
//
// This field is optional, and its absence implies that compression should not be enabled
// globally in HAProxy.
//
// If httpCompressionPolicy exists, compression should be enabled only for the listed
// MIME types, and only using the designated compression algorithm.
type HTTPCompressionPolicy struct {

	// compressionAlgorithm defines the desired compression algorithm.
	//
	// +required
	CompressionAlgorithm CompressionAlgorithmType `json:"compressionAlgorithm,omitempty"`
	
	// compressionMIMETypes is a list of MIME types that should have compression applied
	//
	// +kubebuilder:MinItems=1
	// +required
	CompressionMIMETypes []string `json:"compressionMIMETypes,omitempty"`
}

// CompressionAlgorithmType defines the type of compression algorithm
// +kubebuilder:validation:Enum=gzip;raw-deflate
type CompressionAlgorithmType string
const (
	gzipCompressionAlgorithm CompressionAlgorithmType = "gzip"
	rawdeflateCompressionAlgorithm CompressionAlgorithmType = "raw-deflate"
)
```

To set up the IngressControllerSpec to use HTTP compression on all routes, the configuration
might resemble:
```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpCompression:
    compressionAlgorithm: gzip
    compressionMIMETypes:
    - "text/html"
    - "text/css"
    - "application/pdf"
```

### Route-based HTTP Compression via Annotations
To expose these variables at the route level, the mechanism will be two route annotations that are processed in the
haproxy.cfg `frontend` and `backend` sections:
```go
{{- /* Route-Specific Annotations */ -}}
...
{{- /* compressionAlgorithmHeader configures which compression algorithm is applied to payload */ -}}
{{- $compressionAlgorithmHeader := "haproxy.router.openshift.io/compressionAlgorithm" -}}

{{- /* compressionMimeTypesHeader configures which MIME types have compression applied to payload */ -}}
{{- $compressionMimeTypesHeader := "haproxy.router.openshift.io/compressionMimeTypes" -}}


...
backend ... /* or frontend */
...
{{- with $compressionAlg := firstMatch $compressionAlgoPattern (index $cfg.Annotations $compressionAlgorithmHeader)}}
  compression algo {{ $compressionAlg }}
{{- with $compressionTypes := allMatches $compressionMimeTypes (index $cfg.Annotations $compressionMimeTypesHeader)}}
  compression type {{ $compressionTypes }}
{{- end -}}
{{-end -}}
```

To set up a route to use HTTP compression, the configuration might resemble:
````yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/compressionAlgorithm: "gzip"
    haproxy.router.openshift.io/compressionMimeTypes: "text/html text/css application/pdf"
````

### User Stories

#### As a cluster administrator, I need to enable gzip HTTP compression globally for a selection of MIME types

Select the necessary MIME types and edit the IngressController spec as shown:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpCompression:
    compressionAlgorithm: gzip
    compressionMIMETypes:
    - "text/html"
    - ...
```
#### As a cluster administrator, I need to enable gzip HTTP compression for only one route and one MIME type, "text/html"

To apply HTTP compression to all routes in the cluster, use a command like this. (This command requires
an updated `oc` client that supports the `--all-namespaces` flag on the `oc annotate` verb):
```shell
$ oc annotate route --all --all-namespaces --overwrite=true "haproxy.router.openshift.io/compressionAlgorithm"="gzip"
$ oc annotate route --all --all-namespaces --overwrite=true "haproxy.router.openshift.io/compressionMIMETypes"="text/html text/css text/xml"

```
To apply router compression to all routes in a particular namespace, use commands like these:
```shell
$ oc annotate route --all -n my-namespace --overwrite=true "haproxy.router.openshift.io/compressionAlgorithm"="gzip"
$ oc annotate route --all -n -my-namespace --overwrite=true "haproxy.router.openshift.io/compressionMIMETypes"="text/html text/css text/xml"
```

#### As a cluster administrator, I need to verify all routes have HTTP compression enabled for a specific MIME type
```shell
$ oc -n openshift-ingress-operator get ingresscontrollers/default -o jsonpath=`{spec.httpCompression}`
```

#### As a cluster administrator, I need to verify which routes have HTTP compression enabled, and for which MIME types

To review the HTTP compression annotations on all routes:
```shell
$ oc get route  --all-namespaces -o go-template='{{range .items}}{{if .metadata.annotations}}{{$a := index .metadata.annotations "haproxy.router.openshift.io/compressionMIMETypes"}}{{$n := .metadata.name}}{{with $a}}Name: {{$n}} MIME Types: {{$a}}{{"\n"}}{{else}}{{""}}{{end}}{{end}}{{end}}'

Name: myBigRoute MIME Types: "text/html"
```

Tips for troubleshooting:
- check response headers for evidence of compression: "Content-Encoding" header with `gzip` or `raw-deflate` value
- the content of the response should be smaller than what was sent

### Implementation Details/Notes/Constraints

Compression is not always applied even if it is requested.  If a backend server supports HTTP compression, the
compression directives will become no-op: HAProxy will see the compressed response and will not compress again. If a
backend server does not support HTTP compression, and the header "Accept-Encoding" exists in the request, HAProxy will
compress the matching response.  Also, if the HAProxy doesn't have enough resources to compress, it may skip the
compression in order to stay functional.  Process-level directives exist to tune this behavior: `maxcomprate`,
`maxcompcpuusage`, and `tune.comp.maxlevel` as described in the
[HAProxy compression documentation](https://www.haproxy.com/documentation/hapee/latest/load-balancing/compression/),
but these are not in scope of this proposal.

### Risks and Mitigations

Overuse of compression can cause the HAProxy to devote more resources to compression than to other operations,
compromising its usefulness.  I have mentioned the process-level directives that would tune the dedication of
resources to compression, but for now have decided that adding more tunable knobs at this stage is premature and
could cause more issues than it mitigates.

I am considering disallowing wildcard MIME types (e.g. "text/*"), so that explicit choices must be made on which
MIME types are subject to compression.

Security and UX are not a part of this proposal, but API team members will be invited to comment.

## Design Details
Design details are covered in the Proposal section, but some additional detailed notes from the
[HAProxy compression documentation](https://www.haproxy.com/documentation/hapee/latest/load-balancing/compression/)
are listed below.

### Compression algorithms supported by HAProxy
- deflate: applies deflate compression with zlib format. Not recommended in combination with raw-deflate
- gzip: applies gzip compression
- identity: does not alter content at all; used for debugging purposes only
- raw-deflate: applies deflate compression without zlib wrapper. May be used as an alternative to deflate

### Compression types
MIME types to apply compression to, read from the Content-Type response header field. If not set, HAProxy
Enterprise tries to compress all responses that are not compressed by the server.

### Process-level directives
**maxcomprate**: sets the maximum per-process input compression rate to <number> kilobytes per second.
For each session, if the maximum is reached, the compression level will be decreased during the session.
If the maximum is reached at the beginning of a session, the session will not compress at all. If the maximum
is not reached, the compression level will be increased up to tune.comp.maxlevel. A value of zero means there
is no limit, this is the default value.

**maxcompcpuusage**: sets the maximum CPU usage HAProxy Enterprise can reach before stopping the compression for
new requests or decreasing the compression level of current requests. It works like maxcomprate but measures CPU
usage instead of incoming data bandwidth. The value is expressed in percent of the CPU used by HAProxy Enterprise.
In case of multiple processes (nbproc > 1), each process manages its individual usage. A value of 100 (the default)
disables the limit. Setting a lower value will prevent the compression work from slowing the whole
process down and from introducing high latencies.

**tune.comp.maxlevel**: sets the maximum compression level. The compression level affects CPU usage during compression.
Each session using compression initializes the compression algorithm with this value. The default is 1.

### Open Questions
### Test Plan
- Unit tests will validate that HTTP compression directives appear in the `frontend`/`backend` sections of the HAProxy
  template when it is properly configured in a route.
- Unit tests will validate that the HTTP compression directive appears in the `defaults` section of the HAProxy template
  when it is properly configured on the IngressController spec.
- E2e tests will validate that the IngressController spec accepts and enables compression in the `defaults` section of
  the generated haproxy.cfg

Note that HAProxy Enterprise **disables** HTTP compression when:
- The request does not advertise a supported compression algorithm in the "Accept-Encoding" header
- The response message is not HTTP/1.1 or above
- The HTTP status code is not one of 200, 201, 202 or 203
- The response contains neither a "Content-Length" header, nor a "Transfer-Encoding" header whose last value is `chunked`
- The response contains a "Content-Type" header whose first value starts with `multipart`
- The response contains a "Cache-Control" header with a value of `no-transform`
- The response contains a "Content-Encoding" header, indicating that the response is already compressed (see `compression offload`)
- The response contains an invalid "ETag" header or multiple "Etag" headers

### Graduation Criteria

This feature will be immediately available in release 4.10, and will not have a dev or tech preview.

#### Dev Preview -> Tech Preview
n/a

#### Tech Preview -> GA
n/a

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

An upgrade from a previous installation that had the compression directives exposed, will need to change to use the
IngressController spec.  No previous installation would have any compression directives via route annotations.

If a cluster administrator applied any of the compression configuration options
in 4.10, then downgraded to 4.9, the compression configuration settings would no longer be in effect unless they
used a custom unmanaged HAProxy configuration.

### Version Skew Strategy
n/a

## Implementation History
n/a

## Drawbacks

By implementing this proposal we are giving the user tools that could threaten the operation of HAProxy.
Improper configuration of the MIME types or general over-use of compression could allow the HAProxy to
dedicate more processing time to compression than to its main duties of routing and traffic management.

## Alternatives
n/a
