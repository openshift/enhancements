---
title: vsphere-multi-vcenter
authors:
  - Neil Girard
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - JoelSpeed - machine API
  - ??? - MCO
  - ??? - SCO
  - patrickdillon - installer
  - jcpowermac - vSphere
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - JoelSpeed
  - patrickdillon
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - JoelSpeed
creation-date: 2024-04-11
last-updated: 2024-04-11
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPSTRAT-697
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
superseded-by:
---

# vSphere Multi vCenter Support

## Summary

The desire for OpenShift to support IPI and UPI installs across multiple vCenters 
is emerging as a common environments where customers have multiple vCenters that 
they would like to leverage for clusters.  Additionally, there is a growing
demand for UPI installs as well.  The proposal described in this document 
discusses the implementation of configuring clusters across vCenters as day 0 
and day 2 operations.

## Motivation

Users of OpenShift would like the ability to install a vSphere IPI cluster
across multiple vCenters.

- https://issues.redhat.com/browse/OCPSTRAT-697

### User Stories

As a system administrator, I would like OpenShift to support an installation 
across multiple vCenters so that I can leverage multiple vCenters as part of
our needs for High Availability.

As a system administrator, I would like to scale new nodes across multiple 
vSphere vCenters so that I can leverage various availability zones for 
workloads depending on our organization's needs.

As a system administrator, I would like to add a new vCenter to the existing
OCP cluster so that I can scale out new workloads across a new vCenter.

### Goals
```
Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.
```

- During installation, all nodes are created in all defined vCenters.
  Rational: multiple vCenters is another twist on failure domains.  A new vCenter 
  will appear as a new failure domain to the OCP cluster.  Control plane and 
  compute nodes must be able to be assigned to any FD.

- OCP clusters will be enhanced to leverage new yaml cloud provider config for
  vSphere.
  Rational: The ini configuration has been deprecated and the newer yaml format
  supports multiple vCenters.

- When updating infrastructure after initial installation, the cluster should be
  able to accept the newly defined failure domain which points leverages a new 
  vCenter.  

- Updating the cloud provider config from ini to yaml will be supported.
  Rational: Existing clusters wish to take advantage of migrating loads to a new
  vCenter.  In order for this to happen, we must be able to allow customers to
  update their existing cloud provider config to contain all relevant information
  for the new vCenter.

### Non-Goals

What is out of scope for this proposal? Listing non-goals helps to
focus discussion and make progress. Highlight anything that is being
deferred to a later phase of implementation that may call for its own
enhancement.

- Updating cloud config to yaml format for existing clusters (upgrading OCP to 4.17+)

## Proposal

### Multiple vCenters Configured at Installation

This section will discuss all the enhancements being made to support installing
a new cluster for use with multiple vCenters.

#### Installer Changes

The OCP installer is going to be enhanced to allow the system administrator to 
configure the cluster to use multiple vCenters. In order for this to happen, we
will be locking the multi vCenter ability behind a new feature gate: **VSphereMultiVCenters**.
The installer will also be enhanced to handle creating resources via CAPI env and
will also be enhanced to generate the new YAML vSphere cloud config.

##### Feature Gate

While the multi vCenter feature is not GA'd, you can configure cluster using the
feature set CustomNoUpgrade.  An example of configuring feature gate in the install-config.yaml
using CustomNoUpgrade:
```yaml
apiVersion: v1
baseDomain: openshift.manta-lab.net
featureSet: CustomNoUpgrade
featureGates:
- ClusterAPIInstall=true
- VSphereMultiVCenters=true
```

You may also use the featureSet TechPreviewNoUpgrade to enable multi vCenter 
support; however this will also pull in all other non GA'd features that may
still be a work in progress.  An example of enabling with TechPreviewNoUpgrade:
```yaml
apiVersion: v1
baseDomain: openshift.manta-lab.net
featureSet: TechPreviewNoUpgrade
```

This new feature gate will also be available for various operators to use do
control if multiple vCenters are allowed to be configured and used within each
operator's domain.  More on this in later sections.

##### Install-Config.yaml

The schema for the install-config already allows for the configuration of multiple
vCenters.  The installer originally blocked the configuration of multiple via the
installer code.  This code has now been enhanced to check for the configuration of
the new feature gate.

