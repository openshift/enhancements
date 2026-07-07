# TLS Configuration Injection

This document describes the implementation of [TLS config injection](../user/tls-injection.md).

## Overview

The cluster version operator (CVO) can inject the cluster-wide TLS configuration into the ConfigMap of operators that have opted in. The CVO watches the reference config, and merges it into the config of matching operators, which then restart to apply the change.

## CVO Implementation

### Workflow

The TLS injection is implemented as a ConfigMap modifier. When the resource builder (`lib/resourcebuilder/core.go`) processes a ConfigMap manifest from the release image, and before applying the ConfigMap to the cluster.

It first checks the configmap for a `config.openshift.io/inject-tls: "true"` annotation, bailing out.

It then retrieves the cluster-wide `servingInfo.minTLSVersion` and `servingInfo.cipherSuites` settings from the APIServer cluster resource, using a `ConfigObserverLister`. Those settings are typically derived from a high-level "TLS security profile" setting, but the CVO only concerns itself with the resulting low-level settings.

It then looks for `data` configmaps of the right kind (currently `GenericOperatorConfig` and `GenericControllerConfig`) and corresponding API version. Those `data` nodes are updated with the observed `servingInfo.minTLSVersion` and `servingInfo.cipherSuites`.

The resource builder will then apply the configmap, if it actually differs from the original one.

### Design considerations

The CVO aims for strong operational safety:
* A strict opt-in mechanism gates any action
* Any unexpected condition (such as a bad configmap or an API error) result in a log, early bailout, and no injection
* Great care is taken, with corresponding unittests, to only touch the relevant fields

The TLS config is injected as-is, despite different configs being semantically identical due to Go's opinionated TLS implementation:
* This avoids superficial inconsistencies between the APIServer and individual operators, reducing administrator surprise and the risk of an update loop
* This allows for more obedient TLS stacks to use the config's full semantics
* This potentially causes an unecessary restart

See `lib/resourcebuilder/core_test.go` for comprehensive test cases.

## Operator Implementations

Operators (or more generally controllers) are strongly recommended to
* Be based on `library-go`:
  - Enables the required parsing of CLI args
  - Uses the TLS settings from the configmap
  - Provides reasonable defaults when no TLS settings are injected (yet)
* Check that its configmap data entries use either `kind:GenericOperatorConfig apiVersion: operator.openshift.io/v1alpha1` or `kind:GenericControllerConfig apiVersion: operator.openshift.io/v1`
* Add a `config.openshift.io/inject-tls: "true"` annotation to the configmap yaml
* Add a `--terminate-on-files=/path/to/config.yaml` startup arg to the deployment yaml

Restarting on config.yaml (instead of changing the TLS config at runtime) is more blunt than strictly necessary, but make sure no config is stale and only needs minimal code changes.

## Limitations

- Only supports `GenericOperatorConfig` and `GenericControllerConfg` formats
- Only injects `servingInfo.minTLSVersion` and `servingInfo.cipherSuites`
- Requires the operator to watch and reload its ConfigMap when it changes
- Only available for ConfigMaps in the payload image (not for user-created ConfigMaps)