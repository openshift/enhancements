---
title: cluster-logging-log-forwarding
authors:
  - "@jcantrill"
  - "@igor-karpukhin"
  - "@ewolinetz"
reviewers:
  - "@bparees"
  - "@jcantrill"
  - "@igor-karpukhin"
  - "@alanconway"
  - "@ewolinetz"
approvers:
  - "@ewolinetz"
  - "@jcantrill"
creation-date: 2019-09-17
last-updated: 2019-10-25
status: implementable
see-also:[]
replaces:[]
superseded-by:[]
---

# cluster-logging-es-init

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of moving Elasticsearch initialization logic to the elasticsearch-operator is to reduce the number of times these init scripts are executed.
At the moment we keep these scripts in the ES image, and execute them everytime when ES pod starts. 


### Goals
The specific goals of this proposal are:

* Provide an API that allows users to specify init procedures for the ES cluster
* Allow users to choose what command has to be executed during the init process for NEW and UPGRADE elasticsearch cluster states

## Proposal

The following changes will define how the elasticsearch-operator executes init process for Elasticsearch cluster fresh start and upgrade

* Adding additional section to the elasticsearch-operator called `actions`, which may look like this:

    ```json
    "actions": {
      "image": "quay.io/openshift/origin-logging-elasticsearch6:latest",
      "when": {
          "postNew": "init.sh"
          "postUpgrade": "init_upgrade.sh"
      }
    },
    ```
  The values for actions are commands to be executed from the image
  * `postNew` - execute command after deploying a new cluster 
  * `postUpgrade` - execute command after a cluster upgrade
  
  If this section is not defined at all, nothing will be executed.
  
  If the `image` is provided by the user, `actions` section also has to be filled
  with both actions and their commands. Each `action` refers to a path on the `image`
 
* Adding additional `ClusterConditionType` for each action 
* Executing given scripts with the image that contains all the init logic against the ES API node
* Initialization logic will be executed from outside the ES container by creating a `Job`
* ES cluster will be become available **after** the initialization. During the initalization cluster won't be visible for extrnal service.

The following environment variables are passed into the `Job` by the ES operator:
* `ES_URL` - url to ES API server
* `CA_PATH` - path to mounted certificates
* `CLIENT_CERT_PATH` - path to client ceritificate
* `CLIENT_KEY_PATH` - path to client key

ES operator will treat scripts as successfully finished if the return code from the script is equal to 0. Return codes different from 0 will be treated as an error.

