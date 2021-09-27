---
title: remove-jenkins-payload
authors:
  - "@adambkaplan"
reviewers:
  - "@gabemontero"
  - "@akram"
approvers:
  - "@bparees"
  - "@sbose78"
  - "@derekwaynecarr"
creation-date: 2021-07-20
last-updated: 2021-08-24
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# Remove Jenkins from the OCP Payload

## Release Signoff Checklist

- [ x ] Enhancement is `implementable`
- [ x ] Design details are appropriately documented from clear requirements
- [ x ] Test plan is defined
- [ x ] Operational readiness criteria is defined
- [ x ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal aims to remove Jenkins images from the OCP payload and replace them with images delivered via CPaaS.

## Motivation

Distributing and supporting Jenkins directly has always been challenging for OpenShift.
Due to OpenShift's tight coupling to Jenkins via the Jenkins Pipeline build strategy,
OpenShift 4 has included a Jenkins distribution as well as a default set of agent images used to run Jenkins pipelines.
This meant that any bug fixes or upgrades to Jenkins needed to go through the often arduous backport process to get delivered to customers.

Jenkins itself is not native to Kubernetes and does not use a [Java runtime](https://www.jenkins.io/doc/administration/requirements/java/) that has been optimized for cloud-native environments (i.e. Quarkus).
As a result, it has poor resilience when run on Kubernetes.
The Jenkins templates do not run Jenkins in a highly available mode, leaving its primary instance vulnerable to downtime from slow restarts.
The Jenkins container image is also very large - it is in fact the largest image in the OpenShift payload today.
Removing Jenkins from the OpenShift payload has been a longstanding goal that can reduce the time needed to pull the OCP payload at install/updgrade time.

With the GA release of OpenShift Pipelines, we would like to decouple Jenkins from OpenShift, distributing Jenkins just like any other application.
This would allow bug fixes in our Jenkins distribution to be released on its own cadence.
Customers can continue to deploy Jenkins on OpenShift via its ImageStream and associated templates.
In this fashion, Jenkins becomes just another application on OpenShift and does not retain any preferential status.
The maintenance of Jenkins is also simplified - only one image version will be released and tested against supported OCP versions.

### Goals

* Move Jenkins out of the OCP Payload.

### Non-Goals

* Document how JenkinsPipeline strategy builds can be migrated off of OpenShift.
* Migrate Jenkins users to OpenShift Pipelines/Tekton.
* Stop distributing the Jenkins image and our associated OpenShift plugins.
* Disable the JenkinsPipeline build strategy.

## Proposal

### User Stories

As a developer using Jenkins on OpenShift
I want to continue using my Jenkinsfiles
So that I can continue to use Jenkins for my CI/CD processes

As an OpenShift release engineer
I want to remove Jenkins from the OCP payload
So that I can reduce the size of the payload
And release Jenkins fixes on their own cadence

### Implementation Details/Notes/Constraints [optional]

#### Remove From Payload

Jenkins and its related agent images will be removed from the OCP payload.
The delivery of these container images will be replaced with a CPaaS-based release pipeline.
Jenkins images will be published to the Red Hat Container Catalog, just like any other layered product.
The current [openshif4/ose-jenkins](https://catalog.redhat.com/software/containers/openshift4/ose-jenkins/) image will need to be deprecated in favor of the new non-payload image.

Once the CPaaS pipeline is ready, the Jenkins templates and imagestreams delivered in OpenShift via Samples operator will need to reference the container catalog images (just like other layered products).
The Samples Operator can then remove its special logic for Jenkins imagestreams and templates.

#### Jenkins / OCP Compatibility

The label `io.openshift.versions` will be added to the Jenkins image via a Dockerfile LABEL instruction.
Its value will be an inclusive range of OCP major+minor versions that the image is officially supported on.
For example, the current Jenkins image would set the value `v4.6-v4.8`.

In the event a breaking change is introduced, the `ocp-*` backport branch will maintain the Dockerfile that sets the appropriate OCP supported version label.
The main branch will then update the `io.openshift.versions` label to indicate that prior OCP versions are no longer supported.
For example, if a breaking change is introduced in OCP 4.9, the branch `ocp-4.8` will be added to all Jenkins repositories.
The main branch will then be update the `io.openshift.versions` label on the image to `v4.9`, while the image produced from the `ocp-4.8` branch will continue to use `v4.6-v4.8`.

#### Jenkins Plugin Version Scheme

Jenkins plugins for OpenShift currently use the 1.0.<z> versioning scheme.
This versioning scheme will be updated so that the minor Y version corresponds to the lowest supported version of OCP 4 for the plugin.
For example, the next release of the openshift-client plugin should be 1.6.0, since OCP 4.6 is the lowest supported version of OCP for the plugin (that we can support).

The main/master branch for Jenkins and its plugins will be assumed to support the version of OpenShift under development.
If a breaking change is introduced, a separate branch named `ocp-4.n` should be created to handle bug backports, with `n` representing the highest supported version of OpenShift.
The requisite delivery pipelines will also need to be updated so that bugfix builds can be issued from `ocp-*` branches.

#### Jenkins Image Tagging Scheme

The Jenkins image will use the supported OCP version as the basis of its tagging scheme:

1. `latest` - latest image build
2. `4` - latest image that supports OCP 4
3. `4.<Y>`- latest image that supports up to OCP 4.Y
4. `4.<Y>-N` - latest image that supports up to OCP 4.Y, with incrementing ordinal `N`.

CPaaS and Errata will be configured so that tags for `4.Y` are set appropriately for the given branch, based on the `io.openshift.versions` Dockerfile label above.
For example, if the Dockerfile has the `v4.6-v4.9` version label, the image produced by CPaaS will have the following tags:

- `latest`
- `4`
- `4.6`, `4.7`, `4.8`, and `4.9`
- `4.6-N`, `4.7-N`, `4.8-N`, `4.9-N` (`N` being an increasing ordinal from brew)

The Jenkins imagestreams delivered by the Samples operator will be updated to use this new tagging scheme, with tags added for `latest`, `4`, and the current `4.<Y>` version.
The existing imagestream tag `2` will also be maintained as long as Jenkins is based on Jenkins v2.

#### Web Console

User will continue to be able to deploy Jenkins on OpenShift by instantiating our provided Templates.
As long as the Jenkins Pipeline strategy can be run on a fully supported cluster (i.e upgrades are allowed), existing console integrations with Jenkins should remain in place.

#### Telemetry / Operational Readiness

The Jenkins imagestream will continue to be delivered to OpenShift via the Samples operator.
If the Jenkins image fails to import, the existing ServiceMonitor for the Samples operator will alert that the imagestream failed to import after a reasonable period of time.
These warning alerts will be sent to OpenShift's telemetry for CEE/SRE to debug.

#### Documentation

After Jenkins is removed from the payload, documentation related to Jenkins images will need to add mirroring instructions for disconnected clusters.
Release notes will also need to inform admins that Jenkins images will need to be mirrored separately from the OpenShift payload.

### Risks and Mitigations

**Risk**: Jenkins imagestreams fail to import when the cluster is upgraded.

*Mitigation*: Imagestreams don't report new references in status until the import is complete.
Failures to import the new Jenkins imagestream will eventually fire an alert, unless the cluster admin skips the imagestream.

**Risk**: Jenkins imagestreams are not updated in disconnected clusters.

*Mitigation*: Documentation will need to provide new mirroring instructions for admins planning to upgrade.
Release notes should also clearly indicate that Jenkins is no longer provided in the payload and needs to be mirrored separately.

## Design Details

### Open Questions [optional]

None.

### Test Plan

This proposal is in the provisional state - test plan is not necessary at this time.

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

#### Dev Preview -> Tech Preview

Not required - this continues the deprecation of a GA feature.

#### Tech Preview -> GA

Not required - this continues the deprecation of a GA feature.

#### Removing a deprecated feature

The Jenkins pipeline strategy has been officially deprecated since OCP 4.1.
This proposal does not permanently disable the strategy - rather it changes how we deliver an on-top application.
Documentation will be updated to reflect areas where customers are impacted (mainly for network restricted environments).

### Upgrade / Downgrade Strategy

On upgrade, the Jenkins imagestream will be updated to reflect the new non-payload image by the Samples operator.
The existing templates are already configured to use ImageChange triggers to roll out updates, and should therefore trigger a rollout of any Jenkins provisioned by the template.
Future rollouts will likewise occur via the ImageChange trigger.

On downgrade, the Jenkins imagestream will be restored to its prior payload image.

### Version Skew Strategy

Imagestream imports should not be impacted by version skew - imagestreams delivered by the Samples
operator are not updated until the new version of the Samples operator is deployed and wins leader election.

## Implementation History

2021-07-20: Provisional enhancement draft
2021-08-24: Implementable version

## Drawbacks

Removing Jenkins from the payload will help with the mainenance burden, because Jenkins fixes will not need to be backported to each OCP release.
However, as long as the Jenkins pipeline strategy exists, the sync plugin will need to be maintained and updated.
This sync plugin has been a longstanding source of bugs, and requires continued updates with each OpenShift release.

This proposal also has a high impact on clusters that are run in network retricted environments.
Cluster admins are accustomed to mirroring Jenkins by mirroring the payload.
They will now need to take the extra step of mirroring the Jenkins images they need.

Cluster admins are also accustomed to updating Jenkins by updating their OpenShift clusters as a whole.
We will continue to maintain a Jenkins imagestream and use ImageChange triggers on supported templates.
Customers will need to ensure that their Jenkins deployments use the ImageChange trigger to maintain this capability.

## Alternatives

The only meaningful alternative is to keep the status quo and deliver Jenkins via the payload.

## Infrastructure Needed [optional]

- New CPaaS pipelines to produce the Jenkins images
- CI test suites which run against supported OCP releases.
  Currently we only have one CI suite which runs against the version of OCP under development.