An example of configuring the install-config.yaml for multiple vCenters:
```yaml
apiVersion: v1
baseDomain: openshift.manta-lab.net
featureSet: CustomNoUpgrade
featureGates:
- ClusterAPIInstall=true
- VSphereMultiVCenters=true
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  platform: 
    vsphere:
      zones:      
      - fd-1
      - fd-2
      cpus: 4
      coresPerSocket: 2
      memoryMB: 8192
      osDisk:
        diskSizeGB: 60
  replicas: 0
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform:
    vsphere: 
      zones:
      - fd-1
      - fd-2
      cpus: 8 
      coresPerSocket: 2
      memoryMB: 16384
      osDisk:
        diskSizeGB: 100
  replicas: 3
metadata:
  creationTimestamp: null
  name: ngirard-multi
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.93.43.128/25
  serviceNetwork:
  - 172.30.0.0/16
platform:
  vsphere: 
    apiVIPs:
    - 10.93.43.132
    ingressVIPs:
    - 10.93.43.133
    failureDomains: 
    - name: fd-1
      region: us-east
      server: vcs8e-vc.ocp2.dev.cluster.com
      topology:
        computeCluster: "/IBMCloud/host/vcs-ci-workload"
        datacenter: IBMCloud
        datastore: "/IBMCloud/datastore/vsanDatastore"
        networks:
        - ci-vlan-1148
      zone: us-east-4a
    - name: fd-2
      region: us-east
      server: vcenter.ci.ibmc.devcluster.openshift.com
      topology:
        computeCluster: "/cidatacenter/host/cicluster"
        datacenter: cidatacenter
        datastore: "/cidatacenter/datastore/vsanDatastore"
        networks:
        - ci-vlan-1148
      zone: us-east-1a
    vcenters:
    - datacenters:
      - IBMCloud
      password: "password"
      port: 443
      server: vcs8e-vc.ocp2.dev.cluster.com
      user: user
    - datacenters:
      - cidatacenter
      password: "password"
      port: 443
      server: vcenter.ci.ibmc.devcluster.openshift.com
      user: user
```

In the above example, each vCenter will need to be configured in the **vcenters**
section.  Once the vcenters are configured, you can then use them as server for
any of the configured failure domains.  In this example, each failure domain is 
configured to use a different server.

The installer will consume the install-config and begin creating all artifacts
for the installation process.  Since each vCenter is considered part of one or
more failure domains, the failure domain logic will treat each failure domain
as it did before.  The primary difference comes into play when creating the
bootstrap and control plane machines / nodes.

The installer will only support installing with multiple vCenters when using the
CAPI version of the installer is in use.  The installer by the time this feature
is release may already be changed to have CAPI install logic as the default for
vSphere.  

Due to limitations in CAPI, multiple vCenters cannot be used for a single 
cluster definition.  However, we are able to create multiple CAPI clusters to 
achieve our goal of creating VMs across multiple vCenters.  With this approach,
we will create one CAPI cluster for each vCenter we wish to create a VM for either
bootstrap or control plane machines.  We will not create a CAPI cluster definition
for vCenters that will only have compute nodes.

The generated output files for CAPI wil look as follows.

Directory:
```bash
[ngirard@ip-192-168-133-14 cluster-api]$ ls -lah
total 36K
drwxr-x---. 3 ngirard ngirard 4.0K May 31 14:07 .
drwxr-xr-x. 5 ngirard ngirard  126 May 31 14:07 ..
-rw-r-----. 1 ngirard ngirard  124 May 31 14:07 000_capi-namespace.yaml
-rw-r-----. 1 ngirard ngirard  498 May 31 14:07 01_capi-cluster-0.yaml
-rw-r-----. 1 ngirard ngirard  498 May 31 14:07 01_capi-cluster-1.yaml
-rw-r-----. 1 ngirard ngirard  395 May 31 14:07 01_vsphere-cluster-0.yaml
-rw-r-----. 1 ngirard ngirard  406 May 31 14:07 01_vsphere-cluster-1.yaml
-rw-r-----. 1 ngirard ngirard  214 May 31 14:07 01_vsphere-creds-0.yaml
-rw-r-----. 1 ngirard ngirard  238 May 31 14:07 01_vsphere-creds-1.yaml
drwxr-x---. 2 ngirard ngirard 4.0K May 31 14:07 machines
```

Each 01_capi-cluster-*.yaml file represents each vCenter.

01_capi-cluster-0.yaml
```yaml 
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  creationTimestamp: null
  name: ngirard-multi-8tpnt-0
  namespace: openshift-cluster-api-guests
spec:
  clusterNetwork:
    apiServerPort: 6443
  controlPlaneEndpoint:
    host: ""
    port: 0
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereCluster
    name: ngirard-multi-8tpnt-0
    namespace: openshift-cluster-api-guests
status:
  controlPlaneReady: false
  infrastructureReady: false
```

01_capi-cluster-1.yaml:
```yaml 
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  creationTimestamp: null
  name: ngirard-multi-8tpnt-1
  namespace: openshift-cluster-api-guests
spec:
  clusterNetwork:
    apiServerPort: 6443
  controlPlaneEndpoint:
    host: ""
    port: 0
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereCluster
    name: ngirard-multi-8tpnt-1
    namespace: openshift-cluster-api-guests
status:
  controlPlaneReady: false
  infrastructureReady: false
```

