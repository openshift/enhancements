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

### CRD Changes

The multiple vCenter feature will begin allowing more than one vCenter to be 
configured in the infrastructure resource.  We will be controlling this via
a new feature (VSphereMultiVCenters) and will have different CRDs installed
based on this gate.

Initially, the plans are to allow a max of 3 vCenters to be configured when the
feature gate is enabled.  The way we are going to control this is by adding new
control annotations to the model objects.

The OpenShift controller tools will be enhanced to allow a new Feature Gate 
Aware config option for max size.

Example:
```go
// +kubebuilder:validation:MinItems=0
// +openshift:validation:FeatureGateAwareMaxItems:featureGate="",maxItems=1
// +openshift:validation:FeatureGateAwareMaxItems:featureGate=VSphereMultiVCenters,maxItems=3
// +listType=atomic
// +optional
VCenters []VSpherePlatformVCenterSpec `json:"vcenters,omitempty"`
```

Here you can see the new FeatureGateAwareMaxItems flag that will control how 
the maximum items allowed is configured.  The default feature set config is 
configured with the **featureGate=""**.  This is to cover when the feature gate
VSphereMultiVCenters is not present.  The following line has 
**featureGate=VSphereMultiVCenters** which will generate a config that allows 3
vCenters when the feature gate is enabled (including TechPreview which will be
set feature gate creation).

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

The Machine API Operator (MAO) will need to be enhanced to handle the new yaml
config format.  Currently, the operator only supports the deprecated legacy 
ini config.  By updating the operator to use the newer upstream config object,
the operator will be able to handle both the ini config and the yaml config.

#### Cluster Storage Operator

The Cluster Storage Operator (CSO) is in charge of multiple components.  The
important parts being the vSphere Problem Detector, vSphere CSI Driver Operator 
and the vSphere CSI drivers.  For the CSO itself, we are just going to update it 
to know about the new changes to the infrastructure CRD.  Additionally, it will 
need to update the permissions / roles of the vSphere CSI Driver Operator.

#### vSphere CSI Driver Operator

The vSphere CSI Driver operator has a lot of enhancements done for multi vCenter
support.  These include:
- Added feature gate for multi vCenter
- Moved password from env variables for image to the CSI config file
- Enhanced CSI config file to define each vCenter with user/pass
- Updated check to include a connection for each vCenter
- Created new wrapper config object to contain legacy config values when detected

The important enhancements to discuss revolve around the change to config for csi 
driver, using upstream config and improvement to all the checks.  The first topic
of interest is the change to CSI config.  

Today we are putting only one vCenter into the csi driver INI file.
We are enhancing the process of generating that to now use a template that will 
insert multiple vCenters.  While we are now adding each vCenter into this config 
file, we are also going to be moving the username and password into the INI file 
as well.  The env variables only really works for single vCenter, but does not 
work for multiple.  Upstream has the ability to set all these values in the INI 
config and so we are migrating to putting these there.

For this to follow the security pattern of user/pass being in secrets and not
config maps, we need to make sure that the csi driver INI config is moved from
the configmap location it is currently using into a secret.  There was a separate
PR that already exists that is in the process of doing this, so we will piggy-back
off that PR to get this behavior in place.

An example of the INI file w/ user and password configured:

```ini
# Labels with topology values are added dynamically via operator
[Global]
cluster-id = ngirard-multi-bcw8t

# Populate VCenters (multi) after here
[VirtualCenter "vcs8e-vc.ocp2.dev.cluster.com"]
insecure-flag           = true
datacenters             = IBMCloud
migration-datastore-url = ds:///vmfs/volumes/vsan:523ea352e875627d-b090c96b526bb79c/
password                = password
user                    = user

[VirtualCenter "vcenter.ci.ibmc.devcluster.openshift.com"]
insecure-flag           = true
datacenters             = cidatacenter
migration-datastore-url = ds:///vmfs/volumes/vsan:523ea352e875627d-b090c96b526bb79c/
password                = password2
user                    = user2

[Labels]
topology-categories = openshift-zone,openshift-region
```

Next the operator was enhanced to be able to support using the upstream vSphere 
YAML cloud provider config format.  There is some logic that uses our old legacy 
style config.  To preserve this, we created a wrapper config object that attempts
to load the cloud provider config as either INI or YAML.  If its INI, we will 
also store the INI data into the `LegacyConfig` field so we can access it in 
certain situations.

```go
// VSphereConfig contains configuration for cloud provider.  It wraps the legacy version and the newer upstream version
// with yaml support
type VSphereConfig struct {
	Config       *vsphere.Config
	LegacyConfig *legacy.VSphereConfig
}
```

The operator has also been enhanced in all of the checks that are performed.  
Currently each check assumed everything was against one vCenter.  We need to 
enhance this logic to contain connections to each vCenter.  For example, we
perform checks against each VM to verify hardware version.  Before, all VMs
would be in the same vCenter.  Now, each node we have, we have to check for the
VMs existence in the correct vCenter to prevent false negatives.

To solve the multi vCenter check dilemma, we are enhancing each check to make sure
we take into account this.  We'll create a connection to each vCenter now and
share these across all the checks.  These connections will be stored in the 
CheckContext and can be used by any check.

#### vSphere CSI Driver

