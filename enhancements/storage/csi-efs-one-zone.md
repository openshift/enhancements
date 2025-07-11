---
title: csi-efs-one-zone
authors:
  - "@jsafrane"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@openshift/storage"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@gnufied"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-07-09
last-updated: 2025-07-09
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/STOR-2365
see-also:
  - "/enhancements/csi-efs-operator.md"
replaces:
superseded-by:
---

# AWS EFS CSI driver support for One Zone volumes

## Summary

Bring support of One Zone volumes into the AWS EFS CSI driver.

## Motivation

One Zone EFS volumes bring lower price and higher speed, at the price of less availability. Only machines in the same availability zone (AZ) as the volume can mount One Zone volume efficiently. Cross-AZ mounting is still possible, however, AWS will charge cross-AZ traffic + the speed will suffer.

We want cluster admins to be able to use One Zone volumes in OpenShift.

### User Stories

* As a cluster admin, I can manually create a PersistentVolume (PV) and PersistentVolumeClaim (PVC) for an One Zone EFS volume, so it can be used only by Pods on nodes in the same AZ as the volume.
  * Here we assume the cluster admin sets the correct `spec.nodeAffinity` in the PV, so the scheduler knows where the volume is.
* As a cluster admin, I can create a StorageClass that dynamically provisions PVs from an already existing One Zone EFS volume. Such volumes will be usable only in the same AZ as the volume.
  * As a cluster admin, I must also ensure that the whole cluster is in a single AWS AZ. The dynamically provisioned PVs do not have the correct `spec.nodeAffinity`, see [an upstream issue](https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/1153).
* As a cluster admin, I can upgrade the driver without any explicit re-configuration and my regional PVs will keep working.

### Goals

* Be on par with the upstream CSI driver and support One Zone EFS volumes.
* Keep supporting regional EFS volumes out of the box, including upgrade from older CSI driver versions - all regional PVs keep working after the upgrade without any action needed.

### Non-Goals

* Solve One Zone EFS volumes topology.
  * **Dynamic provisioning of One Zone volumes in a multi-AZ cluster is unsupported.**  The CSI driver does not support topology and thus the scheduler does not know where the volumes are and will schedule pods to wrong zones. See [an upstream issue](https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/1153). It is still possible to have a working application in multi-AZ cluster, however, it is left to the user to set the right node selectors to their apps.

## Proposal

* Include botocore in the AWS EFS CSI driver images. This allows the CSI driver (+efs-utils) to find the IP address of an EFS volume from a different AZ using AWS API instead of DNS.
* Let cluster admins specify a second IAM role to configure support for One Zone EFS volumes. Right now, the CSI driver requires just one IAM role for the control plane Pods to allow them to dynamically provision + delete PVs. One Zone EFS volumes require the **node** CSI driver pods to have permissions to `DescribeAvailabilityZones` and `DescribeMountTargets` to find the IP address of an EFS volume from a different AZ.
  * The CSI driver with just the controller IAM role will still support regional EFS volumes as it is today. The new node IAM role is optional and is needed only for One Zone volumes.

### Workflow Description

#### Upgrade with regional PVs in standalone OCP

1. An OCP cluster is running with an old CSI driver version + some AWS EFS regional PVs + some apps that use them.
2. The cluster admin upgrades the CSI driver via OLM as usual.
3. The OCP cluster is running with the new CSI driver version + the AWS EFS regional PVs + the apps that use them.

I.e., no change in support of reginal EFS volumes.

#### Configuring the EFS CSI for One Zone volumes in mint mode

1. The cluster admin either installs a new CSI driver or upgrades an old one to a version that supports One Zone volumes.
2. If cloud-credentials-operator (CCO) is in mint mode, the cluster admin / users can use One Zone volumes out of the box, without any explicit configuration.

#### Configuring the EFS CSI for One Zone volumes in STS mode

1. The cluster admin either installs a new CSI driver version or upgrades an old one to a version that supports One Zone volumes.
   As explained above, such a CSI driver supports only regional volumes.
2. If the cluster uses AWS Security Token Services (STS), such as in Hosted Control Planes (HCP) or ROSA, the cluster admin must manually create the IAM role for the AWS EFS node Pods and provide its ARN to the OLM Subscription CR:
    ```
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
      name: efs
      namespace: openshift-cluster-csi-drivers
    spec:
        channel: stable
        installPlanApproval: Automatic
        name: aws-efs-csi-driver-operator
        source: redhat-operators
        sourceNamespace: openshift-marketplace
        config:
          env:
          # The controller role ARN
          - name: ROLEARN 
            value: arn:aws:iam::269733383066:role/jsafrane-1-l2mqv-openshift-cluster-csi-drivers-aws-efs-cloud-cre
          # The node role ARN
          - name: NODE_ROLEARN
            value: arn:aws:iam::269733383066:role/jsafrane-1-l2mqv-openshift-cluster-csi-drivers-node-aws-efs-clou
    ```
