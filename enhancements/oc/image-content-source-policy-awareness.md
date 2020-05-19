---
title: image-content-source-policy-awareness
authors:
  - "@sallyom"
reviewers:
  - "@smarterclayton"
  - "@soltysh"
  - "@wking"
approvers:
  - "@smarterclayton"
creation-date: 2020-05-19
last-updated: 2020-08-04
status: implementable
---

# ImageContentSourcePolicy (ICSP) Awareness

## Summary

_Note: In this proposal, 'user-given image' is an image passed on the command line.  The 'underlying image-reference' is the original image, before
it was mirrored.  These 2 may or may not be the same, but in the case of mirrored images, a mirrored image retains its reference to the
original location, and it is this reference that is currently used by `oc adm release`.  Because of this, `oc adm release` commands fail when working with
mirrored images in disconnected environments._ 

ICSP allows OpenShift (CVO, CRI-O) to check down a list of possible mirrors to find an image with the matching digest it is
looking for.  `oc` should do the same.  If an `oc adm release` command fails with the user-given image's underlying image reference, then try to access an ICSP.
If no ICSP found, then try user-given image.

There have been several bugs opened around the experience of a 
user in a disconnected environment using `oc adm release` commands.  If
using a mirrored image and the mirrored source registry is disconnected, 
the following commands do not succeed when in a disconnected environment:

```console
$ oc adm release extract --tools registry.example.com/repo/name:tag
$ oc adm release mirror registry.example.com/repo/name:tag --to someregistry/repo/name
```

This is because the mirrored image tags (the individual component images from a payload)
retain references to the mirrored registry, usually something like 
`quay.io/openshift-release-dev/ocp-v4.0-art-dev`.  

There needs to be logic in `oc` to look for `ImageContentSourcePolicy` from a cluster.
`oc` should look for `ICSP` in the cluster/current context if connected, if the user has permission to 
access ICSPs, and if the current flow of using the image-reference from a given image fails.
With some but not all `oc` commands, users expect that when they interact with a cluster, the cluster's context informs their action.
For example, `oc adm release info` without passing a release will lookup the current cluster context.  However, when a cluster context is unclear
or there's the possibility that the connected cluster could be a different target than expected, `oc` should not default to connecting to
the cluster.  In light of this, `oc adm release` commands should try the current flow of using the image reference from the user-given image first.
If this flow fails, `oc` should gather information about RepositoryDigestMirrors from ICSP and use that
when extracting or mirroring images.  If not currently connected to a cluster or if ICSP is not accessed, silently move on and log at a high debug level.
Lastly, `oc` will try to use the user-given image.  If all fail, return the error returned from the original attempt.
However, if a user passes a flag to explicitly use an ICSP - if the ICSP lookup fails, fail fast and don't proceed.  

Current bugs regarding this Issue:   
* https://bugzilla.redhat.com/show_bug.cgi?id=1823839
* https://bugzilla.redhat.com/show_bug.cgi?id=1823143 and also for 4.3, 4.5, 4.6


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Proposal

* `oc adm release mirror` writes an ICSP file to the current directory or wherever you specify 
* Add logic to `oc adm release` to become aware of ICSP in cluster
* Add logic to `oc adm release` to use an ICSP file to complete extracts, info and mirroring.

## Design Points