For complete support of multiple vCenters, the vSphere CSI driver needs to be
updated to v3.2 in order to get the enhancements made upstream for multiple
vCenter support.  With the current version of the driver (3.1.x), we will
have a log message stating that multiple vCenters are not supported yet.

There is already a card for updating the version of the driver to the latest
version in the backlog of the cluster storage team.  These changes will be 
tracked separately of this enhancement.

#### vSphere Problem Detector

The vSphere Problem Detector (VPD) will also be updated to handle multiple vCenters
similar to the vSphere CSI Driver Operator.
- Enhance to support new YAML cloud provider config
- Update checks to access multiple vCenters
- Create new checker to verify infrastructure config and cloud provider config.

As you can see, the changes for VPD follow what was done for vSphere CSI Driver 
Operator. We are enhancing the operator to use a wrapper config to be able to support
loading INI and YAML based configs.  This is important for backwards compatability.

All checks have been updated to allow for verifying what vCenter needs to be accessed
for the various checks.  This can be a bit confusing for the operator as the OCP
administrator configures the cluster to have a second vCenter.  For clean installs
with multiple vCenters, the config will be correct out-of-the-box; however, for
when Day 2 option is persued, the administrator is more likely to make mistake
updating the cloud provider config or the infrastructure config.  

This leads us to the new checker being added to help the administrator detect when
such a config mistake may have occurred.  The new check will validate the configs
in infrastructure and the cloud config to make sure both are configured for the
same vCenters.  It also makes sure that all failure domains are referencing valid
vCenters that should be defined in the vCenter section of the infrastructure cluster
resource.

### Multiple vCenters Configured as Day 2

NOTE: This section is placeholder for future design / work.

- vSphere updates
  - Create folders, resourcepools, etc need for FD definition
- Update vsphere-creds with new vCenter user and pass
- Update cloud provider config
  - INI
    - Convert ini to yaml
    - Add new vCenter config
  - YAML
    - Updating YAML if coming from install that only had 1 vCenter at install with YAML support.
    - Add new vCenter config
- Update infrastructure (cluster) to contain Failure Domains
  - Infrastructure already has defined failure domains
    - Add new failure domain for new vCenter. 
  - Infrastructure has generated single failure domain
    - Update "generated-failure-domain" to contain actual tags created for the current config.
    - Add new failure domain for the new vCenter.
  - Infrastructure is legacy with no failure domains
    - Create ProviderSpec if not present in infrastructure definition
    - Create failure domain for current config
    - Create failure domain for new vCenter

### Workflow Description

#### Installation (IPI)

1. vSphere administrator configures vCenters with all required tagging for zonal support
2. OpenShift administrator configures `install-config.yaml` with multiple failure domains and up to three vCenters.
3. OpenShift administrator initiates an installation with `openshift-install create cluster`.

#### Day 2 Configuration (IPI)

1. vSphere administrator configure new vCenter
 - Create cluster folder for new FD / vCenter to match name of 
 - Upload template
 - Create resource pool
 - Creates zonal tags and applies them
2. OpenShift administrator updates vsphere-creds secret to contain user/pass entry for new vCenter
3. OpenShift administrator updates the cloud.conf
  - If current cloud config is ini, the administrator will need to convert to yaml and then just add new vcenter.
  - If current cloud config is yaml, the administrator will need to add new failure domains for new vCenter.
4. OpenShift administrator updates masters to get Masters (if going from 1 FD to multi FD)
  - Add labels for region/zone to masters
  - Recreate masters and assign to failure domain 
5. Create MachineSet for each new failure domain in the new vCenter for compute nodes

### API Extensions

This feature does not create any new CRDs; however, it does enhance the following:
- infrastructures.config.openshift.io

This CRD was enhanced to allow up to 3 vCenters to now be defined in the vcenters section of the vsphere platform spec.

See https://github.com/openshift/api/pull/1842 for more information on the API changes.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

None

#### Standalone Clusters

These changes will affect Standalone clusters running on vSphere.

#### Single-node Deployments or MicroShift

This proposal targets multi node clusters that are spanning across more than one vCenter.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

### Drawbacks

## Open Questions [optional]

None yet

## Test Plan

TBD

## Graduation Criteria

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

This feature is not deprecating any features.  This is adding new features.  The
only thing we are using that may get deprecated in the future is our legacy
vSphere cloud provider INI config.  Upstream has already deprecated this to a
degree, and we are behind on using the newer YAML standard.  This feature will
be moving us to the latest standard.

## Upgrade / Downgrade Strategy

Currently, upgrade scenario will enable the ability to use multiple vCenters.  The
cluster, if being configured after the upgrade to leverage multiple vCenters, happens
to fail, the user will be able to undo their config changes and apply the previous
configs.  It is ideal for the customer to take backups of the custom resources 
before starting the reconfiguration process (Day 2).

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives


## Infrastructure Needed [optional]

## Neil's Notes

- https://cloud-provider-vsphere.sigs.k8s.io/tutorials/deploying_cpi_with_multi_dc_vc_aka_zones
- https://docs.vmware.com/en/VMware-vSphere-Container-Storage-Plug-in/3.0/vmware-vsphere-csp-getting-started/GUID-8B3B9004-DE37-4E6B-9AA1-234CDA1BD7F9.html