---
title: observability-ui-operator
authors:
  - "@jgbernalp"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@kyoto"
  - "@zhuje"
  - "@alanconway"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@stleerh"
  - "@eparis"
api-approvers:
  - "@spadgett"
creation-date: 2023-09-18
last-updated: 2023-10-11
tracking-link:
  - https://issues.redhat.com/browse/OU-204
see-also:
  - ""
replaces:
  - ""
superseded-by:
  - ""
---

title: observability-ui-operator

# Observability UI Operator

The Observability UI Operator aims to manage dynamic console plugins for observability signals inside the OpenShift console, ensuring a consistent user experience and efficient management of UI plugins.

## Summary

This proposal introduces the Observability UI Operator, a tool designed to manage UI plugins related to observability signals within the OpenShift console. By centralizing the management of such plugins, we can offer a unified observability experience in the console and accommodate new use cases, all while decoupling UI responsibilities from other operators.

## Motivation

### Why:

The current state of observability signals in the OpenShift console has each operator responsible for its own console plugin. This sometimes results in operators deploying plugins outside their primary scope. As the requirements for the console's UI grow, there's a clear need for a centralized system that can manage diverse UI components spanning across various signals to offer a unified observability experience in the console.

### User Stories

- As an OpenShift user, I want an operator from the Red Hat catalog that can deploy various observability UI components so that all signals supported by the cluster are easily accessible and can be used for troubleshooting.

- As an OpenShift administrator, I want a centralized operator for observability UI components so that I can streamline console requirements and integrate diverse signals effectively.

- As an OpenShift user, I want to customize observability dashboards with various signals so that I can quickly identify and resolve issues.

### Goals

Provide a centralized operator to manage dynamic UI plugins for observability signals within the OpenShift console.

Allow OpenShift users and administrators to access various observability signals through integrated console plugins.

Decouple the responsibility of managing observability UI from operators, enabling each operator to focus solely on its primary functionalities.

