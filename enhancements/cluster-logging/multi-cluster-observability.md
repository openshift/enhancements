---
title: multi-cluster-observability-addon
authors:
  - "@periklis"
  - "@JoaoBraveCoding"
  - "@pavolloffay"
  - "@iblancasa"
reviewers:
  - "@alanconway"
  - "@jcantrill"
  - "@xperimental"
  - "@jparker-rh"
  - "@berenss"
  - "@bjoydeep"
approvers:
  - "@alanconway"
  - "@jcantrill"
  - "@xperimental"
api-approvers:
  - "@alanconway"
  - "@jcantrill"
  - "@xperimental"
creation-date: 2023-11-28
last-updated: 2023-11-28
tracking-link:
  - https://issues.redhat.com/browse/OBSDA-356
  - https://issues.redhat.com/browse/OBSDA-393
  - https://issues.redhat.com/browse/LOG-4539
see-also:
  - None
replaces:
  - None
superseded-by:
  - None
---

# Multi-Cluster Observability Addon

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Multi-Cluster Observability has been an integrated concept in Red Hat Advanced Cluster Management (RHACM) since its inception but only incorporates one of the core signals, namely metrics, to manage fleets of OpenShift Container Platform (OCP) based clusters (See [RHACM Multi-Cluster-Observability-Operator (MCO)](rhacm-multi-cluster-observability)). The underlying architecture of RHACM observability consists of a set of observability components to collect a dedicated set of OCP metrics, visualizing them and alerting on fleet-relevant events. It is an optional but closed circuit system applied to RHACM managed fleets without any points of extensibility.

This enhancement proposal seeks to bring a unified approach to collect and forward logs and traces from a fleet of OCP clusters based on the RHACM addon facility (See Open Cluster Management (OCM) [addon framework](ocm-addon-framework)) by enabeling these signals events to land on third-party managed and centralized storage solutions (e.g. AWS Cloudwatch, Google Cloud Logging). The multi-cluster observability addon is an optional RHACM addon. It is a day two companion for MCO and does not necessarily share any resources/configuration with latter. It provides a unified installation approach of required dependencies (e.g. operator subscriptions) and resources (custom resources, certificates, CA Bundles, configuration) on the managed clusters to collect and forward logs and traces. The addon's name is Multi Cluster Observability Addon (MCOA).

## Motivation

The main driver for the following work is to simplify and unify the installation of log and trace collection and forwarding on an RHACM managed fleet of OCP clusters. The core utility function of the addon is to install required operators (i.e. [Red Hat OpenShift Logging](ocp-cluster-logging-operator) and [Red Hat OpenShift distributed tracing data collection](opentelemetry-operator)), configure required custom
resources (i.e. `Clusterlogging`, `ClusterLogForwarder`, `OpenTelemetryCollector`) and reconcile per-cluster-related companion resources (i.e. `Secrets`, `ConfigMaps` for per-cluster authentication and configuration). This enables centralized control of the fleet's logs and traces collection and forwarding capabilities.

### User Stories

* As a fleet administrator I want to install a homogeneous log collection and forwarding on any set of RHACM managed OCP clusters.
* As a fleet administrator I want to install a homogeneous trace collection and forwarding on any set of RHACM managed OCP clusters.
* As a fleet administrator I want to centrally control authentication credentials for the target log forwarding outputs.
* As a fleet administrator I want to centrally control authentication credentials for the target trace exporters.

### Goals

* Provide an optional RHACM addon to control logs and traces collection and forwarding on managed OCP clusters.
* Enable control of authentication and authorization of storage endpoints from the hub cluster.
* Enable TLS Certificate management local on the hub cluster for managed cluster TLS client certificates.

### Non-Goals

* Provide end-to-end experience to collect, forward, store and visualize logs and traces on the same RHACM fleet.
* Share MCO's ingestion and querying capabilities for logs and traces collected by MCOA.

## Proposal

The following sections describe in detail the required resources as well as the workflow to enable logs and traces collection and forwarding on an RHACM managed fleet of OCP clusters.

### Workflow Description

The workflow implemented in this proposal enables fleet-wide log/tracing collection and forwarding as follows:

