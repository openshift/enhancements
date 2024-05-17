---
title: admin-configured-ca-bundle
authors:
  - "@bhb"
reviewers:
  - "@candita" ## reviewer for cluster-ingress-operator component
  - "@Miciah" ## reviewer for cluster-ingress-operator component
  - "@tgeer" ## reviewer for CFE
approvers:
  - "@candita" ## approval from cluster-ingress-operator component
  - "@Miciah" ## approval from cluster-ingress-operator component
  - "@jerpeter1" ## approval from CFE
api-approvers:
  - None
creation-date: 2023-11-02
last-updated: 2023-12-06
tracking-link:
  - https://issues.redhat.com/browse/RFE-2182
  - https://issues.redhat.com/browse/OCPSTRAT-431
  - https://issues.redhat.com/browse/NE-761
see-also:
  - None
replaces:
  - None
superseded-by:
  - None
---

# Support for admin configured CA trust bundle in Ingress Operator

## Summary

This enhancement allows an OpenShift cluster administrator to configure OpenShift router with custom
trusted Certificate Authorities (CAs) to use for verifying services' certificates of re-encrypt routes.
The enhancement proposes configuring custom trusted Certificate Authorities (CAs) which also includes the
OpenShift service CA certificate for an IngressController to be used for certificate validation on
re-encrypt routes.

## Motivation

When an OpenShift project editor creates a route with the re-encrypt termination type, they have to
also provide a CA certificate for that route if it uses a custom CA instead of OpenShift's service CA for
certificate validation. In addition, when a same CA is used for obtaining several services' certificates it
is time-consuming and error-prone to configure the same CA certificate in every created route object. Instead,
this enhancement proposes a simpler alternative: in which an OpenShift cluster administrator could define a
CA bundle containing the list of trusted CA certificates as a ConfigMap which will be used by OpenShift Router
for verifying the services' certificates. Likewise, we'd like to improve the process for updating a CA
certificate in each route object when the certificate is expired, compromised, or for any other reason in
need of replacement.

### User Stories
- As an OpenShift cluster administrator, I want to configure a custom CA certificate bundle which OpenShift
  router should use for verifying services' certificates, instead of having to configure the same as
  destination CA Certificate in each of the created route objects.
- As an OpenShift cluster administrator, I want OpenShift router to update its configuration whenever the
  OpenShift service CA certificate or the user configured CA certificate bundle is updated or deleted.
- As an OpenShift cluster administrator, I want OpenShift router to verify services' certificates with just
  the configured destination CA Certificate and not the default destination CA certificate.

### Goals

- An OpenShift cluster administrator can configure a CA certificate bundle and expect OpenShift router to use
  it for verifying a service's certificate when destination CA Certificate isn't specified in the route object
  created for the service.
- Openshift router should seamlessly reload the OpenShift service CA certificate or the administrator-configured
  CA certificate bundle when either is modified.

### Non-Goals

- OpenShift cluster administrator defined CA certificate bundle is limited to the IngressController and
  OpenShift router instance usage and for the routes created with re-encrypt termination type, any other
  scenario is outside the scope of this proposal.

## Proposal
Update IngressController API to:
1. Make provision for user to provide reference to a configmap containing the trusted CA certificates. 

Change OpenShift Ingress Operator to:
1. Make the CA certificate bundle defined in `IngressController` available to the OpenShift Router as the 
   default destination CA bundle file to be used for verifying the services' certificate.

Update router to:
1. Watch the destination CA certificate bundle and update its content internally when needed.

### Workflow Description

- An OpenShift cluster administrator configures the list of trusted CA certificates as a ConfigMap, and
  provides reference to it in the `IngressController` object.
- The Ingress Operator configures OpenShift router with a ConfigMap volume mount to use the ConfigMap as 
  the default destination CA certificate to verify a service's certificate when a route with re-encrypt
  termination type is created with an empty destination CA.
- OpenShift router watches the volume mount and reloads HAProxy whenever the CA certificate bundle changes.

### API Extensions

The proposal expects the OpenShift cluster administrator to create a ConfigMap with all the required 
CA certificates, including the OpenShift service CA in the same namespace of the `IngressController` object
in which the configmap reference is provided.

