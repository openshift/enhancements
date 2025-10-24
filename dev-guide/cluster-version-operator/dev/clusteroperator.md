# ClusterOperator Custom Resource

The ClusterOperator is a custom resource object which holds the current state of an operator. This object is used by operators to convey their state to the rest of the cluster.

Ref: [godoc](https://godoc.org/github.com/openshift/api/config/v1#ClusterOperator) for more info on the ClusterOperator type.

## Why I want ClusterOperator Custom Resource in /manifests

Everyone installed by the ClusterVersionOperator must include the ClusterOperator Custom Resource in [`/manifests`](operators.md#what-do-i-put-in-manifests).
The CVO sweeps the release image and applies it to the cluster. On upgrade, the CVO uses clusteroperators to confirm successful upgrades.
Cluster-admins make use of these resources to check the status of their clusters.

## How should I include ClusterOperator Custom Resource in /manifests

### How ClusterVersionOperator handles ClusterOperator in release image

When ClusterVersionOperator encounters a ClusterOperator Custom Resource,

- It uses the `.metadata.name` to find the corresponding ClusterOperator instance in the cluster,
- It waits until the `name-version` pairs of `.status.versions` in the live instance matches the ones in the ClusterOperator from the release image,
- It then continues to the next ClusterOperator.

ClusterVersionOperator will only deploy files with `.yaml`, `.yml`, or `.json` extensions, like `kubectl create -f DIR`.

**NOTE**: ClusterVersionOperator sweeps the manifests in the release image in alphabetical order, therefore if the ClusterOperator Custom Resource exists before the deployment for the operator that is supposed to report the Custom Resource, ClusterVersionOperator will be stuck waiting and cannot proceed.
Also note that the ClusterVersionOperator will pre-create the ClusterOperator resource found in the `/manifests` folder (to provide better support to must-gather operation in case of install or upgrade failure).
It remains a responsibility of the respective operator to properly update (or recreate if deleted) the ClusterOperator Custom Resource.

### What should be the contents of ClusterOperator Custom Resource in /manifests

There are 3 important things that need to be set in the ClusterOperator Custom Resource in /manifests for CVO to correctly handle it.

- `.metadata.name`: name for finding the live instance
- `.status.versions[name=operator].version`: this is the version that the operator is expected to report. ClusterVersionOperator only respects the `.status.conditions` from instances that report their version.
- `.status.versions[name=operator-image].version`: this is the image pull specification that the operator is expected to report.

Additionally, you should include some fundamental [related objects](#related-objects) of your operator (an OpenShift payload informing job reports ClusterOperators that do not define a namespace and at least one other non-namespace object).
The must-gather and insights operator depend on cluster operators and related objects in order to identify resources to gather.
Because cluster operators are delegated to the operator install and upgrade, failures of new operators can fail to gather the requisite info if the cluster degrades before those steps.
To mitigate this scenario the ClusterVersionOperator will do a best effort to fast-fill cluster-operators using the ClusterOperator Custom Resource in /manifests.

Example:

For a cluster operator `my-cluster-operator`, that is reporting its status using a ClusterOperator instance named `my-cluster-operator`.

The ClusterOperator Custom Resource in /manifests should look like,

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: my-cluster-operator
spec: {}
status:
  versions:
    - name: operator
      # The string "0.0.1-snapshot" is substituted in the manifests when the payload is built
      version: "0.0.1-snapshot" 
    - name: operator-image
      # The <operator-tag-name> is tag name for the operator defined in the "image-references"
      # See ./operators.md#how-do-i-ensure-the-right-images-get-used-by-my-manifests
      # The entire string is substituted in the manifests when the payload is built
      version: "placeholder.url.oc.will.replace.this.org/placeholdernamespace:<operator-tag-name>"
    - name: operand-name # OPTIONALLY, for an operand that managed by the operator
      version: "0.0.1-snapshot" 
    - name: operand-name-image # OPTIONALLY, for an operand that managed by the operator
      # The <operand-tag-name> is tag name for the operand defined in the "image-references"
      version: "placeholder.url.oc.will.replace.this.org/placeholdernamespace:<operand-tag-name>"
```

If an operand decides to track some operands and includes them in the above manifest, then versions of the operands have to be reported by the operator as well. See [Section version](#version) below.

## What should an operator report with ClusterOperator Custom Resource

The ClusterOperator exists to communicate status about a functional area of the cluster back to both an admin
and the higher level automation in the CVO in an opinionated and consistent way. Because of this, we document
expectations around the outcome and have specific guarantees that apply.

Of note, in the docs below we use the word `operand` to describe the "thing the operator manages", which might be:

* A deployment or daemonset, like a cluster networking provider
* An API exposed via a CRD and the operator updates other API objects, like a secret generator
* Just some controller loop invariant that the operator manages, like "all certificate signing requests coming from valid machines are approved"

An operand doesn't have to be code running on cluster - that might be the operator. When we say "is the operand available" that might mean "the new code is rolled out" or "all old API objects have been updated" or "we're able to sign certificate requests"

Here are the guarantees components can get when they follow the rules we define:

1. Cause an installation to fail because a component is not able to become available for use
2. Cause an upgrade to hang because a component is not able to successfully reach the new upgrade
3. Prevent a user from clicking the upgrade button because components have one or more preflight criteria that are not met (e.g. nodes are at version 4.0 so the control plane can't be upgraded to 4.2 and break N-1 compat)
4. Ensure other components are upgraded *after* your component (guarantee "happens before" in upgrades, such as kube-apiserver being updated before kube-controller-manager)

### Status

The operator should use the placeholders for `.status.versions.version` in its deployment manifest to get them replaced with the real values when the payload is built. These values should be passed onto the operator at the runtime, e.g., via environment variables or flags, to populate its `.status.versions`.

The operator should ensure that all the fields of `.status` in ClusterOperator are atomic changes. This means that all the fields in the `.status` are only valid together and do not partially represent the status of the operator.

### Version

The operator reports an array of versions. A version struct has a name, and a version. There MUST be a version with the name `operator`, which is watched by the CVO to know if a cluster operator has achieved the new level. The operator MAY report additional versions of its underlying operands.

Example:

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: kube-apiserver
spec: {}
status:
  ...
  versions:
    - name: operator
      # Watched by the CVO
      version: 4.0.0-0.alpha-2019-03-05-054505
    - name: operator-image
      version: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:digest000" 
    - name: kube-apiserver
      # Used to report underlying upstream version
      version: 1.12.4
    - name: kube-apiserver-image
      # Used to report underlying upstream image pull specification
      version: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:digest001" 
```

#### Version reporting during an upgrade

When your operator begins rolling out a new version it must continue to report the previous operator version in its ClusterOperator status.
While any of your operands are still running software from the previous version then you are in a mixed version state, and you should continue to report the previous version.
As soon as you can guarantee you are not and will not run any old versions of your operands, you can update the operator version on your ClusterOperator status.

### Conditions

Refer [the godocs](https://godoc.org/github.com/openshift/api/config/v1#ClusterStatusConditionType) for conditions.

In general, ClusterOperators should contain at least three core conditions:

* `Progressing` indicates that the component (operator and all configured operands)
	is actively rolling out new code, propagating config changes (e.g, a version change), or otherwise
	moving from one steady state to another. Operators should not report
	Progressing when they are reconciling (without action) a previously known
	state. Operators should not report Progressing only because DaemonSets owned by them
	are adjusting to a new node from cluster scaleup or a node rebooting from cluster upgrade.
	If the observed cluster state has changed and the component is
	reacting to it (updated proxy configuration for instance), Progressing should become true
	since it is moving from one steady state to another.
	A component in a cluster with less than 250 nodes must complete a version
	change within a limited period of time: 90 minutes for Machine Config Operator and 20 minutes for others.
	Machine Config Operator is given more time as it needs to restart control plane nodes.
* `Available` indicates that the component (operator and all configured operands)
	is functional and available in the cluster. Available=False means at least
	part of the component is non-functional, and that the condition requires
	immediate administrator intervention.
	A component must not report Available=False during the course of a normal upgrade.
* `Degraded` indicates that the component (operator and all configured operands)
	does not match its desired state over a period of time resulting in a lower
	quality of service. The period of time may vary by component, but a Degraded
	state represents persistent observation of a condition. As a result, a
	component should not oscillate in and out of Degraded state. A component may
	be Available even if its degraded. For example, a component may desire 3
	running pods, but 1 pod is crash-looping. The component is Available but
	Degraded because it may have a lower quality of service. A component may be
	Progressing but not Degraded because the transition from one state to
	another does not persist over a long enough period to report Degraded. A
	component must not report Degraded during the course of a normal upgrade.
	A component may report Degraded in response to a persistent infrastructure
	failure that requires eventual administrator intervention.  For example, if
	a control plane host is unhealthy and must be replaced. A component should
	report Degraded if unexpected errors occur over a period, but the
	expectation is that all unexpected errors are handled as operators mature.

There are two optional conditions:

* `Upgradeable` indicates whether the component (operator and all configured
	operands) is safe to upgrade based on the current cluster state. When
	Upgradeable is False, the cluster-version operator will prevent the
	cluster from performing impacted updates unless forced.  When set on
	ClusterVersion, the message will explain which updates (minor or patch)
	are impacted. When set on ClusterOperator, False will block minor
	OpenShift updates. The message field should contain a human readable
	description of what the administrator should do to allow the cluster or
	component to successfully update. The cluster-version operator will
	allow updates when this condition is not False, including when it is
	missing, True, or Unknown.

* `EvaluationConditionsDetected` indicates the result of the detection
	logic that was added to a component to evaluate the introduction of an
	invasive change that could potentially result in highly visible alerts,
	breakages or upgrade failures. You can concatenate multiple Reason using
	the "::" delimiter if you need to evaluate the introduction of multiple changes.

The message reported for each of these conditions is important.
All messages should start with a capital letter (like a sentence) and be written for an end user / admin to debug the problem.
`Degraded` should describe in detail (a few sentences at most) why the current controller is blocked.
The detail should be sufficient for an engineer or support person to triage the problem.
`Available` should convey useful information about what is available, and be a single sentence without punctuation.
`Progressing` is the most important message because it is shown by default in the CLI as a column and should be a terse, human-readable message describing the current state of the object in 5-10 words (the more succinct the better).

For instance, if the CVO is working towards 4.0.1 and has already successfully deployed 4.0.0, the conditions might be reporting:

* `Degraded` is false with no message
* `Available` is true with message `Cluster has deployed 4.0.0`
* `Progressing` is true with message `Working towards 4.0.1`

If the controller reaches 4.0.1, the conditions might be:

* `Degraded` is false with no message
* `Available` is true with message `Cluster has deployed 4.0.1`
* `Progressing` is false with message `Cluster version is 4.0.1`

If an error blocks reaching 4.0.1, the conditions might be:

* `Degraded` is true with a detailed message `Unable to apply 4.0.1: could not update 0000_70_network_deployment.yaml because the resource type NetworkConfig has not been installed on the server.`
* `Available` is true with message `Cluster has deployed 4.0.0`
* `Progressing` is true with message `Unable to apply 4.0.1: a required object is missing`

The progressing message is the first message a human will see when debugging an issue, so it should be terse, succinct, and summarize the problem well.  The degraded message can be more verbose. Start with simple, easy to understand messages and grow them over time to capture more detail.

#### Happy conditions

Operators should set `reason` and `message` for both happy and sad conditions.
For sad conditions, the strings help explain what is going wrong.
For happy conditions, the strings help convince users that things are going well.

`AsExpected` is a common choice for happy reasons with messages like `All is well` or `NodeInstallerProgressing: 3 nodes are at revision 7`.

Having an explicit happy reasons also make it easier to do things like:

```none
sort_desc(count by (reason) (cluster_operator_conditions{name="cloud-credential",condition="Degraded"}))
```

for convenient aggregation, without having to mix in the time-series values to distinguish happy and sad cases.

#### Conditions and Install/Upgrade

Conditions determine when the CVO considers certain actions complete, the following table summarizes what it looks at and when.


| operation | version | available | degraded | progressing | upgradeable
|-----------|---------|-----------|----------|-------------|-------------|
| Install completion[1] | any(but should be the current version) | true | any | any | any
| Begin upgrade(patch) | any | any | any | any | any
| Begin upgrade(minor) | any | any | any | any | not false
| Begin upgrade (w/ force) | any | any | any | any | any
| Upgrade completion[2]| newVersion(target version for the upgrade) | true | false | any | any

[1] Install works on all components in parallel, it does not wait for any component to complete before starting another one.

[2] Upgrade will not proceed with upgrading components in the next runlevel until the previous runlevel completes.

See also: https://github.com/openshift/cluster-version-operator/blob/a5f5007c17cc14281c558ea363518dcc5b6675c7/pkg/cvo/internal/operatorstatus.go#L176-L189

### Related Objects

Related objects are the fundamental set of objects related to your operator.
As mentioned previously, the insights operator and must-gather depend on related images to collect relevant diagnostic information about each ClusterOperator.

In addition to specifying related objects statically in your operator's manifests, you should also implement logic in your operator to keep related objects up-to-date.
This ensures that another client cannot permanently change your related objects, and it enables the operator to change related objects dynamically based on operator configuration or cluster state. 

Related objects are identified by their `group`, `resource`, and `name` (and `namespace` for namespace-scoped resources).

To declare a namespace as a related object:
```yaml
# NOTE: "" is the core group
- group: ""
  resource: "namespaces"
  name: "openshift-component"
```

To declare a certificate in an operator namespace as a related object:
```yaml
- group: "cert-manager.io"
  resource: "certificates"
  namespace: "openshift-component"
  name: "component-certificate"
```

To declare a configmap in a shared namespace as a related object:
```yaml
- group: ""
  resource: "configmaps"
  namespace: "openshift-config"
  name: "component-config"
```

It is possible to declare related objects without specifying a name or namespace.
If you specify a namespace without specifying a name, you are declaring that all objects of that group/resource _in that namespace_ are related objects.
If you specify only group and resource, you are declaring that all objects of that group/resource _throughout the cluster_ are related objects.

To declare all of a namespaced resource in a particular namespace as related objects:
```yaml
- group: "component.openshift.io"
  resource: "namespacedthings"
  namespace: "openshift-component"
  name: ""
```

To declare all of a particular group/resource throughout the cluster as related objects:
```yaml
- group: "component.openshift.io"
  resource: "componentthings"
  namespace: ""
  name: ""
```