---
title: azure-file-cloning-with-azcopy
authors:
  - "@RomanBednar"
reviewers:
  - "@jsafrane"
approvers:
  - "bbennett"
api-approvers: 
  - "None"
creation-date: 2024-01-20
last-updated: 2024-01-20
tracking-link:
  - "https://issues.redhat.com/browse/STOR-1499"
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# Azure File cloning with azcopy

## Summary

Upstream Azure File CSI Driver added support for volume cloning (v1.28.6) which fully depends on `azcopy` cli tool.
This enhancement is about adding support for volume cloning to the Azure File CSI Driver in OpenShift. This requires
forking upstream `azcopy` repo and shipping it with the driver. 

This can be done either by creating RPM package or including the `azcopy` binary directly in the driver image, that is 
creating a new base image for the driver (same approach as we already have for the AWS EFS CSI Driver).

## Motivation

Volume cloning is a feature that allows to create a new volume from an existing one acting as a source.
The existing volume can be a snapshot or any other PVC (specified in `dataSource` field).

While snapshots are not yet supported by Azure File CSI Driver the addition of `azcopy` is a prerequisite for that.

### User Stories

* As an OpenShift cluster admin or user, I want to create an Azure File volume, so that it is an exact copy of an existing volume.

### Goals

* Allow users to use cloning feature of Azure File CSI Driver.
* Make sure that the driver returns a reasonable error when cloning a NFS volume - this is not supported by the driver currently.

### Non-Goals

* Adding support for snapshots to Azure File CSI Driver.

## Proposal

Ship [azure-storage-azcopy](https://github.com/Azure/azure-storage-azcopy) as a base image for the Azure File CSI Driver.

### Workflow Description

Clones are provisioned like any other PVC with the exception of adding a dataSource that references an existing PVC in the same namespace.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
    name: clone-of-pvc-1
    namespace: myns
spec:
  accessModes:
  - ReadWriteOnce
  storageClassName: cloning
  resources:
    requests:
      storage: 5Gi
  dataSource:
    kind: PersistentVolumeClaim
    name: pvc-1
```

The result is a new PVC with the name clone-of-pvc-1 that has the exact same content as the specified source pvc-1

#### Variation and form factor considerations [optional]

None.

### API Extensions

None.

### Implementation Details/Notes/Constraints [optional]

None.

#### Hypershift [optional]

None.

### Risks and Mitigations

Cloning is a standard feature of the CSI spec and is supported by most CSI drivers. The only risk could be that the
performance of the cloning operation is poor for larger files, we intend to test this to mitigate the risk.

### Drawbacks

None.

## Design Details

We need to get `azcopy` into the container with the Azure File CSI Driver. The `azcopy` tool is a tool written in Go that
supports many different operations apart from copying objects, but we don't intend to support any if the extra features
apart from those required by cloning.

We've chosen to use base image approach due to the following reasons:
- we're familiar with the process
- we don't want to support a generic RPM that can be used outside of OCP
- we want to keep consistency with other tools (e.g. AWS EFS CSI Driver)

1. Fork `github.com/Azure/azure-storage-azcopy` into `github.com/openshift/azure-storage-azcopy`.

2. Create a new base image with `azcopy` included, say `ose-azure-storage-azcopy-base`:

   ```Dockerfile
   FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.16 AS builder
   WORKDIR /go/src/github.com/openshift/azure-storage-azcopy
   COPY . .
   RUN go build -o ./bin/azcopy .
    
   FROM registry.ci.openshift.org/ocp/4.16:base
   COPY --from=builder /go/src/github.com/openshift/azure-storage-azcopy/bin/azcopy /usr/bin/
   ```
3. The Azure File CSI Driver then uses it as the base image:
  
   ```Dockerfile
   FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.16 AS builder
   ... build the image ...
   FROM registry.ci.openshift.org/ocp/4.16:ose-azure-storage-azcopy-base
   COPY --from=builder <driver binary> /usr/bin/
   ```

### Open Questions [optional]

* How do we determine what performance or stress testing is required for this feature? We should assess what volume
sizes are the most common and test against those.

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

We decided not to use a feature gate, creating a clone of a volume must be explicitly requested by a user.

#### Dev Preview -> Tech Preview

None.

#### Tech Preview -> GA

None.

#### GA

* The functionality described above is implemented.
* Manual testing is performed and passing.
* End user documentation exists.
* Test plan is implemented in our CI.
* Performance or stress testing was conducted.
* All bugs with urgent + high severity are fixed.

#### Removing a deprecated feature

* Cloning was not supported in the past - no deprecation is done in scope of this enhancement.

### Upgrade / Downgrade Strategy

Not applicable to this feature, and we don't support downgrading CSI drivers.

### Version Skew Strategy

None.

### Operational Aspects of API Extensions

None.

#### Failure Modes

None.

#### Support Procedures

In case of any issues with the feature, CSI Driver logs should be collected, analysed and eventually attached to the bug
report along with must-gather.

## Implementation History

* OCP 4.16 - initial implementation (Tech Preview)

## Alternatives

* Create RPM package for `azcopy` and ship it with the driver. This would require to create a new repo
  for `azcopy` and maintain it. We don't want to do that.

## Infrastructure Needed [optional]

We need to be able to provision Azure File volumes and clone them in CI which might require some additional work or permission change.
