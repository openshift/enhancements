---
title: windows-node-egress-proxy
authors:
  - "@saifshaikh48"
  - "@mansikulkarni96"
reviewers:
  - "@openshift/openshift-team-windows-containers"
  - "@openshift/openshift-team-network-edge, for general approach to proxy config consumption, motivation, risks, and testing"
approvers:
  - "@aravindhp"
api-approvers:
  - None
creation-date: 2023-02-16
last-updated: 2023-08-29
tracking-link:
  - "https://issues.redhat.com/browse/OCPBU-22"
  - "https://issues.redhat.com/browse/WINC-802"
see-also:
  - "https://github.com/openshift/enhancements/blob/master/enhancements/proxy/global-cluster-egress-proxy.md"
  - Cluster-wide Egress Proxy Initiative doc: "https://docs.google.com/document/d/12bBF7GTgscW8B3apVU2WagtQpWh-PWk2tOWiD0dsdfU/edit"
  - Proxy Bootstrap Workflow: "https://docs.google.com/document/d/1y0t0yEOSnKc4abxsjxEQjrFa1AP8iHcGyxlBpqGLO08/edit#heading=h.y6ieif41wmlc"
  - Operator Proxy Support Dev Workflow: "https://docs.google.com/document/d/1otp9v5KkoOgq5vhN7ieBpdFhRIIy6z2yuaS_0rjm6wQ/edit#heading=h.dzcnuz4qv07o"
---

# Windows Node Cluster-Wide Egress Proxy

