---
title: byo-custom-dns
authors:
  - "@sadasu"
reviewers: 
  - "@patrickdillon, for Installer aspects"
  - "@Miciah, for Ingress Controller aspects"
  - "@cybertron, for in-cluster DNS aspects"
  - "@2uasimojo, for hive expertise"
approvers:
  - "@sdodson"
  - "@patrickdillon"
  - "@zaneb"
  - "@Miciah"
  - "@cybertron"

api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@joelspeed"
creation-date: 2023-04-27
last-updated: 2023-04-27
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORS-1874
replaces:
  - "https://github.com/openshift/enhancements/pull/1276"
---

# BYO Custom DNS

## Summary

This feature provides OpenShift customers running on AWS an option to use a
custom DNS solution that is different from the default cloud DNS. OpenShift does
not have access to this custom DNS solution and hence is not expected to
configure it.

## Motivation

Some customers do not want to use the DNS solution provided by the underlying
cloud due to regulatory [ITAR](https://www.gov-relations.com/) or operational
contraints. It is important for OpenShift to provide a way for these customers
to use their own preferred DNS solution while supporting their IPI deployments.

This external custom DNS solution would be responsible for DNS records for
`api.<cluster domain>`, `api-int.<cluster domain>` and
`*.apps.<cluster domain>`. The Cloud Load Balancers (LBs) are still the
preferred way for providing LB services for `api`, `api-int` and `ingress` in
public and private hosted zones.

In addition, in OpenShift's Managed offering, the external DNS solution is also
responsible for DNS records for required OpenShift resources including quay.io,
registry.redhat.io, api.openshift.com, cloud management endpoints, etc. This
enhancement proposal will not talk about how these entries are configured for
the OpenShift Managed offering. 

### User Stories

"As a cloud admin, I want to run an OpenShift cluster on AWS with my custom DNS
solution in place of the default cloud DNS."

TODO: Is there anything else to add here?

### Goals

- Enable OpenShift customers to use their custom DNS solution instead of the
default cloud solution.
- Start with AWS and lay the groundwork to add this capability for other cloud
platforms like Azure and GCP.
- Continue using the default cloud LB service for API, API-Int and Ingress.

### Non-Goals

We will not use the [Kubernetes External DNS provider](https://github.com/openshift/external-dns)
and/or the [OpenShift External DNS Operator](https://github.com/openshift/external-dns-operator)
to program dns entries across varied DNS providers. The External DNS Operator
relies on being able to program the customer's DNS solution. The customer DNS
solutions in consideration here do not allow OpenShift to configure them.

## Proposal

For the OpenShift installation to make use of a custom DNS solution that is not
configurable by OpenShift, the customer is responsible for configuring the DNS
entries for the API, API-Int and Ingress services.

Currently, during cluster installation, OpenShift is repsonsible for adding
entries for API, API-Int and *.apps into the cloud DNS. These URLs are
configured with the LB DNS Names of the LBs associated with each of these
services. Hence the LBs have to be created before they can used to configure
the DNS solution. Also, important to note is that these LB DNS Names cannot
be generated before they are created.

When it is indicated to the OpenShift installer via an input parameter that the
custom DNS solution is in use, OpenShift does not configure the cloud DNS
solution [Route53 for AWS](https://aws.amazon.com/route53/) for any of its
services. In the case of API and API-Int services, the customer is responsible
for creating the external and internal LBs [Elastic Load Balancer](https://aws.amazon.com/elasticloadbalancing/)
prior to cluster installation. Their custom DNS solution also needs to be
configured with entries for API and API-Int before cluster installation. OpenShift
will continue to configure and manage these cloud LBs created by the user.

DNS configuration for Ingress services will be handled differently. The Ingress
Operator utilizes the upstream k8s LoadBalancer service to create and configure
the LBs for the *.apps service. In this case, the customer pre-creating the LB
would mean the LB service would have to be modified to consume a pre-created LB.
Accomplishing this would require a change to the upstream LB service potentially
adding delays towards the completion of this feature. Since the LB DNS Name for
the *.apps would not be available before cluster installation, the DNS solution
cannot be pre-configured before cluster installation either.

To work around this limitation, the Ingress Operator would continue using the
LoadBalancer service to create and configure the LBs for the *.apps service.
OpenShift will start an in-cluster DNS pod to provide DNS resolution for *.apps
during cluster installation. After the cluster is installed, the user will have
to update their custom DNS solution with an entry for the *.apps service.


### Workflow Description

1. Customer creates an external cloud LB for the API service and an internal
cloud LB for the API-Int service. Since these LBs need to be created on subnets,
these subnets also need to be created before cluster installation. As a result,
the installer created subnets can not longer be supported with an external DNS.
These subnets are created within VPCs and so the customer has to create the
VPCs too before creating the subnets. In the Managed Services (MS) environment,
 the internal(?) LBs for rh-api also need to be pre-created by the customer.

2. The customer then uses the LB DNS names generated after their creation to
configure their custom DNS solution with entries for API, API-Int and if
applicable rh-api (for MS).

3. The customer then updates the install-config to turn on the custom-dns
feature. They also provide the LB Names and types for the pre-created API and
API-Int LBs. The Installer would use these LB Names to configure them.

4. The Installer skips configuring the cloud DNS solution when this feature is
enabled via install-config. Since API and API-Int resolution are pre-requisties
for successful cluster installation, the Installer performs additional
validations to make sure they are configured correctly. The installer provides
a warning but does not stop installation when API or API-Int resolution fails.

5. The Installer creates the [DNS CR](https://github.com/openshift/api/blob/master/config/v1/0000_10_config-operator_01_dns.crd.yaml) without providing values for privateZone
and publicZone. This indicates to the Ingress Operator to skip configuring the
cloud DNS with an entry for *.apps.

6. The Installer also sets a field in the Infrastructure CR indicating that
the custom DNS feature is enabled. This is a way to embed the knowledge of
this configuration in a running cluster so that this information is available
during an upgrade too.

7. The Ingress Operator creates and manages the LB for *.apps by starting a k8s
Service of type LoadBalancer (existing functionality). Updating this Service to
accept already created LBs is an upstream change that we are not pursuing at
this time.

8. When the LoadBalancer Service successfully creates the Ingress LB in the
underlying cloud, it updates the `Hostname` (in AWS) or `IP` (Azure and GCP)
fields within the [LoadBalancerIngress struct](https://github.com/kubernetes/api/blob/cdff1d4efea5d7ddc52c4085f82748c5f3e5cc8e/core/v1/types.go#L4554). These fields represent the hostname
or the IP address of the Ingress LB.

9. OpenShift needs to provide an in-cluster DNS solution for *.apps resolution
because several operators depend on this service being available during cluster
installation. To achieve this, MCO will start a static CoreDNS pod to provide
DNS resolution for *.apps only.

10. Configuration for the CoreDNS pod is provided by its CoreFile that contains
the IP address for the Ingress LB. This IP address is obtained by resolving the
`Hostname` field within the LB service status.

11. runtimecfg monitors the LB service status for changes to the LB IP address
(by resolving the `Hostname`). If there is a change, it will update the
Corefile with the new IP address of the Ingress LB.

12. Once the cluster installation is complete, the customer can add a DNS entry
in their custom DNS solution for *.apps.

13. During cluster deletion, OpenShift is responsible only for deleting the
Ingress LB and the customer is responsible for deleting the LBs for API and
API-Int. The customer would also be responsible for removing entries from
their custom DNS.

#### Variation [optional]

The variation explored in this section has to do with finding ways to allow the
user to pre-create the Ingress LoadBalancer and pre-configure their custom DNS
solution with the *.apps entry. As detailed below, it is possible to use a
pre-created LB for AWS but it has to be IPs (elastic, floating or static IPs)
for Azure and GCP.

A little background is necessary to understand the options discussed below.
Currently, the cluster-ingress-operator is responsible for starting the LB
service for Ingress. This LB service uses the cloud provider to create an
Ingress LB by optionally taking the [`LoadBalancerIP`](https://github.com/kubernetes/api/blob/cdff1d4efea5d7ddc52c4085f82748c5f3e5cc8e/core/v1/types.go#L4721) as input.

Not all cloud providers support this feature which means that this input and
the `Deprecated` comment has given us a reason to pause and investigate the
annotations that are available per cloud platform and what behavior can be
expected when they are used. AWS, Azure and GCP cloud providers have
responded to the deprecated `LoadBalancerIP` and need for customers to bring
their own LBs in different ways.

Once the Ingress LB is created by the service, its `IP` and/or `Hostname` are
available in the [`LoadBalancerIngress`](https://github.com/kubernetes/api/blob/cdff1d4efea5d7ddc52c4085f82748c5f3e5cc8e/core/v1/types.go#L4554) field of the `Status` of the LB
service. The `IP` or the `Hostname` can be used to add the DNS entry for
*.apps.

For the user's custom DNS solution to be configured with the *.apps entry before
cluster installation starts, information about the Ingress LB needs to be
available before the cluster installation starts. So, is it possible for the
user to pre-create the Ingress LB just like they would the API and API-Int LBs
and associate this Ingress LB to the LB service started during cluster install?

The following section tries to answer this question per platform.


## AWS

AWS does not support the use of the `LoadBalancerIP` so we will have to rely on
platform specific annotations for an alternative.

1. [`service.beta.kubernetes.io/aws-load-balancer-name`](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/annotations/#load-balancer-name)

This allows us to pre-create an Ingress LB and provide its name as a value for
this annotation. The user can pre-create the Ingress Lb, provide its name via
the install-config. Installer can add this value to the [`Ingress Controller
CRD`](https://github.com/openshift/api/blob/master/operator/v1/0000_50_ingress-operator_00-ingresscontroller.crd.yaml). The cluster-ingress-operator can then add this value to
the annotation before the LB service is started.

Also, this annotation is supported by `aws-load-balancer-controller` but not by
[`cloud-provider-aws`](https://github.com/openshift/cloud-provider-aws/blob/master/pkg/providers/v1/aws.go).

Please note that aws-load-balancer-controller can be installed by installing
aws-load-balancer-operator, but this is an addon operator. Although the
documentation does not specifically mention it, inspecting the [`implementation']
(https://github.com/rifelpet/aws-alb-ingress-controller/blob/a30c980164db2d9f54596ab971be8d98453c679d/pkg/annotations/constants.go#L52) , it appears that this annotation can be used
only with NLBs.

2. [`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/guide/service/annotations/#eip-allocations)

This annotation allows us to provide a list of static IP addresses for an
external facing LB. This annotation provides the same functionality as
`LoadBalancerIP` (that AWS does not support) as an annotation. Can only be used
with NLBs. Also to keep in mind that [`Elastic IP addresses`](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html) have a cost associated with them
in addition to the LB cost.

The static IP per subnet can be used to configure the custom DNS solution
before installation. It also needs to be provided to OpenShift via the
install-config. The Installer will make it available to the ingress operator
by adding a new field called `eipAllocations` to the Ingress Controller CRD's
[`IngressController.spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancer`](https://github.com/openshift/api/blob/08ec5d25d1b2cf5343f6507813c5b37a21fd126b/operator/v1/types_ingress.go#L631).
More appropiately, the `eipAllocations` could be added to the `AWSIngressSpec` of [`IngressPlatformSpec`](https://github.com/openshift/api/blob/08ec5d25d1b2cf5343f6507813c5b37a21fd126b/config/v1/types_ingress.go#L102)

## Azure

1. [`LoadBalancerIP`](https://github.com/kubernetes/api/blob/cdff1d4efea5d7ddc52c4085f82748c5f3e5cc8e/core/v1/types.go#L4721)

Providing this input would mean that the Ingress LB would be created by the
service. But the DNS entry can be configured before cluster installation to
point to this IP.

Since cloud-provider-azure supports the `LoadBalancerIP` feature, ARO was able
utilize this feature for their Ingress LB pre-creation. Again, the concern here
is the deprecation comment in the documentation.

2. [`service.beta.kubernetes.io/azure-load-balancer-ipv4` and `service.beta.kubernetes.io/azure-load-balancer-ipv6`](https://github.com/kubernetes-sigs/cloud-provider-azure/blob/0cfeb043809bdf796233f47dbffb8608d3a716e4/pkg/consts/consts.go#L207)

Setting this annotation means that the Azure LB would not have to be created
by the user before cluster installation. The custom DNS solution can be
configured before cluster installtion such that *.apps resolves to these
IPv4 (and IPv6 for dual stack) addresses.

## GCP

1. [`LoadBalancerIP`](https://cloud.google.com/kubernetes-engine/docs/concepts/service-load-balancer-parameters#spd-static-ip)

Cloud Provider GCP initially started out with a [`plan`](https://github.com/kubernetes/cloud-provider-gcp/issues/371) to add their own annotation. Instead they pivoted to continue using `spec.loadBalancerIP` and modify [`user documentation`](https://github.com/kubernetes/kubernetes/pull/117051) instead.

2. [`kubernetes.io/ingress.global-static-ip-name`](https://cloud.google.com/kubernetes-engine/docs/concepts/ingress-xlb#static_ip_addresses_for_https_load_balancers)

This annotation was added to [`Ingress-gce`](https://github.com/kubernetes/ingress-gce).
It seems similar to the 2nd option discussed for AWS and Azure where the a
pre-determined static IP is set aside for the external Ingress LB.

Given the different approaches taken by different platforms, it seems that if
we wanted a semantically consistent way to handle to the pre-configuration of
DNS for *.apps we use the platform specific annotation that allows a 
pre-defined static IP to be specified for the Ingress LB.

## API and API-Int

This approach can be extended to the API and API-Int LoadBalancers too. The
user can just create static IP addresses for the internal and external LBs.
The user would not have to pre-create the LBs themselves, just the static IPs
that can be associated with them when they are created. The user also needs to
configure their custom DNS solution with A records for `api`, `api-int` and
`*.apps` before starting installation.

The Installer uses the static IPs passed in via install-config while creating
the LBs for API and API-Int for the [`AWS`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb.html#specifying-elastic-ips), [`Azure`](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/lb_rule.html) and [`GCP`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_address.html) platforms.

### API Extensions

1. Install config is augmented to allow the customer to specify if a custom
DNS solution is being used. In addition, the customer would be able to indicate
the LB type to be used for API and API-Int traffic.

```yaml
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.2
  creationTimestamp: null
  name: installconfigs.install.openshift.io
spec:
  group: install.openshift.io
  names:
    kind: InstallConfig
    listKind: InstallConfigList
    plural: installconfigs
    singular: installconfig
  scope: Namespaced
  versions:
  - name: v1
    schema:
<snip>
                    aws:
                      description: AWS is the configuration used when installing on
                        AWS.
                      properties:
                        userConfiguredDNSLB:
                          description: "UserConfiguredDNSLB contains all the API and API-Int
                            LB information. \n This field is used to Enable the use of a
                            custom DNS solution when the DNS provided by the underlying
                            cloud platform cannot be used. When Enabled, the user can provide
                            information about user created API and API-Int LBs using this
                            field."
                          properties:
                            apiIntLBName:
                              description: ApiIntLBName is the name of the API-Int NLB created
                                by the user.
                              type: string
                            apiLBName:
                              description: ApiLBName is the name of the API NLB created
                                by the user.
                              type: string
                            userDNS:
                              default: Disabled
                              description: UserDNS specifies whether the customer is responsible
                                for configuring DNS entries for API and API-Int prior to
                                cluster installation.
                              enum:
                              - ""
                              - Enabled
                              - Disabled
                              type: string
                          type: object

<snip>
 ```
Setting `userConfiguredDNSLB.userDNS` to `Enabled` and not specifying
`apiIntLBName` would be considered a mis-configuration and will cause
the installation to fail. `apiLBName` is manadatory when `publish` is
`external` and optional is `publish` is `internal`.

2. Add a config item in the platform specific section of the Infrastructure CR
to indicate that a custom DNS solution is in use. `DNSConfigurationType` represents
the state of DNS configuration represented as a [Discriminated Union](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#discriminated-unions) with the `Provider` being the UnionDiscriminator.

```go
// AWSPlatformStatus holds the current status of the Amazon Web Services infrastructure provider.
type AWSPlatformStatus struct {
	<snip>
	// dnsConfig contains information about the type of DNS solution in use
        // for the cluster.
        // +default={"provider": ""}
        // +kubebuilder:default={"provider": ""}
        // +openshift:enable:FeatureSets=TechPreviewNoUpgrade
        // +optional
        DNSConfig *DNSConfigurationType `json:"dnsConfig,omitempty"`
}

// DNSConfigurationType contains information about who configures DNS for the
// cluster.
// +union
type DNSConfigurationType struct {
        // provider determines which DNS solution is in use for this cluster.
        // When the user wants to use their own DNS solution, the `provider`
        // is set to "UserAndClusterProvided".
        // When the cluster's DNS solution is the default for IPI or UPI, then
        // `provider` is set to "" which is also its default value.
        // +default=""
        // +kubebuilder:default:=""
        // +kubebuilder:validation:Enum="UserAndClusterProvided";""
        // +kubebuilder:validation:Required
        // +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="provider is immutable once set"
        // +unionDiscriminator
        // +optional
        Provider DNSProviderType `json:"provider,omitempty"`
}

// DNSProviderType defines the source of the DNS and LB configuration.
type DNSProviderType string

const (
        // DNSUserAndClusterProvided indicates that the user configures DNS for API and API-Int.
        // The cluster handles some of its in-cluster DNS needs without user intervention.
        DNSUserAndClusterProvided DNSProviderType = "UserAndClusterProvided"

        // DNSDefault indicates the cluster's default way of handling DNS configuration.
        // This refers to the default DNS configuration expected for both IPI and UPI installs.
        DNSDefault DNSProviderType = ""
)
```
The Installer is responsible for setting the `Provider` to
"UserAndClusterProvided", when a DNS solution configured by the user is in
use. When the DNS configuration is the respective default for UPI or IPI,
the Installer sets the `Provider` to "".

### Implementation Details/Notes/Constraints [optional]

This feature will start out in TechPreview and graduate to a fully supported
feature when we get confirmation from ARO, ROSA and OpenShift Dedicated on
Google.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

What is detailed here is more of a trade-off rather than a drawback.
The proposed implementation expects the user to pre-create VPCs, subnets,
LBs and pre-configure DNS entries for them. This may not be an issue for some
customers and especially with Managed Services since they are easily equipped
to handle these pre-configuration steps. A traditional IPI customer that would
prefer OpenShift to handle as much of its infrastructure management by itself
might find it harder to adapt to this solution.

## Design Details

### Open Questions [optional]

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

### Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

An alternative solution that involves self-hosted DNS and LB is discussed in (Enhancement Proposal) [https://github.com/openshift/enhancements/pull/1276]. 

## Infrastructure Needed [optional]

Testing and deploying this feature requires access to a DNS solution that is
not the default cloud DNS.
