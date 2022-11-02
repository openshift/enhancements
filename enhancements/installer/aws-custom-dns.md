---
title: aws-custom-dns
authors:
  - "@sadasu"
reviewers: 
  - "@patrickdillon, for Installer aspects"
  - "@Miciah, for Ingress Controller aspects"
  - "@cybertron, for on-prem networking aspects"
approvers:
  - "@patrickdillon"
  - "@sdodson"
  - "@dhellmann"
  - "@zaneb"
  - "@knobunc"
  - "@Miciah"
  - "@cybertron"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@joelspeed"
creation-date: 2022-10-25
last-updated: 2023-01-23
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/CORS-1874"
---

# Custom DNS for AWS

## Summary

This enhancement adds the ability to use a customer managed DNS solution
for API and Ingress resolution on the AWS platform during OpenShift IPI
installation. The default cloud DNS solution (Route 53, in case of AWS)
will not be used in this scenario.

## Motivation

Some customers do not want to use the DNS solution provided by the underlying
cloud due to regulatory [ITAR](https://www.gov-relations.com/) or operational
contraints. It is important for OpenShift to provide a way for these customers
to use their own preferred DNS solution while supporting their IPI deployments.

This external Custom DNS solution would be responsible for DNS records for
`api.<cluster domain>` and `*.apps.<cluster domain>`. OpenShift would be
responsible for in-cluster resolution of `api.<cluster domain>`,
`api-int.<cluster domain>` and `*.apps.<cluster domain>`.
The cloud Load Balancer(LB) is still the preferred solution for providing LB
services for `api`, `api-int` and `ingress` in public and private hosted zones.

### User Stories

- As an administrator, I would like to deploy OpenShift 4 clusters to supported
cloud providers leveraging my custom DNS service. While cloud-based DNS services
provide convenient hostname management, there are a number of regulatory (ITAR)
and operational constraints that prohibit the use of those DNS hosting services
on public cloud providers.

### Goals

- Enable AWS customers to use their custom DNS solution instead of the cloud
solution (Route53).
- Lay the groundwork to add this capability for other cloud platforms like
Azure and GCP.
- Continue using the cloud based LB service for API, Ingress and API-Int. 

### Non-Goals

- We will not focus on Installer changes to support the custom DNS solution on
Azure and GCP platforms. That will come in follow-up work and will rely on the
changes introduced within this solution.

- We will not use the [Kubernetes External DNS provider](https://github.com/openshift/external-dns)
and/or the [OpenShift External DNS Operator](https://github.com/openshift/external-dns-operator)
to program dns entries across varied DNS providers. The External DNS Operator
relies on being able to program the customer's DNS solution. The customer DNS
solutions in consideration here do not allow OpenShift to configure them.

## Proposal

For the OpenShift installation to make use of a custom DNS solution that is not
configurable by OpenShift, the customer is responsible for configuring the
DNS entries for the API and Ingress services.

When it is indicated to the OpenShift installer via an input parameter that the
custom DNS solution is in use, OpenShift does not configure the cloud DNS
solution [Route53 for AWS](https://aws.amazon.com/route53/). OpenShift will
continue to configure and manage the cloud Load Balancing(LB) solution [Elastic
Load Balancer](https://aws.amazon.com/elasticloadbalancing/) for all its
services.

The Installer configures the LBs for the API and API-Int services and the
Ingress Controllers configure the LBs for the *.apps service. There is
currently no way of knowing these LB IP addresses ahead of time. The customer's
DNS solution can be configured before cluster installation if the customer is
willing to configure the LBs and subnets too along with their DNS. This is a
viable alternate solution that is discussed as Option 1 in the `Alternatives`
section.

OpenShift can also be augmented to provide an in-cluster DNS solution during
cluster installation that allows the user to configure their custom DNS post-
install. The current proposal focuses on this approach.

OpenShift will start static CoreDNS pods to provide DNS resolution for API,
Internal API and Ingress services that are essential for cluster creation.
Although not identical, this approach leverages learning from OpenShift's
approach to providing these services for on-prem platforms.

After cluster deployment is completed, the customer will update their external
DNS solution with the same assigned LB IP addresses used for the configuration
of the internal CoreDNS instance. OpenShift will not delete the CoreDNS pod
even after cluster installation completes.

If the user successfully configures their external DNS service with api,
api-int and *.apps services, then they could optionally delete the in-cluster
CoreDNS pod and the cluster is expected to function fully as expected. This is
a completely optional step with this design. If the customer does configure
their custom DNS solution and leave the in-cluster CoreDNS pod running, all
in-cluster entities will end up using the CoreDNS pod's DNS services and all
out of cluster requests will be handled by the external DNS.

### Workflow Description

1. For an OpenShift administrator to use a custom DNS solution, they would
have to enable a config option in the install configuration. This would
cause the Installer and the Ingress Operator to skip configuring the cloud
DNS with entries for API, Internal API and Ingress Services. 

2. The Installer and the Ingress Operator continue to configure the cloud LBs
on the public subnets of Public Hosted Zones for `api` and `*.apps`. They will
continue to configure cloud LBs on the private subnets of Private Hosted Zones
for `api-int` and `*.apps` for in-cluster access to the Ingress service.

3. The Installer will also update the new fields `APIServerDNSConfig` and
`InternalAPIServerDNSConfig` within the `AWSPlatformStatus`, with the LB
IP addresses and the DNS Record types for API, API-Int servers respectively.

4. The Installer instructs the machine-config-operator(MCO) to start a static
CoreDNS pod on the bootstrap and master nodes to provide DNS resolution for
the API and API-Int servers. MCO uses the DNS config in the `AWSPlatformStatus`
to generate the CoreFile to configure the CoreDNS instance in the pod.
Note: This implementation differs from the on-prem case by not staring the
keepalived or haproxy containers for load balancing.

5. The Installer omits the DNS Zones within `dnses.config.openshift.io/cluster`
to influence the behavior of the Ingress Operator by making it skip the
configuration of cloud DNS while continuing with LB configuration.

6. The Ingress Controllers managed by the cluster-ingress-operator continue
to configure the application LBs to enable in-cluster and external accesss to
cluster services. The Ingress Controllers will continue to update the DNSRecord
ConfigResource(CR) with the configuration required to configure DNS for these
services.

7. The MCO will watch the DNSRecord CR for the DNS configuration mentioned in
step 6. MCO will update the newly added `IngressDNSConfig` field within the
`AWSPlatformStatus` with the DNS configuartion for *.apps records by reading
the DNSRecord CR.

8. The runtimecfg utility will monitor the `Infrastructure` CR for changes to
`IngressDNSConfig` of `AWSPlatformStatus` to update the CoreDNS pod's
CoreFile to now also include DNS configuration for both internal and
external access to the cluster services (the `*.apps` records).

9. Once the cluster is functional, the user can look at the contents of the new
fields `APIServerDNSConfig` and `IngressDNSConfig` in the `AWSPlatformStatus`
field of the `Infrastructure` CR to configure their external DNS with entries
for `api` and `*.apps` service.

### API Extensions

1. The AWSPlatformStatus within the PlatformStatus field of the Infrastructure
ConfigResource (CR) is updated to contain all the DNS config required for the
in-cluster CoreDNS solution. This same CR is available to the user post a
successful cluster install, to configure their own DNS solution.

```go
type AWSPlatformStatus struct {
        <snip>
        // AWSClusterDNSConfig contains all the DNS config required to configure a custom DNS solution.
        // +optional
        AWSClusterDNSConfig *ClusterDNSConfig `json:"awsClusterDNSConfig,omitempty"`

        <snip>

}

type ClusterDNSConfig struct {
        // APIServerDNSConfig contains information to configure DNS for API Server.
        // This field will be set only when the userConfiguredDNS feature is enabled.
        APIServerDNSConfig []DNSConfig `json:"apiServerDNSConfig,omitempty"`

        // InternalAPIServerDNSConfig contains information to configure DNS for the Internal API Server.
        // This field will be set only when the userConfiguredDNS feature is enabled.
        InternalAPIServerDNSConfig []DNSConfig `json:"internalAPIServerDNSConfig,omitempty"`

        // IngressDNSConfig contains information to configure DNS for cluster services.
        // This field will be set only when the userConfiguredDNS feature is enabled.
        IngressDNSConfig []DNSConfig `json:"ingressDNSConfig,omitempty"`
}


type DNSConfig struct {
       // recordType is the DNS record type.
       RecordType  string `json:"recordType"`

       // lBIPAddress is the Load Balancer IP address for DNS config
       LBIPAddress string `json:"lbIPAddress"`
}

```

2. Install config is updated to allow the customer to specify if an external
user configured DNS will be used. `UserConfiguredDNS` is added to the
install-config and will have to be explicitly set to `Enabled` to enable this
functionality. This config is not added to any platform specific section of
the config because there are plans to allow this functionality in Azure and GCP
too. The validation for this config will disallow this value being `Enabled` in
platforms that currently do not support it.

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
### Implementation Details/Notes/Constraints [optional]


### Risks and Mitigations


### Drawbacks

## Design Details

One of the decisions that needs to be made early in the design process is to
determine the best way to order OpenShift's configuration of the LB and the
customer's configuration of their custom DNS solution.

Today, the Installer configures the LB first and the IP address of the LB is
used to configure DNS records for the API, API-Int and `*.apps`. Since the IPs
cannot be predicted in advance, configing the customer's external DNS before
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
