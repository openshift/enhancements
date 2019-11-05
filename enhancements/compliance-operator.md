---
title: OpenShift Compliance operator
authors:
  - "@jhrozek"
reviewers:
  - "@vrutkovs"
  - "@cgwalters"
  - "@ashcrow"
approvers:
  - "@JAORMX"
creation-date: 2019-10-22
last-updated: 2019-10-22
status: provisional
---

# OpenShift Compliance operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The Compliance operator provides a way to scan nodes in an OpenShift cluster
to asses compliance against a given standard (for example FedRAMP moderate)
and in a future version, remediate the findings to get the cluster into
a compliant state. The scan is performed by the OpenSCAP tool.
OpenSCAP is well-established, [standards compliant](https://www.open-scap.org/features/standards/)
and [NIST-certified](https://csrc.nist.gov/projects/security-content-automation-protocol-validation-pr/validated-products-and-modules/142-red-hat-scap-1-2-product-validation-record)
project that administrators are aware of and trust.

For the first iteration, the operator would only provide a report with
the findings and proposed remediations which the cluster administrator
can then apply on their own. Remediation would be developed in one
of the future versions.

## Motivation

The default installation of OpenShift is not compliant (and is not expected
to be, as some requirements have impact on performance or flexibility of use
of the cluster) with different standards users in regulated environments
must adhere to.

Enabling users to be compliant with standards their respective industry
requires would enable OpenShift to be used in those environments. This is especially
true of the US public sector.

### Dependencies
As said above, the operator would orchestrate OpenSCAP scans on cluster
nodes. However, there are several areas where OpenSCAP needs to be extended
to be better usable for scanning an OpenShift cluster:

* OpenSCAP did not support merging of results from several nodes into a single
  bundle. Each scan on each node produces a separate report. There is an
  RFE filed against OpenSCAP and planned by the OpenSCAP team to aggregate
  results from all the cluster nodes into a single report.
* The remediations were typically done using a shell script or Ansible. For
  OpenShift we’d want the remediations done using MachineConfigs or other
  Custom Resources. However, the remediations are not a hard prerequisite for
  the first version of the operator, even identifying the gaps in compliance
  would be useful.
* For the gaps in compliance we identify, a check must be written. These
  checks are being compiled as a parallel effort. Having a full set of
  checks and/or remediations is not a hard prerequisite for the operator
  itself, though as the content would be delivered separately.

These have all been filed with the OpenSCAP team and acknowledged by them.

### Goals

A cluster administrator is able to use the operator to assess the degree
of compliance against a given security standard. In particular this would
mean:

* Provide a way for cluster administrator to start cluster scans
* Inspect the scan result on the high level (pass/fail/error)
* Provide a way to collect the detailed results of the scan
  * The results would list a way to remediate the gaps, typically by the means
    of deploying certain manifests to OpenShift (e.g. a Machineconfig), a different
    way of changing cluster configuration or even a free-form recommendation text.

### Non-Goals

The operator itself does not perform any scans or remediations, only
wraps the OpenSCAP scanner and provides an OpenShift-native API to
run the scans.

## Proposal

An instance of a scan would be represented by a CR. The CR must include
the scan profile (e.g. FedRAMP moderate) and the content to scan against
(typically `ocp4`). The CR might optionally contain a set of individual rules
(e.g. is `auditd` enabled on the nodes?) and an image with the content.
The CR would be processed by the operator, which would launch a pod on
each node to scan that node and deliver scan results.

The pod would consist of two containers and an init container. The init
container is started with the image that contains the content for the
scanner and copies the content to a volume to be used by the scanner
container. The two "worker" containers are the scanner and a log collector.
This is because OpenSCAP writes the report to a file, so the scanner and
the collector share a directory where the scanner writes the report, the
collector would wait for the report to be created and write the contents
of that file to a location where the cluster administrator can grab the
report and inspect it.

The scanner container in the pod would mount the node's file system into
a directory and point the `openscap-chroot` binary there.

### User Stories

#### As an OpenShift admin, I want to assess the degree of compliance of my cluster with FedRAMP moderate.

This is a use-case where the admin just wants to visualize the gaps in
compliance. They would perform the remediation themselves, potentially
based on the MachineConfigs the compliance operator suggests.

#### As an OpenShift admin, I want to assess the degree of compliance of my cluster with my own custom content

This user story illustrates that we must allow the scan to run with a
content the user provides and can't just restrict the scanner to use
content provided by the OpenScap upstream or other "pre-approved" content.

### Implementation Details

There [exists a PoC](https://github.com/jhrozek/compliance-operator) of the implementation.

Some things to note from the PoC implementation are:
 * The operator itself schedules a pod per node in the cluster. This was selected
   (instead of e.g. DaemonSet) because the pods don't have to be long-lived and
   can just exit when the scan is done.
 * The collector container in the pod reads the contents of the scan results
   from a file produced by the scanner container and uploads them to a ConfigMap.
   This is perhaps a bit hacky, but most volume types do not support
   ReadWriteMany access mode and we need to get the results from files in
   N pods across N nodes. We could mount a per-pod volume and then gather
   the results into another volume when the scan is done, but this still
   means we need a post-run gather operation.
 * For viewing the results by administrator might use [a script](https://github.com/jhrozek/scapresults-k8s/blob/master/scapresults/fetchresults.py)

### Risks and Mitigations
 * The container running the openScap scan must mount the host root filesystem
   in order to perform checks on the nodes. We would mitigate the risks by
   mounting the volume read-only, but this still means the container would
   run privileged.
 * For checking resources in the cluster, the operator needs to run with a
   serviceAccount bound to a role that can read all the resources the operator
   needs to check. This would be mitigated by checking that the operator only
   has read access to those resources.
 * For cases remediations need to be done, something needs to have permissions
   to create the MachineConfigs or other resources that need to be created.
   What would be a safe scheme to do this so that we minimize the risks in case
   the operator is compromised?

## Design Details

The `compliance-operator` would use the following API to track a compliance scan status:

### API Specification
The proposed compliance scan API instance with all the properties set:

```
apiVersion: complianceoperator.compliance.openshift.io/v1alpha1
kind: ComplianceScan
metadata:
  name: example-scan
spec:
  contentImage: quay.io/compliance-as-code/openscap-ocp4
  rule: xccdf_org.ssgproject.content_rule_no_empty_passwords
  profile: xccdf_org.ssgproject.content_profile_ospp
  content: ssg-ocp4-ds.xml
```

A minimal `ComplianceScan` instance with only the required properties set:
```
apiVersion: complianceoperator.compliance.openshift.io/v1alpha1
kind: ComplianceScan
metadata:
  name: example-scan
spec:
  profile: xccdf_org.ssgproject.content_profile_ospp
  content: ssg-ocp4-ds.xml
```

The content would be provided with an image separate from the scanner
image. This way, we can allow cluster administrator to provide their
own content easily. On the other hand, the `contentImage` property is not
required to be set explicitly and would default to an image with officially
supported content.

## Need Feedback

How we present the remediations and allow the cluster administrator to execute
them is something that should be discussed more.

On a high level, there will be three kinds of remediations:
 * General free-form text guidances. For example, your IDP must support 2FA.
 * More specific advise, but still needs to be applied manually. For example,
   "configure a message of the day so that a legal notice gets displayed after
   login by creating a ConfigMap called `motd` in the `openshift` namespace."
 * Gaps that can be remediated in a completely autonomous fashion. For example
   making sure that the `auditd` service is enabled.

At first, we would like to display the remediation in the HTML report so that
the administrator can copy the advise and, if it's possible to apply directly,
do that.

However, when it comes to applying the remediation, there are two options:

* Output the remediations to yaml files that the user can download and apply.
  The caveat of this is that it makes it cumbersome: First check the report,
  then download the remediations, then apply them manually.

* Add the capability so that the opreator can schedule a workload that will
  apply these remediations. The workflow would be as follows: Check the report
  then modify the CR so that the remediation is applied. This is more
  automatic and user-friendly, however, it requires us giving a lot of
  privileges to the workload, which might not be ideal.


### Test Plan

In addition to unit tests in the operator code itself, a CI test would be
provided that:
 * Rolls out the operator
 * Ensures the operator is running
 * Executes a scan
 * Ensures a result is produced and the scheduled container is cleaned up
   * To make sure the content is applicable, we should pick one rule that would
     always pass on a default installation (perhaps "Do other user accounts
     than root with UID 0 exist?") and one that would always fail (perhaps
     "Does a `motd` configMap exist?") and check that the results are as
     expected.
 * Ensures that all resources created during the scan are cleaned up when
   the parent `ComplianceScan` resource is removed

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

### Upgrade / Downgrade Strategy

It should be noted that this operator is brand new and does not
deprecate any existing operator so at the moment there is nothing
to upgrade from. The operator would also not be installed by default.

That said, with further upgrades we would ensure that any changes
are backwards compatible.

We also control the life cycle of all the containers that this operator uses
so we are able to prevent dependencies changing in an incompatible way.


### Version Skew Strategy

Version skew should not be an issue for this operator, because it relies
either on components that we control, like the content or the scanner image
or stable Kubernetes APIs (pods, configmaps etc). Nothing depends on this
operator either.

Since the operator uses operator-sdk already, we could also use the OLM
to manage the operator upgrades.

## Implementation History

This is a brand new operator. As of Nov-2019 there
[exists a PoC](https://github.com/jhrozek/compliance-operator) of the
implementation.

## Drawbacks

While the operator executes a scan, the scan itself might incur some
load on the cluster, although the checks should mostly be along the
lines of grepping a file, checking properties of an object in the API
etc.

As said earlier, the operator itself needs to mount the node filesystem
to perform checks on the node level and has read access to API objects
it needs to check. Please see the "Risks and Mitigations" section
for more details on that.
