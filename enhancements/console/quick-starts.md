---
title: quick-starts
authors:
  - "@jhadvig"
  - "@rebeccaalpert"
reviewers:
  - "@spadgett"
  - "@alimobrem"
  - "@pweil"
approvers:
  - "@spadgett"
creation-date: 2020-06-02
last-updated: 2021-03-10
status: implementable
see-also:
  - "https://issues.redhat.com/browse/CONSOLE-2255"
  - "https://issues.redhat.com/browse/CONSOLE-2232"
  - "https://issues.redhat.com/browse/SRVLS-262"
  - "https://issues.redhat.com/browse/CONSOLE-2355"
---

# Quick Starts

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary

OpenShift's Serverless team has proposed an idea to create a "Quick Starts"
mechanism which introduces users to various ways of interacting with serverless
in the Console. Quick Starts should be a mechanism we can use to vastly improve
our customer's initial user experience on a empty cluster or with all various
workflows:

The goal of this proposal is to define a lightweight mechanism for OpenShift's
Console component, to guide users thought various workflows, and help them
understand the steps neccesary to get the desired outcome:

* Install operator
* Deployment of showcase application
* Cluster settings
* Networking
* ...

For Quick Starts we need a mechanism for their creation and publishment.

## Motivation

Help users with their understanding the principles of their workflows by guiding them though the necessary steps.

### Goals

1. Provide a mechanism to display user guides for various workflows.
2. Make users understand what are the steps needed to achieve their goals.
3. Provide new CRD format for writing the Quick Starts.
4. Have a repository for the out-of-the-box Quick Starts, those that describe how to install an operator or go though standard workflow.

## Proposal

* Introduce CRD format that will be used for writing Quick Starts.
* Introduce default repository for Quick Starts.

### User Stories

#### Story 1

As an administrator of an OpenShift cluster, I need a guide to walk me through how to install OpenShift Serverless modules (Serving and Eventing) in a cluster.

#### Story 2

As an administrator of an OpenShift cluster, I need a guide to walk me through how to update an OpenShift cluster.

#### Story 3

As a developer, I need a guide to walk me through how to deploy an existing application as a serverless workload.

#### Story 4

As a operator creator I want to provide operator consumers with a guide on how to install and user the my operator.

### Implementation Details

1. In order to provide mechanism to discribe a Quick Start, new CRD named `QuickStarts` will be created.
2. A new `openshift/quick-starts` repository will be created which will contain all supported Quick Starts. The `openshift/quick-starts` repository will have `release-*` branches.
3. `console-operator` will import all the existing Quick Starts CRs from appropriate branch of the `openshift/quick-starts` repository into the `/manifest` directory, so that the CVO can:
   * create them if the CR doesn't exists.
   * update the CRs upon the cluster update (since CVO is doing `apply`).