When we look at the infrastructureRefs, you'll see each one reference the individual
vCenters.

01_vsphere-cluster-0.yaml
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereCluster
metadata:
  creationTimestamp: null
  name: ngirard-multi-8tpnt-0
  namespace: openshift-cluster-api-guests
spec:
  controlPlaneEndpoint:
    host: api.ngirard-multi.openshift.manta-lab.net
    port: 6443
  identityRef:
    kind: Secret
    name: vsphere-creds-0
  server: https://vcs8e-vc.ocp2.dev.cluster.com
status: {}
```

01_vsphere-cluster-1.yaml
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereCluster
metadata:
  creationTimestamp: null
  name: ngirard-multi-8tpnt-1
  namespace: openshift-cluster-api-guests
spec:
  controlPlaneEndpoint:
    host: api.ngirard-multi.openshift.manta-lab.net
    port: 6443
  identityRef:
    kind: Secret
    name: vsphere-creds-1
  server: https://vcenter.ci.ibmc.devcluster.openshift.com
status: {}
```

With CAPI configured this way, each vCenter will reference to allow the configured
bootstrap and control plane VMs to get created.  The machines directory will still
contain all machines needing to be created.  If we look at each one individually, 
we will see that not all machines are for the same vcenter.

```bash
[ngirard@ip-192-168-133-14 cluster-api]$ ls -lah machines/
total 40K
drwxr-x---. 2 ngirard ngirard 4.0K May 31 14:07 .
drwxr-x---. 3 ngirard ngirard 4.0K May 31 14:07 ..
-rw-r-----. 1 ngirard ngirard  667 May 31 14:07 10_inframachine_ngirard-multi-8tpnt-bootstrap.yaml
-rw-r-----. 1 ngirard ngirard  868 May 31 14:07 10_inframachine_ngirard-multi-8tpnt-master-0.yaml
-rw-r-----. 1 ngirard ngirard  887 May 31 14:07 10_inframachine_ngirard-multi-8tpnt-master-1.yaml
-rw-r-----. 1 ngirard ngirard  868 May 31 14:07 10_inframachine_ngirard-multi-8tpnt-master-2.yaml
-rw-r-----. 1 ngirard ngirard  483 May 31 14:07 10_machine_ngirard-multi-8tpnt-bootstrap.yaml
-rw-r-----. 1 ngirard ngirard  520 May 31 14:07 10_machine_ngirard-multi-8tpnt-master-0.yaml
-rw-r-----. 1 ngirard ngirard  520 May 31 14:07 10_machine_ngirard-multi-8tpnt-master-1.yaml
-rw-r-----. 1 ngirard ngirard  520 May 31 14:07 10_machine_ngirard-multi-8tpnt-master-2.yaml
```

machines/10_inframachine_ngirard-multi-8tpnt-master-0.yaml
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereMachine
metadata:
  creationTimestamp: null
  labels:
    cluster.x-k8s.io/control-plane: ""
  name: ngirard-multi-8tpnt-master-0
  namespace: openshift-cluster-api-guests
spec:
  cloneMode: fullClone
  customVMXKeys:
    guestinfo.domain: ngirard-multi.openshift.manta-lab.net
    guestinfo.hostname: ngirard-multi-8tpnt-master-0
    stealclock.enable: "TRUE"
  datacenter: IBMCloud
  datastore: /IBMCloud/datastore/vsanDatastore
  diskGiB: 100
  folder: /IBMCloud/vm/ngirard-multi-8tpnt
  memoryMiB: 16384
  network:
    devices:
    - dhcp4: true
      networkName: /IBMCloud/host/vcs-ci-workload/ci-vlan-1148
  numCPUs: 8
  resourcePool: /IBMCloud/host/vcs-ci-workload/Resources
  server: vcs8e-vc.ocp2.dev.cluster.com
  template: ngirard-multi-8tpnt-rhcos-us-east-us-east-4a
status:
  ready: false
```

machines/10_inframachine_ngirard-multi-8tpnt-master-1.yaml
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereMachine
metadata:
  creationTimestamp: null
  labels:
    cluster.x-k8s.io/control-plane: ""
  name: ngirard-multi-8tpnt-master-1
  namespace: openshift-cluster-api-guests
spec:
  cloneMode: fullClone
  customVMXKeys:
    guestinfo.domain: ngirard-multi.openshift.manta-lab.net
    guestinfo.hostname: ngirard-multi-8tpnt-master-1
    stealclock.enable: "TRUE"
  datacenter: cidatacenter
  datastore: /cidatacenter/datastore/vsanDatastore
  diskGiB: 100
  folder: /cidatacenter/vm/ngirard-multi-8tpnt
  memoryMiB: 16384
  network:
    devices:
    - dhcp4: true
      networkName: /cidatacenter/host/cicluster/ci-vlan-1148
  numCPUs: 8
  resourcePool: /cidatacenter/host/cicluster/Resources
  server: vcenter.ci.ibmc.devcluster.openshift.com
  template: ngirard-multi-8tpnt-rhcos-us-east-us-east-1a
status:
  ready: false
```

