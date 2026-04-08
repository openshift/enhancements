# OpenShift Glossary

**Last Updated**: 2026-04-08  

## A

**API Server**: Kubernetes component that exposes the K8s API, front-end for control plane

**Admission Webhook**: HTTP callback for validating or mutating resources before persistence

**ADR**: Architectural Decision Record - document explaining significant architectural choice

## C

**ClusterOperator**: OpenShift resource for operators to report status to CVO

**ClusterVersion**: OpenShift resource representing cluster version and upgrade state

**Controller**: Control loop that watches resources and reconciles actual vs desired state

**CRD**: CustomResourceDefinition - extends Kubernetes API with custom resource types

**CRI**: Container Runtime Interface - API for container runtimes

**CVO**: Cluster Version Operator - coordinates platform upgrades

## E

**E2E Test**: End-to-end test on full cluster validating user workflows

**etcd**: Distributed key-value store used by Kubernetes for cluster state

**Enhancement**: Proposal document for new OpenShift feature

## F

**Finalizer**: Annotation preventing resource deletion until cleanup completes

## I

**Ignition**: Declarative configuration format for node provisioning (RHCOS)

**Ingress**: Kubernetes resource for HTTP routing to Services

## K

**kubeconfig**: Configuration file for accessing Kubernetes clusters

**kubelet**: Node agent that runs Pods and reports to control plane

## L

**Leader Election**: Pattern ensuring only one controller instance is active

**LGTM**: "Looks Good To Me" - code review approval

## M

**Machine**: OpenShift resource representing a node (machine-api)

**MachineConfig**: OpenShift resource for OS configuration

**MachineConfigPool**: Group of nodes with same MachineConfig

**MCO**: Machine Config Operator - manages node OS configuration

**Must-Gather**: Diagnostic data collection pattern for debugging

## O

**OLM**: Operator Lifecycle Manager - manages operator installation/upgrades

**Operator**: Kubernetes controller implementing operational knowledge

**OWNERS**: File defining code reviewers and approvers

## P

**Pod**: Smallest deployable unit in Kubernetes (group of containers)

**Prow**: Kubernetes CI/CD system used by OpenShift

## R

**RBAC**: Role-Based Access Control - permissions for Kubernetes resources

**Reconciliation**: Process of making actual state match desired state

**Route**: OpenShift resource for exposing Services externally (alternative to Ingress)

## S

**SCC**: SecurityContextConstraints - OpenShift security policy for Pods

**SDN**: Software Defined Networking - OpenShift network implementation (deprecated)

**Service**: Kubernetes resource providing stable network endpoint for Pods

**ServiceMonitor**: Prometheus resource for scraping metrics

**SLI**: Service Level Indicator - measured metric

**SLO**: Service Level Objective - target for SLI

## W

**Webhook**: HTTP callback for admission control or conversion

## Acronyms

| Acronym | Full Name |
|---------|-----------|
| **API** | Application Programming Interface |
| **AWS** | Amazon Web Services |
| **CA** | Certificate Authority |
| **CI/CD** | Continuous Integration / Continuous Delivery |
| **CNI** | Container Network Interface |
| **CRD** | CustomResourceDefinition |
| **CRI** | Container Runtime Interface |
| **CSI** | Container Storage Interface |
| **CVO** | Cluster Version Operator |
| **DNS** | Domain Name System |
| **E2E** | End-to-End |
| **HA** | High Availability |
| **LGTM** | Looks Good To Me |
| **MCO** | Machine Config Operator |
| **OLM** | Operator Lifecycle Manager |
| **OVN** | Open Virtual Network |
| **PR** | Pull Request |
| **RBAC** | Role-Based Access Control |
| **SCC** | SecurityContextConstraints |
| **SDN** | Software Defined Networking |
| **SLI** | Service Level Indicator |
| **SLO** | Service Level Objective |
| **TLS** | Transport Layer Security |

## See Also

- [Repository Index](./repo-index.md) - Find components
- [API Reference](./api-reference.md) - Platform APIs
- [Enhancement Index](./enhancement-index.md) - Feature proposals
