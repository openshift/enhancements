---
title: upgrade-preflight-checks-for-operators
authors:
  - "@LalatenduMohanty"
reviewers:
  - "@dgoodwin"
  - "@crawford"
  - "@sdodson"
  - "@wking"
approvers:
  - "@crawford"
  - "@sdodson"
creation-date: 2020-06-04
last-updated: 2020-09-02
status: implementable
---

# Upgrade Preflight Checks For Operators

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of OpenShift 4 is to make upgrades seamless and risk free.
To improve the upgrade success we should be able to detect if the new version is compatible with the existing version before running the actual upgrade.
As OpenShift V4 is composed of many operators, we need to give the operators ability to run some checks before the upgrade to find out if the new incoming version is compatible with the existing version.

## Motivation

CVO managed operators should be able to run preflight checks before going through the actual upgrade.
It would help operators to find out if they can move to the specific version safely before doing an actual upgrade.
Also administrators are empowered check if upgrade to the target version is going to be successfully without doing an upgrade.
Use preflight checks to improve the success rate of upgrades.


### Goals

* This enhancement proposes a framework for preflight checks and discusses the CVO side implementation of that framework.
* Administrators will be able to run preflight checks without running the upgrade.
* Give Operators (CVO managed) ability to add checks to check if they can safely upgrade to a given release.
* Preflight checks would be idempotent in nature.
* Downloading and verification of images associated with release payload should be part of preflight check.

### Non-Goals

* This enhancement does not commit particular operators for providing preflight implementations or discuss the operator-specific logic that should be executed by preflight checks.
* The preflight checks should not be used as a migration mechanism.
* It is not the responsibility of preflight checks to protect against upgrades involving more than one minor version i.e. 4.y.z to 4.(y+2).z.

## Proposal

Operators need to provide a different bin path for the preflight checks i.e. /bin/preflight.
The bin path need to exist inside of the operator image.
Cluster version operator (CVO) will be running a single job per operator for the preflight checks.
/bin/preflight must be executable. It can be symbolic link to the operator binary or a script or It can be non existent.
Users can skip or override the preflight checks with `--force`.

### User Stories

#### Story 1

Some customers would prefer to disable automatic credential provisioning by Cloud credential operator (CCO) and instead manage it themselves.
However if new permissions are required by the target version, the upgrade could block midway even if the administrator provisioned more powerful credentials.
With the help of preflight checks CCO can check if they can safely upgrade to a given release before doing the actual upgrade.

#### Story 2

