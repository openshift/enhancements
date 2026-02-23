---
title: aws-dual-stack-support-for-ingresscontrollers
authors:
  - "@alebedev87"
reviewers:
  - "@sadasu"
  - "@tthvo"
  - "@nrb"
  - "@davidesalerno"
  - "@rfredette"
  - "@Miciah"
approvers:
  - "@sadasu"
  - "@davidesalerno"
  - "@rfredette"
  - "@Miciah"
api-approvers:
  - None
creation-date: 2026-02-02
last-updated: 2026-02-10
tracking-link:
  - https://issues.redhat.com/browse/NE-2021
status: provisional
see-also:
  - TBD
---

# AWS Dual-stack Support for IngressControllers

## Summary

This enhancement enables dual-stack (IPv4 and IPv6) IP address types for
IngressController publishing services on AWS clusters using Network Load
Balancers (NLB). Currently, IngressControllers only support single-stack
IPv4 load balancers. This enhancement makes the ingress operator
automatically configure dual-stack load balancers for IngressControllers
on clusters installed with dual-stack networking. This is a day-0
feature only, configured at cluster installation time.

## Motivation

AWS NLBs support dual-stack IP address type, allowing
services to be accessible via both IPv4 and IPv6 addresses. However,
OpenShift does not currently support dual-stack for IngressController publishing services.
As IPv6 adoption increases and organizations require
dual-stack networking for compliance, accessibility, or future-proofing
their infrastructure, OpenShift needs to provide a way to configure
IngressController publishing services with dual-stack support.

AWS Classic Load Balancers do not support dual-stack, so this feature
is specifically targeted at clusters using NLBs on AWS.

### User Stories

* As a cluster administrator deploying on AWS with dual-stack IP family and
  NLB type, I want my IngressControllers to automatically use dual-stack
  IP address type so that my applications are accessible over both IPv4 and
  IPv6 networks.

### Goals

* Enable dual-stack IP address type for IngressController publishing
  services on AWS clusters installed with dual-stack networking.

* Automatically configure IngressController publishing services
  with dual-stack support by reading the cluster-wide IP family configuration.

* Ensure dual-stack IngressControllers work correctly across different
  cluster topologies including standalone clusters, Hypershift/hosted
  control planes, single-node deployments, and MicroShift.

* Support smooth upgrades and downgrades, preserving existing single-stack
  configurations on clusters not installed with dual-stack.

### Non-Goals

* Supporting dual-stack IngressControllers on cloud platforms other than
  AWS (e.g. Azure, GCP) in the initial implementation.

* Supporting dual-stack with AWS Classic Load Balancers (CLB), which do not
  have dual-stack capability.

* Implementing single-stack IPv6 IngressControllers. AWS NLBs only support IPv4 single-stack,
  or dual-stack (IPv4 + IPv6).

* Adding API fields to the IngressController CRD for supporting day-2 changes to IP address type.

* Forbidding the creation of an IngressController with CLB type on day-2, as well as changing the type to CLB.

* Supporting dual-stack for Gateway API implementation.

## Proposal

The implementation will automatically configure the publishing service for IngressControllers
with dual-stack IP address type when the cluster's IP family is configured for
dual-stack. The implementation will:

