---
title: microshift-default-csi-plugin
authors:
  - copejon
reviewers:
  - fzdarsky # Frank Zdarsky
  - oglok # Ricky de'Noriega
  - mangelajo #  Miguel Angel Ajo Pelayo
  - nbalacha # Nithya Balachandran
approvers:
    - dhellmann # Doug Hellmann
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
    - None
creation-date: 2022-05-16
last-updated: 2022-05-16
tracking-link:
    - https://issues.redhat.com/projects/USHIFT/issues/USHIFT-41
---
# MicroShift Default CSI Plugin

> IMPORTANT: Upstream plans to update the API group of their CRD before the OCP 4.12 release.
See [Upcoming API Group Changes](#upcoming-api-group-changes).

## Summary

This enhancement proposes the adoption of a
default [Container Storage Interface(CSI) Plugin](https://kubernetes-csi.github.io/docs/)for Microshift. For more
details on MicroShift, see the respective [proposal](./kubernetes-for-device-edge.md). Kubernetes' CSI implementation
provides a standardized model for exposing block and file storage systems to container workloads.

## Motivation

For proof of concept, the KubeVirt Hostpath Provisioner (HPP) was chosen to smooth the ramp-up of MicroShift development
but was not intended for use in production. MicroShift has since matured enough to warrant a storage provisioner that
will work out of the box and be compatible with a standard RHEL 8.X / RHEL for Edge host.

### User Stories

* As an edge device owner, I want to image devices en masse with workload and storage manifests so that when the devices
  are deployed to the field, persistent storage will be created automatically, prior to the workload starting.
* As a device owner, I must be able to protect the host OS, or other workloads, from workloads that may over-consume
  storage, which could destabilize the platform.
* As a device owner, I want to configure system and cluster storage parameters pre-boot and have those configurations
  acted upon when MicroShift starts.
* As a device owner, I want to update cluster storage configuration with the assurance that a faulty config will not
  destabilize the system.

### Goals

* Provide a default, production-ready, dynamic storage solution out of the box.
* Protect the host OS from greedy workloads to assure system stability.
* Align with MicroShift's lifecycle management model via rpm-ostree atomic updates.

### Non-Goals

* Support multi-node cluster topologies
* Support remote storage volume provisioning and access
* OpenShift Enhancement for handling upgrades after the `LogicalVolume.topolvm.cybozu.com` CRD's API group change

## Proposal

[TopoLVM](https://github.com/red-hat-storage/topolvm) (downstream, ODF-LVM), is the operand of the
[LVM-Operator](https://github.com/red-hat-storage/lvm-operator), and is proposed as MicroShift's default supported
CSI plugin. The storage solution is maintained downstream by the OpenShift Data Foundation (ODF) team, and is supported on
Single Node OpenShift (SNO). The plugin is compatible with MicroShift's recommended OS's(RHEL/Centos/Fedora).

ODF-LVM is production-ready, enables isolation of workload and system storage, and is already supported by ODF on Single
Node OpenShift. ODF-LVM implements features not present in HPP, such as volume snapshotting and thin-provisioning.

### Workflow Description

> _Disclaimer:_ Workflows described here illustrate manual processes.  These are very similar to automated workflows we 
> expect in production environements.  The primary difference is that users are not likely to have console access to 
> the device. Instead, device owners will generate new OS layers containing the lvmd config and deploy these onto the
> device.

**Device Owner:** A human user with privileged access on the RHEL host and cluster-admin access in the MicroShift cluster.

**Start MicroShift with default ODF-LVM configuration**

> _Assuming a [RHEL 8.x](https://github.com/openshift/microshift/blob/main/docs/devenv_rhel8_auto.md) or 
> [RHEL for Edge](https://github.com/openshift/microshift/blob/main/docs/rhel4edge_iso.md) host, ..._

1. The device owner will connect to the device terminal.
1. If the MicroShift service is not running, the device owner will start the service via `systemctl enable --now microshift`.
1. The device owner will verify a stable deployment (see: **Validate-ODF-LVM function**)

**Validate ODF-LVM function**
1. The device owner will observe the cluster to confirm ODF-LVM pods are running with `oc get pods -n openshift-storage`.

    ```shell
    $ oc get pods -n openshift-storage
    NAME                                  READY   STATUS    RESTARTS        AGE
    topolvm-controller-8479455f95-tdv7f   4/4     Running   0               30m
    topolvm-node-clv2z                    4/4     Running   0               30m
    ```

2. The device owner will create a simple Pod and PersistentVolumeClaim to test-drive the plugin with:
    ```shell
    cat <<'EOF' | oc apply -f -
    kind: PersistentVolumeClaim
    apiVersion: v1
    metadata:
      name: my-lv-pvc
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 1G
    ---
    apiVersion: v1
    kind: Pod
    metadata:
      name: my-pod
    spec:
      containers:
      - name: nginx
        image: nginx
        command: ["/usr/bin/sh", "-c"]
        args: ["sleep", "1h"]
        volumeMounts:
        - mountPath: /mnt
          name: my-volume
      volumes:
        - name: my-volume
          persistentVolumeClaim:
            claimName: my-lv-pvc
    EOF
    ```

**Maintainer Driven Rebase**

Because MicroShift is based on OCP, it must ensure version parity between its cluster components and those of OCP.
OCP records cluster component image digests in the ocp-release image, which is versioned according to its corresponding
OCP version.  ODF maintains a similar release image called ocs-release, which maps ODF-LVM image digests to OCP versions.
The workflow is automated and documented 
[here](https://github.com/openshift/microshift/blob/9dfddbd6801098c96bbd7db887c2844cad177c69/docs/rebase.md).

#### Variation

**Start MicroShift with a user-provided ODF-LVM configuration**

> After the device owner has logged into the device ... 

1. The device owner will create a file called `/etc/microshift/lvmd.yaml`
1. The device owner will define the lvmd config, according to its 
[documentation](https://github.com/red-hat-storage/topolvm/blob/main/docs/lvmd.md)

   _For example:_
   
    ```shell
    $ cat <<EOF > /etc/microshift/lvmd.yaml
    deviceClasses:
    - default: true
      name: "user-provided-config"
      volume-group: rhel
      spare-gb: 10 # in GB
    socket-name: /run/lvmd/lvmd.sock
    ```

1. The device owner will define a storageClass per LVM VolumeGroup to expose them to cluster workloads, which should be
placed on the device under `/etc/microshift/manifests/` or `/usr/lib/microshift/manifests/`.  An example storageClass
is provided [here](https://github.com/red-hat-storage/topolvm/blob/main/docs/user-manual.md#storageclass).  Note the
device class is referenced as `parameters["topolvm.cybozu.com"]: <DEVICE_CLASS_NAME>`
   
   _Example StorageClass_ 
   ```yaml
   kind: StorageClass
   apiVersion: storage.k8s.io/v1
   metadata:
     name: topolvm-provisioner
   provisioner: topolvm.cybozu.com
   parameters:
     "csi.storage.k8s.io/fstype": "xfs"
     "topolvm.cybozu.com/device-class": "ssd"
   volumeBindingMode: WaitForFirstConsumer
   allowVolumeExpansion: true
   ```
1. The device owner will start the microshift service, and proceed to validate the platform as described in **Validate 
ODF-LVM function**
1. The device owner will validate the system, following workflow **Validate ODF-LVM function**

**Start MicroShift with a *malformed* user-provided ODF-LVM configuration**

> After the device owner has logged into the device ...

1. The device owner creates an misconfigured lvmd config file under /etc/microshift/lvmd.config.  In this example, assume the LVM volume group cannot be found on the host device.

    _For example:_
    ```shell
    [root@ushift ~]# cat <<EOF > /etc/microshift/lvmd.yaml
    deviceClasses:
    - default: true
      name: "user-provided-config"
      volume-group: non_existent_volume_group
      spare-gb: 10 # in GB
    socket-name: /run/lvmd/lvmd.sock
    ```
**Correct a _malformed_ user-provided ODF-LVM configuration and restart MicroShift**

> After the device owner has logged into the device ...

1. The topolvm-node will validate the config on startup and after detecting a misconfiguration, will enter a
CrashLoopBackoff state.  
1. The device owner will observe the topolvm-node crash looping, and access the lvmd container's logs to determine the
cause of the crash: `oc get logs -n openshift-storage topolvm-node-123yz lvmd`
1. The device owner will edit the /etc/microshift/lvmd.yaml file to correct the error
1. The device owner will restart MicroShift: `systemctl restart microshift`
1. The device owner will validate the system, following workflow **Validate ODF-LVM function**

### API Extensions

**Cluster CRDs**

ODF-LVM represents logical volumes with a CRD called `logicalvolumes.topolvm.cybozu.com`.  This CRD is installed on the 
cluster automatically by MicroShift during startup. 

### Implementation Details/Notes/Constraints [optional]

#### Default Values

If a user-defined lvmd config is not provided, MicroShift will provide default values, which are geared towards
[developer](https://github.com/openshift/microshift/blob/main/docs/devenv_rhel8_auto.md) and 
[production](https://github.com/openshift/microshift/blob/main/docs/rhel4edge_iso.md) deployments. An example default
configuration would be:

```yaml
deviceClasses:
- default: true
  name: "default"
  volume-group: "rhel"
  spare-gb: 10 # in GB
socket-name: /run/lvmd/lvmd.sock
```

#### Under the Hood

This workflow ensures that ODF-LVM is reading the most recent version of the lvmd.yaml configuration.  The
lvmd process reads this once at startup.  Changes to the configuration require that a) the data be pushed to the `lvmd`
configMap and b) the lvmd process be restarted so that it will pick up the latest configuration.

1. On startup, MicroShift will check for the existence of a file called `/etc/microshift/lvmd.yaml`.  
   1. If the file exists, MicroShift will read the file into memory
   1. If the file doesn't exist, MicroShift will assume hardcoded default values.
1. MicroShift will calculate a SHA256 checksum from the config data. 
1. MicroShift will generate a ConfigMap to encapsulate the lvmd config data.

    _Example ConfigMap:_
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: lvmd
      namespace: openshift-storage
    data:
      lvmd.yaml: |
        device-classes:
        - name: user-provided-config
          volume-group: rhel
          default: true
        socket-name: /run/lvmd/lvmd.sock
    ```

1. MicroShift will Create / Update the ConfigMap.
1. MicroShift will render the topolvm-node daemonset template, with the checksum value and `socket-name` specified in
the pod template. 
annotation.  This has the following effects:
   1. If the daemonSet does not exist, it will be started
   1. Else, if the file changed, the pod will be restarted.
   1. Else, if the file did not change, the pod will not be restarted

### Risks and Mitigations

- The lvmd configuration is exposed differently in OCP than it is in Microshift, which creates a risk of the schema becoming
skewed between the platforms.

- The API group is planned to merge backwards-compatibility breaking change to the API, upstream. See upstream 
(PR)[https://github.com/topolvm/topolvm/issues/168]. The API will shift from `topolvm.cybozu.com` to `topolvm.io`. However, the controller will not 
intelligently manage the legacy and new versions at the same time.  Instead, existing clusters  will need to set an 
environmental variable in the controller pod to signal if the cluster is using the legacy group or the new one. This creates
a potential for data loss if upgrades do not account for the API difference.

- Topolvm image digests are updated manually.  This must be automated in order to ensure release versions of topovlm do
not skew when rebasing MicroShift to a new version of OCP.  

### Drawbacks

The plugin is depends on an LVM managed storage layer, which will require users to image devices with LVM installed even
when they would not have otherwise. This drawback is outweighed by the benefits gained from workload storage isolation.

## Design Details

### Open Questions [optional]

### Test Plan

MicroShift will be tested via the OpenShift-CI automation, using the existing e2e and
conformance suites in the `openshift-test` tool.  

[//]: # (**Note:** *Section not required until targeted at a release.*)
### Graduation Criteria

#### Current -> Dev Preview (OCP Upstream Version: 4.12)

- Gather actionable feedback from early adopters
- Support thin and thick logical volume user-configuration
- Expand testing to RHEL9
- Expand testing to RHEL for Edge

#### Dev Preview -> Tech Preview (OCP Upstream Version: 4.13)

- Expand test coverage for upgrade/downgrade
- Support ARM architecture
- Resolve early adopter feedback
- User documentation complete
- Enumerate service level indicators (SLIs), expose SLIs as metrics

#### Tech Preview -> GA (OCP Upstream Version: 4.14)

- Available by default
- Sufficient time for feedback
- Document SLOs for the component
- Document and distribute technical enablements
- End user documentation published in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Backhaul SLI telemetry

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

MicroShift is deployed as an application on top of an operating system, and so its upgrade/downgrade model differs
significantly from OCP's.  Because MicroShift's cluster architecture is embedded within the MicroShift binary, 
the entire application is upgraded as a whole, distributed by Red Hat as a set of RPMs and container images.  The ART team will generate new
MicroShift RPMs and make these available to users. Users deploying MicroShift on top of RHEL for Edge are provided a
recipe for generating and deploying a new rpm-ostree.

Downgrades are handled similarly, where users install an older version of MicroShift, or deploy a rpm-ostree layer
containing the older version.

Customers deploying onto RHEL for Edge will be able to rely on [Greenboot](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html-single/composing_installing_and_managing_rhel_for_edge_images/index?extIdCarryOver=true&sc_cid=701f2000001OH7EAAW#how-are-rhel-for-edge-images-restored_managing-rhel-for-edge-images) to ensure that if an upgrade/rollback produces
and unstable operating environment, the system will automatically roll the change back to the last known stable state.

#### Upcoming API Group Changes

Upstream TopoLVM is expected to change the API group of the LogicalVolume CRD to distance the API from the owning company
(see issue topolvm/topovlm#168 | PR topolvm/topolvm#539).  Due to a high risk of data loss, the upstream community will
not implement migration automation.  The changes introduce an environmental variable that users will use to specify the API
group to be used.  Existing ("legacy") clusters are those that are running topolvm API's with the `topolvm.cybozu.com` group domain name,
and will need to explicitly set this environment variable after upgrading to ensure continued access to on-disk data.

### Version Skew Strategy

As stated in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy), MicroShift embeds cluster architecture in its
runtime.  The cluster component versions correlate to the version of OCP that that version of MicroShift is based on. Maps
of these version correlations are produced as part of the OpenShift CI/CD as container image called the ocp-registry. 
The ocp-registry is tagged according to the OCP version it represents, and contains the digests of all core OCP components
associated with that particular version of OCP.

Similarly, ODF maintains a registry image called ocs-registry, which is also tagged according to the OCP version it
represents, and contains a SQL database of images and their digests to be distributed with that version of OCP.

MicroShift leverages these registry images to ensure parity of its cluster components with the upstream OCP version it is
derived from.  

Additionally, as MicroShift deploys as a single node, version skew between nodes is not a concern. 

### Operational Aspects of API Extensions

#### Failure Modes

##### Invalid User-Defined LVMD Configuration

The ODF-LVM node pod (`topolvm-node`), validates configuration on start.  Invalid configurations are treated as fatal
errors and cause the pod to enter a crash loop.  Users can then query the pod's logs to get a concise explanation of the 
configuration error.  See [Variation](#variation) section for steps to correct this state. 

##### Volume Group Does Not Exist

ODF-LVM will verify that LVM Volume Groups specified in the lvmd.yaml file exist at start up.  If any of the volume
groups cannot be found, the `topolvm-node` pod will enter a crash loop. Users can then query the pod's logs to a concise
explanation of the configuration error.  See [Variation](#variation) section for steps to correct this state.

##### No Space Left in VolumeGroup

Storage requests will not be fulfilled if the target volume group does not have the capacity to fulfill the requested
amount.  Remaining capacity is calculated per volume group as the _total unallocated space_ - _spare-gb_ = _remaining
capacity_.  PVC's with a requests greater than _remaing-capacity_ will not be provisioned. Users may specify the
_spare-gb_ in the lvmd configuration, per volume group.

>NOTE! ODF-LVM rounds storage request sizes up to the nearest Gib integer value.  This may result in a size greater than
> the volume groups unallocated space.

This situation presents as PVCs remaining in a "Pending" state indefinitely.  Users should use `oc describe` to examine
the PVC's events for a message indicating the volume group does not have adequate storage for the request.

#### Support Procedures

## Implementation History

## Alternatives

[KubeVirt CSI Hostpath Provisioner](https://github.com/kubevirt/hostpath-provisioner#kubevirtiohostpath-provisioner) 
(KV-CSI-HPP) was also considered as a production-grade storage solution.  While it demonstrated a lower resource overhead
and smaller over-the-wire size, it did not satisfy requirements for workload data isolation.

Exposing only a subset of the user-facing config.  This was initially considered as there is no apparent need to allow users
to specify the socket-name, as it is a path in the pod environment.  The design was rolled back after we concluded that
API parity with the upstream project was preferable.  See [Under the Hood](#under-the-hood) for design details.   

## Infrastructure Needed [optional]

MicroShift must support the ARM cpu architecture, and thus so should its cluster components.  ODF-LVM is not currently
released for ARM and will need to be implemented in the build and release pipeline.
