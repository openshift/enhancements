---
title: Adding Baremetal Installer Provisioned Infrastructure (IPI) to OpenShift
authors:
  - "@sadasu"
reviewers:
  - "@smarterclayton"
  - "@abhinavdahiya"
  - "@enxebre"
  - "@deads2k"
approvers:
  - "@abhinavdahiya"
  - "@smarterclayton"
  - "@enxebre"
  - "@deads2k"
creation-date: 2019-11-06
last-updated: 2019-11-06
status: implemented
---

# Adding Baremetal IPI capabilities to OpenShift

This enhancement serves to provide context for a whole slew of features
and enhancements that will follow to make Baremetal IPI deployments via
OpenShift a reality.

At the time of this writing, code for some of these enhancements have already
merged, some are in progress and others are yet to me implemented. References
to all these features in different stages of development will be provided
below.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Baremetal IPI deployments enable OpenShift to enroll baremetal servers to become
Nodes that can run K8s workloads.
The [Baremetal Operator][1] along with other provisioning services (Ironic and
dependencies) run in their own pod called "metal3". This pod is deployed by the
Machine API Operator when the Platform type is `BareMetal`. The OpenShift
Installer is responsble for providing all the necessary configs required for
a successful deployment.

## Motivation

The motivation for this enhancement request is to provide a background for all the
the subsequent enhancement requests for Baremetal IPI deployments.

### Goals

The goal of this enhancement request is to provide context for all the changes
that have already been merged towards making Baremetal IPI deployments a reality.
All future Baremetal enhancement requests will refer back to this one to provide
context.

### Non-Goals

Raising development PRs as a result of this enhancement request.

## Proposal

Every OpenShift based Baremetal IPI deployment will run a "metal3" pod on
one Master Node. A "metal3" pod includes a container running BareMetal
Operator(BMO) and several other supporting containers that work together.

The BMO and other supporting containers together are able to discover a
baremetal server in a pre-determined provisioning network, learn the
HW attributes of the server and eventually boot it to make it available
as a Machine within a MachineSet.

The Machine API Operator (MAO) currently deploys the "metal3" pod only
when the Platform type is `BareMetal` but the BaremetalHost CRD is exposed
by the MAO as part of the release payload which is managed by the cluster
version operator. The MAO is responsible for starting the BMO and the
containers running the Ironic services and for providing these containers
with their necessary configurations via env vars.

The installer is responsible for kicking off a Baremetal IPI deployment
with the right configuration.

### User Stories

With the addition of features described in this and other enhancements
detailed in this current directory, OpenShift can be used to bring up
a functioning cluster starting with a set of baremetal servers. As
mentioned earlier, these enhancements rely on the [Baremetal Operator (BMO)][1]
running within the "metal3" pod to manage baremetal hosts. The BMO in
turn relies on the [Ironic service][3] to manage and provision baremetal
servers.

1. Will enable the user to deploy a control plane with 3 master nodes.
2. Will enable the user to grow the cluster by dynamically adding worker
nodes.
3. Will enable the user to scale down the cluster by removing worker nodes.

### Implementation Details/Notes/Constraints

Baremetal IPI is integrated with OpenShift through the [metal3.io][8] project.
Metal3.io is a set of Kubernetes controllers that wrap the OpenStack Ironic
project to provide Kubernetes native APIs for managing deployment and
monitoring of physical hosts.

The installer support for Baremetal IPI deployments is described in more detail
in [this document][7]. The installer runs on a special "provisioning host" that
needs to be connected to both a "provisioning network" and an "external
network". The provisioning network is a dedicated network used just for the
purposes of configuring baremetal servers to be part of the cluster. The
traffic on the provisioning network needs to be isolated from the traffic on
the external network (hence 2 seperate networks.). The external network is used
to carry cluster traffic which which includes cluster control plane traffic,
application and data traffic.  More detail on the networking requirements for
the cluster can be found in [this document][10].

Control Plane Deployment

Details about DNS and load balancer automation for this platform are documented
[here][11].

1. A minimin Baremetal IPI deployment consists of 4 hosts, one to be used
first as a provisioning host and later potentially re-purposed as a worker.
The other 3 make up the control plane. These 4 hosts need to be connected
to both the provisioning and external networks.  The provisioning host is a
RHEL 8 host capable of running libvirt VMs.

2. Installation can be kicked off by downloading and running
"openshift-baremetal-install". This image differs from the "openshift-install"
binary only because libvirt is needs to be always linked for the baremetal
install. Removing a bootstrap node would remove the dependency on libvirt
and then baremetal IPI installs can be part of the normal Openshift installer.
This is in the roadmap for this work and being investigated.  Note that it is
still built from the same installer code base and is only a separate binary
build.

3. The installer starts a bootstrap VM on the provisioning host. With other
platform types supported by OpenShift, a cloud already exists and the installer
runs the bootstrap VM on the control plane of this existing cloud. In the case
of the baremetal platform type, this cloud does not already exist, so the
installer starts the bootstrap VM using libvirt.

4. The bootstrap VM needs to be connected to the provisioning network and so the
the network interface on the provisioning host that is connected to the
provisioning network needs to be provided to the installer.

5. The bootstrap VM must be configured with a special well-known IP within the
provisioning network that needs to provided as input to the installer.  This
happens automatically and does not need any intervention by the cluster
operator.

6. The installer user Ironic in the bootstrap VM to provision each host that
makes up the control plane. The installer uses terraform to invoke Ironic API
that configures each host to boot over the provisioning network using DHCP
and PXE.

