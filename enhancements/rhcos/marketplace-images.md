---
title: marketplace-images
authors:
  - "@patrickdillon"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@jlebon, for rhcos stream"
  - "@mike-nguyen, for rhcos stream"
  - "@trozet, for rhcos stream"
  - "@bennerv, for aro hcp"
  - "@bryan-cox, for aro hcp"
  - "@yuqi-zhang, for mco"
  - "@djoshy, for mco boot image management"
  - "@prashanth684, for okd impact"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sdodson"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 2024-07-12
last-updated: 2025-07-21
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/OCPSTRAT-1862"
see-also:
replaces:
superseded-by:
---

# Cloud-Marketplace Images in the RHCOS Stream

## Summary

This enhancement proposes introducing a `Marketplace` extension to the RHCOS
stream to represent first-class, supported RHCOS images, which are published in cloud
marketplaces via a separate process than traditional RHCOS publishing. The marketplace
extension will be published in a separate `marketplace-rhcos.json` and combined with the
existing `rhcos.json` file to create the complete RHCOS stream.
This enhancement provides a concrete implementation for populating the RHCOS stream with
non-paid Azure marketplace images, with the intention that the same pattern will be used
in future work for other clouds (i.e. AWS & GCP) and paid Azure images. 

## Motivation

Marketplace images are officially supported, but are not
included in the RHCOS stream, which is the canonical source
for images. By including marketplace images in the RHCOS stream,
 we will realize multiple benefits:

* Improved user experience for paid marketplace users. 
As marketplace images are not included in the coreos
stream, users must manually enter the image details
to the install config.
With these changes, the experience will be improved
so that users can simply indicate they want to use
marketplace images in the install config and the installer
will select the appropriate image.
* Faster Azure Installs and performance. On Azure, marketplace images are the preferred (and,
in the case of ARO, only supported) method for distributing
production-grade images. Including these images in the stream
would allow the installer to skip uploading images and creating image
galleries. Furthermore, we have seen faster boot times in VMs using
marketplace images. Installs easily complete 15 to 20 minutes more quickly
when using marketplace images.
* ARO Hosted Control Planes (HCP) will utilize marketplace images and would like a canonical
source in the cluster
* [MCO Boot-image management](../machine-config/manage-boot-images.md) will be enabled for VMs using
marketplace images. By moving all Azure installs to marketplace, we will simplify the MCO implementation
so it does not need to create new images for the cluster.

### User Stories

* As an OpenShift developer, I want RHCOS marketplace images to be included in the stream,
so I can use them in my application: e.g. ARO HCP discovering the RHCOS marketplace URN
from the release image.
* As an `openshift-install` user, I want to specify a boolean value for marketplace images in
the install config so that I can utilize marketplace images without looking up specific version numbers.
* As an OpenShift user, I would like the machine-config operator to be able to manage boot images
for my VMs when I use marketplace images. 

### Goals

* A well defined, standard pattern for including marketplace images, published out-of-band, in the RHCOS streams.
* Provide an example Azure implementation that will scale to other clouds

### Non-Goals

* ¯\_(ツ)_/¯

## Proposal