4. Steps in the Quick Starts will support basic markdown thats already in use in the OpenShift's Console.
5. All Quick Starts will be listed available in a separate page that will be accessible from Help Menu.
   ![help-menu](https://raw.githubusercontent.com/jhadvig/images/master/help-menu.png)


Quick Starts CR for [Explore Serverless](https://marvelapp.com/236ge4ig/screen/69908905):
```yaml
apiVersion: console.openshift.io/v1
kind: QuickStart
metadata:
  name: explore-serverless
  labels:
    console.openshift.io/lang: en
    console.openshift.io/name: explore-serverless
spec:
  displayName: Explore Serverless
  tags:
    - serverless
  duration: 10
  description: Install the Serverless Operator to enable containers, microservices and functions to run "serverless"
  prerequisites: Release requirements if any Install X number of resources.
  introduction: Red Hat OpenShift® Serverless is a service based on the open source Knative project. It provides ...
  tasks:
    - title: Install Serverless Operator
      description: The OperatorHub is where you can find a catalog of available Operators to install on your cluster ...
      review:
        instructions: Make sure the Serverless Operator was successfully installed ...
        taskHelp: Try walking through the steps again to properly install the Serverless Operator
      recapitulation:
        success: You've just installed the Serverless Operator! Next, we'll install the required CR's for this Operator to run.
        failed: Check your work to make sure that the Serverless Operator is properly installed
    - title: Create knative-serving API
      description: The first CR we'll create is knative-serving ...
      review:
        instructions: Make sure the knative-serving API was successfully installed ...
        taskHelp: Try walking through the steps again to properly create the instance of knative-serving
      recapitulation:
        success: You've just created an instance of knative-serving! Next, we'll create an instance of knative-eventing
        failed: Check your work to make sure that the instance of knative-serving is properly created
    - title: create knative-eventing API
      description: The second CR we'll create is knative-eventing ...
      review:
        instructions: Make sure the knative-eventing API was successfully installed ...
        taskHelp: Try walking through the steps again to properly create the instance of knative-eventing
      recapitulation:
        success: You've just created an instance of knative-eventing!
        failed: Check your work to make sure that the instance of knative-eventing is properly created
  conclusion: Your Serverless Operator is ready! If you want to learn how to deploy a serverless application, take the Serverless Application tour.
  nextQuickStart: serverless-application
```

#### Quick Start icon

Each Quick Start CR can contain an icon that is specified in the `spec.icon` field. This field is a base64 encoded image that are used as Quick Start's icon.

#### Quick Start's Access Review

Since different users might not have access to specific actions on
different resources, it would be pointless for them to go though a
Quick Start and be unable to complete its tasks. For that reason the
`spec` of the Quick Start CRD contains also `accessReviewResources`
field, that should contain an array of resources and actions done on
them, that user should perform during taking the Quick Start
tour. User's access to the Quick Start is reviewed based on array of
resource actions. In order for the user to see the Quick Start and
take its tour, his access review needs to pass all the listed
resources actions.

#### Quick Start Internationalization

OpenShift can be toggled between multiple languages. The Quick Start
CRD contains a series of labels to specify the Quick Start name and language:
```yaml
console.openshift.io/lang: en
console.openshift.io/country: gb
console.openshift.io/name: explore-serverless
```

The optional `console.openshift.io/lang` label specifies the two-letter
[ISO 639-2 Language Code](https://www.loc.gov/standards/iso639-2/php/code_list.php))
(i.e. "en" for English) for the Quick Start. Quick Starts without this label will be treated
as English Quick Starts.

The optional `console.openshift.io/country` label specifies the alpha-2
[ISO-3166 Country Code](https://www.iso.org/iso-3166-country-codes.html)
(i.e. "gb" for the United Kingdom of Great Britain and Northern Ireland).
Quick Starts without this label will only rely on the `console.openshift.io/lang`
behavior.

The `console.openshift.io/name: explore-serverless` label specifies the name of the
Quick Start. This should be consistent across all translations of a given Quick Start,
as it allows us to avoid showing the same Quick Start content in multiple languages.
If a Quick Start is not translated into a user's language, the English version will be shown.

OpenShift can display the correct
Quick Starts for each language by comparing the value of the
`console.openshift.io/lang` and `console.openshift.io/country` labels to the currently displayed language in OpenShift.
See the [internationalization enhancement proposal](https://github.com/openshift/enhancements/blob/master/enhancements/console/internationalization.md) for more information.
This approach will not handle languages we don't offer support for. If Quick Starts
in non-supported languages become a need, we will need to modify this approach.

Example Explore Serverless Quick Starts CR snippet for Japanese:
```yaml
apiVersion: console.openshift.io/v1
kind: QuickStart
metadata:
  name: explore-serverless
  labels:
    console.openshift.io/lang: ja
    console.openshift.io/name: explore-serverless
```

Example Explore Serverless Quick Starts CR snippet for English (the United Kingdom of Great Britain and Northern Ireland):
```yaml
apiVersion: console.openshift.io/v1
kind: QuickStart
metadata:
  name: explore-serverless
  labels:
    console.openshift.io/lang: en
    console.openshift.io/country: gb
    console.openshift.io/name: explore-serverless
```

#### Air Gapped Environments

Since the supported Quick Starts CRs will be part of the `console-operator`'s manifests, they will be distributed together with image.

### Third party Quick Starts

Third party Quick Starts will need to be created by an operator after it's installed or manually by a cluster administrator.
