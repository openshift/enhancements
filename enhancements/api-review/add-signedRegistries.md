---
title: add-signedRegistries
authors:
  - "@QiWang19"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-08-08
last-updated: 2021-08-08
status: implementable
---

# Add SignedRegistries to Image CR

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today, the cluster-wide Image CR (images.config.openshift.io cluster) doesn't have a way of setting the signature verification policy when pulling a signed image.

The `/etc/containers/policy.json` defines the configurations for signature verification.
This enhancement plans to add `signedRegistries` to allow using Image CR to set up
Image signature verification.

## Motivation

Current image CR does not support the configuration for signature verification. If users require signature verification of images to be pulled, they have to create a drop-in policy.json file containing the GPG keyring identity. However, the drop-in policy.json file will not take priority if an Image CR has been applied to the cluster previously since the machine config created by Image CR has higher priority than the machine config create by the drop-in policy.json file.

To solve the above problem and provide a convenient way for user to apply configuration supported by current Image CR and also set signature verification, add `signedRegistries`, under the `registrySources`, to Image CR.

### Goals

Allow users to use Image CR for Image signature verification.

### Non-Goals

## Proposal

The Image API is extended by adding an optional `signedRegistries` field with type `[]SignedRegistry` to `RegistrySources`:

```go
// RegistrySources holds cluster-wide information about how to handle the registries config.
type RegistrySources struct {
	//...

	// signedRegistries are registries that signature verification will be required when imaged pulled from.
	//
	// +optional
	SignedRegistries []SignedRegistry `json:"signedRegistries,omitempty"`
}

// SignedRegistry holds the policy for verifying the images signed by GPG keys
// Note: If the image referenced by a tag, the identity in the signature must exactly match;
// if the image referenced by digest, the identity in the signature must be in the same repository as the image identity (using any tag)
type SignedRegistry struct {
	// sigRequiredRegistriesOrImages specifies images or registries the signed image will be pulled from
	// the format of each element can be hostname[:port][/namespace[/imagestream [:tag]]]
	// i.e. either specifying a complete name of a tagged image, or prefix denoting a host/namespace/image stream.
	// +kubebuilder:validation:MinItems=1
	// +required
	SigRequiredRegistriesOrImages []string `json:"sigRequiredRegistriesOrImages,omitempty"`
	// gpgKeyPath the path to the local file that contains GPG keyring of one or more public keys that signed the images from sigRequiredRegistriesOrImages
	// +required
	GPGKeyPath string `json:"gpgKeyPath,omitempty"`
}
```

The containerRuntimeConfig controller in the MCO already watches the cluster-wide images.config.openshift.io CR for the allowedRegistries, blockedRegistries. It will now watch for signedRegistries as well and update /etc/containers/policy.json accordingly.

An example image.config.openshift.io/cluster CR will look like:

```yml
apiVersion: config.openshift.io/v1
kind: Image 
metadata:
  name: cluster
spec:
  registrySources:
    allowedRegistries:
    - ...
    signedRegistries:
    - sigRequiredRegistriesOrImages:
      - image-registry.openshift-image-registry.svc:5000
      - quay.io/sign/repository:latest
      gpgKeyPath: /tmp/key1.gpg
    - sigRequiredRegistriesOrImages:
      - example.com
      gpgKeyPath: /tmp/key2.gpg
```

### User Stories

#### As a user, I would like to require signature verification when pulling images

The user can set `signedRegistries` with a list of images and the GPG key path the images has been signed.

The user can run `oc edit images.config.openshift.io cluster` and add `signedRegistries` under `registrySources`. Once this is done, the containerRuntimeConfig controller will roll out the changes to the nodes.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in:
- openshift/api
- openshift/machine-config-operator

This is what the `/etc/containers/policy.json` file currently looks like on the nodes:

```json
{
    "default": [
        {
            "type": "insecureAcceptAnything"
        }
    ],
    "transports":
        {
            "docker-daemon":
                {
                    "": [{"type":"insecureAcceptAnything"}]
                }
        }
}
```

This is an example of the cluster wide images.config.openshift.io:

```yml
apiVersion: config.openshift.io/v1
kind: Image 
metadata:
  name: cluster
spec:
  registrySources:
    allowedRegistries:
    - quay.io/allowed
    signedRegistries:
    - sigRequiredRegistriesOrImages:
      - image-registry.openshift-image-registry.svc:5000
      - quay.io/sign/repository:latest
      gpgKeyPath: //tmp/key1.gpg
    - sigRequiredRegistriesOrImages:
      - example.com
      gpgKeyPath: /tmp/key2.gpg
```



The above Image CR will create a drop-in file `/host/etc/containers/policy.json` on each node, which will look like:

```json
{
   "default":[
      {
         "type":"reject"
      }
   ],
   "transports":{
      "atomic":{
         "quay.io/allowed":[
            {
               "type":"insecureAcceptAnything"
            }
         ],
         "image-registry.openshift-image-registry.svc:5000":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key1.gpg"
            }
         ],
         "quay.io/sign/repository:latest":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key1.gpg"
            }
         ],
         "example.com":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key2.gpg"
            }
         ]
      },
      "docker":{
         "quay.io/allowed":[
            {
               "type":"insecureAcceptAnything"
            }
         ],
         "image-registry.openshift-image-registry.svc:5000":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key1.gpg"
            }
         ],
         "quay.io/sign/repository:latest":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key1.gpg"
            }
         ],
         "example.com":[
            {
               "type": "signedBy",
               "keyType": "GPGKeys",
               "keyData": "/tmp/key2.gpg"
            }
         ]
      },
      "docker-daemon":{
         "":[
            {
               "type":"insecureAcceptAnything"
            }
         ]
      }
   }
}
```

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

Update the tests that are currently in the MCO to verify that signature policies in the policy.json have been added when the cluster wide Image CR is edited to configure `signedRegistries`.

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

GA in openshift will be looked at to
determine graduation.

**Note:** *Section not required until targeted at a release.*

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Users upgraded and configured the `signedRegistries` will be affected after
they downgraded to a version that lack support the `signedRegistries` and
break their signature verification workflow.
The workaround [Verifying Red Hat container image signatures in OpenShift Container Platform 4](https://access.redhat.com/verify-images-ocp4) will
work for version that does not support `signedRegistries` in Image CR.

### Version Skew Strategy

Upgrade skew will not impact this feature. The MCO does not require skew check. CRI-O with n-2 OpenShift skew will still be able to handle the new property.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