1. The fleet administrator registers MCOA on RHACM using a dedicated `ClusterManagementAddOn` resource on the hub cluster.
2. The fleet administrator deploys MCOA on the hub cluster using a Red Hat provided Helm chart.
2. The fleet administrator creates a default `ClusterLogForwarder` stanza in the `open-cluster-management` namespace that describes the list of log forwarding outputs. This stanza will then be used as a template by MCOA when generating the `ClusterLogForwarder` instance per managed cluster.
3. The fleet administrator creates a default `OpenTelemetryCollector` resource in the `open-cluster-management` namespace that describes the list of trace exporters. This stanza will then be used as a template by MCOA when generating the `OpenTelemetryCollector` instance per managed cluster.
4. The fleet administrator creates a default `AddOnDeploymentConfig` resource in the `open-cluster-management` namespace that describes general addon parameters, i.e. operator subscription channel names that should be used on all managed clusters.
5. For each managed cluster the MCOA or the fleet administrator provides on the managed cluster namespace additional configuration resources:
   1. Per Log Output / Trace Exporter `Secret`: For each output a resource holding the authentication settings (See [ClusterLogForwarder Type: OutputSecretSpec](ocp-clusterlogforwarder-outputsecretspec), [OpenTelemetry Collector Authentication](opentelemetry-collector-auth))
   2. Per Log Output / Trace Exporter `ConfigMap`: For each output a resource holding output specific configuration (See [ClusterLogForwarder Type: OutputTypeSpec](ocp-clusterlogforward-outputtypespec), [OpenTelemetry Collector Authentication](opentelemetry-collector-auth))
   3. Each log output related resource needs to provide a special annotation `logging.openshift.io/target-output-name` that corresponds to name the target output spec in the default `ClusterLogForwarder` resource.
   4. Each trace exporter related resource needs to provide a special annotation `opentelemetry.io/v1alpha1/target-extension-name` that corresponds to name the target extension in the default `OpenTelemetryCollector` config spec.
6. The MCOA will render a `ManifestWorks` resource per cluster that consists of a rendered manifest list (i.e. `Subscription`, `ClusterLogForwarder`, `OpenTelemetryCollector` and accompanying `Secret`, `ConfigMap`).
7. The WorkAgentController on each managed cluster will apply each individual manifest from the `ManifestWorks` locally.

#### Variation and form factor considerations [optional]

TBD

### API Extensions

None as the MCOA is not providing any new custom reource definitions or changing any existing ones.

### Implementation Details/Notes/Constraints [optional]

The MCOA implementation sources three different set of manifests acompanying the addon registration and deployment on a RHACM hub cluster:
1. General configuration and fleet-wide stanzas: A `ClusterManagementAddon`, an `AddOnDeploymentConfig` and a `ClusterLogForwarder` (for logs) and/or `OpenTelemetryCollector` (for traces).
2. For multi-cluster logs collection and forwarding: Per log output `Secret` and/or `ConfigMap` resources.
3. For multi-cluster traces collection and forwarding: Per trace exporter `Secret` and/or `ConfigMap` resources.

#### General configuration and fleet-wide stanzas

To support the above workflow MCOA requires along the addon registration and installation two key resources:
- A `ClusterManagementAddOn` resource that describes which resources to be considered by the addon as source configuration.
- An `AddonDeploymentConfig` resource that describes which channel names to used by a managed cluster's Operator Lifecycle Manager (OLM) to install supported operators for logs and traces collection and forwarding.
- A `ClusterLogForwarder` resource that describes the log outputs per log type to be used for log collection and forwarding on all managed clusters.
- An `OpenTelemetryCollector` resource that describes the trace exporters to be used for traces collection and forwarding on all managed clusters.

In detail, the resources look as follows:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ClusterManagementAddOn
metadata:
 name: multi-cluster-observability-addon