Enhancement requires below modifications to the `IngressController` API.
- Add `defaultDestinationCATrust` field to `.spec` of the `ingresscontrollers.operator.openshift.io`
```golang
type IngressControllerSpec struct {
	// defaultDestinationCATrust contains reference to an object containing the trusted
	// Certificate Authorities (CAs) used by the Router for verifying the services'
	// certificates of re-encrypt routes. When Routes don't specify their own
	// destinationCACertificate, defaultDestinationCATrust is used.
	//
	// The user is responsible for including OpenShift service CA along with their own
	// trusted CA certificates in the referenced object. OpenShift service CA can be
	// found in openshift-service-ca.crt configmap in openshift-config namespace.
	//
	// If unset, OpenShift service CA configured in openshift-service-ca.crt configmap will
	// be used as default destination CA certificate.
	//
	// +optional
	DefaultDestinationCATrust DefaultDestinationTrust `json:"defaultDestinationCATrust,omitempty"`
}

// DefaultDestinationTrust specifies the reference to an object containing the trusted CA certificates.
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'ConfigMap' ?  has(self.configMap) : !has(self.configMap)",message="configMap must be configured when type is ConfigMap"
// +union
type DefaultDestinationTrust struct {
	// type is the referenced object type which contains the trusted CA certificates.
	// The default value is ConfigMap. ConfigMap is the only supported type at present.
	//
	// +unionDiscriminator
	// +kubebuilder:default:="ConfigMap"
	// +kubebuilder:validation:Enum:=ConfigMap
	// +kubebuilder:validation:Optional
	// +default="ConfigMap"
	// +optional
	Type string `json:"type,omitempty"`

	// configMap specifies the ConfigMap object name containing the trusted CA bundle.
	// The ConfigMap must exist in the same namespace as of the IngressController it is
	// specified in.
	//
	// The specified ConfigMap object must contain the following key and data:
	// 		ca-bundle.crt: trusted CA certificates, including the OpenShift service CA.
	//
	// +unionMember=ConfigMap
	// +optional
	ConfigMap *configv1.ConfigMapNameReference `json:"configMap,omitempty"`
}
```

### Implementation Details/Notes/Constraints [optional]

An OpenShift cluster administrator will have to create a ConfigMap with the list of all trusted CA certificates 
including the OpenShift service CA in the same namespace as of the `IngressController` object in which the
ConfigMap reference is provided.

OpenShift Ingress operator should make the contents of the defined ConfigMap available for OpenShift Router
to use as default destination CA certificate bundle.

OpenShift Router currently uses the CA certificate provided as a process argument
`default-destination-ca-path` or set as an environment variable `DEFAULT_DESTINATION_CA_PATH` as the
CA certificate for verifying the service's certificate for re-encrypt termination type.

CA certificate bundle is currently made available to the router using the volume mount option at the path
`/var/run/configmaps/service-ca/service-ca.crt` which will be updated to
`/var/run/configmaps/ca-trust/ca-bundle.crt` with the `ingress-ca-bundle` content.

OpenShift Router should create a file watcher to observe `/var/run/configmaps/ca-trust/ca-bundle.crt`
file and reload when the file content has been modified.

### Topology Considerations

#### Hypershift / Hosted Control Planes

None

#### Standalone Clusters

None

#### Single-node Deployments or MicroShift

None

### Risks and Mitigations

CA certificates configured could contain invalid PEM data or has duplicate/expired/revoked certificates
or uses insecure certificate algorithm which could pose risk and cause vulnerability.
  - CA certificates could be validated and those not meeting the requirements and could potentially disrupt
    other services' accessibility can be dropped. And the verification of services' certificates signed
    by the unconsidered certificates would fail and such services will be inaccessible.

### Drawbacks

When a user updates the ConfigMap, its contents are made available to OpenShift Router are eventually
updated, though a certain [delay](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically)
is expected for the new changes to be propagated to the pod.

## Open Questions [optional]

N/A.

## Test Plan

- Create routes with re-encrypt termination type with and without destinationCA.
  - Routes with empty destinationCA should be accessible only when using:
      - A Service that uses a certificate signed by OpenShift CA
      - A Service that uses a certificate signed by custom CA and is present in the CA trust bundle defined
        in the `IngressController` object.
- Upgrade/downgrade testing.
  - Scenarios mentioned in the section `Upgrade / Downgrade Strategy` has expected behavior.
  - Sufficient time for feedback from the QE.
- The feature is available by default and does not have any specific featureGate defined.

## Graduation Criteria

This feature will go directly to GA.

### Dev Preview -> Tech Preview
N/A.

### Tech Preview -> GA
N/A.

### Removing a deprecated feature
N/A.

## Upgrade / Downgrade Strategy

On upgrade:
- OpenShift Ingress Operator would have the functionality to look for new configuration field in the
  `IngressController` objects containing the CA trust bundle and OpenShift Router would refer to the
  new CA bundle content for service's certificate verification.
  - Routes that were not accessible before the upgrade because of services' certificate verification
    failure will be accessible post-upgrade, provided that the CA trust bundle configured in 
    `IngressController` object has the necessary CA certificates for service's certificate verification.

On downgrade:
- OpenShift Ingress Operator would not have the functionality to look for `defaultDestinationCATrust` in the
  `IngressController` objects and OpenShift service CA certificate would be used as the default destination
  CA certificate.
  - Routes created after the upgrade without a `destinationCACertificate` will become inaccessible as the
    services' certificate validation will fail . Until `destinationCACertificate` is configured for each
    inaccessible route.