3. OLM + CCO + AWS EFS CSI driver operator make sure that both roles are propagated into the AWS EFS CSI driver Pods. The CSI driver node pods will use the role + minted token to resolve One Zone volumes during mount.
4. The cluster admin / users can use One Zone volumes.

Without the steps 2+3, the cluster admin / user can still use regional EFS volumes as usual, i.e. with just the controller IAM role. When using an One Zone PVs, they will see odd errors like:
```
Warning  FailedMount  1s (x2 over 2s)  kubelet            MountVolume.SetUp failed for volume "pvc-fa471ca1-0737-43c7-9946-fe6d4a66426d" : rpc error: code = Internal desc = Could not mount "fs-050d7f9a012095956:/" at "/var/lib/kubelet/pods/ccc85e87-d097-475b-8cc1-37314b50995f/volumes/kubernetes.io~csi/pvc-fa471ca1-0737-43c7-9946-fe6d4a66426d/mount": mount failed: exit status 1
Mounting command: mount
Mounting arguments: -t efs -o accesspoint=fsap-0200aeb50407f9168,tls fs-050d7f9a012095956:/ /var/lib/kubelet/pods/ccc85e87-d097-475b-8cc1-37314b50995f/volumes/kubernetes.io~csi/pvc-fa471ca1-0737-43c7-9946-fe6d4a66426d/mount
Output: Failed to resolve "fs-050d7f9a012095956.efs.us-east-1.amazonaws.com". The file system mount target ip address cannot be found, please pass mount target ip address via mount options. 
User: arn:aws:sts::269733383066:assumed-role/my-role/i-0a49649436fbc11d7 is not authorized to perform: elasticfilesystem:DescribeMountTargets on the specified resource
```

Note that the OLM console UI asks only for the controller IAM role during operator installation. The console does not support multiple IAM roles. Cluster admins must either create or edit Subscription CR manually on command line and provide the second IAM role if they want One Zone volume support.

See [tokenized-auth-enablement-operators-on-cloud.md](/enhancements/cloud-integration/tokenized-auth-enablement-operators-on-cloud.md) for details about OLM, console and CCO behavior in STS mode.

### API Extensions

AWS EFS CSI driver operator Subscription CR now accepts `NODE_ROLEARN` env. var, see the example above. The operator works both without the `NODE_ROLEARN` env. var set (= supports only regional volumes) and with it (= enables One Zone volumes support).

### Topology Considerations

#### Hypershift / Hosted Control Planes

Both the AWS EFS CSI driver operator + its operand run exclusively in the HCP and/or ROSA hosted clusters.

Cluster admins already know how to create IAM role for the CSI driver controller + provide it in the operator Subscription. We need to teach them to create the node IAM role + add it to the subscription too.

#### Standalone Clusters

When CCO is in mint mode, the operator will make sure the node IAM role is created automatically and One Zone volumes work out of the box.

In STS mode, cluster admins already know how to create IAM role for the CSI driver controller + provide it in the operator Subscription. We need to teach them to create the node IAM role + add it to the subscription too.

#### Single-node Deployments or MicroShift

Same as the standalone clusters.

### Implementation Details/Notes/Constraints

#### Botocore

efs-utils is a python based mount helper that mounts EFS filesystems that we ship as part of the AWS EFS CSI driver image.

