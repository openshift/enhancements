---
title: enable-network-observability-on-day-0
authors:
  - "@stleerh"
reviewers:
  - "@jotak"
  - "@jpinsonneau"
  - "@memodi"
  - "Mike Fiedler"
  - "@pavolloffay"
  - "@jan--f"
  - "@abhat"
  - "@simonpasquier"
  - "@everettraven"
approvers:
  - "@jotak"
  - "@dave-tucker"
api-approvers:
  - "@jotak"
  - "@dave-tucker"
  - "@everettraven"
creation-date: 2025-09-30
last-updated: 2026-02-25
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2469
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Enable Network Observability on Day 0

## Summary

This feature enhancement makes Network Observability available on day 0 by default.  That is, Network Observability is up and running after you create an OpenShift cluster using `openshift-install`.  It installs Network Observability Operator and creates a basic FlowCollector instance.  There is an option to turn this off.  It also makes it easy to have Network Observability available on day 1.

## Motivation

Network Observability is an optional OLM operator that collects and stores traffic flow information and provides insights into your network traffic, including troubleshooting features like packet drops, latencies, DNS tracking, and more.

### User Stories

* As a cluster admin or developer, I expect to be able to observe and manage my network traffic without having to install other components.  It should just be there.
* As a cluster admin, I should be able to see the networking health of my cluster after creating it.
* As a customer support engineer, I want the customer to be aware that Network Observability exists and can provide insights into their network traffic, including the ability to troubleshoot a number of networking issues.

These are the related issues for this feature enhancement.

