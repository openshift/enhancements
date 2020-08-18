# openshift-operator-developer-doc

## Motivation for this document
This document offers high level information about OpenShift ClusterOperators and Operands.
It also provides information about developing with OpenShift operators and the OpenShift release payload. 
When updating READMEs in core OpenShift repositories, we realized there is overlap of content.
We've created this document to serve as a common link for READMEs to answer `What is an Operator?` and 
`How do I build/update/test/deploy this code?`.

Many OpenShift Operators were built using [openshift/library-go's framework](https://github.com/openshift/library-go/tree/master/pkg/operator)
and also utilize [openshift/library-go's build-machinery-go](https://github.com/openshift/build-machinery-go), a 
collection of building blocks, Makefile targets, helper scripts, and other build-related machinery.  As a result, there are common methods 
for building, updating, and developing these operators.  When adding features or investigating bugs, you may need to swap out a 
component layer of a release payload for a locally built one.  Again, the process for doing this is shared among OpenShift operators, as well
as the process of updating CVO-managed deployments in a cluster.  

## What is an OpenShift ClusterOperator?
OpenShift deploys many operators on Kubernetes. Core OpenShift operators are ClusterOperators (CO). You can list them with this:
```console
$ oc get clusteroperators
```
To get a description of a CO run
```console
$ oc describe co/clusteroperatorname
```
Operators are controllers for a 
[Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).  Operators automate many tasks
in application management, such as deployments, backups, upgrades, leader election, reconciling resources, etc.

The [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#customresourcedefinitions) in OpenShift can be viewed with this:
```
$ oc get crds
```
For a particular CRD (ex: openshiftapiservers.operator.openshift.io):
```
$ oc explain openshiftapiservers.operator.openshift.io
```
For more information, check out [the Operator pattern in Kubernetes](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).    

While ClusterOperators manage individual components (Custom Resources), the 
[Cluster Version Operator](https://github.com/openshift/cluster-version-operator) (CVO) is responsible for managing the ClusterOperators.

## What is an Operand?
The component that an operator manages is its operand.  For example, the cluster-kube-controller-manager-operator CO manages the 
cluster-kube-controller-manager component running in OpenShift. An operator can be responsible for multiple components.  For example, 
the CVO manages all of the COs (in this way ClusterOperators are also operands). 

## What is an OpenShift release image?
To get a list of the components and their images that comprise an OpenShift release image, grab a 
release from the [openshift release page](https://openshift-release.svc.ci.openshift.org/) and run:
```console
$ oc adm release info registry.svc.ci.openshift.org/ocp/release:version
```

If the above command fails, you may need to authenticate against `registry.svc.ci.openshift.org`.    
If you are an OpenShift developer, see [authenticating against ci registry](#authenticating-against-ci-registry)    
You'll notice that currently the release payload is just shy of 100 images.

## CVO Cluster Operator Status
You can check on the status of any ClusterOperator with
```console
$ oc get co/clusteroperatorname -o yaml
```
At any time the staus of a CO may be:
- Available
- Degraded
- Progressing       

[CVO Cluster Operator Status Conditions Overview](https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/clusteroperator.md#conditions)

## Metrics
By default, an OpenShift ClusterOperator exposes [Prometheus](https://prometheus.io) metrics via `metrics` service.

## Debugging
Operators expose events that can help in debugging issues. To get operator events, run following command:

```
$ oc get events -n [cluster-operator-namespace]
```

## How do I build|update|verify|run unit-tests
In an openshift/repo that utilizes [openshift/library-go's build-machinery-go](https://github.com/openshift/build-machinery-go), a useful command to list all make targets for a repository is:     
```
$ make help
```

This builds the binaries:    
```
$ make build
```

To build the images (or, use `make help` to get the target for an individual image):    
```
$ make images
```
note: [issues with imagebuilder and make images](#issues-with-imagebuilder-and-make-images)

To run unit tests:    
```console
$ make test-unit
```

This will run verify-gofmt, verify-govet, verify-bindata (if applicable), verify-codegen     
There may be other verify targets added to individual Makefiles, also.     
```console
$ make verify
```

This will update-bindata (if applicable), update-codegen, update-gofmt:    
```console
$ make update
```

If you have exported your KUBECONFIG to point to a running cluster, you can run    
the end-to-end tests that live in the repository (that would run in CI with e2e-aws-operator)    
```console
$ make test-e2e
```

If the repository utilizes glide for dependency management, you can update dependencies with    
```console
$ make update-deps
```

If the repository uses gomod for dependency management, [this doc](https://github.com/openshift/enhancements/blob/master/enhancements/oc/upgrade_kubernetes.md) is useful.

## How can I test changes to an OpenShift operator/operand/release component? 

When developing on OpenShift, you'll want to test your changes in a running cluster.  Any component image in a 
release payload can be substituted for a locally built image.  You can test your changes in an 
OpenShift cluster like so:     
```
If using quay.io, repositories are by default private.  Visit quay.io and change settings 
to public for any new image repos you push.  This is to allow OpenShift to pull your image.
```

### OPTION A - START WITH A RUNNING CLUSTER
The operator deployment is modified to reference a test operand image, rather than
modifying the operand deployment directly.  This is because an operator is meant to stomp on changes 
made to it's operand.    
[note: operator and operand may share a repository](#operator-repositories-can-house-operands)     

For this example, a change to `openshift-apiserver` operand is being tested.    
1. Build the operand image and push it to a public registry (use any containers cli, quay.io, docker.io).    
   If `make images` doesn't work, your Makefile may need an update, see [here](#issues-with-imagebuilder-and-make-images)
```
$ cd local/path/to/openshift-apiserver
$ make IMAGE_TAG=quay.io/yourname/openshift-apiserver:test images
$ buildah push quay.io/yourname/openshift-apiserver:test
```

2. Edit the operator deployment definition to reference your test image and build the test operator image.
Each operator (ex openshift/cluster-openshift-apiserver-operator) has a `manifests/*.deployment.yaml` 
that sets the env `IMAGE` for its operand image.   
   * Edit the
   [manifests/deployment yaml](https://github.com/openshift/cluster-openshift-apiserver-operator/blob/master/manifests/0000_30_openshift-apiserver-operator_07_deployment.yaml)
   to reference your test image in `spec.containers.env` like so:
   ```yaml
    spec:
      serviceAccountName: openshift-apiserver-operator
      containers:
      - name: openshift-apiserver-operator
        env:
        - name: IMAGE
          value: quay.io/yourname/openshift-apiserver:test
       ---
     ```
   * Build and push the _operator_ image with the updated deployment above to a public registry (use any containers cli, quay.io, docker.io).        
   If `make images` doesn't work, your Makefile may need an update, see [here](#issues-with-imagebuilder-and-make-images)

```
$ cd local/path/to/cluster-openshift-apiserver-operator
$ make IMAGE_TAG=quay.io/yourname/openshift-apiserver-operator:test images
$ buildah push quay.io/yourname/openshift-apiserver-operator:test
```
    
3. Disable the CVO or tell CVO to ignore your component.  The CVO reconciles a cluster to its known good state as
laid out in its resource definitions.  You cannot edit a deployment without first disabling the CVO
(Well, you can, but the CVO will reconcile and stomp on any changes you make).
There are 2 paths to working around CVO, you'll need to either:
    * Set your operator in umanaged state.
    See [here](https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/clusterversion.md#setting-objects-unmanaged)
    for how to patch clusterversion/version object.    
    or    
    * Scale down CVO and edit a deployment in a running cluster like so:
```console
$ oc scale --replicas 0 -n openshift-cluster-version deployments/cluster-version-operator`    
```

4. Edit the ClusterOperator deployment in a running cluster for which you're logged in as admin user.    

```console
$ oc get deployments -n openshift-apiserver-operator
$ oc edit deployment openshift-apiserver-operator -n openshift-apiserver-operator
```
Edit the env `OPERATOR_IMAGE` (if it exists), the env `IMAGE`, as well as:   
```yaml
spec:
  containers:
    image: quay.io/yourname/openshift-apiserver-operator:test
```
```
env: IMAGE
value: quay.io/yourname/openshift-apiserver:test
```
```
env: OPERATOR_IMAGE
value: quay.io/yourname/openshift-apiserver-operator:test
```
exception: service-ca-operator deployment
```
env: CONTROLLER_IMAGE
value: quay.io/yourname/service-ca-operator:test
```
    
You'll see a new deployment rolls out, and in the operand namespace, `openshift-apiserver`, a new deployment 
rolls out there as well, using your `openshift-apiserver:test` image.      

To set your cluster back to its original state, you can simply:
```console
$ oc scale --replicas 1 -n openshift-cluster-version deployments/cluster-version-operator
```
or remove the overrides section you added in `clusterversion/version`. 

### OPTION B - LAUNCH A CLUSTER WITH YOUR CHANGES
#### Build a new release image that has your test components built in. 
For this example I'll start with the release image
`registry.svc.ci.openshift.org/ocp/release:4.2`  
and test a change to the `github.com/openshift/openshift-apiserver` repository.

1. Build the image and push it to a registry (use any containers cli, quay.io, docker.io)    
If `make images` doesn't work, your Makefile may need an update, see [here](#issues-with-imagebuilder-and-make-images)
```
$ cd local/path/to/openshift-apiserver
$ make IMAGE_TAG=quay.io/yourname/openshift-apiserver:test images
$ buildah push quay.io/yourname/openshift-apiserver:test
```
    
2. Assemble a release payload with your test image and push it to a registry
Get the name of the image (`openshift-apiserver`) you want to substitute:
```
$ oc adm release info registry.svc.ci.openshift.org/ocp/release:4.2
```
If the above command fails, you may need to authenticate against `registry.svc.ci.openshift.org`. 
If you are an OpenShift developer, see [authenticating against ci registry](#authenticating-against-ci-registry)    

This command will assemble a release payload incorporating your test image _and_ will push it to the quay.io repository.    
Be sure to set this repository in quay.io as `public`.    
```
$ oc adm release new --from-release registry.svc.ci.openshift.org/ocp/release:4.2 \
  openshift-apiserver=quay.io/yourname/openshift-apiserver:test \
  --to-image quay.io/yourname/release:test
```

If the above command succeeds, move on to Step 3.
If the above command fails, you need to authenticate against `quay.io`. See [authenticating against quay.io](#authenticating-against-quay-registry)

3. Extract the installer binary from the release payload that has your test image.  This will extract 
`openshift-install` binary that is pinned to your test release image.
```console
$ oc adm release extract --command openshift-install quay.io/yourname/release:test
```
4. Run the installer extracted from your release image.
```console
$ ./openshift-install create cluster --dir /path/to/installdir
```
Once the install completes, you'll have a cluster running that was launched with a known-good release payload with whatever test image(s) you've
substituted. 

## Issues with imagebuilder and make images
`make images` utilizes [imagebuilder](https://github.com/openshift/imagebuilder)
1. There are a few known issues with `imagebuilder`:
    - https://github.com/openshift/imagebuilder/issues/144
        - workaround is to prepull any images that fail with docker or buildah as outlined in the issue
    - https://github.com/openshift/imagebuilder/issues/140
        - workaround is something like [this](https://github.com/openshift/cluster-config-operator/pull/98)
    - other issues [here](https://github.com/openshift/imagebuilder/issues)
2. Your Makefile may need to update it's `build-image` call, to follow [this example](https://github.com/openshift/library-go/blob/master/alpha-build-machinery/make/default.example.mk)
3. If `make images` still isn't working, you can replace that with a buildah or docker command like so:    
*Dockerfile name varies per repository, this example uses Dockerfile.rhel*

```console
$ buildah bud -f Dockerfile.rhel -t quay.io/myimage:local .
or    
$ docker build -f Dockerfile.rhel -t quay.io/myimage:local .
```
## Operator repositories can house operands
For some operator repositories, such as openshift/service-ca-operator, the controller (operand) images are included 
in the operator image.  Any changes to the controllers  are made in the operator repository.  When testing 
a change, there is only the single operator image substitution, rather than a separate operand image build 
plus operator image build for such operators.

## Authenticating against ci registry
(Internal Red Hat registries for developer testing)
Add the necessary credentials to your local `~/.docker/config.json` (or equivalent file) like so:
 - visit `https://api.ci.openshift.org`, `upper right corner '?'` dropdown to `Command Line Tools`
 - copy the given `oc login https://api.ci.openshift.org --token=<hidden>`, paste in your terminal
 - then run `oc registry login` to add your credentials to your local config file _usually ~/.docker/config.json_   

## Authenticating against quay registry
Add the necessary credentials to your local `~/.docker/config.json` (or equivalent file) like so:
 - Visit `https://try.openshift.com`, `GET STARTED`, login w/ RedHat account if not already,
   choose any `Infrastructure Provider`, `Copy Pull Secret` to a local file (or download) 
 - Add the quay auth from the pull-secret to ~/.docker/config.json. 
   The config file should have the following:
   
```console
$cat ~/.docker/config.json
{
  "auths": {
    "registry.svc.ci.openshift.org": {
      "auth": "blahblahblah"
    },
    "quay.io": {
      "auth": "blahblahblah"
    }
  }
}
```
