---
title: node-artifacts-during-build-and-image-stream-import
authors:
  - "@rmarasch"
reviewers:
  - "@dmage"
  - "@bparees"
  - "@adambkaplan"
approvers:
  - "@dmage"
  - "@bparees"
  - "@adambkaplan"
creation-date: 2019-12-02
last-updated: 2020-03-23
status: implementable
---

# Using node pull credentials during build and image stream import

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Allow OpenShift users to import and use images from any registry configured
during or after the cluster installation by sharing node's pull credentials
with `openshift-api`, `builder` and `image-registry` pods.

## Motivation

To have image stream, image registry and builds working closely to node avoids
complexity and redundancy as each component won't need to mount different
Secrets to have access to information that is already present on the node's
filesystem.

OpenShift should seamlessly use the cluster-wide pull secret provided during
installation in builds, imagestream imports and pull-through operations. This
is particularly important for images pulled from `registry.redhat.io`, which
requires a pull secret.

Today pull secrets provided during cluster install are available on the
node's filesystem. If user attempts to import an image stream or pull-through
from these locations, OpenShift fails as none of `openshift-api`, `builder` or
`image-registry` use the credentials provided during the installation.

### Goals

- Allow users to use container images from any registry provided during or
  after cluster installation.
- Allow users to run `builds` based on images hosted in any registry provided
  during or after cluster installation without providing any extra credentials.
- Allow `image-registry` to execute pull-through using the node's pull
  credentials.

### Non-Goals

- Change the way users manage cluster pull credentials.
  * Additional credentials may still be provided through secrets on namespace.
- Use Node mirroring config.
  * To use config files files such as `/etc/containers/registries.conf` is not
  part of this proposal.
- Use Node CAs.
  * This should be done in the future through another enhancement proposal as
  we currently have different sources of truth when it comes to CAs (different
  methods of providing them to different parts of the codebase).

## Proposal

### User Stories

As an OpenShift user
I want to be able to import imagestreams from any image registry that the
cluster nodes can pull images from
so that I can use them without manually creating any extra credentials.

As an OpenShift user
I want to be able to use, during builds, images from any image registry that
the cluster nodes can pull images from
so that I can use these images as base for my own images.

As an OpenShift user
I want to be able to pull-through images from any registry that the cluster
nodes can pull images from
so that I can use image registry cache to speed-up my builds.

### Implementation Details/Notes/Constraints

This is the node's filesystem path we are taking into account:

- `/var/lib/kubelet/config.json` contains the node's pull secret.

#### Image Stream Import

- Mount the pull secret as `readOnly`, `hostPath` in
  `openshift-apiserver` deployment under `/var/lib/kubelet/config.json`.

```yaml
volumeMounts:
- mountPath: /var/lib/kubelet/config.json
  name: node-pull-credentials
  readOnly: true
volumes:
- name: node-pull-credentials
  hostPath:
    path: /var/lib/kubelet/config.json
    type: File
```

- When a user imports an imagestream, `openshift-apiserver` will merge all pull
  credentials found in `/var/lib/kubelet/config.json` with other credentials
  that may exist in the namespace.


#### Builds

- `controller-manager` would mount pull credentials inside `build` pod as 
  `hostPath`(similar to what is done for Image Stream Import).
- Builder image parses node's pull credentials and uses them during build,
  merging with other pull credentials that are linked to the `builder` service
  account. If the `BuildConfig` specifies a pull secret, we will continue the
  current behavior of using the provided pull secret as an override.

#### Registry pull-through

- As done for Image Stream Import, mount pull credentials inside the image
  registry pod.
- Pull credentials will then be consumed by the registry.

### Risks and Mitigations

#### Pull credentials for registry may exist on namespace and on node

User may have created a secret containing credentials for a registry that node
already has credentials to.

Mitigations:

Always prioritize namespace secrets over node's credentials. If a credential
exists on the namespace, do not use node's credentials.

#### Pull credentials inside build pod may be visible to the user

Cluster wide credentials mounted inside the builder pod may be a security risk
as the user may be able to shell into the pod and copy them. Another possible
risk is that the user may copy the credentials into a resulting image.

Mitigations:

As far as I verified(and this needs to be once more tested) it is impossible
to spawn a shell inside the builder pods. I also tried to copy the credentials
from builder's filesystem into a resulting image but it failed.

#### Pushing to registries using node's credentials

If pull credentials are used during builds we may allow users to push images to
any registry configured on the node. This is not an ideal scenario as cluster
administrators may not to be aware or wanting this.

Mitigations

Pull credentials must be used **only** for pulling images. On pushing we must
use only user provided(not node) credentials'.

#### Absence of mounted path on node filesystem

If the path we are trying to mount through `hostPath` directive does not exist
on the node where the pod is running the pod won't come up.

Mitigations

Mounted path always exist on worker and master nodes.

#### Image stream secrets endpoint

Currently `builder` pod obtains a set of credentials by a request to
`openshift-apiserver` done by the `openshift-controller-manager`, this approach
could create a data leak if we decide to use the same endpoint to also return
Node's credentials as well.

Mitigations

We should not change the endpoint behavior. By mounting the Node's pull
credentials inside the `builder` pod we don't need to change the endpoint as
the `builder` pod can merge credentials internally. Node's pull secret should
never be exposed through any API endpoint.

#### Pull secrets may be exposed through an ephemeral container

Kubernetes implements a feature that allows users to temporary create 
[ephemeral](https://kubernetes.io/docs/concepts/workloads/pods/ephemeral-containers/)
containers into a running pod. This could potentially allow users to copy
mounted pull secrets from a build pod as the ephemeral pod may allow `rsh`.

Mitigations

Build pods run using special permissions and regular users are not be able to
spawn them due to it. As an user is not allowed to spawn a build pod it should
also not be allowed to change/patch a running build pod definition by inserting
ephemeral containers to it. We need to ensure that this is implemented once the
ephemeral container API goes beta and ephemeral containers are enabled by
default.

## Design Details

### Test Plan

- Create a project and attempt to import images streams from
  `registry.redhat.io` without provide any other pull credential.
- Pull OpenShift's `base image` during a build.
- Attempt to use an image from `registry.redhat.io` as input/source during a
  (build)[https://docs.openshift.com/container-platform/4.2/builds/creating-build-inputs.html#image-source_creating-build-inputs]
- Attempt, on build, to **push** images to a registry for which credentials
  only exist on node. This should fail as node's pull credentials are only used
  when pulling images.
- Try to pull-through OpenShift's `base image`.

### Graduation Criteria

#### Tech Preview

Not applicable.

#### Generally Available

1. QE has test cases for all scenarios defined on Test Plan.
2. Regressions tests are passing.
3. Documentation in place regarding how node's credentials are used during
   builds.

### Upgrade / Downgrade Strategy

Does not apply. 

## Implementation History

2019-12-02: Initial draft
2019-12-03: Added node's docker certificates to the enhancement proposal.
2019-12-04: Added extra test cases to our Test Plan.
2019-12-05: Using node credentials only for pull, never for push.
2019-12-10: Removing node's certificates from this proposal scope.
2019-12-12: Added pull-through support.
2019-12-13: Added note on `readOnly` and image registry pull-through.
2020-01-20: Added note on `ephemeral` containers under Risks and Mitigations.
2020-03-23: Documenting credentials path as present on master nodes as well.

## Infrastructure Needed

None.
