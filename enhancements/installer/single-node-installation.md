---
title: Single node installation
authors:
  - "@eranco74"
reviewers:
  - "@romfreiman"
  - "@hardys"
  - "@crawford"
  - "@markmc"
approvers:
  - "@hardys"
  - "@crawford"
  - "@markmc"
creation-date: 2020-08-18
last-updated: 2020-08-30
status: provisional
see-also:
  - "https://github.com/openshift/enhancements/pull/302"
  - "https://github.com/openshift/installer/pull/3978"
---


# Single node installation

Add a new `create single-node-config` command to `openshift-installer` which
allows a user to create an `aio.ign` Ignition configuration which
launches a minimal all-in-one control plane using static pods.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The new installer `create single-node-config` command generates an `aio.ign`
file with a minimal all-in-one control plane using static pods. The
installer command will use the same `install-config.yaml` input as the
`create cluster` command.

This minimal control plane is similar to the bootstrap control plane
used in regular installations, and consists only of etcd,
kube-apiserver, kube-controller-manager, and kube-scheduler. The key
differences from the usual `bootstrap.ign` configuration are:

- `bootkube.sh` will finish when it sets up the control plane without
  applying all the OpenShift manifests.
- `kubelet` will be configured to be a part of the cluster.

When a user boots a CoreOS image with the `aio.ign` configuration,
they will observe a similar flow as a bootstrap node in a regular
installation, but they will end up with a minimal control plane. The
intent is that the user will be able to expand and customize this
"all-in-one" configuration, but this will be covered by a future
enhancement. There is no intention to allow users to grow these
single-node installations into multi-node installations.

This enhancement can be seen as a first step of what is described in
https://github.com/openshift/enhancements/pull/302

## Motivation

Demonstrate a prototype of creating a simple static Ignition file that
boots an RHCOS machine and launches a basic Kube control plane.

