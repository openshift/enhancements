---
title: marketplace-images
authors:
  - "@patrickdillon"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sdodson"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 2024-07-12
last-updated: 2024-10-14
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "SPSTRAT-271"
see-also:
replaces:
superseded-by:
---

# Cloud-Marketplace Images in the RHCOS Stream

## Summary

This enhancement proposes a process for including cloud (AWS, Azure, & GCP)
marketplace images in the coreos stream. RHCOS tooling (`plume cosa2stream`
and, perhaps, `stream-metadata-go`) would be updated to find the latest available 
marketplace images and include them in the stream.

By using a process of discovery for marketplace images during the RHCOS stream generation
the publication of the image is decoupled from the build. The result would be
that the generated RHCOS stream will always include the most recent marketplace images.
Practically speaking, this means that during the initial bump for a new RHCOS build the
generated stream would continue to use the previous marketplace images. The
same stream generation command would be rerun to update the marketplace images
as they become available. 

In order to include a marketplace image, criteria would need to be defined in order
to select the images via the cloud API. A concrete implementation for extending
`plume cosa2stream` to update Azure marketplace images is presented, with the
intention that this will provide the groundwork for other cloud marketplaces.


## Motivation

Marketplace images are officially supported, but are not
included in the RHCOS stream, which is the canonical source
for images. This omission results in a degraded experience
for both users and developers:

* As marketplace images are not included in the coreos
stream, users must manually enter the image details.
With these changes, the experience will be improved
so that users can simply indicate they want to use
marketplace images in the install config and the installer
will select the appropriate image.
* On Azure, marketplace images are the preferred (and,
in the case of ARO, only supported) method for distributing
production-grade images. Including these images in the stream
would allow the installer to undo the workarounds required for
non-marketplace images and thereby dramatically speed up installs;
as well as, unblocking ARO HCP.
* Integrating ROSA marketplace images (which this enhancement enables
but does not achieve) into the stream would unblock ROSA CI

### User Stories

* As an OpenShift developer, I want RHCOS marketplace images to be included in the stream,
so I can use them in my application.
* As an `openshif-install` user, I want to specify a boolean value for marketplace images in
the install config so that I can utilize marketplace images without looking up specific version numbers.

### Goals

* A well defined standard pattern for including marketplace images, published out-of-band, in the RHCOS streams.
* Provide an example Azure implementation that will scale to other clouds

### Non-Goals

* To determine the publication process for marketplace images.
* Future work: define the install-config API for using marketplace images

## Proposal

Marketplace images will be included in the
`.architectures[("aarch64","x86_64")].rhel_coreos_extensions` fields. 

In order to populate the stream, the RHCOS tooling (such as `plume`) would be extended
to query the cloud API (`plume` is already setup to utilize these APIs) to
find the marketplace images based on criteria defined for that particular image. If a
marketplace image is found that matches the version of the release being generated,
it is used in the stream. If not, fall back to the most recent version available.

The marketplace image bumps will lag behind the initial image bumps, so the first
bump for version `x.y.z` will (typically) include marketplace images at `x.y.z-1`. #TODO: this is enforced by convention
Once marketplace images are available, the command can be rerun to bump marketplace
images to `x.y.z`. See the open question below about whether this pattern is unacceptable
to `plume cosa2stream`'s idempotency guarantees.

### Workflow Description


**cosa2stream command** is the specific invocation of the plume command, for example:
```
plume cosa2stream --target data/data/coreos/rhcos.json                \
    --distro rhcos --no-signatures --name 4.18-9.4                    \
    --url https://rhcos.mirror.openshift.com/art/storage/prod/streams \
    x86_64=418.94.202410090804-0                                      \
    aarch64=418.94.202410090804-0                                     \
    s390x=418.94.202410090804-0                                       \
    ppc64le=418.94.202410090804-0
```

**RHCOS engineer** a member of the RHCOS team

**OpenShift engineer** a member of any openshift engineering team

1. **RHCOS engineer** runs the **cosa2stream command** to generate `rhcos.json` for a release.
All of the non-marketplace images have been created for the release `x.y.z` and are populated
in the stream, but new marketplace images have not yet been created, so the previous release
`x.y.z-1` marketplace images are still used to populate the stream for marketplace images.
2. **RHCOS engineer** opens a PR against the installer, which is CI tested, and merged.
3. (orthogonal) A notification is generated for marketplace publishers, who upload
images based on the new `rhcos.json` to their respective cloud marketplaces.
4. **OpenShift engineer** reruns the same **cosa2stream command** for the `x.y.z` release. Now
that cloud images have been uploaded to the marketplace, the marketplace images are updated to
release `x.y.z` while the rest of `rhcos.json` is not updated.
5. Profit


### API Extensions

Once marketplace images are included in the stream, machine pools in the install config would be
updated to allow users to more simply opt in to using marketplace images without specifying particular image details.
The details of that will be discussed separately in order to keep focus on the RHCOS stream.

