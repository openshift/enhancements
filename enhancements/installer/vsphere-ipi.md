---
title: vsphere-ipi
authors:
  - "@jcpowermac"
reviewers:
  - "@mtnbikenc"
approvers:
  - TBD
creation-date: 2019-12-19
last-updated: 2019-12-19
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - "https://github.com/openshift/enhancements/pull/148"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/image-registry/remove-registry-baremetal.md"
replaces:
  - ""
superseded-by:
  - ""
---

# vSphere IPI


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

- If the machine-api recreates a control plane node how will it recreate the
  behavior of creating a hostname file:
  https://github.com/openshift/installer/pull/2841/files#r359698716
- Does vsphere need any metadata changes:
  - `pkg/asset/cluster/vsphere`
  - `pkg/asset/cluster/vsphere/vsphere.go`
- Do cloudCreds need to change?
  - `pkg/asset/manifests/openshift.go`
  - `data/data/manifests/openshift/cloud-creds-secret.yaml.template`
- How do we gather ip addresses from bootstrap and control plane machines?
- Do we need to document the steps for preparing for a disconnected installation?
  - mirroring registry
  - downloading rhcos image
  - etc???


## Summary

Provide an IPI installation method for VMware vSphere platform.
VMware vSphere does not provide networking services like DNS or Load Balancing
unless NSX-T is available which is an additional cost. With this in mind to
meet the lowest common denominator of vSphere environments we require the ability
to automate DNS and load balancing services internal to the cluster.
The solution that the baremetal team architected and designed fit this problem.
The [baremetal-networking](https://github.com/openshift/enhancements/pull/148)
enhancement document has further explaination of additional services provided.

It will be assumed the customer's enivornment will provide properly configured
DHCP server that is available on the layer 2 network that the vSphere networking
is configured to.

The existing supported vSphere versions will still remain supported including 6.5 and
6.7.

## Motivation

VMware vSphere platform represents a significant number of OpenShift installations.
Customers would like the option that currently includes UPI to be able to install OpenShift
using the more automated IPI method.

### Goals

- Install OpenShift on vSphere using IPI method
- The "baremetal networking" solution will be used for required OpenShift DNS and load balancing

### Non-Goals

- Day two replacment of internal DNS or load balancers
- Linked clones

## Proposal

### UPI vs IPI

Need to distinuish between vSphere UPI and IPI.  Customers that use UPI will
will want to utlized their existing DNS and load balancing - the additional services
are unnecessary.

If the VIPs within `install-config.yaml` are undefined, nil or empty ("") then
we will assuming UPI.  If defined and non-empty we will assume IPI.

### openshift/api

- Add `VSpherePlatformStatus` struct which contains `APIServerInternalIP`, `IngressIP`, and `NodeDNSIP`.
These ip variables are used for VIPs the api server, ingress and DNS.

### openshift/installer

- Add additional required parameters to vSphere platform
  - `pkg/types/vsphere/platform.go`

```go
type Platform struct {
	VCenter string `json:"vCenter"`
	Username string `json:"username"`
	Password string `json:"password"`
	Datacenter string `json:"datacenter"`
	DefaultDatastore string `json:"defaultDatastore"`
	Folder string `json:"folder,omitempty"`
	Cluster string `json:"cluster,omitempty"`

	APIVIP string `json:"apiVIP,omitempty"`
	IngressVIP string `json:"ingressVIP,omitempty"`
	DNSVIP string `json:"dnsVIP,omitempty"`

    // override url for RHCOS image
    ClusterOSImage string `json:"clusterOSImage,omitempty"`
}
```

- Add the ability to download the OVA using the url provided in `rhcos.json`
and upload to vsphere as a VM template. Assign `Template` variable. Based
on the [spike](https://issues.redhat.com/browse/CORS-1295):
  - The installer will use the baseURI and images.vmware.path from rhcos.json to download the published OVA.
  - For disconnected, the user can provide in the install-config.yaml a full URI path (file or URL) to the OVA which the user has previously downloaded and made available.
  - The installer will import the OVA and create a VM template and include the infraID in the name. The VM template will be placed in the root of the vSphere hierarchy.
  - All created entities should include the infraID to allow cleanup when running 'destroy'.
  - Out of scope:
    - Custom placement of the VM template
    - Custom name of the VM template
  - `pkg/rhcos/vsphere.go`
- Add machines and machinesets
  - `pkg/asset/machines/vsphere`
  - `pkg/types/validation/machinepools.go`
  Need to add additional struct fields to machinepool and validate
- Add internal terraform and create terraform variable asset json.
  Terraform will not create a vSphere vCenter Resource Pool but will
[use the root rp](https://issues.redhat.com/browse/CORS-1329).
  Each vCenter object that is created by terraform will be taged with the cluster-id
for deletion.
  Virtual Machine `extraConfig` will be used inplace of vApp properties.  vApp only support 64kb string.  In
  testing an extraConfig can be created with a value length of 20MB.
  - `pkg/asset/cluster/tfvars.go
  - `pkg/tfvars/vsphere/vsphere.go`
  - `data/data/vsphere`
- Add vSphere validation: vSphere API access, permissions, etc...
  - Gather a list of roles and permissions required for cloudprovider and cluster-api
  - `pkg/asset/installconfig/vsphere`
  - `pkg/asset/installconfig/platformcredscheck.go`
- Update cloudproviderconfig to use upstream (or forked version) of API / structs
  - `pkg/asset/manifests/vsphere/cloudproviderconfig.go`
- Modifications required for "baremetal" networking
  - `pkg/asset/ignition/machine/node.go`
  - `pkg/asset/manifests/infrastructure.go`
  - `pkg/asset/tls/mcscertkey.go`
  - `data/data/bootstrap/files/usr/local/bin/bootkube.sh.template`
- Add vsphere destroy
  - `pkg/destroy/bootstrap/bootstrap.go`
  - `pkg/destroy/vsphere/`
  - `cmd/openshift-install/destroy.go`
- Add Gather [Add terraform vsphere plugins (gather)]
  If `vmtoolsd.service` is running vCenter will be provided the ip addresses for the instance.
  If `vmtoolsd.service` is not running we will have no other method to retrieve addresses.
  Using `govc vm.ip` as an example: https://github.com/vmware/govmomi/blob/master/govc/vm/ip.go#L147-L168
  we should be able to retrieve the running instance's address.
  - `pkg/terraform/exec/plugins`
  - `pkg/terraform/gather/vsphere/ip.go`
  - `cmd/openshift-install/gather.go`
- Add ClusterPlatformMetadata
  - `pkg/types/clustermetadata.go`
- Confirm defaults are still valid
  - `pkg/types/defaults/installconfig.go`
- Add additional platform validations
  Need to validate that the VIPs are correctly defined and resolvable. See OpenStack and Baremetal.
  - `pkg/types/vsphere/validation/platform.go`

### openshift/machine-config-operator


- bootstrap:
  - `./manifests/vsphere`
  - `pkg/operator/bootstrap.go`
- control plane and compute:
  - `./templates/common`
- control plane:
  - `./templates/master`
- compute:
  - `./templates/worker`
- Regenerate: pkg/operator/assets/bindata.go
- Confirm no render changes are needed:
  - `pkg/controller/template/render.go`


### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they releate.

### Risks and Mitigations

- Limited capacity in existing Packet vSphere CI environment.  Need migration plan to VMware Cloud on AWS.

## Design Details

### Test Plan

There are two options for CI enivornments.  Packet or VMware Cloud on AWS (VMC).
VMC provides significant advantages in capacity but would be difficult to implement
when requiring `gather bootstrap` to access ssh through the VPC to the virtual machines
running on vSphere.

Packet is significantly easier since each RHCOS virtual machine has a public IPv4 address.
The negative with packet is limited capacity including: CPU, Disk, RAM and "only" a `/25`
public IPv4 address space.

With those options with our limited time presently moving forward with Packet is the
best option:

- Use phpipam to provide VIP addresses
- The VIPs will need be resolvable in some manner - either via `/etc/hosts` entries
or Route53 addresses.
- Create `install-config.yaml`
- Execute `openshift-install create cluster`
- Use existing failure options from template

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default


##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy


### Version Skew Strategy


## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

- `baremetal-runtimecfg` requires control plane nodes hostname to contain `master`

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