Rob Szumski’s demo of [“Kubelet with no control
plane"](https://developers.redhat.com/devnation/tech-talks/kubelet-no-masters)
shows the possibilities of a single node using Kubelet and static
pods, with no Kubernetes control plane. Rob references a
[gist](https://gist.github.com/dmesser/ffa556788660a7d23999427be4797d38)
which describes how to configure Kubelet, CRI-O, and CNI for this use
case. He also talks about how to use Ignition and CoreOS to configure
such a machine. This proposal differs from Rob’s demo in that we will
use Kubelet to launch a control plane, and actual apps will be
deployed using the regular Kubernetes control plane constructs.

Seth Jennings [rhcos-kaio](https://github.com/sjenning/rhcos-kaio)
prototype shows an AIO Kubernetes control plane could be launched with
Ignition, where the control plane components (like etcd and
kube-apiserver) are managed by systemd. This proposal differs from
Seth’s prototype in that we will use Kubelet managed static pods for
these components.

### Goals

1. Add installer support for creating a single node cluster composed
   of static pods, similar to the installer bootstrap.

### Non-Goals

1. Single Ignition config that can be used for multiple installations
   - the installer must be used to generate the configuration for each
  cluster.
2. Support for expanding this all-in-one cluster to a regular cluster.

3. cert rotation won't be handled. as a first step we will create certs with 10 years expiry.

## Proposal

The new installer `create single-node-config` command generates an `aio.ign`
file, based on an `install-config.yaml` file. The Ignition
configuration includes assets such as TLS certificates and keys. When
a machine is booted with CoreOS and this Ignition configuration, the
`aiokube` systemd service is launched, which is similar to bootkube in
the bootstrap Ignition.

`aiokube` uses the operators for the minimal control plane components
to render the static pod manifests and related assets. Once those
components have been launched, `kubelet` is configured to join the
cluster.


### Implementation Details/Notes/Constraints [optional]


### Initial POC

What I did:
- Added a new command to the openshift-installer `create aio-config`
- Added aiokube.service and auikube.sh (based on bootkube)
- Added kubelet.service and kubelet.conf for the aio-config target 
- The aiokube setup the control plane (etcd, kube-apiserver, kube-controller-manager, kube-scheduler)

To try this out:
- Installer branch: https://github.com/eranco74/installer/tree/aio (you can use https://github.com/eranco74/installer/tree/aio-4.6 for installing 4.6 release)
- Build the installer (./hack/build.sh)
- Add your pull secret to the ./install-config.yaml
- Generate ignition - `make generate` (will copy an install config to ./mydir and run `./bin/openshift-install create aio-config --dir=mydir`)
- Set up networking - `make network` (Provides DNS for `Cluster name: test-cluster, Base DNS: redhat.com`)
- Download rhcos image - `make image` (will place rhcos-46.82.202007051540-0-qemu.x86_64.qcow2 under /tmp)
- Spin up a VM with the aio.ign - `make start`
- Monitor the progress using `make ssh` and `journalctl -f -u aiokube.service`

Result:

```
$ kubectl --kubeconfig=./mydir/auth/kubeconfig get nodes 
NAME      STATUS   ROLES           AGE   VERSION
master1   Ready    master,worker   37s   v1.18.3+1a1d81c

$ kubectl --kubeconfig=./mydir/auth/kubeconfig get pods -A
NAMESPACE     NAME                              READY   STATUS    RESTARTS   AGE
kube-system   kube-apiserver-master1            2/2     Running   0          30s
kube-system   kube-controller-manager-master1   1/1     Running   1          9s
kube-system   kube-scheduler-master1            1/1     Running   0          11s

```

Note that `etcd-metrics`, `etcd-member`, and `cluster-version-operator` are running but don’t show up as pods:

```
[root@master1 core]# crictl ps
CONTAINER           IMAGE                                                                                                                                CREATED             STATE               NAME                             ATTEMPT             POD ID
1a55645d7a460       bcca9970c64938e2a61bb67313fb98e5e447251708cb4ba71a7e1e69911896e1                                                                     3 minutes ago       Running             kube-controller-manager          0                   a1c059679362e
6a1fab4f5960c       0d67431212aa49df6db29ffcd340f32ca7eb72403463afb942ed7f54d22ddb6e                                                                     3 minutes ago       Running             kube-apiserver-insecure-readyz   0                   681adb673a945
ddf0f2bfedc55       bcca9970c64938e2a61bb67313fb98e5e447251708cb4ba71a7e1e69911896e1                                                                     3 minutes ago       Running             kube-apiserver                   0                   681adb673a945
41e16a14674bd       bcca9970c64938e2a61bb67313fb98e5e447251708cb4ba71a7e1e69911896e1                                                                     3 minutes ago       Running             kube-scheduler                   0                   b7f54ad48d58f
c357b83f8f303       registry.svc.ci.openshift.org/origin/release@sha256:7a9734b654bd2831586a616cba34fab7cba5e250bb12d1c22714fa775fb710d7                 4 minutes ago       Running             cluster-version-operator         0                   b406ff9d7f0d5
e8a5e3883053d       f06b568f2423c2398529fdae134c5544da43c8c12a76b32ff878375e82dde489                                                                     5 minutes ago       Running             etcd-metrics                     0                   33279c46d7d8c
bdddc21a94c98       registry.svc.ci.openshift.org/origin/4.5-2020-08-13-143015@sha256:7000dcd670635518998e9f2386c71db8bb42e66a07d92d19d2b0fa659c3f4dfc   5 minutes ago       Running             etcd-member                      0                   33279c46d7d8c
```

The node is schedulable:

```
oc --kubeconfig=./mydir/auth/kubeconfig get deployment
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
nginx-deployment   3/3     3            3           30m
```

Issues:

- At some point the certs will expire (24 hours)!
- Kube-controller-manager (not a real problem) fail in case we don’t have DNS
```
unable to load configmap based request-header-client-ca-file: Get https://api-int.eran.redhat:6443/api/v1/namespaces/kube-system/configmaps/extension-apiserver-authentication: dial tcp: lookup api-int.eran.redhat on 192.168.122.1:53: read udp 192.168.122.154:47020->192.168.122.1:53: i/o timeout
```
- Kubelet fail to register the node when using the master kubeconfig (as a workaround I placed the admin kubeconfig under /etc/kubernetes/)
```
Attempting to register node localhost
ent(v1.ObjectReference{Kind:"Node", Namespace:"", Name:"localhost", UID:"localhost", APIVersion:"", ResourceVersion:"", FieldPath:""}): type: 'Normal' reason: 'NodeHasSufficientMem>
ent(v1.ObjectReference{Kind:"Node", Namespace:"", Name:"localhost", UID:"localhost", APIVersion:"", ResourceVersion:"", FieldPath:""}): type: 'Normal' reason: 'NodeHasNoDiskPressur>
ent(v1.ObjectReference{Kind:"Node", Namespace:"", Name:"localhost", UID:"localhost", APIVersion:"", ResourceVersion:"", FieldPath:""}): type: 'Normal' reason: 'NodeHasSufficientPID>
tus.go:92] Unable to register node "localhost" with API server: nodes is forbidden: User "system:anonymous" cannot create resource "nodes" in API group "" at the cluster scope
```

### Open Questions [optional]

1. Are there any other ways these bootstrap static pods are deficient compared to the regular control plane static pods?
