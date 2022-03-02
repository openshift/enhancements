---
title: cert-manager-operator-for-red-hat-openShift
authors:
  - "@slaskawi"
reviewers:
  - "@stlaz"
  - "@ibihim"
  - "@s-urbaniak"
approvers:
  - "@stlaz"
  - "@ibihim"
  - "@s-urbaniak"
creation-date: 2022-02-16
last-updated: 2022-02-16
tracking-link:
    - https://issues.redhat.com/browse/AUTH-5
---

# Cert Manager operator for Red Hat OpenShift

## Summary

This enhancement outlines the implementation details of a Cert Manager support
for OpenShift.

The key things to highlight are:
- This feature will be provided as an OLM-based Operator
- The Operator implementation will be based on `library-go`
- The productization pipeline will be used for the release process

## Motivation

Cert Manager is currently a major player in the certificate management space in 
Kubernetes. The OpenShift Auth Team decided to productize it and ship under
Red Hat flag. This proposal summarizes the use cases and highlights the most
common workflows when using Cert Manager.

### Goals

- Provide use cases and estimated timeline for shipping Cert Manager Operator features
- Describe the most common workflows

### Non-Goals

- Provide overview of the productization process. This has been done in the
downstream documentation.

## Proposal

### Personas

NOTE: The division by personas is done mainly with business use cases in mind - not technical.

- **Cluster admin** - Provisions cluster resources. Installs and spins up Cert Manager Operator.
- **Service Infrastructure Admin** - Provisions infrastructure for workload applications. 
Configures Certificate Issuers (using Issuer and ClusterIssuer CRs). Also, 
troubleshoots the configuration, e.g. inspects Order or Challenge CRs.
- **Service Developer** - Creates applications that use the infrastructure created by 
the Service Infrastructure Admin. Creates Certificate CRs.
- **Service End User** - A user who uses a Service developed by a Service Developer. 
Typically, connects using an Ingress or a Route. Less often, using a Load Balancer Service. 
In some scenarios it connects using a Global Load Balancer in front of an OCP cluster (e.g. F5).
- **OpenShift Engineering Team** - Use cases for OpenShift that minimize the maintenance burden.

### User Stories

| Persona                      | Use case description                                                                                                                            | Delivery plan                                                 | Notes                                                                                                                |
|------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Cluster Admin                | As a Cluster Admin I want to install and remove Cert Manager Operator using Operator Marketplace and by creating CRs.                           | Initial release                                               | Uninstall doesn’t work properly, see [AUTH-105](https://issues.redhat.com/browse/AUTH-105)                           |
| Cluster Admin                | As a Cluster Admin I want to avoid damaging the other Cert Manager Operators installed in the cluster.                                          | Initial release                                               | Delegated to OLM                                                                                                     |
| Cluster Admin                | As a Cluster Admin I want to be able to monitor both Cluster Manager and its Operator.                                                          | Consider for the future                                       ||
| Consider for the future      | As a cluster Admin I want to use the `oc get clusteroperators` command to check if the Cert Manager Operator is up and running.                 | Hold off till we reach consensus in Group B. Probably for GA. ||
| Service Infrastructure Admin | As a Service Infrastructure Admin I want to configure certificate issuers using CRs that are already installed for me by Cluster Admin.         | Initial release                                               ||
| Service Infrastructure Admin | As a Service Infrastructure Admin I want to configure routes and ingresses to use configured certificate issuers.                               | Initial release run only for Ingresses                        | This use case is shared with Service Developer                                                                       |
| Service Infrastructure Admin | As a Service Infrastructure Admin I want to automatically accept certificates created by Service Developers based on a policy.                  | Consider for the future                                       | See Cert Manager Approver Policy                                                                                     |
| Service Infrastructure Admin | As a Service Infrastructure Admin I want to automatically accept certificates created by Service Developers using cmctl tool.                   | Consider for the future                                       | See cmctl                                                                                                            |
| Service Developer            | As a Service Developer I want to get a new certificate based on pre-configured issuer.                                                          | Initial release                                               ||
| Service Developer            | As a Service Developer I want to configure routes and ingresses to use configured certificate issuers.                                          | Initial release runs only for Ingresses                       | This use case is shared with Service Infrastructure Admin                                                            |
| Service Developer            | As a Service Developer I want to use certificates issued by Cert Manager for in-cluster communication.                                          | Consider for the future                                       | See Cert Manager CSI Driver                                                                                          |
| Service Developer            | As a Service Developer I want to transparently renew my certificates.                                                                           | Initial release                                               | It is up to the application to automatically pick the TLS certificate changes from the Secret (e.g. via hot-reload). |
| Service End User             | As a Service End User I want to be able to connect to the Service protected by a Let’s Encrypt (ACME) certificate.                              | Consider for TP                                               | A duplicated use case from the ones mentioned the above                                                              |
| OpenShift Engineering Team   | As an OpenShift Engineering Team member I want to lower the maintenance burden on Service CA by replacing it with Cert Manager and CSI drivers. | Consider for the future (post GA)                             |


### Implementation Details/Notes/Constraints

OpenShift Auth Team will maintain a forked Jetstack Cert Manager (for the operand) and a Cert Manager Operator repos
inside OpenShift organization. Every commit into those repos should result in producing a new build in CPaaS. Our goal
is to make the whole process as seamless and transparent as possible.

### Risks and Mitigations

One of the biggest risks is poor support for supplying certificates into OpenShift Routes. At the moment,
the most convenient way to do it is to create an Ingress and let the Ingress Controller to convert it into a Route with
proper certs written into its spec. In the future, we might want to enhance this behavior. 

## Design Details

### Open Questions [optional]

N/A

### Test Plan

N/A - written in Polarion.

### Graduation Criteria

There are two milestones we plan for Cert Manager Operator:
* Tech Preview
* GA

The Tech Preview is tentatively targeted at the end of March. GA is targeted around OCP 4.11 GA.

#### Tech Preview -> GA

- Full end-to-end test coverage
- Implemented use cases marked for GA
- Propose OpenShift Cert Manager to Jetstack Cert Manager community

### Upgrade / Downgrade Strategy

There are two types of scenarios here:
* OCP platform upgrade
* Cert Manager Operator upgrade

Both cases are transparent for the Cert Manager Operator.

## Implementation History

TBD

## Alternatives

N/A - decided on the strategy level.
