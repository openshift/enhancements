---
title: customize-error-code-pages
authors:
  - "@miheer"
reviewers:
  - "@Miciah"
  - "@danehans"
  - "@knobunc"
approvers:
  - "@Miciah"
  - "@danehans"
  - "@knobunc"
creation-date: 2021-03-07 
last-updated: 2020-03-07
status: implementable 
see-also:
replaces:
superseded-by:
---
# Customize error code paged returned by the haproxy router
## Release Signoff Checklist
- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)
## Summary
There is no supported method to customize an IngressController's error pages in OCP 4.
Users may want to customize an IngressController's error pages for branding or other reasons.
For example, users may want a custom HTTP 503 error page to be returned if no pod is available.
When the requested URI does not exist, users may want an IngressController to return a custom 404 page.
## Motivation
The primary motivation is that users want to customize the error pages that are returned, for example for branding purposes.
Secondary motivation is that users say 503 page is served even when the page is not found when it shall serve 404.
The reason that happens is because we provide only 503 in our haproxy template.

### Goals
- Enable the cluster administrator to specify custom error code pages for haproxy router
- Return a distinct HTTP 404 error page when the requested URI does not exist, instead of returning the HTTP 503 error page.

### Non-Goal
- Custom error pages for HTTP responses other than 503 and 404.
- Enabling users to configure custom error pages for individual routes.

## Proposal
0. User creates a custom error code page configmap `my-custom-error-code-pages` in `openshift-config`
1. The IngressController API is extended by adding an optional
`HttpErrorCodePages` field with type string
```go
	// httpErrorCodePages specifies a configmap with custom error pages.
	// The administrator must create this configmap in the openshift-config namespace.
	// This configmap should have keys in the format "error-page-<error code>.http",
	// where <error code> is an HTTP error code.
	// For example, "error-page-503.http" defines an error page for HTTP 503 responses.
	// Currently only error pages for 503 and 404 responses can be customized.
	// Each value in the configmap should be the full response, including HTTP headers.
	// If this field is empty, the ingress controller uses the default error pages.
	HttpErrorCodePages configv1.ConfigMapNameReference `json:"httpErrorCodePages,omitempty"`
```
By default, an IngressController uses error pages built into the IngressController image.
The `HttpErrorCodePages` field enables the cluster administrator to
specify custom error code pages.
```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  httpErrorCodePage: "my-custom-error-code-pages" 
```
2. The configmap keys shall have the format `error-page-`<error code>`.http`. Right now only error-page-503.http and
   error-page-404.http will be available.
```yaml
$ oc -n openshift-config create configmap  my-custom-error-code-pages \
--from-file=error-page-503.http \
--from-file=error-page-404.http

$ oc -n openshift-config get configmaps my-custom-error-code-pages -o yaml
apiVersion: v1
data:
  error-page-404.http: "HTTP/1.0 404 Not Found\r\nConnection: close\r\nContent-Type: text/html\r\n\r\n<html>\r\n<head><title>Not Found</title></head>\r\n<body>\r\n<p>The requested document was not found.</p>\r\n</body>\r\n</html>\r\n"
  error-page-503.http: "HTTP/1.0 503 Service Unavailable\r\nConnection: close\r\nContent-Type: text/html\r\n\r\n<html>\r\n<head><title>Application Unavailable</title></head>\r\n<body>\r\n<p>The requested application is not available.</p>\r\n</body>\r\n</html>\r\n"
kind: ConfigMap
metadata:
  creationTimestamp: "2021-03-30T01:25:24Z"
  managedFields:
  - apiVersion: v1
    fieldsType: FieldsV1
    fieldsV1:
      f:data:
        .: {}
        f:error-page-404.http: {}
        f:error-page-503.http: {}
    manager: kubectl-create
    operation: Update
    time: "2021-03-30T01:25:24Z"
  name: my-custom-error-code-pages
  namespace: openshift-config
  resourceVersion: "1564207"
  selfLink: /api/v1/namespaces/openshift-config/configmaps/my-custom-error-code-pages
  uid: 49009c69-7b3e-443d-983b-f0a4219c445d

```  
3. In response to the user configuration, the IngressController will configure `errorfile` stanzas as needed in the IngressController's `haproxy.config` file
   as per https://www.haproxy.com/blog/serve-dynamic-custom-error-pages-with-haproxy/
4. A default error page for HTTP 404 responses will be added in openshift/router.
5. A  new controller to sync the cluster admin created configmap having custom error pages from `openshift-config`
   to `openshift-ingress`.
6. The ingress operator will configure the IngressController deployment to mount the configmap from the `openshift-ingress` namespace and use the error pages defined therein.

### Validation
1. Omitting `spec.httpErrorCodePages`  specifies the default behavior i.e serves default error code pages.
the keys of the configmap then it will be ignored and default page will be served.
2. If the user specifies an invalid value, then the IngressController ignores the provided configmap.
3. If the user provides an unrecognized key in the configmap (i.e., one that does not match the `error-page-<error code>.http` format), then the configmap will
   be ignored, and the default error page will be served.
4. It is up to the user to ensure that the provided data is a valid HTTP response.

### User Stories

#### As a cluster administrator, I expect to have custom error code pages getting served
1. The administrator creates a configmap named "my-custom-error-code-pages" in the `openshift-config` namespace.
2. The administrator patches the ingresscontroller to reference the "my-custom-error-code-pages" configmap by name.
3. The ingress operator copies the "my-custom-error-code-pages" configmap from the `openshift-config` namespace to the
   `openshift-ingress` namespace.
4. The ingress operator configures the router deployment with a volume and volume mount that mounts
   the custom error pages from the http503page configmap on top of the existing error pages in the
   router pods.

### Risks and Mitigations
1. If a user provides a key that is not of the format `error-page-<error code>.http` in
   the configmap, then the configmap will be ignored, and default error pages will be served.
2. It is up to the user to ensure that the provided data is a valid HTTP response.

## Design Details

### Test Plan
Unit test cases will be added for the new controller which sync the configmap from `openshift-config`
to `openshift-ingress`.
Unit test cases will be added for the ingress controller where the router deployment get a volume mount
of the custom error code page

### Graduation Criteria
N/A.

### Upgrade / Downgrade Strategy
On upgrade, the default configuration does not perform error code page customization.  On
downgrade, the operator ignores the `spec.httpErrorCodePages` API field.

### Version Skew Strategy
N/A.

## Implementation History
Following are the most salient PRs in the feature's history:
1. https://github.com/openshift/api/pull/843  - Adds a field called `httpErrorCodePages` to the ingress controller API to get the custom error page configmap created
   in the `openshift-config` namespace which is then synced to `openshift-ingress` namespace
2. https://github.com/openshift/cluster-ingress-operator/pull/571
   a. Controller to sync configmaps from the `openshift-config` namespace to the `openshift-ingress` namespace ([NE-535](https://issues.redhat.com/browse/NE-535)).
   b. Mount router deployment with the configmap ([NE-543](https://issues.redhat.com/browse/NE-543)).
3. https://github.com/openshift/router/pull/274 Add errorfile stanzas and dummy default html files to the router.
## Alternatives
N/A.
