---
title: cloud-custom-dns
authors:
  - "@sadasu"
reviewers: 
  - "@patrickdillon, for Installer aspects"
  - "@Miciah, for Ingress Controller aspects"
  - "@cybertron, for on-prem networking aspects"
  - "@2uasimojo, for hive expertise"
  - "@yuqi-zhang, for MCO aspects"
approvers:
  - "@zaneb"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@joelspeed"
creation-date: 2022-10-25
last-updated: 2023-01-23
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/CORS-1874"
---

# Custom DNS for AWS, GCP and Azure

## Summary

This enhancement adds the ability to use a customer managed DNS solution for
API. API-Int and Ingress resolution on AWS, Azure and GCP platforms during
OpenShift IPI installation. The default cloud provided DNS solutions will not
be configured and used when this feature is enabled.

## Motivation

Some customers do not want to use the DNS solution provided by the underlying
cloud due to regulatory [ITAR](https://www.gov-relations.com/) or operational
contraints. It is important for OpenShift to provide a way for these customers
to use their own preferred DNS solution while supporting their IPI deployments.

This external Custom DNS solution would be responsible for DNS records for
`api.<cluster domain>` and `*.apps.<cluster domain>`. OpenShift would be
responsible for in-cluster resolution of `api.<cluster domain>`,
`api-int.<cluster domain>` and `*.apps.<cluster domain>`. The cloud LBs are
still the preferred way for providing LB services for `api`, `api-int` and
`ingress` in public and private hosted zones.

In addition, in OpenShift's Managed offering, the customer's DNS solution is
also responsible for DNS records for required OpenShift resources including
quay.io, registry.redhat.io, api.openshift.com, cloud management endpoints, etc.
This enhancement proposal will not talk about how these entries are configured
for the OpenShift Managed offering.

### User Stories

- As an administrator, I would like to deploy OpenShift 4 clusters to supported
cloud providers leveraging my custom DNS service. While cloud-based DNS services
provide convenient hostname management, there are a number of regulatory (ITAR)
and operational constraints that prohibit the use of those DNS hosting services
on public cloud providers.

- As an administrator, I want to continue using the LB services provided by the
underlying cloud platform.

- As a user running their cluster on AWS GovCloud, I would like my cluster to
be publicly accessible. Currently, with Route53, only private clusters can be
created in AWS GovCloud.


### Goals

- Enable AWS, Azure, and GCP customers to use their custom DNS solution in
place of the cloud solution (For example, Route53 for AWS).
- Provide in-cluster DNS solution for successful cluster installation without
dependence on customer configured infrastructure items.
- Continue using the cloud based LB service for API, Ingress and API-Int. 

### Non-Goals

- We will not use the [OpenShift External DNS provider](https://github.com/openshift/external-dns)
and/or the [OpenShift External DNS Operator](https://github.com/openshift/external-dns-operator)
to program dns entries across varied DNS providers. The External DNS Operator
relies on being able to program the customer's DNS solution. Because 
programmable DNS is precluded in some environments due to technical limitations
or organizational policy, we cannot require programmable DNS.

- Although the upstream [Kubernetes External DNS provider](https://github.com/kubernetes-sigs/external-dns) does support configuring CoreDNS, this operator can run only on a running cluster as a day-2 operator for `*.apps` records and hence cannot be used at this time.

## Proposal

For the OpenShift installation to make use of a custom DNS solution that is not
configurable by OpenShift, the customer is responsible for configuring the
DNS entries for `api` and `*.apps` services.

When it is indicated to the OpenShift installer via an input parameter that the
custom DNS solution is in use, OpenShift does not configure the respective cloud
DNS solutions [Route53 for AWS](https://aws.amazon.com/route53/), [Azure DNS](https://learn.microsoft.com/en-us/azure/dns/dns-overview) and [Cloud DNS for GCP](https://cloud.google.com/dns).

The Installer configures the LBs for the API and API-Int services and the
Ingress Controllers configure the LBs for the `*.apps` service. There is
currently no way of knowing these LB IP addresses before their creation. The
customer would have to wait to configure their custom DNS solution until after
the LBs are created by OpenShift and the cluster installation has completed.

For the cluster installation to succeed before the custom DNS solution is setup
for `api`, `api-int` and `*.apps` resolution, OpenShift will have to provide a
self-hosted in-cluster DNS solution. This would allow the customer to configure
their DNS solution post-install. The current proposal focuses on this approach.

OpenShift will start static CoreDNS pods to provide DNS resolution for API,
Internal API and Ingress services that are essential for cluster creation.
In order for a master or worker node to come online, they need the
machine config server via the api-int domain. So, it is essential for the
api-int domain to be resolvable on the bootstrap node. The static CoreDNS
pod started on the bootstrap node needs entries for just api-int resolution
and the static CoreDNS pod started on the control plane nodes needs to be
able to resolve `api`, `api-int` and `*.apps` domains. Although not identical, this
approach leverages learnings from OpenShift's approach to providing in-cluster 
DNS for on-prem platforms.

After cluster deployment is completed, the customer will update their external
DNS solution with the same assigned LB IP addresses used for the configuration
of the internal CoreDNS instance. OpenShift will not delete the CoreDNS pod
even after cluster installation completes.

If the user successfully configures their external DNS service with `api`,
`api-int` and `*.apps` services, then they could optionally delete the in-cluster
CoreDNS pod and the cluster is expected to function fully as expected. This is
a completely optional step with this design. If the customer does configure
their custom DNS solution and leave the in-cluster CoreDNS pod running, all
in-cluster entities will end up using the CoreDNS pod's DNS services and all
out of cluster requests will be handled by the external DNS.

### Workflow Description

1. To use the custom DNS solution, an OpenShift administrator enables it
via a config option in the install-config. Supplying this config would
cause the Installer and the Ingress Operator to skip configuring the cloud
DNS with entries for API, Internal API and Ingress Services.

2. The Infrastructure CR is updated to indicate that the custom DNS and not
the cloud default DNS is in use.

3. The Installer and the Ingress Operator continue to configure the cloud LBs
on the public subnets of Public Hosted Zones for `api` and `*.apps`. They will
continue to configure cloud LBs on the private subnets of Private Hosted Zones
for `api-int` and `*.apps` for in-cluster access to the Ingress service.

4. After the Installer uses the cloud specific CAPI providers to create
the LBs for API and API-Int, it will add the LB DNS Names of these LB to a [ConfigMap](https://kubernetes.io/docs/concepts/configuration/configmap/). This ConfigMap is
only created when the custom DNS feature is enabled. This `lbConfigforDNS`
ConfigMap gets appended to the Bootstrap Ignition file created by the
Installer.

5. The Installer starts the machine-config-operator(MCO) on the bootstrap
host by providing it with the `lbConfigforDNS` ConfigMap as an optional
input parameter.

6. MCO resolves the LB DNS Name for api-int provided within `lbConfigforDNS`.
It then starts the CoreDNS pod on the bootstrap node with [runtimecfg](https://github.com/openshift/baremetal-runtimecfg) as an initContainer. It provides the api-int LB IP
as an input parameter to `runtimecfg`.

7. The Installer omits the DNS Zones within `dnses.config.openshift.io/cluster`
to influence the behavior of the Ingress Operator by making it skip the
configuration of cloud DNS while continuing with LB configuration.

8. The Ingress Controllers managed by the cluster-ingress-operator (also
running in the bootstrap node) configures the application LBs to enable
in-cluster and external accesss to cluster services. The Ingress Controllers
will continue to update the DNSRecord ConfigResource(CR) with the [information]
(https://github.com/openshift/api/blob/master/operatoringress/v1/types.go#L39)
required to configure DNS for these services.

9. The MCO will read `DNSName` from the DNSRecord CR. The `DNSName` corresponds
to the hostname of the Ingress LB that would be added to the DNS record. MCO
resolves this `DNSName` to the internal igress LB's IP and pass it as an input
parameter to runtimefg which is started as an initContainer in the CoreDNS pod
mentioned in step 5.

10. With the 2 parameters to `runtimecfg` mentioned in steps 5 and 8, it can
update the CoreFile for the CoreDNS pod with DNS entries for api-int and
ingress thus providing in-cluster DNS resolution required for the bootstrap
phase. 

11. In addition, `runtimecfg` also updates the platform specific Status field
within the `Infrastructure` CR with the api and api-int LN DNS names obtained
from the `lbConfigforDNS` ConfigMap. It also updates the same Status field
with the hostname or IP address (depending on the platform) of the internal and
external LBs for the *.apps service. This is necessary because the user can
refer to these fields to configure their custom DNS.

12. The machine-config-operator on the master nodes, starts the CoreDNS pod if
the Spec field in the Infrastructure CR indicates that the custom DNS feature
is enabled. As in the case of the CoreDNS pod that was run on the bootstrap node,
the CoreDNS pod running on the master node also runs `runtimecfg` as an
initContainer. `runtimecfg` will be passed in the IP addresses for the internal
and external api servers and also for the ingress service. `runtimecfg` uses
this information to generate the CoreDNS CoreFile. 

### API Extensions

1. A new ConfigMap called `lbConfigForDNS` is created by the Installer. It can
be created in any namespace but we are choosing to create it in the same
namespace as the CoreDNS pods. Hence, the namespace name would be constructed
as: openshift-$platform_name-infra

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: LBConfigForDNS
  namespace: openshift-aws-infra
data:
  internal-api-lb-dns-name: "abc-123"
  external-api-lb-dns-name: "xyz-456"
```

2. Install config is updated to allow the customer to specify if an external
user configured DNS will be used. `UserConfiguredDNS` is added to the platform
portions of the install-config. The useer will have to be explicitly set it to
`Enabled` to enable this functionality. This field is added to the AWS, Azure
and GCP platforms.

```yaml
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
  aws/azure/gcp:
    properties:
      userConfiguredDNS:
        description: UserConfiguredDNS is set to `Enabled` when the customer
          wants to use a DNS solution external to the cluster and OpenShift is
          not allowed to configure it. The default value is `Disabled`. Current
          set of supported platforms is limited to AWS.
        enum:
        - ""
        - Enabled
        - Disabled
        type: string
```

3. Update the PlatformStatus field within the Infrastructure CR to include
the type of DNS solution in use and if needed, the IP addressses of the
API, API-Int and Ingress Load Balancers.

```yaml

type GCPPlatformStatus struct {
<snip>
        // cloudLoadBalancerConfig is a union that contains the IP addresses of API,
        // API-Int and Ingress Load Balancers created on the cloud platform. These
        // values would not be populated on on-prem platforms. These Load Balancer
        // IPs are used to configure the in-cluster DNS instances for API, API-Int
        // and Ingress services. `dnsType` is expected to be set to `ClusterHosted`
        // when these Load Balancer IP addresses are populated and used.
        // 
        // +default={"dnsType": "PlatformDefault"}
        // +kubebuilder:default={"dnsType": "PlatformDefault"}
        // +openshift:enable:FeatureSets=CustomNoUpgrade;TechPreviewNoUpgrade
        // +optional
        // +nullable
        CloudLoadBalancerConfig *CloudLoadBalancerConfig `json:"cloudLoadBalancerConfig,omitempty"`
}

// CloudLoadBalancerConfig contains an union discriminator indicating the type of DNS
// solution in use within the cluster. When the DNSType is `ClusterHosted`, the cloud's
// Load Balancer configuration needs to be provided so that the DNS solution hosted
// within the cluster can be configured with those values.
// +kubebuilder:validation:XValidation:rule="has(self.dnsType) && self.dnsType != 'ClusterHosted' ? !has(self.clusterHosted) : true",message="clusterHosted is permitted only when dnsType is ClusterHosted"
// +union
type cloudLoadBalancerConfig struct {
        // dnsType indicates the type of DNS solution in use within the cluster. Its default value of
        // `PlatformDefault` indicates that the cluster's DNS is the default provided by the cloud platform.
        // It can be set to `ClusterHosted` to bypass the configuration of the cloud default DNS. In this mode,
        // the cluster needs to provide a self-hosted DNS solution for the cluster's installation to succeed.
        // The cluster's use of the cloud's Load Balancers is unaffected by this setting.
        // The value is immutable after it has been set at install time.
        // Currently, there is no way for the customer to add additional DNS entries into the cluster hosted DNS.
        // Enabling this functionality allows the user to start their own DNS solution outside the cluster after
        // installation is complete. The customer would be responsible for configuring this custom DNS solution,
        // and it can be run in addition to the in-cluster DNS solution.
        // +default="PlatformDefault"
        // +kubebuilder:default:="PlatformDefault"
        // +kubebuilder:validation:Enum="ClusterHosted";"PlatformDefault"
        // +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="dnsType is immutable"
        // +optional
        // +unionDiscriminator
        DNSType DNSType `json:"dnsType,omitempty"`

        // clusterHosted holds the IP addresses of API, API-Int and Ingress Load
        // Balancers on Cloud Platforms. The DNS solution hosted within the cluster
        // use these IP addresses to provide resolution for API, API-Int and Ingress
        // services.
        // +optional
        // +unionMember,optional
        ClusterHosted *CloudLoadBalancerIPs `json:"clusterHosted,omitempty"`
}

// CloudLoadBalancerIPs contains the Load Balancer IPs for the cloud's API,
// API-Int and Ingress Load balancers. They will be populated as soon as the
// respective Load Balancers have been configured. These values are utilized
// to configure the DNS solution hosted within the cluster.
type CloudLoadBalancerIPs struct {
        // apiIntLoadBalancerIPs holds Load Balancer IPs for the internal API service.
        // These Load Balancer IP addresses can be IPv4 and/or IPv6 addresses.
        // Entries in the apiIntLoadBalancerIPs must be unique.
        // A maximum of 16 IP addresses are permitted.
        // +kubebuilder:validation:Format=ip
        // +listType=set
        // +kubebuilder:validation:MaxItems=16
        // +optional
        APIIntLoadBalancerIPs []IP `json:"apiIntLoadBalancerIPs,omitempty"`

        // apiLoadBalancerIPs holds Load Balancer IPs for the API service.
        // These Load Balancer IP addresses can be IPv4 and/or IPv6 addresses.
        // Could be empty for private clusters.
        // Entries in the apiLoadBalancerIPs must be unique.
        // A maximum of 16 IP addresses are permitted.
        // +kubebuilder:validation:Format=ip
        // +listType=set
        // +kubebuilder:validation:MaxItems=16
        // +optional
        APILoadBalancerIPs []IP `json:"apiLoadBalancerIPs,omitempty"`

        // ingressLoadBalancerIPs holds IPs for Ingress Load Balancers.
        // These Load Balancer IP addresses can be IPv4 and/or IPv6 addresses.
        // Entries in the ingressLoadBalancerIPs must be unique.
        // A maximum of 16 IP addresses are permitted.
        // +kubebuilder:validation:Format=ip
        // +listType=set
        // +kubebuilder:validation:MaxItems=16
        // +optional
        IngressLoadBalancerIPs []IP `json:"ingressLoadBalancerIPs,omitempty"`
}
```

### Implementation Details/Notes/Constraints [optional]


### Risks and Mitigations


### Drawbacks

## Design Details

One of the decisions that needs to be made early in the design process is to
determine the best way to order OpenShift's configuration of the LB and the
customer's configuration of their custom DNS solution.

Today, the Installer configures the LB first and the IP address of the LB is
used to configure DNS records for the API, API-Int and `*.apps`. Since the IPs
cannot be predicted in advance, configuring the customer's external DNS before
cluster install is not a possibility.

Then we have the option of configuring the LB manually(by the customer) and
using the LB IP addresses to configure the customer's DNS. This works great
when the customers create their own subnets too. OpenShift Installer currently
supports the option of creating subnets for the user and this use case would
not be supported with this approach.

Finally, OpenShift could provide its own in-cluster solution to create the
cluster and provide the user with all the information they need to also
configure their custom DNS solution. All in-cluster communication is handled
by the in-cluster DNS provided using CoreDNS and all external requests are
handled by the custom DNS.

### Open Questions [optional]

None

## Alternatives

1. Manually pre-configuring Cloud Load Balancer and Custom DNS

As alluded to in the `Design Details` section, there is another approach to
providing this functionality to the user. The user would have to manually
configure the internal and external Load Balancers before cluster installation.
The user can then use the IP addresses for the external and internal LBs to
configure DNS entries for API and Internal API servers. Similarly the cluster's
Ingress Load Balancers and DNS entry would have to be pre-configured manually
for the cluster installation to be successful.

This solution would also require the user to pre-configure their subnets and
then the option for the Installer to create subnets during cluster installation
would no longer be available.

Although this option has additional pre-configure steps, it has the advantage
of providing a simpler and quick solution. With Installer adding a set of
robust validations, we could manage the risk of misconfiguration by the users.

At the start of the design effort, it was believed that this would be the
preferred solution within the Managed Services space but we have received
feedback that they don't have a clear preference for either approaches.

Although the solution detailed in the `Proposal` is more complex to implement,
it requires the least amount of pre-configuration by the user. So, we are
proceeding with that solution and maintining the pre-configure solution as an
alternative at this time. We are open to revisiting this decision in a future
release and/or in light of additional data and change course if required.

2. Using /etc/hosts to provide DNS config per host

One option considered was to provide the DNS config within the /etc/hosts file
and for the Installer to provide this file to Machine Configs. This approach
would mean that the Ingress Controller would have to append the /etc/hosts file
after the Ingress Controllers had configured their LBs. We abandoned this
approach in favor of leveraging the prior work done for on-prem platforms and
decided to go with the static CoreDNS pods.

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


## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
