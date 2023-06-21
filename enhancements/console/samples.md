---
title: samples
authors:
  - "@jerolimov"
reviewers:
  - "@vikram-raj"
  - "@spadgett"
  - "@JoelSpeed"
approvers:
  - "@spadgett"
api-approvers:
  - "@JoelSpeed"
creation-date: 2023-05-09
last-updated: 2023-06-21
tracking-link:
  - https://issues.redhat.com/browse/ODC-7241
see-also:
  - "/enhancements/console/quick-starts.md"
---

# Samples

## Summary

The Developer Console contains a sample list that helps users initialize workloads and play with features like the "Import from Git" flow.

Until 4.13, the [samples-operator](https://github.com/openshift/cluster-samples-operator) and the external Devfile registry ([registry.devfile.io](https://registry.devfile.io/viewer?types=sample)) provide these samples.

A `ConsoleSample` CRD will allow other teams, operators, and customers to add additional samples to the console.

## Background info

The samples are one of the main entry points to create a Kubernetes resource from the Developer perspective:

![Screenshot of the add page that shows a lot of option, incl. an option to "View all samples"](samples-add-page.png)

The user can select one of the many samples:

![Project access tab with role select field to select an access role](samples.png)

Currently, this opens a form to import a git repository. Other sample types aren't supported at the moment.

## Motivation

Creating a `ConsoleSample` CRD will promote the samples feature in the Developer Console to an extendable feature similar to QuickStarts. This makes it much easier for other teams to provide their own samples, for example with their operator.

Samples are no longer tied to the exact OpenShift version and could be updated with the operator. Also customers can add their own samples if needed.

This will also allow us to migrate from the (deprecated) [samples-operator](https://github.com/openshift/cluster-samples-operator) to a simple custom resource in the future and to add other sample types in the future.

### User Stories

* As an OpenShift Serverless engineer, I want to provide additional samples that are tied to an operator version, not an OpenShift release.
* As a cluster administrator, I want to add additional samples to the Developer Console sample catalog.

### Goals

1. This feature should allow other teams to provide their Developer Console samples without applying them to the sample-operator or the console-operator.
2. Samples should be allowed to localize/translated.

### Non-Goals

1. We don't want to turn off the support for the (deprecated) [samples-operator](https://github.com/openshift/cluster-samples-operator) right now. So the console will not provide any samples on its own (in 4.14).
2. A way to hide installed samples. Operators creating a sample resource should have a way to opt out of their samples.

## Proposal

### Workflow Description

The Console Operator will provide a cluster-wide `ConsoleSample` CRD.

Other operators like the OpenShift Serverless operator will install their own `ConsoleSample` CRs.

The web console will automatically load all `ConsoleSample` CRs and show them in the sample catalog.

Users can select one of samples and follow the import flows to create a demo application.

### API Extensions

The console operator automatically installs a new `ConsoleSample` CRD.

### Risks and Mitigations

#### The OpenShift Console is optional and could be disabled

Other operators should expect that the `ConsoleSample` CRD is unavailable like the other `Console*` CRDs.

### Drawbacks

Another extra (but specific) custom resource just for console samples.

## Design Details

An "Import from Git" example of how we have it today as part of the Builder Images annotation:

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleSample
metadata:
  name: java-sample
spec:
  title: Java
  description: |
    Build and run Java applications using Maven and OpenJDK.
    Sample repository: https://github.com/jboss-openshift/openshift-quickstarts
  icon: base64 encoded image
  provider: Red Hat
  badge: Serverless function
  tags:
  - java
  - jboss
  - openjdk
  source:
    type: GitImport
    gitimport:
      url: https://github.com/jboss-openshift/openshift-quickstarts
```

An "Import container image" example:

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleSample
metadata:
  name: minimal-ubi-container
spec:
  title: Minimal UBI container
  description: |
    The Red Hat Universal Base Image is free to deploy on Red Hat or non-Red Hat platforms
    and freely redistributable...
  icon: base64 encoded image
  provider: Red Hat
  badge: Empty container
  source:
    type: ContainerImport
    containerimport:
      image: registry.access.redhat.com/ubi8/ubi-minimal:8.8-860
```

### Localization

The `ConsoleSample` resources provide the same optional localization annotations then `ConsoleQuickStarts`.

The `console.openshift.io/name: explore-serverless` label specifies the name of the sample. This should be consistent across all translations of a given sample,
as it allows us to avoid showing the same Quick Start content in multiple languages.

If a sample is not translated into a user'ss language, the english version will be shown.

See the [internationalization](https://github.com/openshift/enhancements/blob/master/enhancements/console/internationalization.md) and [QuickStart enhancement proposal](https://github.com/openshift/enhancements/blob/master/enhancements/console/quick-starts.md) for more information.

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleSample
metadata:
  name: java-sample
  annotations:
    console.openshift.io/name: java-sample # optional, metadata.name is also fine
spec:
  title: Java sample
  description: |
    Build and run Java applications using Maven and OpenJDK.
    Sample repository: https://github.com/jboss-openshift/openshift-quickstarts
  icon: base64 encoded image
  provider: Red Hat
  badge: Serverless function
  tags:
  - java
  - jboss
  - openjdk
  source:
    type: GitImport
    gitimport:
      url: https://github.com/jboss-openshift/openshift-quickstarts
---
apiVersion: console.openshift.io/v1
kind: ConsoleSample
metadata:
  name: java-samples-de
  annotations:
    console.openshift.io/lang: de
    console.openshift.io/name: java-sample # same as annotation or metadata.name above
spec:
  title: Java Beispiel
  description: |
    Beispiel Java Anwendung basierend auf Maven und OpenJDK.
    Repository: https://github.com/jboss-openshift/openshift-quickstarts
  icon: base64 encoded image
  provider: Red Hat
  badge: Serverless function
  tags:
  - java
  - jboss
  - openjdk
  source:
    type: GitImport
    gitimport:
      url: https://github.com/jboss-openshift/openshift-quickstarts
```

### Test Plan

Provide e2e tests as part of the console that adds a `ConsoleSample` and verify that it was shown in the UI.

### Graduation Criteria

This feature will be released directly as GA.

The Knative/Serverless team and operator might be the first user of this resource.

#### Dev Preview -> Tech Preview

N/A

This feature should be released directly as GA.

The risk is low since this CRD will not consumed by any operator and similar to existing console CRDs like `ConsoleQuickStart`.

#### Tech Preview -> GA

N/A

This feature should be released directly as GA.

The risk is low since this CRD will not consumed by any operator and similar to existing console CRDs like `ConsoleQuickStart`.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The console-operator will install the CRD on an upgrade.

Other operators should install the sample resources then automatically.

A downgraded (old) console version will ignore the sample resources.

### Version Skew Strategy

The CRD can be extended with new sample metadata or types in the future.

The web console is the only consumer of this configuration for now and handles the resources just in the frontend. The console operator will not consume or update the sample resources.

Other operators will create and maintain their samples as cluster wide resources.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

* Initial version
* Added localization

## Alternatives

### Add annotations to the Builder images

The console shows samples already for some "Builder Images". For this, the annotation of some ImageStreams was used. It might be possible to add additional annotations to this ImageStreams.

```diff
  kind: ImageStream
  metadata:
    name: java
    annotations:
      samplesRepo: https://github.com/jboss-openshift/openshift-quickstarts
+     serverlessFunctionSampleRepo: https://github.com/knative/...
+     serverlessFunctionSampleTitle: Java Serverless function sample
+     serverlessFunctionSampleDescription: ...
```

### Multiple samples in one sample resource

The CRD could provide multiple samples in one custom resource.
In an initial code review, we decided that a single sample per resource
is more aligned with other custom resources.

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleSample
metadata:
  name: java-samples
spec:
  samples:
    - type: gitimport
      title: Java
      description:  Build and run Java applications using Maven and OpenJDK.
      provider: Red Hat
      tags:
      - java
      - jboss
      - openjdk
      gitimport:
        url: https://github.com/jboss-openshift/openshift-quickstarts
```

### Using OpenShift Templates

An [OpenShift Template](https://docs.openshift.com/container-platform/4.13/openshift_images/using-templates.html) are also example of how to start application running on OpenShift Container Platform.

The web console supports OpenShift templates. They are also shown in the "Developer Catalog". But the template focuses on creating Kubernetes resources instead of delivering web console features like the git import.