OCP policies and RHEL 9 as our base image do not allow us to use its plain upstream [requirements.txt](https://github.com/openshift/aws-efs-utils/blob/8a4b34a05b85228513b3ff709da7bf10b8097905/requirements.txt) of efs-utils.
We will create a new `requirements.txt.ocp` that will be used in OpenShift builds, with following changes:

* For Python packages that are in RHEL, use the version from RHEL instead of the version required by efs-utils.
* For Python packages that are not in RHEL, try to remove the dependency. That applies to many test packages that we don't need to build the image.
* For botocore, which the only package not avialalbe in RHEL and required for efs-utils to work, use a version that works in RHEL9. We've chosen 1.34.140, which is used at least by some newer efs-utils version. Still, the version number is quite arbitrary.

In the end, we will use very different package versions than upstream `requirements.txt`. On the other side, efs-utils call only few functions from botocore, and also they use other deps very lightly.

#### Two CredentialRequests, two Secrets

Until now, the EFS CSI driver used just one CredentialsRequest + one IAM role. It was used by the controller Pods to handle dynamic provisioning of volumes. Now the driver needs a second one. The second IAM role + CredentialsRequest is **optional**. The CSI driver can still handle regional EFS volumes without it.

* With CCO in mint mode, the EFS CSI driver operator will create both CredentialsRequests and One Zone volumes will work out of the box.
* With CCO in Manual mode, the EFS CSI driver will require only the controller IAM role + related Secret to exist. The driver will be able to handle only regional EFS volumes, until the cluster admin provides the node IAM role + its secret.
  * A cluster with STS auth. is a variation of the above. CCO is in manual mode, but it still provides Secrets for CredentialsRequest. The AWS EFS CSI driver operator will always issue CredentialsRequest for the controller role, but will issue CredentialsRequest for the node role only when `NODE_ROLEARN` env. var. was provided in the OLM Subscription CR (and then it was propagated to the operator Deployment by OLM).

In any case, the CSI driver must work with regional volumes when it has just the controller IAM role.

#### DNS

Until now, the CSI driver Pods used `hostNetwork: true` and `dnsPolicy: ClusterFirstWithHostNet`. Therefore all DNS queries to resolve EFS volume IP address were sent to the in-cluster DNS, which ran on a random node in the cluster. The DNS server just forwarded the query to an upstream one. When the in-cluster DNS server pod is the same AZ as the One Zone EFS volume, the upstream (AWS) DNS server responds with the address of the volume. If the DNS pod runs in another zone, upstream retuns error. Notice that the success or error depends on location of the DNS pod, not on location of the CSI driver pod. Such errors were non-deterministic and could cause unexpected cross-AZ mount of One Zone volumes.

We will set `dnsPolicy: Default` in the CSI driver node pods, so the CSI driver will resolve IP address of One Zone volumes only when it's being mounted in the same zone as the volume. efs-utils will allow cross-AZ mount only when an explicit mount option `az` is sent in the EFS PV, so cross-AZ mount does not happen accidentally and users get consistent error messages.

### Risks and Mitigations

### Drawbacks

* We use a different botocore version than upstream. We think there is only a small risk associated with it, since the CSI driver uses it only to issue `DescribeMountTargets` and `DescribeAvailabilityZones` calls.
* Dynamic provisioning of One Zone EFS volumes is unsupported in multi-AZ clusters, because the CSI driver dooes not support topology. There are no metrics / alerts that could warn users they use One Zone StorageClass in multi-AZ cluster. The only indicator will be ContainerCreating Pods with errors mounting their volumes.

## Alternatives (Not Implemented)

### Botocore

* We've explored how other teams / projects use botocore. We haven't found anyone using a compatible botocore version than we could re-use.
* We've explored how other teams / projects import unpackaged Python dependencies. We found both RHEL AI and OpenShift ART implemented a pipeline in Konflux that caches Python wheels. We've chosen the ART one, as it's part of the OCP development.

### IAM roles

* Right now, the node CSI driver pods use node's IAM Role when talking to the cloud. We thought we could add necessary permission to the role (DescribeAvailabilityZones + DescribeMountTargets), but there is no OCP component that would update the role during cluster upgrade. In addition, DescribeMountTargets could potentially reveal all EFS volumes in the AWS account and an evil user might mount them + access all their data.
* We also considered running the CSI driver node pods with the controller role. The node pods would get `efs:*`, i.e. permission to do anything with any EFS volume in the account, which sounds too dangerous.

A separate IAM is better in both regards. On the flip side, UX is much worse - the cluster admin must create another role + manually add it to the operator Subscription.

## Open Questions [optional]

## Test Plan

E2E tests (jobs):
* Install a single-zone OCP cluster, create One Zone volume + a StorageClass for it and run the regular CSI certification tests.

Undecided if manual or e2e:
* Upgrade the operator with some apps using regional EFS PVs. The apps must work after the driver upgrade.
* Use manually provisioned One Zone EFS PVs with the right `nodeAffinity` in a multi-AZ cluster.
  * Test in all OCP configurations, like disconnected clusters, FIPS, ARM, HCP/ROSA, government zones, ...

Manual test:
* Cross account One Zone volume access.

## Graduation Criteria

There is no DevPreview / TechPreview for OLM based operators. The feature is enabled by presence of a second env. var. in the operator Subscription.
We will need to disable it before the operator is released if One Zone volume support is not GA.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The operator will follow generic OLM upgrade path, and we do not support downgrade of the operator.

## Version Skew Strategy

There is a skew when the CSI driver is being upgraded. Until all CSI driver Pods are on the new version, the old Pods may not be able to mount One Zone volumes. Such skew is typically short, as it depends only on a DaemonSet upgrading its Pods.

The whole CSI driver does not depend on OCP version. Other OCP components themseves can be at various versions and the CSI driver does not care.

## Operational Aspects of API Extensions

There is no metric that would expose if the cluster uses One Zone volumes or not. We're considering adding some upstream.

## Support Procedures

TODO

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

## Infrastructure Needed [optional]

None