spec:
 addOnMeta:
   displayName: Multi Cluster Observability Addon
   description: "multi-cluster-observability-addon is the addon to configure spoke clusters to collect and forward logs/traces to a given set of outputs"
 supportedConfigs:
   # Describes the general addon configuration applicable for all managed clusters. It includes:
   # - Default subscription channel name for install the `Red Hat OpenShift Logging` operator on each managed cluster.
   # - Default subscription channel name for install the `Red Hat OpenShift distributed tracing data collection` operator on each managed cluster.
   - group: addon.open-cluster-management.io
     resource: addondeploymentconfigs
     defaultConfig:
       name: multi-cluster-observability-addon
       namespace: open-cluster-management

   # Describe per managed cluster sensitive data per target forwarding location, currently supported:
   # - TLS client certificates for mTLS communication with a log output / trace exporter.
   # - Client credentials for password based authentication with a log output / trace exporter.
   - resource: secrets

   # Describe per managed cluster auxilliary config per log output / trace exporter.
   - resource: configmaps

   # Describes the default log forwarding outputs for each log type applied to all managed clusters.
   - group: logging.openshift.io/v1
     resource: ClusterLogForwarder
     # The default config is the main stanza of a ClusterLogForwarder resource
     # that describes where logs should be forwarded for all managed cluster.
     defaultConfig:
       name: instance
       namespace: open-cluster-management

   # Describe the default trace forwarding locations for each OTEL exporter applied to all managed clusters.
   - group: opentelemetry.io/v1alpha1
     resource: OpentelemetryCollector
     # The default config is the main stanza of an OpenTelemetryCollector resource
     # that describes local receivers, remote exporters and generic usable extensions.
     defaultConfig:
       name: instance
       namespace: open-cluster-management
```

and a default `AddonDeploymentConfig` describing the operator subscriptions to be used on each managed cluster:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: multi-cluster-observability-addon
  namespace: open-cluster-management
spec:
  customizedVariables:
    - name: cluster-logging-channel
      value: stable-5.8
    - name: otel-channel
      value: stable
```

#### Multi Cluster Log Collection and Forwarding

For all managed clusters the fleet administrator is required to provide a single `ClusterLogForwarder` resource stanza that describes the log forwarding configuration for the entire fleet in the default namespace `open-cluster-management`.

The following example resource describes a configuration for forwarding application logs to a Grafana Loki instance and infrastructure/audit logs to AWS Cloudwatch:

```yaml
apiVersion: logging.openshift.io/v1
kind: ClusterLogForwarder
metadata:
  name: instance
  namespace: open-cluster-management
spec:
  outputs:
  - loki:
      labelKeys:
      - log_type
      - kubernetes.namespace_name
      - kubernetes.pod_name
      - openshift.cluster_id
    name: application-logs
    type: loki
  - cloudwatch:
      groupBy: logType
      region: PLACEHOLDER # Use a placeholder value because field is required but will be set per cluster.
    name: cluster-logs
    type: cloudwatch
  pipelines:
  - inputRefs:
    - application
    name: application-logs
    outputRefs:
    - application-logs
  - inputRefs:
    - audit
    - infrastructure
    name: cluster-logs
    outputRefs:
    - cluster-logs
```

For each managed cluster the addon configuration that enables being considered by MCOA looks as follows. It references managed specific configuration resources, i.e. Secrets/ConfigMaps per `ClusterLogForwarder` output:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: multi-cluster-observability-addon
  namespace: managed-ocp-cluster-1
spec:
  installNamespace: open-cluster-management-agent-addon
  configs:
  # Secret with mTLS client certificate for application-logs output
  - resource: secrets
    name: managed-ocp-cluster-1-application-logs
    namespace: managed-ocp-cluster-1
  # ConfigMap with CloudWatch configuration for cluster-logs output
  - resource: configmaps
    name: managed-ocp-cluster-1-cluster-logs
    namespace: managed-ocp-cluster-1
```

For the `application-logs` log forwarding output the MCOA provides a Secret `managed-ocp-cluster-1-application-logs` with all required authentication information, e.g. TLS client Certificate:

```yaml
apiVersion: v1
kind: Secret
metadata:
  annotation:
    logging.openshift.io/target-output-name: "application-logs"
  name: managed-ocp-cluster-1-application-logs
  namespace: managed-ocp-cluster-1
data:
  url: "https://loki-outside-this-cluster.com:3100"
  'tls.crt': "Base64 encoded TLS client certificate"
  'tls.key': "Base64 endoded TLS key"
  'ca-bundle.crt': "Base64 encoded Certificate Authority certificate"