1. Update the ingress-operator to read the cluster's IP family configuration
   from the Infrastructure CR status field
   ([`.status.platformStatus.aws.ipFamily`](https://github.com/openshift/api/blob/fca93aff74172d801b89f6c0881a910fb79931da/config/v1/types_infrastructure.go#L555-L565)),
   similar to how the operator currently reads
   [AWS resource tags](https://github.com/openshift/cluster-ingress-operator/blob/8afaffbf8ddbe65565bad52eea6267b615eceec2/pkg/operator/controller/ingress/load_balancer_service.go#L448-L458).

2. When the cluster is configured with dual-stack networking (ipFamily is
   `DualStackIPv4Primary` or `DualStackIPv6Primary`), automatically configure
   the load balancer service's `ipFamilies` and `ipFamilyPolicy` fields:
   - For `DualStackIPv4Primary`: set `service.spec.ipFamilies: ["IPv4", "IPv6"]`
     and `service.spec.ipFamilyPolicy: RequireDualStack`
   - For `DualStackIPv6Primary`: set `service.spec.ipFamilies: ["IPv6", "IPv4"]`
     and `service.spec.ipFamilyPolicy: RequireDualStack`
   - The AWS cloud provider (cloud-provider-aws) will read these service
     fields and configure the NLB with dual-stack support accordingly.

3. When the cluster is configured with IPv4-only networking (ipFamily is
   `IPv4` or not set), the ingress-operator does not set the `ipFamilies`
   and `ipFamilyPolicy` fields, allowing them to default to single-stack
   IPv4 behavior.

4. Update the [AWS DNS provider implementation](https://github.com/openshift/cluster-ingress-operator/blob/8afaffbf8ddbe65565bad52eea6267b615eceec2/pkg/dns/aws/dns.go)
   to create both Route53 Alias A and Alias AAAA records when the cluster IP
   family is dual-stack. The IP family is passed to the provider at
   initialization time, similar to [AWS region](https://github.com/openshift/cluster-ingress-operator/blob/8afaffbf8ddbe65565bad52eea6267b615eceec2/pkg/dns/aws/dns.go).


### Workflow Description

**cluster administrator** is a human user responsible for installing and
managing OpenShift clusters.

**ingress-operator** is the OpenShift operator responsible for managing
IngressController resources and their associated router deployments.

**installer** is the OpenShift installer responsible for creating the
cluster and configuring the Infrastructure CR.

1. The cluster administrator installs an OpenShift cluster on AWS with
   the `AWSDualStackInstall` feature gate enabled and [a dual-stack IP family](https://github.com/openshift/installer/blob/b0514c8e022d8445e57f303852487d3cd59c4a0a/pkg/types/aws/platform.go#L134-L144) (`DualStackIPv4Primary` or `DualStackIPv6Primary`),
   which configures the cluster's VPC, subnets, and network stack for dual-stack.

2. The installer creates the Infrastructure CR with the cluster's IP family
   configuration in `.status.platformStatus.aws.ipFamily` (`DualStackIPv4Primary` or `DualStackIPv6Primary`).

3. The ingress-operator starts and reads the Infrastructure CR to
   determine the cluster's platform and IP family configuration.

4. When the ingress-operator reconciles an IngressController resource
   configured with `LoadBalancerService` endpoint publishing strategy and
   AWS NLB type, it reads the Infrastructure CR to determine the cluster's platform
   and IP family configuration and configures the service's `ipFamilies` and `ipFamilyPolicy`.

5. The AWS cloud provider reads the service's
   `ipFamilies` and `ipFamilyPolicy` fields and provisions a dual-stack NLB
   with a hostname (e.g., `abc123.elb.us-east-1.amazonaws.com`) that
   resolves to both IPv4 and IPv6 addresses.

6. The Kubernetes LoadBalancer service status is updated with the AWS
   NLB hostname in `status.loadBalancer.ingress[].hostname`.

7. The ingress-operator creates or updates the DNSRecord resource with
   the AWS NLB hostname as the target.

8. The ingress-operator's AWS DNS provider (which was initialized with the cluster's IP
   family) creates Route53 Alias records for the wildcard domain:
   - For dual-stack clusters: creates both Alias A and Alias AAAA records
     pointing to the AWS NLB hostname
   - For IPv4-only clusters: creates only Alias A record (existing behavior)

9. When clients query the wildcard domain:
   - IPv4 queries for A records → Route53 Alias A → NLB IPv4 addresses
   - IPv6 queries for AAAA records → Route53 Alias AAAA → NLB IPv6
     addresses

10. Applications exposed through routes become accessible via both IPv4
    and IPv6 addresses.

#### Variation: Cluster Not Installed with Dual-Stack

1. On a cluster installed without dual-stack networking (standard IPv4-only
   installation), the Infrastructure CR's `.status.platformStatus.aws.ipFamily`
   is set to `IPv4` (or not set, defaulting to `IPv4`).

2. The ingress-operator reads the IP family from the Infrastructure CR
   and determines that the cluster is IPv4-only.

3. The ingress-operator does not set the `ipFamilies` and `ipFamilyPolicy`
   fields on the LoadBalancer service (defaults to single-stack IPv4).

4. The ingress-operator does not create an Alias AAAA record for the wildcard domain.

5. AWS provisions an IPv4-only CLB or NLB (depending on [the load balancer type from Ingress Config API|https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/config/v1/types_ingress.go#L153]).

6. IngressControllers function normally with IPv4 connectivity only.

#### Variation: Dual-Stack on Unsupported Load Balancer Type

1. A cluster is installed with dual-stack networking on AWS, but an
   IngressController is configured to use CLB instead of NLB on day-2.

2. The ingress-operator detects that the load balancer type is CLB,
   which does not support dual-stack.

3. The ingress-operator sets the status to `Progressing=True` with a message to the user
   about the need to recreate the service manually. Also, the message highlights the fact
   that CLB does not support the cluster-wide dual-stack IP family (to be added as part of this enhancement).

4. When the user proceeds with a service recreation, the ingress-operator creates the
   LoadBalancer service without the `ipFamilies` and `ipFamilyPolicy` fields,
   falling back to IPv4-only configuration for that specific IngressController.
   The ingress-operator sets the status back to `Progressing=False`.

### API Extensions

- **No IngressController API changes are required** for this enhancement.
The dual-stack configuration is determined automatically by the
ingress-operator based on the cluster's installation configuration, not
through per-IngressController API fields.

- **No DNSRecord API changes are required** for this enhancement.
The dual-stack configuration is determined automatically by the
ingress-operator based on the cluster's installation configuration.

### Topology Considerations

#### Standalone Clusters

This enhancement is fully applicable to standalone clusters running on
AWS with NLBs. This is the primary use case for this feature.

#### Hypershift / Hosted Control Planes

This enhancement is applicable to Hypershift deployments. In Hypershift
architectures, the IngressController runs in the hosted cluster and the
load balancer is provisioned in the hosting infrastructure. The dual-stack
configuration will work as long as:
- The hosting AWS infrastructure supports dual-stack networking
- The hosted cluster's network is configured for dual-stack

#### Single-node Deployments or MicroShift

Single-node deployments (SNO) and MicroShift can benefit from this
enhancement if they are deployed on AWS with NLB and require dual-stack
ingress capabilities. The resource consumption impact is minimal, as the
dual-stack configuration only affects the load balancer provisioning and
does not significantly increase CPU or memory usage of the
IngressController operator or router pods.

#### OpenShift Kubernetes Engine

This enhancement is applicable to OKE deployments on AWS that use NLBs.
OKE includes ingress capabilities, and dual-stack support for IngressControllers
would be available if the underlying infrastructure supports it.

### Implementation Details/Notes/Constraints

**Notes:**
- The installer provides an [install config validation for IP family and load balancer type](https://github.com/openshift/installer/pull/10256).
  It's impossible to request the creation of a cluster with dual-stack IP family and CLB type.
  This way, the NLB becomes the default type for any IngressController.

- Router pods already support dual-stack traffic. The ingress-operator reads
  the cluster network configuration from the Network CR
  (`.status.clusterNetwork`) and configures the router with the appropriate
  IP mode via the `ROUTER_IP_V4_V6_MODE` environment variable (see
  [pkg/operator/controller/ingress/deployment.go#L990-L1009](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/controller/ingress/deployment.go#L990-L1009)),
  so no additional changes are needed for router dual-stack support.

- The dual-stack support in cloud-provider-aws is currently being
  implemented (see [kubernetes/cloud-provider-aws#1313](https://github.com/kubernetes/cloud-provider-aws/pull/1313)).
  The exact mapping of Infrastructure CR IPFamily values to service
  `ipFamilies` and `ipFamilyPolicy` fields may change based on the
  cloud-provider-aws implementation details.

### Risks and Mitigations

**Risk**: Users may expect to be able to change from IPv4 to dual-stack
after installation, but this is not supported as a day-2 operation.

**Mitigation**: Clearly document that dual-stack support is a day-0
feature only and requires cluster installation with the
`AWSDualStackInstall` feature gate. Document the limitations and the
rationale for the day-0-only approach.

**Risk**: On a dual-stack cluster, if an IngressController is configured
with CLB type, it will fall back to IPv4-only, which may be unexpected.

**Mitigation**: The ingress-operator should log a warning or update the
IngressController status with a condition indicating that dual-stack is
not available with CLB type. Documentation should clearly state that
NLB is required for dual-stack support.

### Drawbacks

* This enhancement is platform-specific (AWS only initially), which may
  create expectations for similar support on other cloud platforms.

* Being a day-0 feature only means organizations cannot migrate existing
  clusters from IPv4 to dual-stack without reinstalling. This limits
  adoption for existing deployments.

## Alternatives (Not Implemented)

### Alternative 1: Per-IngressController API Configuration

Instead of automatically applying dual-stack IP family based on cluster
configuration, add an `ipAddressType` (or `ipFamily`) field to the IngressController API.
Additionally, add an `ipFamily` field to the Ingress Config API which records
the installer's intent and is used as the default value for all IngressControllers.
The ingress-operator reads the Ingress Config API `ipFamily` during reconciliation
and sets the IngressController's `ipAddressType` to match. This alternative is similar
to [Allow Users to specify Load Balancer Type during installation for AWS](https://github.com/openshift/enhancements/blob/e91105bda6aa67fd821f7c749696ba911672e213/enhancements/installer/aws-load-balancer-type.md).

**Why not chosen**: This approach adds significant API and implementation
complexity while duplicating the installer's intent, which is already recorded
in the Infrastructure CR and is strongly recommended to be respected by all
OpenShift components that create cloud resources.
Additionally, [Kubernetes doesn't allow switching the `ipFamilies` on an existing service](https://kubernetes.io/docs/concepts/services-networking/dual-stack/#services),
which imposes a limitation on day-2 changes to the `ipAddressType` field in the
IngressController API. The ingress-operator would need a workaround that consists
of requiring users to delete and recreate the load balancer service to switch the
`ipAddressType`, creating operational overhead and service disruption.
This alternative could be considered in a future enhancement if per-IngressController
`ipAddressType` configuration is explicitly requested.

### Alternative 2: Support Day-2 IP Family Changes

Support changing the cluster's IP family from IPv4 to dual-stack (or
vice versa) after installation, and have the ingress operator detect and
apply these changes.

**Why not chosen**: Changing the cluster's network IP family after
installation is a complex operation that affects many components beyond
ingress (networking, storage, etc.). The installer and cluster networking
components do not currently support day-2 IP family changes. Limiting
this enhancement to day-0 reduces complexity and aligns with the current
cluster networking capabilities. Day-2 support could be considered in a
future enhancement if the underlying infrastructure components add
support for it.

## Open Questions [optional]
N/A

## Test Plan

**Test Strategy:**
- Unit tests for operator logic that reads IP family from Infrastructure
  CR and configures services accordingly.
- E2E tests verifying:
  - On clusters installed with `AWSDualStackInstall` feature gate and a dual-stack IP family:
    - IngressControllers with NLB are automatically configured with
      dual-stack IP address type.
    - Load balancer service is provisioned with correct `ipFamilies` and
      `ipFamilyPolicy` fields.
    - AWS NLB hostname is populated in service status.
    - DNS alias records of the wildcard domain point to the AWS NLB hostname and resolve to both IPv4 and IPv6 addresses.
    - Applications are accessible via both IPv4 and IPv6.
    - IngressControllers with CLB type fall back to IPv4-only.

## Graduation Criteria

This feature depends on the `AWSDualStackInstall` installer feature gate,
which is owned by the installer team. The ingress operator implementation
itself does not require a separate feature gate, as it automatically
adapts to the cluster's IP family configuration set by the installer.

**Graduation path:**
- The ingress operator changes can be implemented and merged independently
  of the `AWSDualStackInstall` installer feature gate state.
- When the installer's `AWSDualStackInstall` feature gate is enabled and the IPFamily is dual-stack,
  the ingress operator will automatically configure the dual-stack IP address type for
  publishing services of IngressControllers.
- When the installer's `AWSDualStackInstall` feature gate is not used,
  the ingress operator will continue to configure IPv4-only publishing services
  for IngressControllers.

**Testing requirements:**
- Minimum 5 tests with the `[OCPFeatureGate:AWSDualStackInstall]` label.
- Tests run at least 7 times per week.
- Tests run at least 14 times on AWS platform with dual-stack configuration.
- 95% pass rate across all tests.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

The actual availability of dual-stack ingress to customers will depend on
the installer's `AWSDualStackInstall` feature gate graduation.

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy

**Upgrade from version without feature to version with feature:**
The feature is day-0 only, only fresh installs are supported.

**Downgrade to version without feature:**
- On clusters installed with the `AWSDualStackInstall` feature gate and dual-stack IP family:
  - The older ingress-operator version will not read the IP family from
    the Infrastructure CR and will fall back to IPv4-only configuration.
  - The IngressController's publishing service will be reconciled and the `ipFamilies` and
    `ipFamilyPolicy` fields will be removed if previously set, defaulting to IPv4-only.
  - The AWS NLB hostname remains the same; AWS removes AAAA record.
  - IPv4 connectivity continues to work; IPv6 connectivity is lost.
  - The Alias A and AAAA records for the wildcard domain remain unchanged as they point to the same NLB hostname.
    However, the Alias AAAA record will not work anymore because the IPv6 address for the load balancer will disappear.

## Version Skew Strategy
No special version skew handling is required beyond normal OpenShift upgrade procedures.

## Operational Aspects of API Extensions

### Implementation Impact

N/A because no API changes are required.

## Support Procedures

**Detecting dual-stack configuration issues:**

Symptoms:
- `ingress-operator` logs show errors related to dual-stack configuration
  or load balancer provisioning
- Load balancer service does not have a hostname in its status or the
  hostname does not resolve to both IPv4 and IPv6 addresses
- On a dual-stack cluster, IngressControllers are only getting IPv4
  connectivity

Diagnosis:
1. Verify the cluster is installed with dual-stack: `oc get
   infrastructure cluster -o jsonpath='{.status.platformStatus.aws.ipFamily}'`
   should show `DualStackIPv4Primary` or `DualStackIPv6Primary` for
   dual-stack clusters, or `IPv4` for IPv4-only clusters.
2. Check IngressController status: `oc -n openshift-ingress-operator get ingresscontroller <name> -o yaml`.
3. Review ingress-operator logs: `oc -n openshift-ingress-operator logs deployment/ingress-operator` -
   look for messages about reading IP family from Infrastructure CR.
4. Check load balancer service: `oc -n openshift-ingress get svc router-<name> -o yaml` -
   verify it has `spec.ipFamilies: ["IPv4", "IPv6"]` (or `["IPv6", "IPv4"]`) and `spec.ipFamilyPolicy: RequireDualStack`,
   and a hostname in `status.loadBalancer.ingress[].hostname`.
5. Verify the AWS NLB hostname resolves to both IPv4 and IPv6: `dig A <nlb-hostname>` and `dig AAAA <nlb-hostname>`.
6. Verify platform is AWS and load balancer type is NLB (Classic LB does
   not support dual-stack).
7. Check DNSRecord status: `oc -n openshift-ingress-operator get dnsrecord <name>-wildcard -o yaml`.
8. Verify DNS alias is published: `dig <wildcard-domain>` should show AWS NLB's IP.