Enhance the observability experience on the console by providing components like [Perses](https://github.com/perses/perses) for customizable dashboards and [korrel8r](https://github.com/korrel8r/korrel8r) for correlation.

Ensure that the Observability UI operator remains independent of the OpenShift Container Platform (OCP) release cycle, thus allowing more frequent updates with new features and fixes.

### Non-Goals

Deploying third-party Observability UI solutions like Grafana or Kibana.

Supporting UIs coming from other observability projects such as Jaeger UI, Prometheus/Thanos UI, or AlertManager UI.

Replacing or superseding plugins that have a clear 1:1 relationship with an existing operator such as:

- the monitoring plugin deployed by the Cluster Monitoring Operator. This plugin is responsible for displaying and managing alerts coming from the cluster monitoring stack.
- the network observability plugin deployed by the Network Observability Operator

## Proposal

The proposal is to introduce an Observability UI Operator that primarily aims at centralizing the management of dynamic console plugins dedicated to observability signals for the OpenShift console.

### Current State and Challenges:

Currently, individual operators, each responsible for specific observability signals, deploy their own console plugins in OpenShift, allowing users to access related signal data. Some operators end up deploying plugins not related to their primary function. For example, the logging operator might deploy a plugin exclusively using services from the Loki operator.

The current design sees a rapid expansion of the console's UI requirements, which now exceed just interfacing with the default signal sources. Users are demanding components that can work across various signals, encompassing features like customizable dashboards in Perses, and observability UIs that can read signals from multiple operators in a cluster, such as Advanced Cluster Management (ACM) or Service Telemetry Framework (STF).

A significant drawback of the existing system is that, for advanced features like customizable dashboards, UI components require backend services that do not fit seamlessly within the current operators.

### Design:

The Observability UI Operator will be available in the Red Hat catalog.

It will manage the deployment of several components which will be added incrementally based on priority, as shown in the table below:

| Component                          | Component Type         | Current functionality                                                                                                                                                                                                                                              | Planned functionality                                                                                                                          |
| ---------------------------------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Dashboards console plugin          | Console Dynamic Plugin | Allows the current dashboards to fetch data from other Prometheus services in the cluster, different from the cluster monitoring                                                                                                                                   | Integrate Perses UI for dashboards <br> Allow new dashboards to consume data from several data sources <br> Allow new dashboards customization |
| Logging view plugin                | Console Dynamic Plugin | Displays logs under the observe section for the admin perspective <br> Displays logs in the pod detail view <br> Displays logs as a tab in the observe menu for the dev perspective <br> Merges log-based alerts with monitoring alerts in the observe alerts view | Allow dev console users to see logs from several namespaces at the same time <br> Display log metrics                                          |
| Perses Operator                    | Operator               | N/A – not yet released                                                                                                                                                                                                                                             | Allow dashboards definitions as CRDs <br> Manage dashboards authorization <br> Manage schema migrations from grafana                           |
| Distributed tracing console plugin | Console Dynamic Plugin | N/A – not yet released                                                                                                                                                                                                                                             | Display a scatter plot with a list of traces <br> Display a gantt chart to display the detail of a trace                                       |
| Korrel8r console plugin            | Console Dynamic Plugin | N/A – not yet released                                                                                                                                                                                                                                             | Display a side panel to allow navigate across observability signals present on the cluster                                                     |

To maintain a level of flexibility and ensure compatibility, each plugin within this operator will come with a set of feature toggles. These toggles can be activated or deactivated based on the OCP version and the version of the respective signal operator.

The release cycle for the Observability UI Operator will be distinct from the OpenShift Container Platform's minor version releases. With this approach, we anticipate more frequent updates with newer features and fixes. Tentatively, minor releases are planned every 3 months, subject to change based on the specific features targeted for each release.

Migration plans will be crucial. Current plugins deployed by other signal operators are optional. These can be disabled and replaced by plugins from the Observability UI Operator. Existing operators deploying an observability plugin will need to direct customers towards migration to the Observability UI Operator and strategize for the phasing out of their plugin support.

### Console plugin configuration

The Observability UI Operator will be configured through a series of custom resource (CR) that will allow the user to enable or disable the console plugins and link them with the corresponding backend from signal operators.

### Objective:

The core intention behind this change is twofold:

To streamline the observability experience on the OpenShift console, offering users a unified and adaptable approach.

Decouple the UI responsibilities from specific operators, allowing them to focus exclusively on their main functions.

By centralizing the observability UI components under one operator, we hope to minimize redundancy, improve user experience, and cater more efficiently to the expanding requirements of the console's UI.

### Workflow Description

The workflow for the Observability UI Operator focuses on enhancing the user experience when interfacing with the OpenShift console. This proposal targets a more unified and enhanced observability experience through a new Kubernetes operator.

**OpenShift cluster administrator** is responsible for installing, enabling, configuring, and managing the plugins and operators within the OpenShift environment.
**OpenShift user** is the end-user interfacing with the OpenShift console and making use of the observability signals presented by the dynamic console plugins.

1. The cluster administrator installs the ObservabilityUI operator from the RedHat Catalog.
2. If there is an existing observability UI plugin deployed by another operator:
   - If the new plugin defines console feature flag to disable the existing plugin extensions, only the new plugin extensions will be enabled.
   - If the new plugin does not define console feature flag to disable the existing plugin extensions, the cluster administrator disables the existing plugin.
3. The cluster administrator configures the operator adding a custom resources (CR) to deploy the desired plugins and link them with the corresponding signal operators.
4. The operator will reconcile the necessary deployments for each enabled plugin, then it will reconcile the required [CRs for the console operator](https://github.com/openshift/enhancements/blob/master/enhancements/console/dynamic-plugins.md#delivering-plugins) so they become be available in the OpenShift console.
5. The user accesses the OpenShift console and interacts with the observability signals through the plugins deployed by the operator.

### API Extensions

This enhancement introduces a new CRD to represent observability UI console plugins. The `ObservabilityUIConsolePlugin` CR for a plugin will be defined as follows:

```yaml
apiVersion: observability-ui.openshift.io/v1alpha1
kind: ObservabilityUIConsolePlugin
metadata:
  name: logging-view-plugin
  namespace: openshift-observability-ui
spec:
  displayName: "Logging View Plugin"
  deployment:
    containers:
      - name: logging-view-plugin
        image: "quay.io/gbernal/logging-view-plugin:latest"
        ports:
          - containerPort: 9443
            protocol: TCP
  backend:
    type: logs
    sources:
      - alias: logs-backend
        caCertificate: '-----BEGIN CERTIFICATE-----\nMIID....'
        authorize: true
        endpoint:
          type: Service
          service:
            name: lokistack-dev
            namespace: openshift-logging
            port: 8080
  settings:
    timeout: 30s
    logsLimit: 300
```

#### Behavior Modification of Existing Resources:

No existing resources are modified by this operator. However, operators that previously deployed their own UI plugins may need to consider to point their users to the Observability UI Operator docs so they can continue using the plugin in the console.

#### Operational Aspects of API Extensions:

- Labels and Annotations: Owner references will be added to the deployments and console operator CRs so resources are cleaned up when an observability UI component is deleted.

- Compatibility and Feature Flags: The Observability UI operator will add a set of feature toggles to each plugin deployment to ensure that only compatible features are deployed. This compatibility is based on the OCP version and the signal operator version. See the [logging view plugin compatibility matrix](https://github.com/openshift/logging-view-plugin/tree/main#compatibility-matrix)

### Risks and Mitigations

| Risk                 | Description                                                                                                                                                                                                            | Mitigation                                                                                                                                                                                                                                                                |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Compatibility Issues | Given that each plugin will have feature toggles based on the OCP version and signal operator version, there is a potential for compatibility issues to arise, especially as more plugins and features are introduced. | Ensure robust testing mechanisms that simulate different environments and configurations. Maintain a detailed compatibility matrix and ensure that it is updated regularly. Provide a mechanism for users to report compatibility issues, and prioritize addressing them. |
| Migration Challenges | Existing operators deploying observability plugins might face challenges when migrating to the new Observability UI Operator.                                                                                          | Develop a step-by-step migration guide. Offer dedicated support during the initial migration phase to help teams seamlessly transition. Provide tooling, if possible, to automate or simplify parts of the migration process.                                             |

Review Processes:

- Security Review: Security will be reviewed by the ProdSec team. They will evaluate the architecture, perform vulnerability assessments, and validate the security practices adopted in the operator.

- UX Review: The UX team will review the design and user experience of the observability UI plugins. Feedback will be incorporated to ensure a seamless and consistent user experience across the OpenShift console.

- Stakeholder Engagement: Involve teams that work on Loki, Jaeger, Prometheus, and other related projects in the review process. Their feedback will be invaluable in understanding the broader implications and ensuring that the operator integrates well.

### Drawbacks

- An additional operator to install and upgrade might be a burden for users. However, the Observability UI Operator will be available in the Red Hat catalog, and it will be possible to install it and upgrade it within the console.
- There is an inherent dependence of the UI plugins with the signal operators. However, users will have the flexibility to choose which plugins they want to install and use.

## Design Details

### Open Questions

- How does the operator enables the plugins from the Observability UI Operator without having to patch the console operator? On going discussion with the console team: Plugins signed by Red Hat could be enabled by default.

### Test Plan

Alongside [Ginkgo](https://github.com/onsi/ginkgo) unit tests, we'll use [Cypress](https://github.com/cypress-io/cypress) for e2e tests on operator-created components, specifically the dynamic plugins toggles for a specific OpenShift console version from the [compatibility matrix](####operational-aspects-of-api-extensions). These e2e tests will run in a CI pipeline, and will verify plugin presence and correct toggles, while plugin functionality tests reside in their respective repositories.

### Graduation Criteria

The initial release offers a Dev preview of the operator, featuring:

- Dashboards console plugin
- Plugin compatibility tests
- Operator deployment script
- Basic installation and CRD plugin-enabling documentation

#### Dev Preview -> Tech Preview

- End user documentation
- Gather feedback from users
- Allow installation from the console catalog
- Sufficient test coverage
- Logging view plugin

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

As plugins are managed by the console operator, during upgrade or downgrade operations, plugins might be unavailable for a window when the console version and operator are at different versions.

The Observability UI Operator will redeploy the plugins when the console operator is updated as different versions of the console might enable different plugin features.

### Version Skew Strategy

To solve the version skew the Observability UI Operator will manage the plugin feature toggles based on the OCP version and the signal operator version. The compatibility between signal operators and the plugin is responsibility of the plugin.

In the case feature toggles are not enough because a different version of the plugin image is required, The Observability UI Operator can contribute multiple plugins with different version ranges to support different OpenShift versions. If the version ranges don't overlap, the console will only load the correct plugin.

### Operational Aspects of API Extensions

- Each plugin managed by the operator is expected to create only one instance of a CRD. Therefore, it's anticipated that there will be no significant impact on the overall API throughput.

#### Failure Modes

- When the plugin deployment does not serve the plugin files the console will show a default error message but the console and cluster will continue to work as expected.
- If the plugin deployment fails, the operator will retry the deployment until it succeeds or the operator or the CR are deleted.

#### Support Procedures

- When deleting the ObservabilityUIConsolePlugin, the console CR and the plugin deployment will be deleted, with no impact on the cluster.
- When plugins are re enabled the deployment will be recreated and the console shows a message to refresh the page to see the changes.

## Implementation History

N/A

## Alternatives

- Deliver the plugins as part of the signal operators. This is the current behavior for some operators such as the logging operator, in which an unrelated plugin is deployed. This approach would require the signal operators to maintain the plugins and would increase the complexity of the signal operators.
- Include the plugins as part of the console operator. The console team created the dynamic plugins to give other teams the flexibility to extend the console without having to patch the console operator. This approach would require the console team to maintain the plugins and would increase the complexity of the console operator.