```

For `cluster-logs` log forwarding output the MCOA needs to provide a ConfigMap `managed-ocp-cluster-1-cluster-logs` with all output configuration, e.g. AWS Cloudwatch:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotation:
    logging.openshift.io/target-output-name: "cluster-logs"
  name: managed-ocp-cluster-1-cluster-logs
  namespace: managed-ocp-cluster-1
data:
  region: us-east-1
  groupPrefix: a-prefix
```

In turn the addon will compile a `ManifestWork` for the managed cluster `managed-ocp-cluster-1` as follows and pass it over it's WorkAgentController:

```yaml
kind: ManifestWork
metadata:
  name: addon-logging-ocm-addon-deploy-0
  namespace: managed-cluster-1
spec:
  workload:
    manifests:
    - apiVersion: operators.coreos.com/v1alpha1
      kind: Subscription
      metadata:
        name: cluster-logging
        namespace: openshift-logging
      spec:
        channel: stable-5.8 # Pulled from the AddOnDeploymentConfig
        installPlanApproval: Automatic
        name: cluster-logging
        source: redhat-operators
        sourceNamespace: openshift-marketplace
        startingCSV: cluster-logging.v5.8.0
    - apiVersion: v1
      kind: Secret
      metadata:
        annotation:
          logging.openshift.io/target-output-name: "application-logs"
        name: managed-ocp-cluster-1-application-logs
        namespace: openshift-logging
      data:
        'tls.crt': "Base64 encoded TLS client certificate"
        'tls.key': "Base64 endoded TLS key"
        'ca-bundle.crt': "Base64 encoded Certificate Authority certificate"
    - apiVersion: logging.openshift.io/v1
      kind: ClusterLogForwarder
      metadata:
        name: instance
        namespace: open-cluster-management
      spec:
        outputs:
        - loki:
            labelKeys:
            - log_type
            - kubernetes.namespace_name
            - kubernetes.pod_name
            - openshift.cluster_id
          name: application-logs
          type: loki
          url: "https://loki-outside-this-cluster.com:3100" # Pulled from the referenced secret
          secret:
            name: managed-ocp-cluster-1-application-logs
        - name: cluster-logs
          type: cloudwatch
          cloudwatch:
            groupBy: logType
            region: us-east-1 # Pulled from the `managed-ocp-cluster-1-cluster-logs` configmap
            groupPrefix: a-prefix # Pulled from the `managed-ocp-cluster-1-cluster-logs` configmap
        pipelines:
        - inputRefs:
          - application
          name: application-logs
          outputRefs:
          - application-logs
        - inputRefs:
          - audit
          - infrastructure
          name: cluster-logs
          outputRefs:
          - cluster-logs
```

#### Multi Cluster Trace Collection and Forwarding

TBD

#### Hypershift [optional]

TBD

### Drawbacks

TBD

## Design Details

### Open Questions [optional]

TBD

### Test Plan

TBD

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

None

### Version Skew Strategy

None

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

TBD

## Alternatives

TBD

## Infrastructure Needed [optional]

None

[ocm-addon-framework]:https://github.com/open-cluster-management-io/addon-framework
[ocp-cluster-logging-operator]:https://github.com/openshift/cluster-logging-operator
[ocp-clusterlogforwarder-outputsecretspec]:https://github.com/openshift/cluster-logging-operator/blob/627b0c7f8c993f89250756d9601d1a632b024c94/apis/logging/v1/cluster_log_forwarder_types.go#L226-L265
[ocp-clusterlogforward-outputtypespec]:https://github.com/openshift/cluster-logging-operator/blob/627b0c7f8c993f89250756d9601d1a632b024c94/apis/logging/v1/output_types.go#L21-L40
[opentelemetry-collector-auth]:https://opentelemetry.io/docs/collector/configuration/#authentication
[opentelemetry-operator]:https://console-openshift-console.apps.ptsirakiaws2311285.devcluster.openshift.com/github.com/open-telemetry/opentelemetry-operator
[rhacm-multi-cluster-observability]:https://github.com/stolostron/multicluster-observability-operator/
