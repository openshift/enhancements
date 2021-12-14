---
title: csi-proxy
authors:
- "@alinaryan"
- "@selansen"
reviewers:
- "@@openshift/openshift-team-windows-containers"
approvers:
  - TBD
creation-date: 2021-12-09
last-updated: 2021-12-09
status: implementable
---

# CSI Storage in Windows Containers

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
The intent of this enhancement is to allow customers to provision CSI storage 
in Windows Containers in 4.10+ using the Windows Machine Config Operator (WMCO).
CSI drivers are typically deployed as privileged containers in order to perform
storage related operations. Windows does not currently support privileged 
containers. CSIProxy makes it so that node plugins can be deployed as 
unprivileged pods and then use the proxy to perform privileged storage 
operations on the node.

## Motivation
In-tree storage is deprecated and will be removed in K8s 1.24, leaving customers
with no storage options until privileged containers are supported in Windows.

### Goals

As part of this enhancement, we plan to do the following:
* Build and ship csi-proxy.exe
* Run csi-proxy.exe as a Windows service

### Non-Goals

As part of this enhancement, we do not plan to support:
* Build and ship cloud provider plugins including:
  * AWS EBS CSI
  * AzureDisk CSI
  * AzureFile CSI
  * vSphere CSI

## Proposal
The WMCO will build and ship csi-proxy.exe and run CSIProxy as a Windows service

### User Stories

#### Story 1
As a WMCO user, I want csi-proxy.exe built and shipped as part of the WMCO 
payload so that it can be run as a Windows service on my node.

#### Story 2
As a WMCO user, I want csi-proxy running as a service on my Windows node so that
I can use it with external CSI plugins to perform privileged storage operations 
on my node.

#### Story 3
As a Windows storage user, I want documented steps for using the vSphere CSI 
driver so that I can dynamically provision storage volumes on my Windows 
workloads in vSphere.

#### Story 4
As a Windows storage user, I want documented steps for using the AWS EBS CSI 
driver so that I can dynamically provision storage volumes on my Windows 
workloads in AWS.

### Risks and Mitigations

vSphere CSI support for Windows based nodes are still in alpha stage, which is 
not recommended for production use. 

## Design Details

## Alternatives
An alternative approach would be to wait until [privileged containers](https://github.com/kubernetes/enhancements/issues/1981) 
are supported in Windows in Kubernetes 1.25. 

We are deciding against this approach considering there would be a gap in 
storage support between K8s 1.23 and 1.25. 