7. The bootstrap VM runs a DHCP server on the isolated provisioning network and
responds with network infomation and PXE instructions when Ironic powers on
a host. The host boots the Ironic Agent image which is hosted on the httpd
instance also running on the bootstrap VM.  Note that this DHCP server moves
into the cluster as part of the `metal3` pod once the cluster comes up.

8. After the Ironic Agent on the host boots and runs from its ramdisk image, it
looks for the Ironic Service either using an URL passed in as a kernel command line
arguement in the PXE response or by using MDNS to seach for Ironic in the local L2
network.

9. Ironic on the bootstrap VM then copies the RHCOS image hosted on the httpd
instance to the local disk of the host and also writes the necessary ignition files
so that the host can start creating the control plane when it runs the local image.

10. After Ironic writes the image and ignition configs to the local disk of the host,
Ironic power cycles the host causing it to reboot. The boot order on the host is set
to boot from the image on the local drive instead of PXE booting.

11. After the control plane hosts have an OS, the normal bootstrapping process continues
with the help of the bootstrap VM. The bootstrap VM runs a temporary API service to talk
to the etcd cluster on the control plane hosts.

12. The manifests constructed by the installer are pushed into the new cluster. The
operators launched in the new cluster would bring up other services and reconcile cluster
state and configuration.

13. The Machine API Opentaror (MAO) running on the control plane cluster detects the
platform type as being "baremetal" and launches the "metal3" pod and the cluster-api-
provider-baremetal (CAPBM) controller. The metal3 pod runs several Ironic services in
containers in addition to the baremetal-operator (BMO). After the control plane is
completely up, the bootstrap VM is destroyed.

14. The baremetal-operator that is part of the metal3 service starts monitoring hosts
using the Ironic service which is also part of metal3. The baremetal-operator uses the
BareMetalHost CRD to get information about the on-board controllers on the servers. As
mentioned previously in this document, this CRD exists in non baremetal platform types
too but does not represent any usable information for other platforms.

Worker Deployment

Unlike the control plane deployment, the worker deployment is managed by metal3. Not
all aspects of worker deployment are implemented completely.

1. All worker nodes need to be attached to both the provisioning and external networks
and configured to PXE boot over the provisioning network. A temporary provisioning IP
address in the provisioning network are assigned to each of these hosts.

2. The user adds hosts to the available inventory for their cluster by creating
BareMetalHost CRs. For more information about the 3 CRs that already exist for a host
transitioning from a baremetal host to a Node, please refer to [this doc][9].

3. The cluster-api-provider-baremetal (CAPBM) controller finds an unassigned/free
BareMetalHost and uses it to fulfill a Machine resource. It then sets the configuration
on the host to start provisioning with the RHCOS image (using RHCOS image URL present
in the Machine provider spec) and the worker ignition config for the cluster.

4. Baremetal operator uses the Ironic service to provision the worker nodes in a
process that is very similar to the provisioning of the control plane except for
some key differences. The DHCP server is now running within the metal3 pod instead
of in the bootstarp VM.

5. The provisioning IP used to bring up worker nodes remains the same as the control
plane case and the provisoning network also remains the same. The installer also
provides with a DHCP range within the same network that the workers are assigned IP
addresses from.

6. The ignition configs for the worker nodes are as passed as user data in the config
drive. Just as in the control plane hosts, Ironic power cycles the hosts that boot
using the RHCOS image now in their local disk. The host then joins the cluster as a
worker.

Currently, there is no way to pass the provisioning config known to the installer to
metal3 that is responsible for provisioning the workers.

### Risks and Mitigations

Will be specified in follow-up enhancement requests mentioned above.

## Design Details

### Test Plan

True e2e and integration testing can happen only after implementation for
[this enhancement][2] lands. Until then, e2e testing is being performed with the
help of some developer scripts.

Unit tests have been added to MAO and the Installer to test additions
made for the Baremetal IPI case.

### Graduation Criteria

Metal3 integration is in tech preview in 4.2 and is targeted for GA in 4.6.

Metal3 integration is currently missing an important piece to information on
the baremetal servers and ther provisioning environment. Without this, true
end to end testing cannot be performed in order to graduate to GA.

### Upgrade / Downgrade Strategy

Metal3 integration is in tech preview in 4.2 and missing key pieces that allows
a user to specify the baremetal server details and its provisioning setup. It
is really not usable in this state without the help of external scripts that
provied the above information in the form of a Config Map.

In 4.4, when all the installer features land, the Metal3 integration would be
fully functional within OpenShift. Due to those reasons, at this point an
upgrade strategy would not be necessary.

### Version Skew Strategy

This enahncement serves as a backgroup for the rest of the enhancements. We will
discuss the version skew strategy for each enhancement individually in their
respective requests.

## Implementation History

Implementation to deploy a Metal3 cluster from the MAO was added via [this
commit][4].

## Infrastructure Needed

The Baremetal IPI solution depends on the Baremetal Operator and the baremetal
Machine actuator both of which can be found [here][5].
OpenShift integration can be found [here][6].
Implementation is complete on the metal3-io and relevant bits have been
added to the OpenShift repo.

[1]: https://github.com/metal3-io/baremetal-operator
[2]: https://github.com/openshift/enhancements/blob/master/enhancements/baremetal/baremetal-provisioning-config.md
[3]: https://github.com/openstack/ironic
[4]: https://github.com/openshift/machine-api-operator/commit/43dd52d5d2dfea1559504a01970df31925501e35
[5]: https://github.com/metal3-io
[6]: https://github.com/openshift-metal3
[7]: https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md
[8]: https://metal3.io/
[9]: https://github.com/metal3-io/metal3-docs/blob/master/design/nodes-machines-and-hosts.md
[10]: https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md
[11]: https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md
