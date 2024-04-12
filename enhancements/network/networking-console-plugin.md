---
title: networking-console-plugin
authors:
  - "@upalatucci"
reviewers:
  - "@orenc1"
  - "@zshi-redhat"
  - "@pcbailey"
  - "@metalice"
  - "@spadgett"
  - "@jhadvig"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2024-04-12
last-updated: 2024-04-12
tracking-link:
  - https://issues.redhat.com/browse/CONSOLE-3952
---


# Create console plugin for the networking section



## Summary


At present, several pages within the networking section of our console are defined within the console repository. 
However, working with the console repository has slowed down development and there are multiple motivations to convert static console plugins into dynamic ones described [here](https://github.com/spadgett/enhancements/blob/master/enhancements/console/dynamic-plugins.md#motivation). 
To address this, we propose relocating the relevant code to a separate repository named "networking-console-plugin" and integrating it into the cluster-networking-operator.
Specifically: Services, Route, Ingress, NetworkPolicy, and NetworkAttachmentDefinition pages for listing, creating, and editing those resources.

## Motivation

This section is for explicitly listing the motivation, goals and non-goals of
this proposal. Describe why the change is important and the benefits to users.

### User Stories

N/A. For the end user the UI will be the same. The cluster admin will have the option to disable and enable the plugin but it's not relevant. 


### Goals


Enhanced Development Efficiency: By isolating networking console pages into a dedicated repository, developers can expedite development cycles and foster faster iterations, ultimately enhancing overall development efficiency.

Improved Maintainability: Segregating the networking console pages from the main console repository will simplify maintenance and testing efforts.

Dedicated Team: Currently, backend networking features are progressing faster than the corresponding UI console pages. This change will make the CNV UI team directly responsible for enhancing pages and making future features easy to use and discover.

### Non-Goals

N/A

## Proposal

To execute the migration of networking console pages to the "networking-console-plugin" repository and integrate them into the cluster-networking-operator, we propose the following steps:

Create "networking-console-plugin" Repository (Done): Establish a new repository named "networking-console-plugin" to house the codebase for networking console pages. This repository will serve as the centralized location for all networking-related functionalities.

Move Networking Console Pages (Done): Transfer relevant code files and resources from the console repository to the "networking-console-plugin" repository and refactor them. Ensure accurate migration of dependencies and configurations to maintain seamless functionality.

Integration with Cluster-Networking-Operator: Modify the cluster-networking-operator to incorporate the networking console plugin as an operand. Update configurations and references accordingly to seamlessly integrate the networking pages with the operator.

Incorporation into OpenShift Release Payload: Upon successful migration and integration, include the networking console plugin in the OpenShift release payload. This ensures the plugin is delivered by default with the console, providing users immediate access to enhanced networking functionalities.


### Workflow Description

N/A

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A
### Implementation Details/Notes/Constraints

N/A

### Risks and Mitigations

N/A

### Drawbacks

This means extra work in the beginning to integrate the operatand with the operator. 

## Test Plan

E2e tests will run at every pr on the networking-console-plugin using prow just like other console dynamic plugins

## Graduation Criteria


N/A

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A
## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives

Continue to work in the console using a static plugin. 

## Infrastructure Needed [optional]

N/A