## Release Sign-off Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement proposal is to allow Windows nodes to consume and use global egress proxy configuration
when making external requests outside the cluster's internal network. OpenShift customers may require that external
traffic is passed through a proxy for security reasons, and Windows instances are no exception. There already exists a
protocol for publishing [cluster-wide proxy](https://docs.openshift.com/container-platform/4.12/networking/enable-cluster-wide-proxy.html)
settings, which is consumed by different OpenShift components (Linux worker nodes and infra nodes, CVO and OLM managed
operators) but Windows worker nodes do not currently consume or respect proxy settings. This effort will work to plug
feature disparity by making the [Windows Machine Config Operator](https://github.com/openshift/windows-machine-config-operator)
(WMCO) aware of cluster proxy settings at install time and reactive during runtime.

## Motivation

The motivation here is to expand the Windows containers production use case, enabling users to add Windows nodes and run
workloads easily and successfully in a proxy-enabled cluster. This is an extremely important ask for customer
environments where Windows nodes need to pull images from registries secured behind the client's proxy server or make
requests to off-cluster services, and those that use a
[custom public key infrastructure](https://docs.openshift.com/container-platform/4.12/networking/configuring-a-custom-pki.html).

### Goals

* Create an automated mechanism for WMCO to consume global egress proxy config from existing platform resources, including:
  + [Proxy connection information](https://docs.openshift.com/container-platform/4.12//rest_api/config_apis/proxy-config-openshift-io-v1.html#spec)
  + [Additional certificate authorities](https://docs.openshift.com/container-platform/4.12//rest_api/config_apis/proxy-config-openshift-io-v1.html#spec-trustedca) required to validate the proxy's certificate
* Configure the proxy settings in WMCO-managed components on Windows nodes (kubelet, containerd runtime)
* React to changes to the cluster-wide proxy settings during WMCO runtime
* Maintain normal functionality in non-proxied clusters

### Non-Goals

* First-class support/enablement of proxy utilization for user-provided applications
* *ingress* and reverse proxy settings are out of scope
* Monitor cert expiration dates or automatically replace expired CAs in the cluster's trust bundle

## Proposal

There are two major undertakings:
- Adding proxy environment variables (`NO_PROXY`, `HTTP_PROXY`, and `HTTPS_PROXY`) to Windows nodes and WMCO-managed Windows services.
- Adding the proxy’s trusted CA certificate bundle to each Windows instance's local trust store.

Since WMCO is a day 2 operator, it will pick up proxy settings during runtime regardless of if proxy settings were set
during cluster install time or at some point during the cluster's lifetime. When global proxy settings are updated, WMCO will react by:
- overriding proxy vars on the instance with the new values
- copying over the new trust bundle to Windows instances and updating each instance's local trust store (old certs should be removed) 

All changes detailed in this enhancement proposal will be limited to the Windows Machine Config Operator and its 
sub-component, Windows Instance Config Daemon (WICD).

### User Stories

User stories can also be found within the node proxy epic: [WINC-802](https://issues.redhat.com/browse/WINC-802)

### Workflow Description and Variations

**cluster creator** is a human user responsible for deploying a cluster.
**cluster administrator** is a human user responsible for managing cluster settings including network egress policies.

There are 3 different workflows that affect the cluster-wide proxy use case.
1. A cluster creator specifies global proxy settings at install time
2. A cluster administrator introduces new global proxy settings during runtime in a proxy-less cluster
3. A cluster administrator changes or removes existing global proxy settings during cluster runtime

The first scenario would occur through their [install-config.yaml](https://docs.openshift.com/container-platform/4.12/networking/configuring-a-custom-pki.html#installation-configure-proxy_configuring-a-custom-pki).
The latter 2 scenarios occur through changing the [`Proxy` object named `cluster`](https://docs.openshift.com/container-platform/4.12/networking/enable-cluster-wide-proxy.html#nw-proxy-configure-object_config-cluster-wide-proxy)
or by modifying certificates present in their [trustedCA ConfigMap](https://docs.openshift.com/container-platform/4.12/security/certificates/updating-ca-bundle.html#ca-bundle-replacing_updating-ca-bundle).

In all cases, Windows nodes can be joined to the cluster after altering proxy settings, which would result in WMCO
applying proxy settings during initial node configuration. In the latter 2 scenarios, Windows nodes may already exist in
the cluster, in which case WMCO will react to the changes by updating the state of each instance.

### Risks and Mitigations

The risks and mitigations are similar to those on the [Linux side of the cluster-wide proxy](https://github.com/openshift/enhancements/blob/master/enhancements/proxy/global-cluster-egress-proxy.md#risks-and-mitigations).
Although cluster infra resources already do a best effort validation on the user-provided proxy URL schema and CAs,
a user could provide non-functional proxy settings/certs. This would be propagated to their Windows nodes and workloads,
taking down existing application connectivity and preventing new Windows nodes from being bootstrapped.

### Drawbacks

The only drawbacks are the increased complexity of WMCO and the potential complexity of debugging customer cases that
involve a proxy setup, since it would be extremely difficult to set up an accurate replication environment.
This can be mitigated by proactively getting the development team, QE, and support folks familiar with the expected
behavior of Windows nodes/workloads in proxied clusters, and comfortable spinning up their own proxied clusters.

#### Support Procedures

In general the support procedures for WMCO will remain the same. There are two underlying mechanisms we rely on, the
publishing of proxy config to cluster resources and the consuming of the published config. If either of the underlying
mechanisms fail, Windows nodes will become proxy unaware. This could involve an issue with the user-provided proxy
settings, the cluster network operator, OLM, or WMCO. This would result in all future egress traffic circumventing the
proxy, which could affect inter-pod communication, existing application availability, and security. Also, the pause
image may not be able to be fetched, preventing new Windows nodes from running workloads. This would require manual
intervention from the cluster admin or a new release fixing whatever bug is causing the problem.

### API Extensions

N/A, as no CRDs, admission and conversion webhooks, aggregated API servers, or finalizers will be added or modified.
Only the WMCO will be extended which is an optional operator with its own lifecycle and SLO/SLAs, a
[tier 3 OpenShift API](https://docs.openshift.com/container-platform/4.12/rest_api/understanding-api-support-tiers.html#api-tiers_understanding-api-tiers).

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

## Design Details

### Configuring Proxy Environment Variables

As it stands today, the source of truth for cluster-wide proxy settings is the `Proxy` resource with the name `cluster`.
The contents of the resource are both user-defined, as well as adjusted by the cluster network operator (CNO). Some
platforms require instances to access certain endpoints to retrieve metadata for bootstrapping. CNO has logic to inject
additional `no-proxy` entries such as `169.254.169.254` and `.${REGION}.compute.internal` into the `Proxy` resource.

OLM is a subscriber to these `Proxy` settings -- it forwards the settings to the CSV of managed operators, so the WMCO
container will automatically get the required `NO_PROXY`, `HTTP_PROXY`, and `HTTPS_PROXY` environment variables on startup.
In fact, OLM will update and restart the operator pod with proper environment variables if the `Proxy` resource changes.

In proxy-enabled clusters, WMCO will read the values of the 3 proxy variables from its own environment and store them in 
the name-value pairs within the `EnvironmentVars` key found in the [`windows-services` ConfigMap](./health-management.md#services-configmap) spec.
However, in clusters without a global proxy, these variables will not be present in the services ConfigMap.
WICD will periodically poll to check if the proxy vars have changed by comparing each variable's value on the node to 
the expected value in the ConfigMap. If there is a discrepancy, WICD controller will reconcile and update the 
proxy environment variable on the Windows instances.

WMCO also specifies a list of environment variables monitored by WICD through the `WatchedEnvironmentVars` key in the
services ConfigMap spec. This list will now include `NO_PROXY`, `HTTP_PROXY`, and `HTTPS_PROXY` as the names of proxy
specific environment variables watched by WICD.
When a proxy variable is removed from the cluster-wide proxy settings in the `Proxy` resource, WICD will take corrective 
action to remove the proxy variable from the Windows OS registry.

### Configuring Custom Trusted Certificates

1. In proxied clusters, WMCO will create a new ConfigMap on operator startup. This resource will contain a trusted CA
   injection request label so it will be updated by CNO when the global `Proxy` resource changes. If the resource is
   deleted at any point during operator runtime, WMCO will re-create it to make sure CNO can provide up-to-date proxy settings.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  labels:
    config.openshift.io/inject-trusted-cabundle: "true"
  name: trusted-ca
  namespace: openshift-windows-machine-config-operator
```

  Note that we cannot add this ConfigMap into WMCO's bundle manifests because OLM treats bundle resources as static
  manifests and would actively kick back any changes, including the CA injections from CNO.

2. For Windows instances that have not yet been configured, WMCO reads the trusted CA ConfigMap data during node
   configuration and uses it to update the local trust store of all Windows instances.

3. For existing Windows nodes, WMCO reacts to changes in the custom CA bundle by reconciling Windows nodes. This will be
   done through a Kubernetes controller that watches the `trusted-ca` ConfigMap for create/update/delete events. On
   change, copy the new trust bundle to Windows instances, deleting old certificates (i.e. not present in the current
   trust bundle) off the instance and importing new ones.

How-to references:
* [import cert via powershell](https://docs.microsoft.com/en-us/powershell/module/pki/import-certificate?view=windowsserver2019-ps)
* [delete cert via powershell](https://stackoverflow.com/questions/37228851/delete-certificate-from-computer-store)

---

### Test Plan & Infrastructure Needed

In addition to unit testing individual WMCO packages and controllers, an e2e job will be added to the release repo for
WMCO's master/release-4.14 branches. A new CI workflow will be created using 
[existing step-registry steps](https://github.com/openshift/release/tree/master/ci-operator/step-registry/ipi/conf/vsphere/proxy/https),
which creates a vSphere cluster with hybrid-overlay networking and an HTTPS proxy secured by additional certs. 
This workflow will be used to run the existing WMCO e2e test suite to ensure the addition of the egress proxy feature 
does not break existing functionality. We will add a few test cases to explicitly check the state of proxy settings on Windows
nodes. When we release a community offering with this feature, we will add a similar CI job using a cluster-wide proxy on OKD.
QE should cover all platforms when validating this feature.

### Release Plan

The feature associated with this enhancement is targeted to land in the official Red Hat operator version of WMCO 9.0.0
within OpenShift 4.14 timeframe. The normal WMCO release process will be followed as the functionality described in this
enhancement is integrated into the product.

A community version of WMCO 8 or 9 will be released with incremental additions to Windows proxy support
functionality, giving users an opportunity to get an early preview of the feature using OKD/OCP 4.13 or 4.14.
It will also allow us to collect feedback to troubleshoot common pain points and learn if there are any shortcomings.

An Openshift docs update announcing Windows cluster-wide proxy support will be required as part of GA. The new docs
should list any Windows-specific info, but linking to existing docs should be enough for overarching proxy/PKI details.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

See Release Plan above.

#### Removing a deprecated feature

N/A, as this is a new feature that does not supersede an existing one.

### Upgrade / Downgrade Strategy

The relevant upgrade path is from WMCO 8.y.z in OCP 4.13 to WMCO 9.y.z in OCP 4.14. There will be no changes to the
current WMCO upgrade strategy. Once customers are on WMCO 9.0.0, they can configure a cluster-wide proxy and the Windows
nodes will be automatically updated by the operator to use the `Proxy` settings for egress traffic.

When deconfiguring Windows instances, proxy settings will be cleared from the node. This involves undoing some node
config steps i.e. removing proxy variables and deleting additional certificates from the machine's local trust store.
This scenario will occur when upgrading both BYOH and Machine-backed Windows nodes.

Downgrades are generally [not supported by OLM](https://github.com/operator-framework/operator-lifecycle-manager/issues/1177),
which manages WMCO. In case of breaking changes, please see the
[WMCO Upgrades](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator-upgrades.md#risks-and-mitigations)
enhancement document for guidance.

### Version Skew Strategy

N/A. There will be no version skew since this work is all within 1 product sub-component (WMCO). The 8.y.z version of the
official Red Hat operator will not have cluster-wide egress proxy support for Windows enabled. Then, when customers move
to WMCO 9.0.0, the proxy support will be available.

## Implementation History

The implementation history can be tracked by following the associated work items in Jira and source code improvements in
the WMCO Github repo.

## Alternatives & Justification

### Design

* Another possible way for WMCO to retrieve the proxy variables is to watch the `rendered-worker` `MachineConfig` for
  changes and parse info from the `proxy.env` file. MCO re-renders this `MachineConfig` when CVO injects new proxy
  variables into its pod spec. The difficulty of this approach comes from figuring out when we need to update
  node's env vars. Ideally such reconfiguring happens only when the values change in the pod spec, but how can we detect
  if the proxy env vars changed or the `rendered-worker` `MachineConfig` was updated for some other reason? 
  We want to avoid kicking polling of all nodes every time the `rendered-worker` spec updates.

* There are a few other ways to set the required environment variables on the node. 
  - Using Powershell instead of Windows API calls, e.g.
    ```powershell
    [Environment]::SetEnvironmentVariable('HTTP_PROXY', 'http://<username>:<pswd>@<ip>:<port>', 'Machine')
    [Environment]::SetEnvironmentVariable('NO_PROXY', '123.example.com;10.88.0.0/16', 'Machine')
    ```
    But since WICD runs directly on the node, syscalls are more direct and efficient.
  - In order to avoid a system reboot after setting node environment variables, we can reconcile services by setting 
    their process-level environment variables, and then restarting the individual services. This can be done by
    adding a Powershell pre-script to the config for each service in the windows-services ConfigMap.
    ```powershell
    [string[]] $envVars = @("HTTP_PROXY=http://<username>:<pswd>@<ip>:<port>", "NO_PROXY=123.example.com,10.88.0.0/16")
    Set-ItemProperty HKLM:SYSTEM\CurrentControlSet\Services\<$SERVICE_NAME> -Name Environment -Value $envVars
    Restart-Service <$SERVICE_NAME>
    ```
    But since pre-scripts run each time WICD checks for changes in service spec, a constant polling operation during
    operator runtime, this would be run unnecessarily often and bloat each service's configuration.

* Also note that there is another way to get the [trusted CA data](#configuring-custom-trusted-certififactes) required
  rather than accessing the ConfigMap directly, but it leaves open the same concern around unnecessary reconciliations
  -- how to detect if the operator restart was due to a trust bundle file change or the pod just restarted for another
  reason? For completeness, the approach is listed out here:
  * Update the operator’s Deployment to support trusted CA injection by mounting the trusted CA ConfigMap
  ```yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: windows-machine-config-operator
    namespace: openshift-windows-machine-config-operator
    annotations:
      config.openshift.io/inject-proxy: windows-machine-config-operator
  spec:
      ...
        containers:
          - name: windows-machine-config-operator
            volumeMounts:
            - name: trusted-ca
              mountPath: /etc/pki/ca-trust/extracted/pem
              readOnly: true
        - name: trusted-ca
          configMap:
            name: trusted-ca
            items:
              - key: ca-bundle.crt
                path: tls-ca-bundle.pem
  ...
  ```
  * Create a file watcher that watches changes to the mounted trust bundle that kills the main operator process and
    allows the backing k8s Deployment to start a new Pod that mounts the updated trust bundle.
    Implementation example: [cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator/pull/334)

* A workaround that would deliver the same value proposed by this enhancement would be to validate and provide guidance
  to make cluster administrators responsible for manually propagating proxy settings to each of their Windows nodes, and
  underlying OpenShift managed components. This is not a feasible alternative as even manual node changes can be
  ephemeral. WMCO would reset config changes to OpenShift managed Windows services in the event of a node reconciliation.

### Testing

* Instead of adding a new vSphere job, we can leverage an [existing proxy test workflow on AWS](https://steps.ci.openshift.org/workflow/openshift-e2e-aws-proxy).
  However, this workflow does not test an HTTPS proxy requiring an additional trust bundle, so we would need to make
  improvements to the pre-install steps. Since vSphere is our most used platform, testing would be better suited there.
  The required proxy config steps already exist in the release repo for vSphere anyway.
