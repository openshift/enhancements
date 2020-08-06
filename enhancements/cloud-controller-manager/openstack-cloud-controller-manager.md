---
title: openstack-cloud-controller-manager
authors:
  - "@mfedosin"
reviewers:
  - "@crawford"
  - "@derekwaynecarr"
  - "@enxebre"
  - "@eparis"
  - "@mrunalp"
  - "@sttts"
approvers:
  - "@crawford"
  - "@derekwaynecarr"
  - "@enxebre"
  - "@eparis"
  - "@mrunalp"
  - "@sttts"
creation-date: 2020-08-04
last-updated: 2020-09-28
status: implementable
---

# OpenStack Cloud Controller Manager

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal describes the migration of OpenStack platform from the deprecated [in-tree cloud provider](https://kubernetes.io/docs/concepts/cluster-administration/cloud-providers/#openstack) to the [Cloud Controller Manager](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-openstack-cloud-controller-manager.md#get-started-with-external-openstack-cloud-controller-manager-in-kubernetes) service that implements `external cloud provider` [interface](https://github.com/kubernetes/cloud-provider).

## Motivation

Using Cloud Controller Managers (CCMs) is the Kubernetes' [preferred way](https://kubernetes.io/blog/2019/04/17/the-future-of-cloud-providers-in-kubernetes/) to interact with underlying cloud platforms as it provides more flexibility and freedom for developers. It replaces existing in-tree cloud providers, which have been deprecated and will be permanently removed in Kubernetes 1.21. But they are still used in OpenShift and we must start a smooth migration towards CCMs. As a pioneer platform, it is proposed to use OpenStack.

It's especially important to do this for OpenStack because switching to the external cloud provider fixes many issues and limitations with the in-tree cloud provider, such as it's reliance on Nova metadata service. For OpenStack platform, this means the possibility for deploying on provider networks and at the edge.

Another motivation is to be closer to upstream by helping developing the Cloud Controller Manager, which is benefiting both OpenShift and Kubernetes.

### Goals

- OpenShift, when installed on OpenStack, doesn't utilize the deprecated in-tree cloud provider and uses OpenStack CCM instead.
- The CCM is deployed and managed by the related cluster operator - `cluster-cloud-controller-manager-operator`.
- There is an upgrading path from older OpenShift versions with the in-tree cloud provider to CCM'ed ones.
- There is a downgrading path from newer OpenShift versions with the CCM to those that use the in-tree cloud provider.

### Non-Goals

- [Cinder CSI driver integration](https://github.com/openshift/enhancements/pull/437) is out of scope of this work.
- Deprecating `hyperkube` binary and switching to standalone binaries for all related components (`kube-apiserver`, `kube-scheduler`, `kube-controller-manager` and others) is also not a goal.

## Proposal

Our main goal is to start using Cloud Controller Manager in OpenShift 4 on OpenStack. So we are going to use the implementation available in upstream as a part of [cloud-provider-openstack](https://github.com/kubernetes/cloud-provider-openstack/tree/master/pkg/cloudprovider) repo.

To maintain the lifecycle of the CCM we want to implement a cluster operator, that will handle all administrative tasks: deploy, restore, upgrade, and so on. Later this operator can be reused to manage CCMs for other platforms (AWS, Azure, GCP and so on).

### Action plan

#### Implement reading config from secret for Cloud Controller Manager

Now upstream Cloud Controller Manager can only read configuration from static files on local filesystem. This doesn't comply with OpenShift's security requirements, since these files contain sensitive information and should not be available.
To avoid this, we need to implement a feature similar to [this patch](https://github.com/kubernetes/kubernetes/pull/89885). The feature will introduce two new config parameters: `secret-name` and `secret-namespace`. If they are specified, CCM will read data from the given secret, and not from the local file.

Actions:

- Implement the feature in [Upstream](https://github.com/kubernetes/cloud-provider-openstack)

- Backport it in [OpenShift](https://github.com/openshift/cloud-provider-openstack)

#### Build OpenStack CCM image by OpenShift automation

To start using OpenStack CCM in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The CCM image should be automatically tested before it becomes available.
The upstream repo provides the Dockerfile, so we can reuse it to complete the task. Upstream image is already available in [Dockerhub](https://hub.docker.com/r/k8scloudprovider/openstack-cloud-controller-manager).

Actions:

- Configure CI operator to build OpenStack CCM image.

CI operator will run containerized and End-to-End tests and also push the resulting image in the OpenShift Quay account.

#### Test Cloud Controller Manager manually

When all required components are built, we can manually deploy OpenStack CCM and test how it works.

Actions:

- Manually install CCM's [daemonset](https://github.com/kubernetes/cloud-provider-openstack/blob/master/manifests/controller-manager/openstack-cloud-controller-manager-ds.yaml) on a working OpenShift cluster deployed on OpenStack.

- Update configuration of `kubelet` by replacing `--cloud-provider openstack` with `--cloud-provider external` and removing `--cloud-config` parameters.
For `kube-apiserver` and `kube-controller-manager` we need to remove both `--cloud-provider` and `--cloud-config` parameters and restart `kubelet`.

**Note:** Example of a manual testing: https://asciinema.org/a/303399?speed=2

#### Write Cluster Cloud Controller Manager Operator

The operator should be able to create, configure and manage different CCMs (including one for OpenStack) in OpenShift. The architecture of the operator will be the same as for [Kubernetes Controller Manager](https://github.com/openshift/cluster-kube-controller-manager-operator) and [Kubernetes API server](https://github.com/openshift/cluster-kube-apiserver-operator) operators.

Actions:

- Create a new repo in OpenShift’s github: https://github.com/openshift/cluster-cloud-controller-manager-operator (Done)

- Implement the operator, using [library-go](https://github.com/openshift/library-go) primitives.

#### Build the operator image by OpenShift automation

To start using CCM operator in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The image should be automatically tested before it becomes available. Dockerfile should be a part of the operator's repo.

Actions:

- Configure CI operator to build CCM operator image.

CI operator will run containerized and End-to-End tests and also push the resulting image in the OpenShift Quay account.

#### Integrate the solution with OpenShift

Actions:

- Make sure Cinder CSI driver is supported in OpenShift.

- Add CCM operator support in Cluster Version Operator. CCM operator will be deployed on early stages of installation and then it deploys and configures OpenStack CCM itself.

- Change the config observer in [library-go](https://github.com/openshift/library-go/blob/16d6830d0b80dc2a3315207116d009ed2dd4cebf/pkg/operator/configobserver/cloudprovider/observe_cloudprovider.go#L154) to disable setting `--cloud-provider` and `--cloud-config` parameters for OpenStack. Then the library should be bumped in `cluster-kube-apiserver-operator` and `cluster-kube-controller-manager-operator` respectively.

- Change `kubelet` configuration for OpenStack in [Machine Config Operator templates](https://github.com/openshift/machine-config-operator/blob/d044c74ea4b9900c269ee8de8131ed49ba6fedc8/templates/master/01-master-kubelet/openstack/units/kubelet.service.yaml#L30) to adopt external cloud provider.

### Cloud Controller Manager installation workflow

Starting from OpenShift 4.7 the installation of OpenShift on OpenStack will be next:

- All Kubernetes components that require cloud provider functionality (`kubelet`, `kube-apiserver`, `kube-controller-manager`) are preliminary configured to use `external` cloud provider. In other words, `kubelet` is launched with the `--cloud-provider external` only; `kube-apiserver`and `kube-controller-manager` specify neither `--cloud-provider` nor `--cloud-config` parameters.

- CCM operator provides initial manifests that allow to deploy CCM on the bootstrap machine with `bootkube.sh` [script](https://github.com/openshift/installer/blob/master/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template).

- `cluster-version-operator` starts `cluster-cloud-controller-manager-operator`.

- `cluster-cloud-controller-manager-operator` checks if it runs on OpenStack, populates [configuration](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-openstack-cloud-controller-manager.md#config-openstack-cloud-controller-manager) for OpenStack Cloud Controller Manager, creates a static [OpenStack CCM pod](https://github.com/kubernetes/cloud-provider-openstack/blob/master/manifests/controller-manager/openstack-cloud-controller-manager-pod.yaml) and monitors its status.

#### Configuration of Cloud Controller Manager

At the initial stage of CCM installation installer creates a config map with `cloud.conf` key that contains configuration of CCM in `ini` format. The contents of `cloud.conf` are static and generated by the installer:

```txt
[Global]
secret-name = openstack-credentials
secret-namespace = kube-system
```

Real config is also generated by the installer and available in the given secret. Based on this static config, CCM fetches the real one from the secret and uses it.

**NOTE**: This is also how the in-tree cloud provider for OpenStack is configured in this moment. This part is already implemented and available in [the installer](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/openstack/cloudproviderconfig.go).

### Risks and Mitigations

#### OpenStack CCM doesn’t work properly on OpenShift

The manager has not been tested on either OSP or OCP. The team uses Devstack + K8s plugin for the development.

Severity: medium-high
Likelihood: 100% (we need to implement reading config from a secret to comply with OpenShift's requirements).

#### OpenShift components can't work with the CCM properly

Since OpenShift imposes additional limitations compared to Kubernetes, some collisions are possible.
So far there are no CCMs available in OpenShift, and there can be some problems with operability.

Severity: medium-high
Likelihood: low

#### Cinder CSI driver is not available in OpenShift

Integration with OpenStack Cinder (Persistent Volume Manager) was included in the in-tree cloud provider. But CCM doesn't contain this feature, so Cinder CSI driver must be installed in the system to work properly. Despite the fact that a cluster with some limitations can be installed without the driver, it will be impossible to work on it in production.

Severity: medium-high
Likelihood: low

## Design Details

### Test Plan

Testing will consist in running of serial and parallel e2e-openstack jobs on the modified system with OpenStack CCM. If they pass successfully, we can consider the solution works.

No changes to CI system are required, but additional tests should be run on a cloud with self-signed certificates.

### Upgrade / Downgrade Strategy

Upgrade from previous versions of OpenShift on OpenStack will look like:

- On the initial stage of upgrading `cluster-version-operator` starts `cluster-cloud-controller-manager-operator`.

- `cluster-cloud-controller-manager-operator` creates all required resources for CCM (Namespace, RBAC, Service Account).

- `cluster-cloud-controller-manager-operator` creates a static pod manifest for CCM.

- `kubelet` is restarted to start CCM in a static pod.

- `kube-apiserver` and `kube-controller-manager` are restarted without the `--cloud-provider` option.

- `kubelet` is restarted with `--cloud-provider external` option.

Downgrade:

- `kubelet` is restarted with `--cloud-provider openstack` option.

- `kube-apiserver` and `kube-controller-manager` are restarted with the `--cloud-provider openstack` option.

- `cluster-cloud-controller-manager-operator` destroys OpenStack CCM manifests and all related resources.

- `kubelet` is restarted again.

- `cluster-cloud-controller-manager-operator` stops working.

### Version Skew Strategy

See the upgrade/downgrade strategy.

## Alternatives

Only one alternative is to keep the deprecated in-tree cloud providers in OpenShift and support them with our own resources.

## Infrastructure Needed

Additional OpenStack cloud may be required to test how CCM works with self-signed certificates. Our current CI doesn't allow this.

## Open questions

1. Should we reuse the existing cloud provider config or generate a new one?
CCM config is backward compatible with the in-tree cloud provider. It means we can reuse it.

2. How to migrate PVs created by the in-tree cloud provider?
[CSIMigration](https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-csi-migration-beta/) looks like the best option, especially if it is GA in 1.19.

## FAQ

Q: Can we disaster-recover a cluster without CCM running?
A: We can't. Basically, without CCM running, nodes can only join the cluster, but they will be unschedulable. Which means it's impossible to start any workloads on the nodes. [Source](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager)

Q: What is non-functional in a cluster during bootstrapping until CCM is up?
A: If CCM is not available, new nodes in the cluster will be left unschedulable.

Q: Who talks to CCM?
A: There are no components that communicate with CCM. But CCM sends requests to kube-apiserver to fetch current status of managed objects and update them if necessary.

Q: Does CCM provide an API? How is it hosted? HTTPS endpoint? Through load balancer?
A: No. CCM doesn't provide an API, it is just a collection of controllers.

Q: What are the thoughts about certificate management?
A: Not required since CCM doesn't provide an interface.

Q: What happens if the KCM leader has no access to CCM? Can it continue its work? Will it give up leadership?
A: Since KCM does not communicate with CCM it can continue to work if CCM is not available.

Q: How does a kubelet talk to the CCM?
A: Basically, kubelet registeres nodes initially, and then nodes are managed by CCM. There is no direct interaction between kubelet and CCM. They both communicate with kube-apiserver only.

Q: Does every node need a CCM?
A: No. Control plane nodes only. [Source](https://kubernetes.io/docs/concepts/overview/components/#cloud-controller-manager)

Q: How does SDN depend on CCM?
A: They are not related.

Links:

- [Installation and configuration of OpenStack CCM](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-openstack-cloud-controller-manager.md)

- [Example of live migration](https://asciinema.org/a/303399?speed=2) from the in-tree cloud provider to CCM.

- [Kubernetes Cloud Controller Managers](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)

- [Cloud Controller Manager pod example manifest](https://github.com/kubernetes/cloud-provider-openstack/blob/master/manifests/controller-manager/openstack-cloud-controller-manager-pod.yaml)

- [Example of Cloud Controller Manager config file](https://github.com/kubernetes/cloud-provider-openstack/blob/master/manifests/controller-manager/cloud-config)
