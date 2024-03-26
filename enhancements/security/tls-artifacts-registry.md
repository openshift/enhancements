---
title: tls-artifacts-registry
authors:
  - "@vrutkovs"
  - "@dgrisonnet"
  - "@dinhxuanvu"
reviewers:
  - "@deads2k"
approvers:
  - "@deads2k"
api-approvers:
  - "@deads2k"
creation-date: 2023-10-26
last-updated: 2024-02-14
tracking-link:
  - https://issues.redhat.com/browse/API-1603
---

# TLS Artifacts Registry

## Summary

This enhancement describes a single registry to store all produced 
TLS artifacts. Both engineering and customers have expressed concern 
about a lack of understanding what is the purpose of a particular 
certificate or CA bundle, who issued it, and which team is responsible 
for its contents and lifecycle. This enhancement describes a registry 
to store necessary information about all certificates in the cluster 
and sets requirements for TLS artifacts metadata.

## Motivation

TLS artifacts registry is useful for several teams in Red Hat (i.e. 
which component manages a particular certificate) and customers - 
for instance, helping them to separate platform-managed certificates 
from user-managed.

### User Stories

- As an Openshift administrator, I want to differentiate 
  platform-managed TLS artifacts from user-managed.
- As an Openshift developer, I want to be able to find which team 
  manages a particular certificate.
- As an Openshift developer, I want to have a test which verifies 
  that all certificates managed by the team have sufficient metadata
  to explain its purpose
- As a member of OpenShift concerned with the release process 
  (TRT, dev, staff engineer, maybe even PM), I want to make sure all 
  managed certificates have sufficient metadata to find the team 
  responsible for its lifecycle.

### Goals

Create a registry of all managed TLS artifacts, and ensure that no new 
certificates are added without sufficient metadata.
Add a test to the conformance suite that verifies the actual cluster TLS 
artifacts have necessary metadata.

### Non-Goals

* Include certificates provided by user (additional CA bundle, custom ingress / API certificates)
* Include other encrypted artifacts - e.g. JWT tokens
* Include objects, which include TLS artifacts - e.g. kubeconfigs
  * They contain CA bundles, cert and key - but verifying them requires kubeconfig parsing
* List all TLS artifact users. This registry only keeps track of TLS artifacts producers, not consumers
* Track MicroShift TLS artifacts

## Proposal

* Define a structure for the certificate registry origin repo
  * Each TLS artifact can be either
    * on-disk file
    * in-cluster secret or config map
  * Additional expected metadata stored for this artifact
    * JIRA component
    * TLS artifact description (one sentence describing the purpose 
      of this artifact)
  * Each TLS artifact is associated with a set of required metadata
* Create a test which saves existing information about TLS artifacts 
  in cluster.
  * Some secrets may be versioned, contain hash or IP address / DNS 
    address - i.e. 
    `etcd-peer-ip-10-10-10-10.us-east-1.internal` becomes 
    `etcd-peer-<master-2>` etc.
    The test would not store versioned copies and replace IPs with "master-n"
    to make them look uniform when saving raw TLS information
* Create a new test, which ensures that all secrets are present in the 
  registry
* Create a test that verifies that TLS artifacts in a cluster have 
  necessary annotations to provide required metadata.
* Some TLS artifacts can be added to known violation list so that the test 
  is not permafailing to give teams time to update metadata. Each metadata 
  property has its own violation list.
* Metadata is split into required and optional properties. The test would 
  record missing optional metadata and fail if required metadata is missing.
  This gives time to update certificate metadata when optional metadata 
  requirements become required. Violation files are meant to be "remove-only" 
  mode, meaning when metadata property becomes required users are not meant 
  to be added to the respective violation file.
* Initially required metadata properties would be "owning JIRA component" and 
  "one line description of artifact purpose". Later on other properties would be 
  added and eventually become required for new certificates.
* TLS artifact registry is being composed as a JSON file from several 
  parts (usually generated from the test job artifacts).
* In the `origin` repo create `hack/update-tls-artifacts.json` and 
  `hack/verify-tls-artifacts.json` scripts to compose TLS artifact 
  registry JSON from several JSONs and verify that all necessary 
  metadata is set respectively. `update` script also generate violation lists
  in JSON and Markdown formats. For each required metadata field the Markdown 
  report can be customized.
* Update library-go functions to set metadata when the TLS artifact is 
  being generated, so that most operators would use the recommended 
  method.

### Workflow Description

#### Adding a new TLS artifact

The cluster administrator opens the TLS artifacts registry in the `origin` repo 
and consults it to ensure that a particular certificate is platform-managed. 
The administrator can use JIRA component metadata to file a 
new issue related to this certificate.

When a components requires a new certificate created in cluster, . In order to properly 
register a new TLS certificate the developer should do the following:

* the developers should start create a pull request adding this functionality.
  "all tls certificates should be registered" test would fail. 
  The test job generates `rawTLSInformation` file which 
  contains collected information from the prow job, containing info about this new 
  certificate.
* the developer creates a PR to `origin` repo, adding this file to 
  `tls/raw-data` directory, runs `hack/update-tls-artifacts.sh` script.
  The script would update `tls/ownership/tls-ownership.json` so that the test would no longer fail.
  This script would also update `violation` files, so that the test verifying required metadata 
  would include this certificate
* the developer reruns tests and makes sure that the certificate is indeed registered and 
  its metadata has required properties and it matches the expected values stored in `origin` repo.
  If tls artifact tests still fail, the developer repeats previous step making necessary changes.

Similarly, TRT members use this registry to file issues found in 
periodic or release-blocking jobs.

#### Adding a new requirement for TLS artifacts

