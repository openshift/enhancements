---
title: marketplace-images
authors:
  - "@patrickdillon"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@jlebon"
  - "@mike-nguyen"
  - "@trozet"
  - "@bennerv"
  - "@bryan-cox"
  - "@yuqi-zhang"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sdodson"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 2024-07-12
last-updated: 2025-01-23
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "SPSTRAT-271"
see-also:
replaces:
superseded-by:
---

# Cloud-Marketplace Images in the RHCOS Stream

## Summary

This enhancement proposes introducing a `Marketplace` extension to the RHCOS
stream. The marketplace extension will be published in a separate `marketplace-rhcos.json`
and combined with the existing `rhcos.json` file to create the complete RHCOS stream.
This enhancement provides a concrete implementation for populating the RHCOS stream with
non-paid Azure marketplace images, with the intention that the same pattern will be used
in future work for other clouds (i.e. AWS & GCP) and paid Azure images. 

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
in the case of ARO, only-supported) method for distributing
production-grade images. Including these images in the stream
would allow the installer to undo the workarounds required for
non-marketplace images and thereby dramatically speed up installs;
as well as, unblocking ARO HCP.
* Integrating ROSA marketplace images (which this enhancement enables
but does not achieve) into the stream would unblock ROSA CI

### User Stories

* As an OpenShift developer, I want RHCOS marketplace images to be included in the stream,
so I can use them in my application: e.g. ARO HCP discovering the RHCOS marketplace URN
from the release image.
* As an `openshift-install` user, I want to specify a boolean value for marketplace images in
the install config so that I can utilize marketplace images without looking up specific version numbers.

### Goals

* A well defined, standard pattern for including marketplace images, published out-of-band, in the RHCOS streams.
* Provide an example Azure implementation that will scale to other clouds

### Non-Goals

* To determine the publication process for marketplace images.
* Future work: define the install-config API for using marketplace images

## Proposal