## Version Skew Strategy

N/A.

## Operational Aspects of API Extensions

Below commands can be used to create and update ConfigMap used for `defaultDestinationCATrust`.
- Creating a ConfigMap with trusted CA certificates.
  1. Create a file with name `ca-bundle.crt` having the list of CA certificates to be included in the
     ConfigMap. The filename must be `ca-bundle.crt` since it is the key name expected in the ConfigMap.
  2. Append OpenShift service CA certificate to the file.
     ```console
     oc get configmap -n openshift-config openshift-service-ca.crt -o jsonpath='{.data.service-ca\.crt}' >> ./ca-bundle.crt
     ```
  4. Use below command to create the ConfigMap in same namespace as `IngressController` object with the
     contents of `ca-bundle.crt` file.
     ```console
     $ oc create configmap <name> --from-file=./ca-bundle.crt -n <namespace>
     ```
- Updating the `IngressController` object with `defaultDestinationCATrust`.
  1. Use below command to configure `defaultDestinationCATrust` in the required `IngressController` object
     with the ConfigMap containing the trusted CA certificates.
     ```console
     oc patch -n <namespace> ingresscontrollers.operator.openshift.io default -p '{"spec":{"defaultDestinationCATrust":{"name":"<configmap_name>"}}}' --type merge
     ```
- Updating CA bundle ConfigMap
  1. Update the `ca-bundle.crt` file with list of all required CA certificates to be included in the
     ConfigMap.
  2. Use below command to update the ConfigMap in same namespace as `IngressController` object with the
     contents of `ca-bundle.crt` file.
     ```console
     $ oc create configmap <name> --from-file=./ca-bundle.crt -n <namespace> --dry-run -o yaml | oc replace -f -
     ```
- Deleting CA bundle ConfigMap
  1. Use below command to delete the ConfigMap.
     ```console
     $ oc delete configmap <name> -n <namespace>
     ```

## Failure Modes

When the CA certificates could not be reconciled due to any issue and the verification of the services'
certificate would fail and service's will be inaccessible.

## Support Procedures

If a route is inaccessible, please check
- CA certificate used for signing route's service certificate is present either in the `defaultDestinationCATrust`
  configured in the respective `IngressController` object or in the corresponding Ingress/Route object.
- After updating `defaultDestinationCATrust` referencing ConfigMap or creating Ingress/Route object, it was
  successfully processed by ingress operator and router without any errors by checking the respective logs.
- Enable HAProxy logging by following the steps [here](https://docs.openshift.com/container-platform/4.14/rest_api/operator_apis/ingresscontroller-operator-openshift-io-v1.html#spec-logging) to check for TLS handshake error or any other error
  impacting the HAProxy functioning.
- `DEFAULT_DESTINATION_CA_PATH` environment variable in OpenShift Router pod is set and has the value
  `/var/run/configmaps/ca-trust/ca-bundle.crt`.
  ```console
  $ oc exec -n openshift-ingress <router_pod_name> -- printenv DEFAULT_DESTINATION_CA_PATH
  ```
- Required CA Certificate is present in the bundle made available in the OpenShift Router pod using the
  below command
  ```console
  $ oc exec -n openshift-ingress <router_pod_name> -- bash -c 'cat $(printenv DEFAULT_DESTINATION_CA_PATH)'
  ```
- Service is accessible when queried using the pod's(hosting the server) IP address by using below command
  ```console
  $ oc exec -n openshift-ingress <router_pod_name> -- bash -c 'curl -v -sS https://<service_name>:<service_port>/<path> --resolve <service_name>:<service_port>:<pod_ip_address> --cacert $(printenv DEFAULT_DESTINATION_CA_PATH)
  ```
- If service becomes accessible on configuring the `destinationCACertificate` in the route created for
  the service.


## Implementation History

N/A.

## Alternatives

Below alternatives were considered
1. Reuse CA Bundle certificate configured in the Proxy object.
   - This would be ideal for users who use a common CA certificate for signing the certificates used for
    services hosted on premise. Since CA certificates in Proxy object is defined for a particular usage
    and would match for certain customer use cases, reusing it for ingress operator usage and would conflate
    chains of trust.
2. Make use of destination CA certificate configuration option in Ingress/Router object.
   - This is still a valid scenario which can be used, but this enhancement proposes to make it simpler
     when there is a big number of route objects to manage.
3. Replace OpenShift Service CA with own CA certificate.
   - A user could replace the OpenShift Service CA with their own CA or intermediate CA certificate, but
     this has its own shortcomings, a few like below and compared to which the proposed solution provides
     a simpler solution.
     - Having to share the CA private key.
     - Having to replace certificates of all the service's using OpenShift Service CA certificate signed
       certificates, which could cause core components becoming inaccessible due to any misconfiguration.

## Infrastructure Needed [optional]

N/A.