* `oc` will use context of the cluster when the action the user is taking is clearly expected to talk to the cluster, such as with `oc get` or
`oc adm release info` without passing a specific release.  If that expectation is not clear, `oc` will not default to connecting to a cluster.
* When a user's action is not obviously expected to connect to the cluster, such as in `oc adm release info release:tag`, the CLI _may_ attempt to load
ICSP from an existing cluster, but if that fails, the CLI _must_ ignore that failure, by logging it at a high debug level.
* In the case where the CLI attempts to lookup the ICSP in order to help the user, that will happen after the first attempt to retrieve the content from the
location fails, in which case the ICSP should be looked up and an attempt made to find the alternate location from the ICSP sources.  As a final attempt, the
CLI will try the user-given image, ie, if a user provided the mirrored-registry/repo/release:tag look there rather than original-registry/repo/release:tag.
If all of those fail, the original error (the error you'd get from the current flow of looking up the original source of a release) must be returned.
* When a user specifies an explicit ICSP, the CLI will fail if that ICSP cannot be loaded, and the order defined in the ICSP will be honored.

## Flags/Decided against

* Thoughts on flags to add:
    * Flags required:
        * *--release-image-icsp-to-dir* will define where to write an ICSP file to.  If unset, `oc adm release mirror` will write to current directory.
        * *--icsp-file* will define where to get ICSP from a file.  If set, `oc adm release extract|mirror` will use this ICSP data.
    * Flags decided against:
        * boolean `--use-icsp` and if true, check for cluster and/or icsp file?  This is problematic, because even if a user is currently connected to a
        cluster, it doesn't mean they want to use information from that cluster with an `oc adm release ...` command.  
        So here, we'd need a different `--cluster-icsp` boolean and `--icsp-file` string flag.  The flags are adding up here, and that is not desireable. 
        * string `--image-content-source aregistry/arepo/arelease`. I don't like this, because it would be redundant for a user to run something like this: 
        `oc adm release extract --command oc myreg:5000/myrepo/release:tag --image-content-source myreg:5000/myrepo/release`.  
        * boolean `--set-prefix` would allow a user to specify "I want to use the prefix of the release image I have specified, 
        rather than any underlying image reference."
* This proposal is to introduce 2 new flags, something like `--release-image-icsp-to-dir` to designate where to write an ICSP file and `--icsp-file` that will specify
an ICSP file to use (rather than from a cluster).  In the absense of the flag `--icsp-file`, `oc` will try the image-reference from a user-given image
(the current flow) and if that fails, will try using an ICSP from a connected cluster.  If those fail, `oc` will try to use the user-given image. 
* See `User Stories` below for examples. 

## User Stories

Given a `mirrored-registry.example.com/repo/release:tag`, mirrored from `registry.example.com/repo/release:tag`, a user runs:  

1. `oc adm release extract --tools mirrored-registry.example.com/repo/release:tag`
    * `oc` will proceed to lookup the registry.example.com/repo/release@toolsha.  If that fails, will look for ICSP and if found, will extract from the
    ICSP mirror (mirrored-registry/arepo/release@toolsha) rather than the original that the user will not have access to if in disconnected environment.
    If ICSP not found, will try to extract from the user-given mirrored-registry.example.com/repo/release@toolsha. The extract will succeed if user has
    access and permission to any of these.  It will fail with the error from the first try, and will log at high level other failed attempts.
2. `oc adm release extract --icsp-file /path/to/icsp.yaml --tools mirrored-registry.example.com/repo/release:tag`
    * `oc` will try to use data from the icsp file, it will extract from the ICSP mirror (mirrored-registry.example.com/arepo/release@toolsha).
    It will fail fast if ICSP lookup fails if the --icsp-file flag is provided.  
3. `oc adm release extract --icsp-file /path/to/icsp.yaml --tools registry.example.com/repo/release:tag`
    * `oc` will try to use data from the icsp file, it will try to extract from ICSP mirror sources, in the order they appear in the file.
    In this case will extract from mirrored-registry.example.com/arepo/release@toolsha.
    It will fail fast if ICSP lookup fails if the --icsp-file flag is provided.  
3. `oc adm release mirror --release-image-icsp-to-dir /path/to/file registry.example.com/repo/release:tag --to mirrored-registry.example.com/repo/release`
    * `oc` will write an ICSP file to provided path via --release-image-icsp-to-dir (or similar) flag.  This is similar to how a release-image-signature file is written 
    during `oc adm release`.  A parallel flag to the current `--release-image-signature-to-dir` will be introduced, `--release-image-icsp-to-dir`, rather than combining
    the two files into a single flag, because enough users are already using the signature-to-dir flag.
    In the absence of the flag, the ICSP file will be written to a configured path or current directory. 

## Alternatives

Added logic in `oc adm release mirror|info|extract` to replace the `registry/repo/name` of a referenced image with a user-given image.  This
worked, but was a hack.  ICSP awareness needs to be added. 