[stream-metadata-go](https://github.com/coreos/stream-metadata-go/blob/main/stream/rhcos/rhcos.go) will be extended to define a catch-all `Marketplace` extension with cloud-specific marketplace types nested below. See [API Extensions](#api-extensions) for further details.

The JSON serialization of the `Marketplace` extension would be published in the Installer repo, at
`data/data/coreos/marketplace-rhcos.json` alongside the existing `rhcos.json` stream. The installer will combine the two files when producing the canonical RHCOS stream, specifically:

* `hack/build-coreos-manifest.go`, which generates the `coreos-bootimages` configmap in the `openshift-machine-config-operator` namespace
* `openshift-install coreos print-stream-json`

A standalone program, `hack/rhcos/populate-marketplace-images.go`, would be introduced to publish
the `marketplace-rhcos.json` file. The intent of the program is to discover images published out-of-band by separate teams, which allows updating marketplace images at different cadences, initiated by a bump to RHCOS.json.

Release informing CI jobs can be put in place to ensure that marketplace images stay up to date.
See [Test Plan](#test-plan) for more details.

### Workflow Description

**RHCOS engineer** a member of the RHCOS team

**Marketplace Publisher** an engineer, such as engineers from the Software Production team or ARO, who published RHCOS images to a cloud marketplace

**Installer engineer** a member of the installer engineering team

1. **RHCOS engineer** merges a PR bumping rhcos.json to a new build
1. **Marketplace Publisher** is alerted of updated RHCOS build (this could be a slack alert, jira ticket, or failing test, see open questions)
1. **Marketplace Publisher** publishes new marketplace images based on rhcos build from step 1.
1. Once new marketplace images are ready (this process may take days), **Marketplace Publisher** or
**Installer Engineer** uses `hack/rhcos/populate-marketplace-images.go` to update marketplace-rhcos.json and creates a PR.
1. CI tests are run on marketplace-image-bump PR, and PR is merged, which incorporates the new stream into the release image


### API Extensions

A `Marketplace` extension for containing and defining cloud-specific marketplace schemas
would be added to the RHCOS extensions in `stream-metadata-go`:

```go
// Extensions is data specific to Red Hat Enterprise Linux CoreOS
type Extensions struct {
	AzureDisk   *AzureDisk   `json:"azure-disk,omitempty"` //existing field
	Marketplace *Marketplace `json:"marketplace,omitempty"`
}
```

Each cloud marketplace would define its own type. Here is
a proof of concept implementation for Azure marketplace images:

```go
// Marketplace contains marketplace images for all clouds.
type Marketplace struct {
	Azure *AzureMarketplace `json:"azure,omitempty"`
}

// AzureMarketplace lists images, both paid and
// unpaid, available in the Azure marketplace.
type AzureMarketplace struct {
	// NoPurchasePlan is the standard, unpaid RHCOS image.
	NoPurchasePlan *AzureMarketplaceImages `json:"no-purchase-plan,omitempty"`

  // OCP is the paid marketplace image for OpenShift Container Platform.
  OCP *AzureMarketplaceImages `json:"ocp,omitempty"`

  // OPP is the paid marketplace image for OpenShift Platform Plus.
  OPP *AzureMarketplaceImages `json:"opp,omitempty"`

  // OKE is the paid marketplace image for OpenShift Kubernetes Engine.
  OKE *AzureMarketplaceImages `json:"oke,omitempty"`

}

// AzureMarketplaceImages contains both the HyperV- Gen1 & Gen2
// images for a purchase plan.
type AzureMarketplaceImages struct {
	Gen1 *AzureMarketplaceImage `json:"hyperVGen1,omitempty"`
	Gen2 *AzureMarketplaceImage `json:"hyperVGen2,omitempty"`
}

// AzureMarketplaceImage defines the attributes for an Azure
// marketplace image.
type AzureMarketplaceImage struct {
	Publisher    string `json:"publisher"`
	Offer        string `json:"offer"`
	SKU          string `json:"sku"`
	Version      string `json:"version"`
	PurchasePlan bool   `json:"thirdParty"`
}

// URN returns the image URN for the marketplace image.
func (i *AzureMarketplaceImage) URN() string {
	return fmt.Sprintf("%s:%s:%s:%s", i.Publisher, i.Offer, i.SKU, i.Version)
}
```

This Go API results in a json representation, for x86:

```json
./openshift-install coreos print-stream-json | jq '.architectures.x86_64."rhel-coreos-extensions"'
{
  "azure-disk": {
    "release": "418.94.202410090804-0",
    "url": "https://rhcos.blob.core.windows.net/imagebucket/rhcos-418.94.202410090804-0-azure.x86_64.vhd"
  },
  "marketplace": {
    "azure": {
      "no-purchase-plan": {
        "hyperVGen1": {
          "publisher": "azureopenshift",
          "offer": "aro4",
          "sku": "aro_418",
          "version": "418.94.20250122",
          "purchasePlan": false
        },
        "hyperVGen2": {
          "publisher": "azureopenshift",
          "offer": "aro4",
          "sku": "418-v2",
          "version": "418.94.20250122",
          "purchasePlan": false
        }
      },
      "ocp": {
        "hyperVGen1": {
          "publisher": "redhat",
          "offer": "rh-ocp-worker",
          "sku": "rh-ocp-worker-gen1",
          "version": "413.92.2023101700",
          "purchasePlan": true
        },
        "hyperVGen2": {
          "publisher": "redhat",
          "offer": "rh-ocp-worker",
          "sku": "rh-ocp-worker",
          "version": "413.92.2023101700",
          "purchasePlan": true
        }
      },
      "opp": {
        "hyperVGen1": {
          "publisher": "redhat",
          "offer": "rh-ocp-worker",
          "sku": "rh-ocp-worker-gen1",
          "version": "413.92.2023101700",
          "purchasePlan": true
        },
        "hyperVGen2": {
          "publisher": "redhat",
          "offer": "rh-ocp-worker",
          "sku": "rh-ocp-worker",
          "version": "413.92.2023101700",
          "purchasePlan": true
        }
      },
      "oke": {
        "hyperVGen1": {
          "publisher": "redhat",
          "offer": "rh-oke-worker",
          "sku": "rh-oke-worker-gen1",
          "version": "413.92.2023101700",
          "purchasePlan": true
        },
        "hyperVGen2": {
          "publisher": "redhat",
          "offer": "rh-oke-worker",
          "sku": "rh-oke-worker",
          "version": "413.92.2023101700",
          "purchasePlan": true
        }
      }
    }
  }
}
```

Paid marketplace images do not support ARM, and they are filtered out, leaving only nonpaid images:

```json
./openshift-install coreos print-stream-json | jq '.architectures.aarch64."rhel-coreos-extensions"'
{
  "azure-disk": {
    "release": "418.94.202410090804-0",
    "url": "https://rhcos.blob.core.windows.net/imagebucket/rhcos-418.94.202410090804-0-azure.aarch64.vhd"
  },
  "marketplace": {
    "azure": {
      "no-purchase-plan": {
        "hyperVGen2": {
          "publisher": "azureopenshift",
          "offer": "aro4",
          "sku": "418-arm",
          "version": "418.94.20250122",
          "purchasePlan": false
        }
      }
    }
  }
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

ROSA and ARO both make use of marketplace images. ARO HCP depends on the
availability of Azure marketplace images.

#### Standalone Clusters

Standalone Azure clusters will have a faster installation time as they will no
longer be required to upload a VHD with which to create a managed image.

#### Single-node Deployments or MicroShift

N/A

#### OKD

SCOS images should be published in [Azure Community Galleries](https://techcommunity.microsoft.com/blog/linuxandopensourceblog/community-images-in-azure---a-new-way-to-share-images-on-azure/4090759)
in order to maintain a similar installation path as OCP (and to allow the installer to drop the legacy code of creating galleries and images) & realize the performance benefis. Community galleries
are preferred over additional marketplace listings because community galleries allow an easier publishing process and less discoverability. Community galleries offer less production-level performance
guarantees, but that is an acceptable tradeoff for OKD. In order to include SCOS publication in community galleries, the RHCOS stream will need to be extended. Gallery image ids are passed as a single
string: 

```go
fmt.Sprintf("/resourceGroups/%s/providers/Microsoft.Compute/galleries/gallery_%s/images/%s/versions/latest", resourceGroup, galleryName, id)
```

And we could likewise represent it as a single string in the RHCOS stream, or decompose it into the variable fields:

```go
type azureGallery struct {
  ResourceGroup string `json:"resourceGroup"`
	Gallery       string `json:"gallery"`
	Image         string `json:"image"`
	Version       string `json:"version"`
}
```

There are open questions regarding the publication of these images (see below), and the OpenShift installer can maintain
its image creation process until these issues are resolved and publication has been established.

### Implementation Details/Notes/Constraints

The `populate-marketplace-images.go` program will discover existing images based on criteria
defined in the program and use those to populate the new stream. To discuss the details of this
discovery process, let's first get some background details on Azure images.

#### Azure Marketplace Image Attributes

Azure marketplace images are identified by four attributes: Publisher, Offer, SKU, & Version. When combined (separated by `:`)
these are referred to as a "URN". Architecture is an attribute of the image as well, but not part of the URN. 

The following command shows existing images published by ARO in Azure. I'm using the `--sku 417` flag to limit the results to
4.17 images.

```shell
$ az vm image list --publisher azureopenshift --offer aro4  --all -o table --sku 417
Architecture    Offer    Publisher       Sku      Urn                                          Version
--------------  -------  --------------  -------  -------------------------------------------  ---------------
Arm64           aro4     azureopenshift  417-arm  azureopenshift:aro4:417-arm:417.94.20240701  417.94.20240701
x64             aro4     azureopenshift  417-v2   azureopenshift:aro4:417-v2:417.94.20240701   417.94.20240701
x64             aro4     azureopenshift  aro_417  azureopenshift:aro4:aro_417:417.94.20240701  417.94.20240701
```

There are three images: an x86 hyperVGen1 image, an x86 hyperVGen2 image, and an ARM image
(ARM is only supported on hyperVGen2). All three of these images would be captured in the RHCOS stream schema
captured above.

The `offer` and `publisher` are static values. `Sku` is variable, but deterministic based on the release inputs.


##### Image Versions and Overrides

As mentioned, the `populate-marketplace-images.go` program will attempt to
match the exact version specified by the build in rhcos.json, but if an exact
match is not found, the latest version (availble for that SKU) will be returned.

Additionally, a specific build version can be specified:

```bash
STREAM_RELEASE_OVERRIDE=417.94.202407010000-0 go run -mod=vendor ./hack/rhcos/populate-marketplace-imagestream.go
```

This is needed, for example, to populate the 4.18 branch, where a 4.18 marketplace image is not yet available.

##### Merging rhcos.json & marketplace-rhcos.json to create the RHCOS stream

Now that both the rhcos.json & marketplace-rhcos.json files represent the stream,
they need to be combined to create the canonical rhcos stream for the `openshift-install coreos print-stream-json`
command and the `coreos-bootimages` configmap.

This process is relatively simple: both json files are deserialized into their respective go types, the `Marketplace`
struct is added to the `Stream` struct and then reserialized.

### Risks and Mitigations

One risk is that there will be version drift between the RHCOS-team-published RHCOS builds and the separately
published marketplace images. We consider this a small risk: RHCOS bumps are not frequent, so it will not be challenging to
keep up with the pace; and, furthermore, the Azure images have been relatively stable and not required many bug fixes.
In order to mitigate version drift, we can add a release-informing test which compares the build of rhcos.json with the version
in marketplace-rhcos.json. This will send alerts to the installer team to notify that a bump is needed.

Similarly, ARO, as publisher of marketplace images, will need to publish images aligned with the
OpenShift relesae cycle. After reviewing [the commit history for changes to the RHCOS stream](https://github.com/openshift/installer/commits/main/data/data/coreos),
we see that there were 10 bumps to rhcos.json in 2024, but several of these bumps were pre-release. To mitigate unnecessary toil,
we propose that ARO would only be expected to bump marketplace images on release branches.

### Drawbacks

One drawback is that this potentially increases the operational burden on the installer team to take on responsibility for
boot images or at least to facilitate the update process. That said, this burden does not seem significant and something that
could be addressed in the future if needed.

## Open Questions [optional]

* What should be the process for notifying publishers that RHCOS has been bumped and we need new images?
** Jira ticket: Process wise, this is ideal, but there is not github/prow automation to create Jira tickets.
** Slack alert: it is possible to integrate slack alerts with prow postsubmits (so an alert is generated when the PR is merged)
** Test failure: we plan on introducing tests to inform when there is version skew (see below), so these could be used as well 

* Could the RHCOS or an upstream team take on publishing SCOS to an Azure community gallery and including it in the scos stream?
It seems like this be added as part of the standard stream--not an extension like marketplace images--and as a result it would need
to be handled by the team responsible for bumping SCOS images. The Installer team would be willing to take on some of this toil, if needed.
**Answered: Maybe** We will defer this (see graduation criteria below). FCOS is making progress in 
https://github.com/coreos/fedora-coreos-tracker/issues/148.

* Should Azure GovCloud, and therefore other Azure clouds get treatment in the RHCOS stream? The marketplace is separate for different clouds
and ARO publishes unpaid marketplace images in Azure Government cloud. My opinion is that we should depend on consistent publishing across
clouds to reduce cardinality within the stream; that is, rather than having top-level fields for `AzureCloud` and `AzureGovernment`, we should
ensure that images are published with the same details in both clouds. **Answered: No** There is no. distinction in the publication of images to
MAG, so we do not need this distinction.

* Now that the RHCOS stream has moved to versioning releases based on the RHEL version, can we standardize the way the image versions are listed
in the paid marketplace and ARO? For example, the most recent rhcos release in the installer is [. ](https://github.com/openshift/installer/blob/fc9c0c69f117ac423d2e83fd7c3c94ae63924934/data/data/coreos/rhcos.json#L11C23-L11C37), compared to `418.94.202501221327-0` for 4.18. Ideally, both ARO and paid marketplace images would use the exact
release as the version, trimming the suffix of the patch version as needed, according to Azure requirements.


## Test Plan

We will add a release-informing test to compare the rhcos.json & marketplace-rhcos.json to ensure that we are aware of any version creep. A release
informing job will be run that checks whether the release from rhcos.json matches the version of azure images in marketplace-rhcos.json. If the job
fails, the installer team will be notified.

## Graduation Criteria


### Dev Preview -> Tech Preview

* coreos/stream-metadata-go updated to include marketplace extension
* azure paid & non-paid marketplace images are included in the coreos stream

### Tech Preview -> GA

* installer is updated to use marketplace images, including testing in Azure GovCloud
* agreement is reached with ARO team for marketplace publishing cadence

### Removing a deprecated feature

* Azure community gallery info is included in scos stream
* Image creation is removed from okd installs and from the installer

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives (Not Implemented)

N/A

## Infrastructure Needed [optional]

TODO