* (Feature) [OCPSTRAT-2469](https://issues.redhat.com/browse/OCPSTRAT-2469) Provide a default OpenShift install experience for Network Observability
* (Epic) [NETOBSERV-2454](https://issues.redhat.com/browse/NETOBSERV-2454) Install Network Observability operator by default on OpenShift clusters
* (Spike) [NETOBSERV-2236](https://issues.redhat.com/browse/NETOBSERV-2236) What it would take to enable Network Observability by default in the console
* (PoC) [NETOBSERV-2247](https://issues.redhat.com/browse/NETOBSERV-2247) Have network observability be available and enabled on day 0

### Goals

Being able to manage and observe the network in an OpenShift cluster is critical in maintaining the health and integrity of the network.  Without it, there’s no easy way to verify whether your changes are working as expected or whether your network is experiencing issues.

Currently, Network Observability is an optional operator and a majority of customers do not have Network Observability installed.  Customers are missing out on features that they should have and have already paid for.

Network observability should be an integral part of networking and not thought of as a separate component.  You shouldn't have to ask, "Do I need observability?" any more than you would ask "Do I need security?"  Because it requires resources, basic observability should exist and additional features can be enabled as needed.  There are a few scenarios where you might not want Network Observability, so there is an easy way to opt out.

There is no one-size-fits-all solution in terms of configuring Network Observability, but the goal is to keep this part simple, while still providing as much value as possible given the constraints, and make it an easy way to change parameters on day 2.

### Non-Goals

There are other proposals to make Network Observability more visible and prominent, such as displaying a panel that would describe the features of Network Observability and provide a button to install it.  However, this feature enhancement addresses [OCPSTRAT-2469](https://issues.redhat.com/browse/OCPSTRAT-2469) that explicitly calls for Network Observability to be up and running after install.

There is a separate effort to add Network Observability to OpenShift Assisted Installer ([NETOBSERV-2486](https://issues.redhat.com/browse/NETOBSERV-2486)).  That addresses some installation cases but not all.

Network Observability Operator manages the components, such as flowlog pipelines.  Therefore, there is no need to consider the lifecycle management, since that will not change.

## Proposal

There are three OpenShift repositories that this proposal changes.  They are [openshift/api](https://github.com/openshift/api), [openshift/cluster-network-operator](https://github.com/openshift/cluster-network-operator), and [openshift/install](https://github.com/openshift/installer).

### Repository: openshift/api

The openshift/api repository is a shared repository for defining the API.  This adds the `networkObservability` field and a nested `installationPolicy` field in the Network Custom Resource Definition (CRD) under the spec section.

```yaml
apiVersion: config.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  networkObservability:
    installationPolicy: InstallAndEnable | DoNotInstall
```

This allows flexibility for future growth as opposed to having a simple true/false field.

Listing 1: Network manifest

If the value is `InstallAndEnable` or doesn't exist, Network Observability is enabled.  That is, Network Observability will be installed and a FlowCollector custom resource will be created (more details below).  If it is set to `DoNotInstall`, Network Observability is not enabled or to be precise, *nothing is done*.  It doesn’t remove Network Observability if it is set to `DoNotInstall`.

### Repository: openshift/cluster-network-operator

The actual enabling of Network Observability is done in the Cluster Network Operator (CNO).  The rationale is that we want the network observability feature to be part of networking.  This is as opposed to being part of the general observability or as a standalone entity.  Yet, there is still a separation at the lower level so that the two can be independently developed and released at different times, particularly for bug fixes.

In CNO, it adds a new controller for observability and adds it to the manager.  The controller is a single Go file where the Reconciler reads the state of the `installationPolicy` field.  If set to `InstallAndEnable`, it does the following:

1. Check if Network Observability Operator (NOO) is installed. If yes, exit.
2. Install NOO using OLM's OperatorGroup and Subscription.
3. Wait for NOO to be ready.
4. Create the "netobserv" namespace if it doesn't exist.
5. Check if a FlowCollector instance exists. If yes, exit.
6. Create a FlowCollector instance.

The Reconciler leverages the existing framework and reuses the concept of client, scheme, and manager.  It provides a clear ownership by having a separate controller for it.  If the Network CR changes, the Reconciler will repeat the above steps.  Note it doesn’t monitor NOO or any of NOO's components for changes, and it doesn’t do any upgrades.  That is still the responsibility of NOO.

### Repository: openshift/install

The openshift/install repository contains the source code for the **openshift-install** binary.  This adds the same fields as in the Network CRD but under the existing `networking` section in the **install-config.yaml** file.

```yaml
apiVersion: v1
baseDomain: devcluster.openshift.com
networking:
  networkObservability:
    installationPolicy: InstallAndEnable | DoNotInstall
```

Listing 2: install-config.yaml

The `installationPolicy` value is passed on to CNO to set the field of the same name in the Network CRD.  If this field is set to `InstallAndEnable` or doesn’t exist, it sets the Network CR’s `installationPolicy` field to `InstallAndEnable`.  To *not* enable Network Observability, set it to `DoNotInstall`.  This then sets the Network CR’s `installationPolicy` field to `DoNotInstall`.

### FlowCollector Custom Resource (CR)

Here is the FlowCollector Custom Resource (CR) that is instantiated.

```yaml
apiVersion: flows.netobserv.io/v1beta2
kind: FlowCollector
metadata:
  name: cluster
spec:
  agent:
    ebpf:
      features:
        - DNSTracking
      sampling: 400
    type: eBPF
  deploymentModel: Service
  loki:
    enable: false
  namespace: netobserv
```

Listing 3: FlowCollector configuration

Other eBPF features were considered, but the criteria was to avoid features that needed privilege mode and features that consumed significant resources.

Summary:

* Sampling at 400
* No Loki
* No Kafka
* DNSTracking feature enabled

### Workflow Description

Network Observability is enabled by default on day 0 (planning stage).  You don’t have to configure anything when using `openshift-install`, and Network Observability Operator will be installed and a FlowCollector custom resource (CR) will be created (Listing 3 above).

If you don’t want Network Observability enabled, first create the **install-config.yaml** file using the command below.

`$ openshift-install create install-config`

Then add the following as shown in Listing 4.

```
networking:
  networkObservability:
    installationPolicy: DoNotInstall
```

Listing 4: Don't enable Network Observability in install-config.yaml

Here's an alternate approach.  Using your **install-config.yaml** file, you can create manifests from it and add the change there instead.  To create the manifests, enter:

`$ openshift-install create manifests`

This creates a **manifests** directory.  Of particular relevance in this directory is a file named **cluster-network-02-config.yml**, which is the Network CR.  Under the `spec` section, add the following as shown in Listing 5.

```
spec:
  networkObservability:
    installationPolicy: DoNotInstall
```

Listing 5: Don't enable Network Observability in Network CR

Finally, to create the cluster, enter:

`$ openshift-install create cluster`

When you bring up the OpenShift web console, you should see that NOO is installed just as it would be if you had gone to **Ecosystem > Software Catalog** to install **Network Observability** from Red Hat (not the Community version).  In **Installed Operators**, there should be a row for **Network Observability**.  In the **Observe** menu, there should be a panel named **Network Traffic**.

The Technology Preview (TP) release will have a feature gate named `NetworkObservabilityInstall` that needs to be enabled.  To enable this on day 0, enter:

```
$ openshift-install create install-config
$ openshift-install create manifests
```

Now create a file named **99-feature-gate.yml** in the **manifests** directory with the following:

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
      - NetworkObservabilityInstall
```

Then enter:

`$ openshift-install create cluster`

If you have a running cluster, you can update the feature gate by entering `oc edit featuregate` and make the changes shown above.

At General Availability (GA), the feature gate for this feature will be enabled by default, so you no longer need to modify the FeatureGate resource.

### API Extensions

See Listing 2 above for the changes to Network CRD.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This proposal doesn't change how Network Observability works in a Hosted Control Plane (HCP) environment. Network Observability is supported on host clusters and the management cluster, therefore it will be enabled by default.

#### Standalone Clusters

This proposal applies to standalone clusters.

#### Single-node Deployments or MicroShift

Due to resource constraints, Single Node OpenShift (SNO) is an exception and will not be enabled by default.

MicroShift is not supported since Network Observability and CNO are not supported on that platform.

#### OpenShift Kubernetes Engine

OpenShift Kubernetes Engine is supported.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

* Network Observability requires CPU, memory, and storage that the customer might not be aware of.  See the Test Plan section for the target goals.

  **Mitigation:** The default setting stores only metrics at a high sampling interval to minimize the use of resources. If this isn’t sufficient, more fine-tuning and filtering can be done in the provided default configuration (e.g. filtering on specific interfaces only).

* Some of the Network Observability features aren’t enabled in order to use minimal resources.  Therefore, users might not know about these features.

  **Mitigation:** Determine what features, particularly related to troubleshooting, can be enabled with minimal CPU and memory impact. Mention other features in the panels.

### Drawbacks

Rather than actually installing NOO and creating the FlowCollector instance, it is less risky and simpler to just display a panel or a button to let the user install and enable Network Observability.  This resolves the awareness issue.  However, it goes against the principle that networking and network observability should always go hand-in-hand and be there from the start.

## Alternatives (Not Implemented)

### Alternative #1: Make NOO a core component of OpenShift

Rather than have CNO enable Network Observability, take the existing Network Observability Operator (NOO) and have it be installed by default in the cluster.  There needs to be some logic to accept the values in openshift-install to decide whether NOO should be enabled or not.

The core components of OpenShift are operators like Cluster Network Operator (CNO) and Cluster Storage Operator (CSO).  NOO is a much smaller component and should not reside at the top level.

### Alternative #2: Have COO enable Network Observability

Instead of CNO enabling Network Observability, have Cluster Observability Operator (COO) do it instead.  COO is becoming the operator and the central place for core observability components to be installed.  In addition, it provides services like metrics, Perses for dashboards, and troubleshooting via Korrel8r (Observability Signal Correlation).

A critical issue is that COO is itself an optional operator, so it can’t enable Network Observability on day 0, because it has to be installed first.  The central question is, "Is Network Observability part of OpenShift Networking or part of Cluster Observability?"  The answer is the former.  Component-based observability, such as Network Observability, is a layer on top of COO rather than a part of COO.

### Alternative #3: Have CVO enable Network Observability

Similar to alternative #1, this explicitly suggests having the Cluster Version Operator (CVO) enable Network Observability.  CVO currently manages larger scope operators that represent core cluster functions, such as CNO and CSO, rather than specific operators like Network Observability.

## Test Plan

The test plan will consider the following:

- Different architectures
- Different size clusters
- Hosted Control Plane (HCP) environment
- e2e tests in [OpenShift Release Tooling](https://github.com/openshift/release)

Performance testing will be done to optimize the use of resources and to determine the specific FlowCollector settings, with the goal of using less than 5% resources (CPU and memory) and an ideal target of less than 3%, including external components that are affected.

## Graduation Criteria

### Dev Preview -> Tech Preview

Network Observability reached GA back in January 2023.  Because the feature is to simply enable Network Observability, which has already existed for 3+ years, the plan is to forego the Dev Preview and go directly to Tech Preview.

### Tech Preview -> GA

There are many different customer scenarios and cluster profiles.  The Tech Preview will allow us to gauge the customer responses and make optimizations to the FlowCollector configuration or even the Network CRD if necessary.  To enable the feature gate for this feature, see the **Workflow Description** above.

Here are the GA requirements.

* [NETOBSERV-2533](https://issues.redhat.com/browse/NETOBSERV-2533) Performance testing in Loki-less mode with default settings
    - Provide guidance on CPU, memory, and storage resources
    - Measure the impact on Prometheus in the In-Cluster Monitoring
    - Optimize the default FlowCollector configuration
* [NETOBSERV-2534](https://issues.redhat.com/browse/NETOBSERV-2534) Have a way to pause Network Observability functions
* [NETOBSERV-2535](https://issues.redhat.com/browse/NETOBSERV-2535) Security audit on Network Observability code
* [NETOBSERV-2428](https://issues.redhat.com/browse/NETOBSERV-2428) New Service deployment model
* User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The upgrade strategy is treated like any other feature.  At Tech Preview, you will need to enable the feature gate for this feature.  At GA, Network Observability will be enabled by default without additional user intervention.

On a downgrade, it will no longer enable Network Observability.  If Network Observability Operator and/or FlowCollector exists, they will remain and will not be removed.

## Version Skew Strategy

There are no issues with version skew, since the logic to enable Network Observability only resides in CNO.

## Operational Aspects of API Extensions

N/A

## Support Procedures

Check the CNO logs and search for "observability\_controller.go" to determine whether Network Observability did or did not get enabled.  This will also be reported in the Status conditions.