##### YAML Cloud Config

In addition to updating the CAPI process, the installer is being updated to create
the newer upstream YAML configuration for the vSphere cloud provider.  The YAML 
cloud provider config was designed to handle multiple vCenters.  The config will
be generated and placed into the same config map that is used today: `oc get cm cloud-provider-config -n openshift-config`

New YAML config:
```yaml
global:
  user: ""
  password: ""
  server: ""
  port: 0
  insecureFlag: true
  datacenters: []
  soapRoundtripCount: 0
  caFile: ""
  thumbprint: ""
  secretName: vsphere-creds
  secretNamespace: kube-system
  secretsDirectory: ""
  apiDisable: false
  apiBinding: ""
  ipFamily: []
vcenter:
  vcenter.ci.ibmc.devcluster.openshift.com:
    user: ""
    password: ""
    tenantref: ""
    server: vcenter.ci.ibmc.devcluster.openshift.com
    port: 443
    insecureFlag: true
    datacenters:
    - cidatacenter
    soapRoundtripCount: 0
    caFile: ""
    thumbprint: ""
    secretref: ""
    secretName: ""
    secretNamespace: ""
    ipFamily: []
  vcs8e-vc.ocp2.dev.cluster.com:
    user: ""
    password: ""
    tenantref: ""
    server: vcs8e-vc.ocp2.dev.cluster.com
    port: 443
    insecureFlag: true
    datacenters:
    - IBMCloud
    soapRoundtripCount: 0
    caFile: ""
    thumbprint: ""
    secretref: ""
    secretName: ""
    secretNamespace: ""
    ipFamily: []
labels:
  zone: openshift-zone
  region: openshift-region
```

The usage of this new YAML file means several operators will need to be enhanced
to properly use this new config.  It is also important to note that storage details
are not defined in this file.  The infrastructure object and its failure domains are
used to configure these parts.  For all clusters that are still using the legacy ini
file format, we will make sure the INI data can be loaded and used as well.  More on
this in later sections with each operator.

##### Installer `Create Cluster` Process

The rest of the bootstrapping process is business as usual.  The CAPI testenv
will read in each of these configs and create the resources as configured.  The
installer will monitor each CAPI cluster to verify when the infrastructure is up
and running.  After that, the normal OCP installation process will happen with
the Bootstrap node configuring each of the control plane nodes.

#### Machine API Operator Enhancements



#### Cluster Storage Operator



#### vSphere CSI Operator



### Multiple vCenters Configured as Day 2

- Update cloud provider config
  - Convert ini to yaml
  - Updating YAML if coming from install that only had 1 vCenter at install with YAML support.
- Update infrastructure (cluster) to contain Failure Domains
  - Infrastructure already has defined failure domains
  - Infrastructure has generated single failure domain
  - Infrastructure is legacy with no failure domains

### Workflow Description

Explain how the user will use the feature. Be detailed and explicit.
Describe all of the actors, their roles, and the APIs or interfaces
involved. Define a starting state and then list the steps that the
user would need to go through to trigger the feature described in the
enhancement. Optionally add a
[mermaid](https://github.com/mermaid-js/mermaid#readme) sequence
diagram.

Use sub-sections to explain variations, such as for error handling,
failure recovery, or alternative outcomes.

For example:

**cluster creator** is a human user responsible for deploying a
cluster.

**application administrator** is a human user responsible for
deploying an application in a cluster.

1. The cluster creator sits down at their keyboard...
2. ...
3. The cluster creator sees that their cluster is ready to receive
   applications, and gives the application administrator their
   credentials.

See
https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md#high-level-end-to-end-workflow
and https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md for more detailed examples.

### API Extensions

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

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

## Graduation Criteria

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

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

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

## Version Skew Strategy

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

## Operational Aspects of API Extensions

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

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

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

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

## Neil's Notes

- https://cloud-provider-vsphere.sigs.k8s.io/tutorials/deploying_cpi_with_multi_dc_vc_aka_zones
- https://docs.vmware.com/en/VMware-vSphere-Container-Storage-Plug-in/3.0/vmware-vsphere-csp-getting-started/GUID-8B3B9004-DE37-4E6B-9AA1-234CDA1BD7F9.html