---
title: externaldns-operator
authors:
  - "@danehans"
  - "@sgreene570"
reviewers:
  - "@alebedev87"
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@mcurry-rh"
  - "@Miciah"
  - "@miheer"
  - "@rfredette"
  - "@russellb"
  - "@seanmalloy"
approvers:
  - "@danehans"
  - "@knobunc"
  - "@Miciah"
creation-date: 2020-08-26
last-updated: 2021-06-25
status: implementable
---

# ExternalDNS Operator

## Table of Contents

<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
  - [Implementation Details](#implementation-details)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Open Questions](#open-questions)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Upgrade and Downgrade Strategy](#upgradedowngrade-strategy)
  - [Version Skew Strategy](#version-skew-strategy)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
<!-- /toc -->

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal is for adding an operator to manage
[ExternalDNS](https://github.com/kubernetes-sigs/external-dns). ExternalDNS has been chosen for managing external DNS
requirements of OpenShift clusters. Initially, the operator will focus on managing external DNS records for OpenShift
[Routes](https://github.com/openshift/api/blob/master/route/v1/types.go), in addition to Kubernetes Services.
Additionally, a new ExternalDNS operator CRD will be used to initially expose limited ExternalDNS configuration capabilities.

Existing operators will be used to guide the design of the ExternalDNS operator. Tooling will be
included to make it easy to build, run, test, etc. the operator for OpenShift, and for vanilla Kubernetes.

## Motivation

ExternalDNS is a controller that manages external DNS records. Our primary goal is to integrate ExternalDNS as a supported
OpenShift component. As with other platform components, ExternalDNS requires an operator to provide lifecycle management of
the component.

Secondarily, the benefits of operators in OpenShift are well established,
the [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) is increasingly accepted in the
upstream Kubernetes community, and the ExternalDNS community would also theoretically benefit from the introduction
of an ExternalDNS operator. If such an operator were compatible with vanilla Kubernetes (in addition to OpenShift),
it could be utilized by other downstream ExternalDNS users to simplify the deployment and lifecycle management of ExternalDNS.

This enhancement proposal aims to bring the benefits of operators to ExternalDNS. Ideally, the upstream ExternalDNS community
would ultimately participate in the design and construction of the ExternalDNS operator.

### Goals

* Explore the use of an operator for managing ExternalDNS.
* Create patterns, libraries, and tooling so that ExternalDNS operator is of high quality, consistent in its API
surface (common fields on relevant ExternalDNS CRDs, consistent labeling of created resources, etc.), yet is easy to build.
* Build an operator that is suitable for production use of ExternalDNS.
* Focus on supporting the OpenShift Route resource as a source for ExternalDNS.
* Additionally support the Kubernetes [service source](https://github.com/kubernetes-sigs/external-dns/blob/master/source/service.go) (namely LoadBalancer services)
as sources for ExternalDNS, since OpenShift routes are not the only way to provide Ingress for a given workload.
* Support ExternalDNS [providers](https://github.com/kubernetes-sigs/external-dns/tree/master/provider) relevant to OpenShift. This should at minimum
include include AWS, Azure, and GCP in addition to BlueCat and Infoblox.

### Non-Goals

* Create an operator that is only compatible with OpenShift.
* Replace the functionality of existing operators. __Note:__ The ExternalDNS operator intends to eventually replace external DNS
management provided by existing components, such as the [Ingress Operator](https://github.com/openshift/cluster-ingress-operator),
in the long-term.
* Support all available ExternalDNS [sources](https://github.com/kubernetes-sigs/external-dns/tree/master/source) and
[providers](https://github.com/kubernetes-sigs/external-dns/tree/master/provider) (at least, initially).
* Support the ExternalDNS [CRD source](https://github.com/kubernetes-sigs/external-dns/tree/master/docs/contributing/crd-source)
by augmenting the existing OpenShift [ingressoperator/dnsrecord](https://github.com/openshift/api/blob/master/operatoringress/v1/types.go) resource.
Support for the ExternalDNS CRD Source should be added to the ExternalDNS operator in OCP 4.10.
* Support Gateway APIs in ExternalDNS/ExternalDNS operator. See [upstream issue 2045](https://github.com/kubernetes-sigs/external-dns/issues/2045)
for more context. Support for Gateway APIs should be added after the initial "tech-preview" ExternalDNS operator is available in OCP 4.9
* Perform a gap analysis between ExternalDNS operator and the [Ingress Operator's DNS provider implementations](https://github.com/openshift/cluster-ingress-operator/tree/master/pkg/dns).
This could be performed as a part of the ExternalDNS operator work for OCP 4.10.

## Proposal

The proposal is based on experience gathered and work accomplished with existing OpenShift operators.

The following tasks would be completed as a part of this enhancement:

* Create a GitHub repository to host the operator source code in OpenShift's GitHub org,
with the hopes of moving the operator's source code into an upstream k8s-sigs repository in the future.
* Leverage popular frameworks and libraries, e.g. controller-runtime and kubebuilder, to simplify development
of the operator and align with upstream communities.
* Manage dependencies through [Go Modules](https://blog.golang.org/using-go-modules).
* Create user and developer documentation for running ExternalDNS operator (on both OpenShift and vanilla Kubernetes).
* Create tooling to simplify building, running, testing, etc. the operator.
* Create tests that reduce bugs in the code and allow for continuous integration (tests should be compatible
with OpenShift CI, in addition to vanilla Kubernetes environments running in public clouds).
* Integrate the operator with the OpenShift toolchain, e.g. [openshift/release](https://github.com/openshift/release),
which would ultimately be required for productization.
* Provide manifests for quickly deploying the operator (including a minimal ExternalDNS operator CR manifest).
* Create tooling to simplify ExternalDNS troubleshooting. For example, provide detailed information via `status.conditions` and `status.relatedObjects`
in the externalDNS `clusteroperator` resource when running on OpenShift.
* Integrate ExternalDNS with the OpenShift monitoring toolchain. For example, add
[metrics support](https://github.com/openshift/cluster-monitoring-operator) for the operator and its operand(s).


In addition, the following functionality is expected as part of the operator (note that this is not an exhaustive list):

* Introduce a CRD to be used by the operator to manage ExternalDNS and its dependencies.
* Expand upon the existing OpenShift [ingressoperator/dnsrecord](https://github.com/openshift/api/blob/master/operatoringress/v1/types.go)
CRD to implement the ExternalDNS base [CRD source](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md) (post OCP 4.9).


### ExternalDNS Operator CRD

The following example CR will be used throughout this section to explain the proposed CRD.

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalDNS
metadata:
  name: default
  namespace: my-externaldns-namespace
spec:
  domains:
    - matchType: Exact
      name: my-cluster.my-domain.com
      filterType: Include
    - matchType: Exact
      name: apps.my-cluster.my-domain.com
      filterType: Exclude
  provider:
    type: AWS
    credentials: my-secret-reference
  source:
    type: OpenShiftRoute
    hostnameAnnotationPolicy: Allow
    AnnotationFilter:
      "some.annotation.io/foo": "bar"
  zones:
    - my-public-zone-id
    - my-private-zone-id
status:
  conditions:
  - lastTransitionTime: "2020-08-20T23:01:33Z"
    reason: Valid
    status: "True"
    type: Admitted
  - lastTransitionTime: "2020-08-20T23:07:05Z"
    status: "True"
    type: Available
  - lastTransitionTime: "2020-08-20T23:07:05Z"
    message: The deployment has Available status condition set to True
    reason: DeploymentAvailable
    status: "False"
    type: DeploymentDegraded
  provider:
    type: AWS
    credentials: my-secret-reference
  zones:
    - my-public-zone-id
    - my-private-zone-id
```

Additional `spec` and/or `status` fields may be introduced based on requirements.

By exposing broad spec fields such as `zones` and `provider`, ExternalDNS operator users would have the ability to
enable ExternalDNS for multiple providers across multiple DNS zones. In general, a single instance of the ExternalDNS operator
CRD would correlate to a single [deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/) of ExternalDNS.
Note that multiple instances of ExternalDNS (operands) would be necessary when a common domain is shared across DNS zones,
or when using multiple DNS Providers. See ExternalDNS' upstream
[AWS public/private zone tutorial docs](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/public-private-route53.md)
as an example. For this example ExternalDNS CR above, it may be acceptable to run one ExternalDNS instance for a given public zone, and one ExternalDNS instance for a
given private zone, via the same deployment (See the [Open Questions](#open-questions) section).

The `zones` field contains a list of DNS zone IDs. When the `zones` field is omitted on OpenShift, the ExternalDNS operator could read
the cluster DNS config resource and extract the available public/private zone IDs (or tags, if the cluster platform is AWS).
The ExternalDNS operator could then pass the extracted public/private zone IDs/tags to the relevant ExternalDNS deployment.
See the ExternalDNS deployment example loosely based off of this ExternalDNS operator CRD instance in [Implementation Details](#implementation-details).

On OpenShift, the ExternalDNS Operator could create a `credentialsrequest.cloudcredential.openshift.io` resource so that
the cluster administrator would not have to worry about providing cloud credentials by hand when creating an ExternalDNS operator resource.
The ExternalDNS Operator would the be responsible for configuring the relevant ExternalDNS deployments to consume the secret
produced from the `credentialsrequest.cloudcredential.openshift.io` resource.

Note that ExternalDNS requires different permissions for each cloud provider, as documented in the
[tutorial section of the ExternalDNS docs](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials).
In general, ExternalDNS needs proper permissions to view, update, and delete DNS resource records in the relevant DNS zones.

Support for AWS clusters that use
[STS](https://docs.openshift.com/container-platform/4.7/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#sts-mode-installing-manual-run-installer)
for AWS credentials should also be added to the ExternalDNS operator. While ExternalDNS has some level of support for AWS STS, there is a
[known caveat upstream](https://github.com/kubernetes-sigs/external-dns/issues/1944) that may affect OCP.

In the event that invalid provider credentials are passed to ExternalDNS, a `ValidCredentials` status condition could be set to false on the
ExternalDNS operator CR. On OpenShift, this status condition could be reflected by the ExternalDNS ClusterOperator resource
(should the ExternalDNS operator be included in the Core OCP product).

Outside of OpenShift, a user could set the `providerCredentials` field to be an object reference to a resource that holds their cloud credentials,
such as a secret or configmap in an arbitrary namespace.

By default, the operator would assume the cluster's platform is the desired DNS Provider. The operator would allow a user to specify
an alternative External DNS Provider, such as BlueCat DNS. The ExternalDNS operator would be able to create multiple ExternalDNS deployments
to run in concert, one for each ExternalDNS operator Custom Resource (similar to how the OpenShift [Ingress Operator](https://github.com/openshift/cluster-ingress-operator)
consumes one or more Ingress Controller resources). The `domains` field would be used to filter source resources based on the desired hostname.
The `domains` field should be expressive enough to allow a user to specify multiple sets of domains that are considered included or excluded.
In the ExternalDNS operator CR above, ExternalDNS is configured to create DNS records with hostnames that fit under `my-cluster.my-domain.com` that are not under
`apps.my-cluster.my-domain.com`.

By default, ExternalDNS would create DNS records for all instances of the given Source resource. A union type would be defined to allow for
source-specific parameters. For the example ExternalDNS CR above, the ExternalDNS operator would configure ExternalDNS to create DNS records for Route resources that have the annotation
`some.annotation.io/foo=bar`, and would allow the ExternalDNS custom Hostname Annotation (see [Implementation Details](#implementation-details) for more details about ExternalDNS
annotations).

Note that only one Source resource can be specified for each ExternalDNS operator CR. This is to highlight the fact that a single ExternalDNS instance cannot manage more than one
source resource, which means that one ExternalDNS deployment is required for each source resource. See [Upstream Issue 1961](https://github.com/kubernetes-sigs/external-dns/issues/1961)
to better understand this design implementation of ExternalDNS.

ExternalDNS's `--annotation-filter` option filters source resource instances via annotations using label selector semantics, which is somewhat unexpected.
Note that the upstream CRD source implementation actually filters based on resource labels. If upstream ExternalDNS is open to the idea of implementing
proper label selector logic for all source types (instead of label selector based annotations), work may need to be done in the upstream ExternalDNS
community accordingly.

As a side note, ExternalDNS deployments will be configured to run ExternalDNS pods on cluster control-plane nodes, as is common practice
for add-on control-plane components.

### ExternalDNS CRD Source Support (Post OCP 4.9)

As an example, here is what a compatible DNSRecord Custom Resource instance may look like:

```yaml
apiVersion: operator.externaldns.k8s.io/v1alpha1
kind: DNSRecord
metadata:
  name: default-wildcard
  namespace: openshift-ingress-operator
spec:
  dnsName: 'foo.example.com'
  providerSpecific:
  - name: my-provider-setting-name
    value: my-provider-setting-value
  recordTTL: 30
  recordType: A
  targets:
  - 1.2.3.4
status:
  observedGeneration: 1
```

This manifest creates a DNS A record for the hostname `foo.example.com` that resolves to address `1.2.3.4` in the configured
provider, based on the resource's `dnsName` field value. Additional `spec` and/or `status` fields may be introduced based on requirements.
Note the optional `providerSpecific` field, in which name/value pairs can be added as necessary.

Note that currently, the ExternalDNS CRD source implementation only updates the `observedGeneration` status subfield.
An [issue has been opened upstream](https://github.com/kubernetes-sigs/external-dns/issues/2092) to better understand
upstream's intentions with CRD source status fields. ExternalDNS does provide adequate debug logs, should a record fail to update,
in addition to a few [high-level metrics](https://github.com/kubernetes-sigs/external-dns/blob/master/controller/controller.go).
However, status fields to reflect the availability of the requested record in the given provider would be useful for other controllers running
in a cluster, as well as for cluster administrators who want to verify that their requested resources have been created properly.

Currently, the upstream ExternalDNS CRD source only supports record creation for A or CNAME type records. See
[upstream issue 1647](https://github.com/kubernetes-sigs/external-dns/issues/1647) for more context to that end. Contributions may need to be made to
the upstream CRD source implementation to allow for other record types to be created via the DNSRecord CRD on OpenShift, such as `TXT` or `AAAA` records.

__Note:__ Any API(s) introduced by this enhancement will follow the Kubernetes API
[deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/). Example API groups are also subject to change.

### User Stories

#### Story 1

As a developer, I need the ability to manage DNS records for OpenShift routes without manually creating
and updating DNS records by hand.

#### Story 2

As a cluster administrator, I need the ability use a single operator to manage external DNS requirements for all
platform components.

#### Story 3

As a security-conscious cluster administrator, I cannot in good conscience let the Ingress Operator create wildcard DNS records
for cluster Ingress. I need to have the ability to manage fully-qualified DNS records for cluster workloads with Routes.

#### Story 4

As a cluster administrator, I want ExternalDNS to create DNS records for all of my LoadBalancer services in Route53,
in addition to creating DNS records for all of my OpenShift routes in BlueCat DNS, so that I can easily manage my multi-provider
DNS requirements.

To satisfy this, a cluster administrator could create 2 different ExternalDNS operator Custom Resources, one for each DNS provider.

#### Story 5

As a cluster administrator, I want the ExternalDNS operator to deploy one ExternalDNS instance for each of my DNS zones, with each deployment
being configured to use a different set of credentials, since I do not want to reuse common credentials for both zones due to security and auditing concerns.

### Implementation Details

When using the ExternalDNS Route Source on OpenShift, ExternalDNS will create a CNAME record for each name in the
Route status's `host` field, targeting the name in the Route status's `routerCanonicalHostname` field.
A recently merged [PR for the Ingress Operator](https://github.com/openshift/cluster-ingress-operator/pull/610)
enhanced the `routerCanonicalHostname` field in OCP 4.8 so that the field has a unique domain that falls under
the IngressController's given wildcard domain (as opposed to the IngressController's base domain, e.g., `router-default.apps.<cluster-domain>`
instead of `apps.<cluster-domain>`).

As an alternative to using CNAME records, the ExternalDNS Route source implementation could point Route hostnames directly to
LoadBalancer IP addresses or hostnames. This is not ideal since coordinating with administrator-managed load-balancers in this situation would be
difficult. Using CNAME records that point the Route's hostname to the hostname set in `routerCanonicalHostname` simplifies ExternalDNS for OpenShift Routes
by breaking out Route name resolution and LoadBalancer name resolution into separate records. This means that if a LoadBalancer service hostname or IP address
were to change, ExternalDNS would not have to update every managed Route resource's DNS records.

Note that the OpenShift [Route source implementation](https://github.com/kubernetes-sigs/external-dns/blob/master/source/ocproute.go)
in ExternalDNS supports some of the common [ExternalDNS annotations](https://github.com/kubernetes-sigs/external-dns/blob/master/source/source.go),
such as the ExternalDNS TTL, hostname, and target override annotations.

Currently, the Ingress Operator publishes wildcard DNS Records to point an Ingress Controller's domain to the Ingress Controller's
LoadBalancer type Service (or alternatively, a cluster administrator configures this for their administrator-managed Load Balancer, etc.).
In the future, the Ingress Operator could create DNSRecord Custom Resources to be managed by ExternalDNS (instead of the Ingress Operator's bespoke
DNS controller). The Ingress Controller API could be expanded to include the ability for a cluster administrator to specify whether or not a Wildcard DNSRecord
is desired for a given Ingress Controller. In other words, the introduction of ExternalDNS to OCP will ultimately remove the need for the bespoke DNS controller code
currently maintained as a part of the Ingress Operator.


Here is an example of what an ExternalDNS deployment (created by the operator) would look like on AWS:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
  namespace: openshift-external-dns
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: external-dns
  template:
    metadata:
      labels:
        app: external-dns
    spec:
      serviceAccountName: external-dns
      containers:
      - name: external-dns-private
        image: k8s.gcr.io/external-dns/external-dns:v0.8.0
        args:
        - --source=openshift-route
        - --provider=aws
        # can only have one zone at a time if domains are overlapping over zones
        - --domain-filter=my-cluster.my-domain.com
        - --exclude-domains=apps.my-cluster.my-domain.con
        - --aws-zone-type=private
        - --registry=txt
        - --txt-owner-id=externalDNS-default
        - --log-level=debug
        # Needs to be unique per container
        - --metrics-address=:7979
        - --zone-id-filter=/hostedzone/my-public-zone-id
        env:
          - name: AWS_ACCESS_KEY_ID_FILE
            value: my-access-key-file
          - name: AWS_SECRET_ACCESS_KEY_FILE
            value: my-secret-key-file
      - name: external-dns-public
        image: k8s.gcr.io/external-dns/external-dns:v0.8.0
        args:
        - --source=openshift-route
        - --provider=aws
        # can only have one zone at a time if domains are overlapping over zones
        - --domain-filter=my-cluster.my-domain.com
        - --exclude-domains=apps.my-cluster.my-domain.con
        - --aws-zone-type=public
        - --registry=txt
        - --txt-owner-id=externalDNS-default
        - --log-level=debug
        # Needs to be unique per container
        - --metrics-address=:7980
        # Filter from aws dev account
        - --zone-id-filter=/hostedzone/my-private-zone-id
        env:
          - name: AWS_ACCESS_KEY_ID_FILE
            value: my-access-key-file
          - name: AWS_SECRET_ACCESS_KEY_FILE
            value: my-secret-key-file
```

Provider credentials should be provided via a mounted secret volume, as opposed to being directly visible via a pods environment variables.

Note the use of multiple ExternalDNS containers within the same pod in the above deployment. Metrics listening ports would have
to be configured on a per-container basis to avoid collisions.

Metrics will be exposed via an insecure channel by default.
On OpenShift, ExternalDNS instances can be deployed with a reverse proxy sidecar, such as kube-rbac-proxy, to provide a secure means of metrics scraping.

### Risks and Mitigations

Cluster administrators may not want to give users the ability to create arbitrary DNS records in an external provider when they grant users permission to create routes.
ExternalDNS could observe a subfield of a route resource when determining whether or not to publish records to the upstream provider.
Cluster administrators could restrict a user's ability to set this route subfield, using a mechanism similar to that for route custom hosts.

Similarly, cluster administrators may want to restrict the ability for users to specify the ExternalDNS
[DNS target override annotation](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/openshift.md#prepare-router_canonical_hostname-in-defaultrouter-deployment).
on route resources, since the override annotation could give users the ability to create DNS records for alternative domains.

Cluster administrators could also restrict ExternalDNS source resource instances to a particular namespace, as seen in the ExternalDNS operator CR example laid out
earlier in this enhancement.

## Design Details

### Open Questions

1. Can more than one ExternalDNS instance (e.g., one instance for a given public zone and one instance for a given private zone) be run using a single deployment?
1. Similar to the above, would it be sufficient for a user to create multiple ExternalDNS operator Custom Resources should they desire separate deployments
for cross-zone ExternalDNS functionality? Note that there is an [upstream issue](https://github.com/kubernetes-sigs/external-dns/issues/1961) focused on the
addition of a CRD for configuring a single instance of ExternalDNS to serve multiple providers and zones simultaneously.
1. Will the wildcard DNS record created for the Default Ingress Controller by the Ingress Operator conflict with how end-users want to use ExternalDNS on OpenShift?
1. Will the initial ExternalDNS operator be available via OLM, or the core OCP product, in OCP 4.9? Any reason to not aim to have ExternalDNS operator
as a part of Core OCP in 4.10, with a tech/dev preview option available via OLM in 4.9? Still need to make a firm decision here.

### Test Plan

- Develop e2e, integration, and unit tests.
- Create a CI job to run tests.
- Focus on writing e2e tests to run in relevant cloud providers (on either an OpenShift or vanilla Kubernetes cluster),
which would require a new externalDNS e2e-operator OpenShift CI job.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end-to-end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The operator will follow operator best practices for supporting upgrades and downgrades.
After some time, the initial ExternalDNS operator CRD may be deprecated, especially if it is moved upstream.
The initial ExternalDNS operator CRD will be noted as alpha and subject to change.

### Version Skew Strategy

N/A

## Implementation History

See https://github.com/openshift/enhancements/pull/456.

## Drawbacks

* The amount of time required to build and maintain the operator.
* ExternalDNS offers lots of configurables via command-line options and environment variables. Supporting all of these options via a CRD may become unwieldy.
* Building an operator that is compatible with both OpenShift and vanilla Kubernetes adds complexity to the operator source code in general.


## Alternatives

* Use Ingress Operator instead of ExternalDNS to manage openshift/route DNS records.
* Productize ExternalDNS, but don't provide an operator to configure it (and instead provide thorough documentation).
* [Upstream issue 1961](https://github.com/kubernetes-sigs/external-dns/issues/1961) would greatly simplify the complexity
of the ExternalDNS operator if it were resolved, but may not be a popular pivot-point in the upstream ExternalDNS community.

## Infrastructure Needed

* A repo to host the operator source code in the OpenShift GitHub namespace.
* Test infrastructure to verify the operator works as expected when interfacing with BlueCat and Infloblox DNS.
* Note that if test infrastructure for these DNS Providers is not feasible, then manual verification could be peformed
as long as access to developer accounts for these providers is granted.