Reports and violations mechanisms can be extended to add new requirements. To add a new 
certificate metadata requirements developers would need to:
* Implement [`Requirement` interface](https://github.com/openshift/origin/blob/master/pkg/cmd/update-tls-artifacts/generate-owners/tlsmetadatainterfaces/types.go#L9-L14):
```golang
type Requirement interface {
  GetName() string

  // InspectRequirement generates and returns the result for a particular set of raw data
  InspectRequirement(rawData []*certgraphapi.PKIList) (RequirementResult, error)
}


type RequirementResult interface {
	GetName() string

	// WriteResultToTLSDir writes the content this requirement expects in directory.
	// tlsDir is the parent directory and must be nested as: <tlsDir>/<GetName()>/<content here>.
	// The content MUST include
	// 1. <tlsDir>/<GetName()>/<GetName()>.md
	// 2. <tlsDir>/<GetName()>/<GetName().json
	// 3. <tlsDir>/violations/<GetName()>/<GetName()>-violations.json
	WriteResultToTLSDir(tlsDir string) error

	// DiffExistingContent compares the content of the result with what currently exists in the tlsDir.
	// returns
	//   string representation to display to user (ideally a diff)
	//   bool that is true when content matches and false when content does not match
	//   error which non-nil ONLY when the comparison itself could not complete.  A completed diff that is non-zero is not an error
	DiffExistingContent(tlsDir string) (string, bool, error)

	// HaveViolationsRegressed compares the violations of the result with was passed in and returns
	// allViolationsFS is the tls/violations/<GetName> directory
	// returns
	//   string representation to display to user (ideally a diff of what is worse)
	//   bool that is true when no regressions have been introduced and false when content has gotten worse
	//   error which non-nil ONLY when the comparison itself could not complete.  A completed check that is non-zero is not an error
	HaveViolationsRegressed(allViolationsFS embed.FS) ([]string, bool, error)
}
```

A simplified type exists for annotation requirements:
```golang
type annotationRequirement struct {
	// requirementName is a unique name for metadata requirement
	requirementName string
	// annotationName is the annotation looked up in cert metadata
	annotationName string
	// title for the markdown
	title string
	// explanationMD is exactly the markdown to include that explains the purposes of the check
	explanationMD string
}

func NewAnnotationRequirement(requirementName, annotationName, title, explanationMD string) AnnotationRequirement {
	return annotationRequirement{
		requirementName: requirementName,
		annotationName:  annotationName,
		title:           title,
		explanationMD:   explanationMD,
	}
}
```

Example:
```golang
package autoregenerate_after_expiry

import "github.com/openshift/origin/pkg/cmd/update-tls-artifacts/generate-owners/tlsmetadatainterfaces"

const annotationName string = "certificates.openshift.io/auto-regenerate-after-offline-expiry"

type AutoRegenerateAfterOfflineExpiryRequirement struct{}

func NewAutoRegenerateAfterOfflineExpiryRequirement() tlsmetadatainterfaces.Requirement {

	md := tlsmetadatainterfaces.NewMarkdown("")
  md.Text("This is a report header, customized for particular requirement")
  md.Text("Afterwards `NewAnnotationRequirement` will add a list of TLS artifacts, grouped by type and owning component")

	return tlsmetadatainterfaces.NewAnnotationRequirement(
		// requirement name
		"autoregenerate-after-expiry",
		// cert or configmap annotation
		annotationName,
    // report name
		"Auto Regenerate After Offline Expiry",
    // report name
		string(md.ExactBytes()),
	)
}
```

In order to ensure that the requirement is backed by e2e, annotation value should contain e2e test name 
verifying this requirement property:
```
annotations:
	certificates.openshift.io/auto-regenerate-after-offline-expiry: https//github.com/link/to/pr/adding/annotation, "quote escaped formatted name of e2e test that ensures the PKI artifact functions properly"
```


### API Extensions

Not required

### Topology Considerations

#### Hypershift / Hosted Control Planes

No specific changes required

#### Standalone Clusters

No specific changes required

#### Single-node Deployments or MicroShift

No specific changes required for SNO. Tracking TLS artifacts in MicroShift cluster is a non-goal

### Implementation Details/Notes/Constraints

None

### Risks and Mitigations

Teams are responsible for accurate descriptions and timely owners 
update of TLS artifacts

### Drawbacks

None

## Design Details

### Open Questions

* Which metadata should be present for all TLS artifacts?
  * Minimum set:
    * Description
    * JIRA component
  * Additional data:
    * Validity period
    * Refreshed after n days / at n% of lifetime
    * Can be versioned / can contain IP address or hash
  * Suggested by customers
    * SSLConnections - which IP address / DNS name this certificate 
      is valid for.
      * This can be derived from certificate content, not required to 
        be included in the TLS artifact registry and may depend on the base 
        domain
    * Parent - a reference to another TLS artifact this cert is derived 
      from.
    * Management type - operator-managed, user-managed.


## Test Plan

`origin` unit test would verify that the TLS artifact registry is valid:
* All TLS artifacts have owners
* All TLS artifacts have the necessary metadata

e2e conformance tests would ensure that:
* All certificates in `openshift-*` namespaces or on disk are 
  included in the TLS registry.
* in-cluster certificates have necessary annotations and their value 
  matches registry entries.

## Graduation Criteria

Not applicable

### Dev Preview -> Tech Preview

Not applicable

### Tech Preview -> GA

Not applicable

### Removing a deprecated feature

Not applicable

## Upgrade / Downgrade Strategy

Not applicable

## Version Skew Strategy

Not applicable

## Operational Aspects of API Extensions

Not applicable

## Support Procedures

Not applicable

## Implementation History

* Current implementation: https://github.com/openshift/origin/pull/28305

## Alternatives

None known