[stream-metadata-go](https://github.com/coreos/stream-metadata-go/blob/main/stream/rhcos/rhcos.go) will be extended to define a catch-all `Marketplace` extension with cloud-specific marketplace types nested below. See [API Extensions](#api-extensions) for further details.

The JSON serialization of the `Marketplace` extension would be published in the Installer repo, at
`data/data/coreos/marketplace-rhcos.json` alongside the existing `rhcos.json` stream. The installer will combine the two files when producing the canonical RHCOS stream, specifically:

* `hack/build-coreos-manifest.go`, which generates the `coreos-bootimages` configmap in the `openshift-machine-config-operator` namespace
* `openshift-install coreos print-stream-json`

A standalone program, `hack/rhcos/populate-marketplace-images.go`, would be introduced to publish
the `marketplace-rhcos.json` file. The intent of the program is to discover images published out-of-band by separate teams, which allows updating marketplace images at different cadences, initiated by a bump to RHCOS.json.

Release informing CI jobs can be put in place to ensure that marketplace images stay up to date.

### Workflow Description

**RHCOS engineer** a member of the RHCOS team

**Marketplace Publisher** an engineer who published RHCOS images to a cloud marketplace

**Installer engineer** a member of the installer engineering team

1. **RHCOS engineer** merges a PR bumping rhcos.json to a new build
1. Jira ticket is created to alert **Marketplace Publisher** of updated RHCOS build (see open questions)
1. **Marketplace Publisher** publishes new marketplace images based on rhcos build from step 1
1. Once new marketplace images are ready (this process may take days), **Marketplace Publisher** or
**Installer Engineer** runs `hack/rhcos/populate-marketplace-images.go` to update marketplace-rhcos.json
1. Engineer from previous step opens PR with updated marketplace-rhcos.json file.
1. CI tests are run on new marketplace images, and PR is merged, which incorporates the new stream into the release image


### API Extensions

A `Marketplace` extension for containing and defining cloud-specific marketplace scehmas
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

// AzureMarketplaceImage defines the attributes for an Azure
// marketplace image.
type AzureMarketplaceImage struct {
	Publisher       string `json:"publisher"`
	Offer           string `json:"offer"`
	SKU             string `json:"sku"`
	Version         string `json:"version"`
	ThirdPartyImage bool   `json:"thirdParty"`
}

// URN returns the image URN for the marketplace image.
func (i *AzureMarketplaceImage) URN() string {
	return fmt.Sprintf("%s:%s:%s:%s", i.Publisher, i.Offer, i.SKU, i.Version)
}

// AzureMarketplaceImages contains both the HyperV- Gen1 & Gen2
// images for a purchase plan.
type AzureMarketplaceImages struct {
	Gen1 *AzureMarketplaceImage `json:"hyperVGen1,omitempty"`
	Gen2 *AzureMarketplaceImage `json:"hyperVGen2,omitempty"`
}

// AzureMarketplace lists images, both paid and
// unpaid, available in the Azure marketplace.
type AzureMarketplace struct {
	// NoPurchasePlan is the standard, unpaid RHCOS image.
	NoPurchasePlan *AzureMarketplaceImages `json:"no-purchase-plan,omitempty"`
  // Add future paid plans here.
}
```

This Go API results in a json representation that looks like:

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
          "sku": "aro_417",
          "version": "417.94.20240701",
          "thirdParty": false
        },
        "hyperVGen2": {
          "publisher": "azureopenshift",
          "offer": "aro4",
          "sku": "417-v2",
          "version": "417.94.20240701",
          "thirdParty": false
        }
      }
    }
  }
}
```

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

The `populate-marketplace-images.go` program will discover existing images based on criteria
defined in the program and use those to populate the new stream. To discuss the details of this
discovery process, let's first get some background details on Azure images.

##### Azure Marketplace Image Attributes

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

We can see there are three images: an x86 hyperVGen1 image, an x86 hyperVGen2 image, and an ARM image
(ARM is only supported on hyperVGen2). All three of these images would be captured in the RHCOS stream schema
captured above.

The `offer` and `publisher` are static values. `Sku` is variable, but deterministic based on the release inputs.
Let's discuss how we would discover these images based on the inputs.

##### Image Discovery and Stream Population

As `offer` and `publisher` are static values, these would be hardcoded in populate-marketplace-images.go:

```go
  // image attributes for the NoPurchasePlan image,
	// published by ARO.
	pubARO   = "azureopenshift"
	offerARO = "aro4"
```

The SKU and version would be determined based on the coreos build from the in-repo rhcos.json file. That is,
when `populate-marketplace-images.go` is executed, it inspects `.architectures.x86_64.rhel-coreos-extensions.release`
from rhcos.json, it then uses the following logic to convert the build to the SKU and version used by ARO:

```go
// parse takes the release from coreos stream and
// uses conventions to generate the SKU (gen1 & gen2) and version.
// For instance, with a coreos release of "418.94.202410090804-0"
// gen1SKU: "aro_418"
// gen2SKU: "418-v2"
// version: "418.94.20241009" (removes timestamp & build number)
func parse(release, arch string) (string, string, string) {
	xyVersion := strings.Split(release, ".")[0]
	var gen1SKU, gen2SKU string
	switch arch {
	case x86:
		gen1SKU = fmt.Sprintf("aro_%s", xyVersion)
		gen2SKU = fmt.Sprintf("%s-v2", xyVersion)
	case arm64:
		gen1SKU = ""
		gen2SKU = fmt.Sprintf("%s-arm", xyVersion)
	}
	return gen1SKU, gen2SKU, release[:len(release)-6]
}
```

##### Image Versions and Overrides

As mentioned, the `populate-marketplace-images.go` program will attempt to
match the exact version specified by the build in rhcos.json, but if an exact
match is not found, the latest version (availble for that SKU) will be returned.

Additionally, a specific build version can be specified:

```
STREAM_RELEASE_OVERRIDE=417.94.202407010000-0 go run -mod=vendor ./hack/rhcos/populate-marketplace-imagestream.go
```

This is needed, for example, to populate the 4.18 branch, where a 4.18 marketplace image is not yet available.

##### Merging rhcos.json & marketplace-rhcos.json to create the RHCOS stream

Now that we have both the rhcos.json & marketplace-rhcos.json files to represent the stream,
they need to be combined to create the canonical coreos stream for the `openshift-install coreos print-stream-json`
command and the `coreos-bootimages` configmap.

This process is relatively simple: both json files are deserialized into their respective go types, the `Marketplace`
struct is added to the `Stream` struct and then reserialized.

### Risks and Mitigations

One of the risks we run is that there will be version drift between the RHCOS-team-published RHCOS builds and the separately
published marketplace images. We consider this a small risk: RHCOS bumps are not frequent, so it will not be challenging to
keep up with the pace; and, furthermore, the Azure images have been relatively stable and not required many bug fixes.

In order to mitigate version drift, we can add a release-informing test which compares the build of rhcos.json with the version
in marketplace-rhcos.json. This will send alerts to the installer team to notify that a bump is needed.

### Drawbacks

One drawback is that this potentially increases the operational burden on the installer team to take on responsibility for
boot images or at least to facilitate the update process. That said, this burden does not seem significant and something that
could be addressed in the future if needed.

## Open Questions [optional]

* Who could publish FCOS Azure Marketplace images? The current proposal will create
a separate install experience for FCOS, unless we have parity with available FCOS images. The installer team could
potentially take on this responsibility but we are unsure of how to obtain a Microsoft Partner Account and maybe need
some handholding with FCOS?

* What should be the process for notifying publishers that RHCOS has been bumped and we need new images?
** A CI Post-submit test when rhcos.json is bumped could send a slack alert (at which point we create a jira)
** We can add release-informing tests to compare rhcos.json & marketplace-rhcos.json, which would also result in a slack alert


## Test Plan

We will add a release-informing test to compare the rhcos.json & marketplace-rhcos.json to ensure that we are aware of any version creep.

## Graduation Criteria

N/A

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives

N/A

## Infrastructure Needed [optional]

TODO