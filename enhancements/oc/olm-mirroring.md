---
title: Compact Clusters
authors:
  - "@ecordell"
reviewers:
  - "@smarterclayton"
  - "@shurley"
  - "@mhrivnak"
  - "@soltysh"
approvers:
  - "@soltysh"
creation-date: "2019-10-16"
last-updated: "2019-10-22"
status: implementable
see-also:
replaces:
superseded-by:
---

# OC Tooling for Disconnected OLM

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement outlines the changes necessary to `oc` to support disconnected distribution of OLM catalogs. It relies on the [Disconnected OLM enhancement](https://github.com/operator-framework/enhancements/blob/master/enhancements/opm-mirroring.md).

## Motivation

Disconnected installation of OLM catalogs can be done with the following `opm` commands:

```
$ opm registry build --appregistry-namespace=quay.io/community-operators --to disconnected-registry:5000/openshift/community-operators-catalog:4.2.1 
pulling quay.io/community-operators/etcd...
buliding catalog...
mirroring catalog...
quay.io/community-operators mirrored to disconnected-registry:5000/openshift/community-operators-catalog:4.2.1

$ docker pull disconnected-registry:5000/openshift/community-operators-catalog:4.2.1

$ opm registry images --from=disconnected-registry:5000/openshift/community-operators-catalog:4.2.1 --to=disconnected-registry:5000/community-operators --manifests=./mirror-manifests | xargs oc image mirror

$ oc apply -f ./mirror-manifests
```

This requires both `oc` and `opm` to be installed and available on bastion hosts. We would like to distribute one tool for creating disconnected catalogs, not two.

### Goals
* Define `oc` commands which can build and mirror all images required for mirroring to a disconnected environment

### Non-Goals
* Provide an api for fine-grained control over which operators / operands are mirrored. 

## Proposal

The basic approach will be to:

- Generate catalog container images that can replace the appregistry repositories
- For each operator within the catalog image, mirror it to the disconnected environment

### Generate and mirror a catalog

`oc` will be extended with a command (equivalent to the `opm` command)

```sh
$ oc adm catalog build --appregistry-namespace=community-operators --to quay.io/ecordell/community-operators-catalog:4.2.1 
```

`oc adm catalog build` will:

- Use appregistry protocol to retrieve all of the operators in a namespace of Quay.io specified by `appregistry-namespace`
- Load all of the downloaded operators into a versioned `operator-registry` sqlite artifact
- Build a runnable operator-registry image by appending the database to the `operator-registry` base image using the same machinary as `oc image append`
- Mirror that image to the target defined by `--to`, using the same machinary as `oc image append`

It will have the following flags:

- `--from=ref` - the base image to add the built operator database into. Defaults to the operator-registry image shipped with the version of OpenShift that `oc` came from.
- `--to=ref` - the location that the image will be mirrored.
- `--auth-token=string` - the auth token ([instructions](https://github.com/operator-framework/operator-courier#authentication)) for authenticating with appregistry.
- `--appregistry-endpoint=url` - the CNR endpoint to authenticate against. Defaults to `"https://quay.io/cnr"`, the endpoint used by OpenShift 4.1-4.3.
- `--appregistry-org=string` - the organization (namespace) in appregistry to mirror. Each repository within the namespace represents one operator. 
- `--to-db=path` - if set, the operator database file will be saved at this path.

This command generates and mirrors in one step, because we do not assume that there is any registry available aside from the target disconnected registry.

### Extract the contents of a catalog for mirroring

`oc` will be extended with a second command:

```sh
$ oc adm catalog mirror --to-manifests=./mirror-manifests quay.io/ecordell/community-operators-catalog:4.2.1 localhost:5000/community-operators/
mirroring ...

$ ls ./mirror-manifests
imagecontentsourcepolicy.yaml
catalogsource.yaml
```

`oc adm catalog mirror` will:

- Pull the catalog image referenced by the first argument, using `oc` machinery
- Read the database to get the list of operator and operand images
- Build a mapping of images to the disconnected registry
- Mirror all of the referenced images to the registry namespace in the second arg by (interally) calling `oc image mirror`
- Output a set of manifests that, if applied to a cluster that has access to the mirrored images, will correctly configure nodes and OLM to use those images.

`oc adm catalog mirror [FLAGS] SRC DEST` has the following arguments

- `SRC` - the source catalog image to extract content from, such as one built by `oc adm catalog
  build`).
- `DEST` - the destination registry namespace to mirror content into. Should be in the form
  `registry/namespace/`

and the following flags:

- `--to-manifests=path` - default `./manifests`, the path at which manifests required to mirror these images will be created. This includes an `ImageContentSourcePolicy` that can configure nodes to translate between the image references stored in operator manifests and the mirrored registry, and a `CatalogSource` that configures OLM to read from the mirrored catalog image referenced by `--from`.

### Full Example

```sh
$ oc adm catalog build --appregistry-org=community-operators --to disconnected-registry:5000/openshift/community-operators-catalog:4.2.1 
pulling quay.io/community-operators/etcd...
buliding catalog...
mirroring catalog...
quay.io/community-operators mirrored to disconnected-registry:5000/openshift/community-operators-catalog:4.2.1

$ oc catalog mirror disconnected-registry:5000/openshift/community-operators-catalog:4.2.1 disconnected-registry:5000/community-operators/

$ oc apply -f ./manifests
```

## Deprecation Plan 

`oc adm catalog build` is only required for building catalog images from appregistry namespaces, which is supported in openshift 4.1-4.3. 

Support for appregistry catalogs will be deprecated for 4.4, and after that point, support for `oc adm catalog build` can be removed. The images to feed to `oc adm catalog mirror` will be available for mirroring without further work by a user.