cluster-ingress-operator uses ingress controller as the operand. The cluster-ingress-operator configures the ingress controller using an administrator-provided default serving certificate.  When we moved to RHEL8 base image it brought a new version of OpenSSL that tightened restrictions on TLS keys, which caused the new operand image i.e. ingress controller to reject keys that the old operand image accepted ([BZ#1874278](https://bugzilla.redhat.com/show_bug.cgi?id=1874278)).

In this case a preflight check can be added for cluster-ingress-operator that would use the new operand image to verify that it did not reject the current certificate in use.

#### Story 3

The MCO could make use of these to help with the migration off of v2 Ignition configs (we are moving to v3). This is a similar situation to the one described by the Cloud Credential Operator described in [user story 1](#story-1). Alternative would be to use Upgradeable=false, but that requires adding the check one release before we are going to remove it. It's certainly possible, but it's a whole lot simpler to add the check at the same time (and in the same place) that we remove support for v2.

#### Story 4

Currently operators set upgradeable = false if they do not want the cluster to upgrade to another version. But this might not be effective in below situation but preflight checks would be better. 

* Assume Cluster is running release A.
* Cluster begins updating to B, but operator "foo" has not yet updated to B.
* Admin re-targets cluster to release C, but there are some AB-vs-C incompatibilities for operator "foo".

In the "upgradeable = false" world even if we need to backport Upgradeable=False checks to release B to check the incompatibilities. So lets version B has the right checks to find the incompatibility. It is possible that operator "foo" still in version A and hasn't learned about the check. Because operator "foo" is not in version B yet it is not in a place to perform the check. So the update to C will not be blocked.

In the above scenario the edges from A to C might be blocked but the code performing the check may be from release A. Blocking the edge from A->C would not be sufficient to ensure folks hit operator "foo" in release B before leaping over to C.

In a preflight world, the checking code in release C and this will ensure we check the compatibility before upgrading. We are isolated from any version-skew due to heterogenous versions of operator in a cluster.

### Implementation Details

CVO will run the preflight check from the new operator image to which the current operator image is trying to upgrade.
Because the current version of operator would not have knowledge of what code changes coming with the new version.
So its the incoming version's responsibility to figure out if the current image can update safely to it.
However CVO need to go through signature verification to pass on the target release before we launch preflight checks.

CVO will look for manifests with a specific annotation i.e. openshift.io/preflight-check: "true" and use the manifests to start the preflight jobs.

Some more important points:

* Operators can choose the name and namespace for the job and there will be no restriction from CVO on this.
* However when CVO will run these jobs it will append some randomized suffix for keeping the jobs unique. Because there can be many preflight jobs running at the same time, so we need a way keep them unique.
* Preflight checks should not mutate any existing resources as it will make the checks non-idempotent. So it is advisable to give read only access to preflight checks.


#### Registering for preflight checks

Operators need to register themselves for a preflight check by defining a Kubernetes job (v1) manifest.
Preflight checks will be skipped for operator which does not have the manifest.

An example of preflight job manifest
```
$ cat 0000_50_cloud-credential-operator_11_preflight-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: cloud-credential-preflight-checks
  namespace: openshift-cloud-credential-operator
  annotations:
    openshift.io/preflight-check: "true"
spec:
  template:
    spec:
      serviceAccountName: openshift-cloud-credential-preflight
      containers:
      - name: cloud-credential-operator
        image: quay.io/openshift/origin-cloud-credential-operator:latest
          resources:
            requests:
               cpu: 10m
              memory: 150Mi
        command: ["/bin/preflight",  "foo", "baar"]
  backoffLimit: 4
```

The standard [images-references] replacement applies will be apllicable to this manifest too.

*Note:*
To give specific access to preflight jobs, you need to create a new ServiceAccount which the job can use and give the ServiceAccount required RBAC.

#### Triggering execution of preflight checks

We should be able to run the preflight check as below

##### Case1

Preflight checks would run implicitly right after the update is selected, as we currently do for preconditions checks.

##### Case 2

Run preflight checks on-demand for cluster admins using `--dry-run`.
Example:

```sh
oc adm upgrade --to-latest --dry-run
```

#### Collecting results from preflight checks

The preflight job for a release will be created with an unique label i.e. the set of preflight jobs triggered for doing a preflight check for a specific version will have the unique labels.
This unique label will be used to keep track of the jobs and collect result from these.
Also at any point of time there might be many preflight jobs running in the cluster.
So it would also help to keep track of the preflight jobs based on when it was started.
To collect and display the result we can query for all the jobs by the label.

The objective is to collect the results i.e. pass or fail status and error message from preflight jobs.

### Risks and Mitigation

The responsibility of using implementation of actual preflight tests are on respective operator developers. So any bugs in the tests can create false positives or false negative results.

## Design Details

### Open Questions

1. Should we save the preflight history? Will it be really useful for users?
2. Tf the pre-flight is already running for a specific release and user again starts the preflight for the same image (same signature) we can just tell the user that the preflight is already running and here are the results so far, right?

### Test Plan

To be done

## Drawbacks

The current proposal does not support preflight checks for a new operator. As preflight checks should not be responsible for deploying new operators. Also we do not want preflight checks to change anything as it will complicate the design.

## Alternatives

None

[images-references]: https://github.com/openshift/cluster-version-operator/blob/d6475eff00192a56cd6a833aa7f73d6f4f4a2cef/docs/dev/operators.md#how-do-i-ensure-the-right-images-get-used-by-my-manifests