### Topology Considerations

#### Hypershift / Hosted Control Planes

ROSA and ARO both make use of marketplace images. ARO HCP depend on the
availability of Azure marketplace images. 
#### Standalone Clusters

Standalone Azure clusters will have a faster installation time as they will no
longer be required to upload a VHD with which to create a managed image.

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

#### RHCOS Stream

The RHCOS stream, [a json file in the installer repo](https://github.com/openshift/installer/blob/master/data/data/coreos/rhcos.json), is the source
of truth `openshift-install` uses to select & specify versioned first-boot images
when provisioning machines. The stream contains a top-level `.architectures`
object and each architecture contains:

* `artifacts`: platform-specific details regarding the source artifact, such
as file format, file location, SHA, etc.
* `images`: details about images in specific cloud platforms, particularly
region to image mappings
* `rhel-coreos-extensions` (optional): is an extensible object, where the
only current value is `azure-disk`

Marketplace images would be added to `rhel-coreos-extensions`. Below is an implementation for Azure images.
Other clouds would be implemented at the same level.

##### Azure Marketplace Images

Paid Azure marketplace images would be used from the publisher `redhat`. Non-paid images will be used from the publisher
`azureopenshift`. Note that these criteria values would be in the RHCOS tooling and could be updated when needed.

Here is an imaginary, simplified example of output using the `az` cli to inspect 4.18 non-paid images:

```shell
$ az vm image list --publisher azureopenshift --offer aro4  --all -o table --sku 418
Architecture    Offer    Publisher       Sku      Urn                                                Version
--------------  -------  --------------  -------  -------------------------------------------------  ---------------------
Arm64           aro4     azureopenshift  418-arm  azureopenshift:aro4:418-arm:418.94.202410090804-0  418.94.202410090804-0
x64             aro4     azureopenshift  418-v2   azureopenshift:aro4:418-v2:418.94.202410090804-0   418.94.202410090804-0
x64             aro4     azureopenshift  aro_418  azureopenshift:aro4:aro_418:418.94.202410090804-0  418.94.202410090804-0
```

The `offer` and `publisher` are static values. `Sku` is variable, but deterministic based on the release inputs

##### Azure Stream Representation

An Azure Marketplace extension, `azure-marketplace`, would be added, with unpaid and paid (both North America & EMEA)
marketplace images listed as child objects: 

```yaml
"rhel-coreos-extensions": {
  "azure-disk": {
    "release": "418.94.202410090804-0",
    "url": "https://rhcos.blob.core.windows.net/imagebucket/rhcos-418.94.202410090804-0-azure.x86_64.vhd"
  },
  "azure-marketplace": {
    "no-purchase-plan": {
      "publisher": "RedHat",
      "offer": "rhcos",
      "sku": "rhcos",
      "version": "418.94.202410090804"
    },
    "purchase-plan-north-america": {
      "publisher": "RedHat",
      "offer": "rh-ocp-worker",
      "sku": "rh-ocp-worker",
      "version": "418.94.202410090804"
    },
    "purchase-plan-emea": {
      "publisher": "redhat-limited",
      "offer": "rh-ocp-worker",
      "sku": "rh-ocp-worker",
      "version": "418.94.202410090804"
    }
  }
```

[Hyperv Generation 1 images](https://learn.microsoft.com/en-us/azure/virtual-machines/generation-2#features-and-capabilities)
would be published, identified by a `gen1` suffix, but would not be included in the RHCOS stream. Any time that the installer
or users need to select a gen1 image, they would simply add the `gen1` suffix to the SKU identified in the RHCOS stream.  


### Risks and Mitigations

TODO

### Drawbacks

TODO

## Open Questions [optional]

`plume cosa2stream` is idempotent. I believe the changes proposed here are sufficient for the idempotency guarantee,
but the values for marketplace images would change based on the availability of those images. So the same command
run at two different times would produce different results--but invoking the command multiple times would never
cause any problems.

## Test Plan

TODO

## Graduation Criteria

TODO

### Dev Preview -> Tech Preview

TODO

### Tech Preview -> GA

TODO

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

TODO

## Version Skew Strategy

TODO

## Operational Aspects of API Extensions

TODO

## Support Procedures

TODO

## Alternatives

Early feedback on this enhancement proposed splitting marketplace images into a separate file.
Multiple files solves the issues regarding holding the initial bump PR to wait for marketplace
images to be uploaded, but there are many assumptions throughout openshift that the stream is contained
in a single file, such as `openshift-install coreos print-stream-json`, the 
[coreos configmap manifest](https://github.com/openshift/installer/blob/master/hack/build-coreos-manifest.go),
& perhaps more. I believe the current approach solves the problems we were trying to solve with this alternative
but in a way that requires less reworking of existing code and keeps `rhcos.json` as a single canonical file.

## Infrastructure Needed [optional]

TODO