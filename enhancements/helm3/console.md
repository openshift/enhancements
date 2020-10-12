---
title: helm-charts-in-developer-catalog
authors:
  - "@sbose78"
  - "@pedjak"
reviewers:
  - TBD
approvers:
  - "@deads2k"
  - "@spadgett"
  - "@bparees"
  - "@derekwaynecarr"
creation-date: 2020-01-09
last-updated: 2020-04-15
status: implementable
---

# Helm Charts in the Developer Catalog

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Managing Helm charts using the Developer perspective from DevConsole

## Motivation

Helm is a Kubernetes package manager.  Helm 3.0 is a major release of helm which brings in a rich set of features and addresses major security concerns around tiller.  

Red hat Openshift wants to bring Helm based content support to Openshift 4.4 Developer Catalog along with Operators to strengthen the helm based ecosystem.

![Developer Catalog](../helm3/assets/dev-catalog.png)

### Goals

* Provide RESTful API for managing Helm charts and releases
* Support disconnected installs
* Support easy management of available charts, aggregation from multiple sources and filtering
  
### Non-Goals

* Infrastructure for serving the default chart repository
* Process for curating charts within the default chart repository
  
## Proposal
  
### Charts in the Developer Catalog

The charts that would show up in the Developer Catalog will be powered by a [standard](https://helm.sh/docs/topics/chart_repository) Helm chart repository instance.

In the initial phase, the chart repository would be served of [redhat-helm-charts](https://redhat-developer.github.io/redhat-helm-charts) public [GitHub repository](https://github.com/redhat-developer/redhat-helm-charts).

New charts will be added and/or existing curated by submitting PRs against the above mentioned GitHub repository.

### How would the UI discover the charts

1. The UI would invoke `/api/helm/charts/index.yaml` endpoint to get [the repository index file](https://helm.sh/docs/topics/chart_repository/#the-index-file) so that the available charts can be rendered in the developer catalog. 

2. The above endpoint would proxy requests to the configured chart repository


![Helm Charts Repo Service](../helm3/assets/charts-repo.png)

### How would disconnected installs work 

1. The user would need to 'clone' the content of the chart repository over the fence

   * The public GitHub repository could be cloned into inside-the-network GitHub or Gitlab instance and configured to serve static content ( "Pages" ).
   * The content of the chart repository could be crawled and served using a (containerized or external) HTTP server, e.g. nginx

2. The URL serving the above static content would need to be passed to chart repository proxy running inside the cluster. 

### Configuring Helm Chart Repository location

Since the experience we are building needs a representation of the chart repository in-cluster, there needs to be a standard way to define a Kubernetes resource for the same.

Configuring Helm repository location could be modeled similar to [`OperatorSource`](https://github.com/operator-framework/operator-marketplace/blob/7d230952a1045624b7601b4d6e1d45b3def4cf76/deploy/crds/operators_v1_operatorsource_crd.yaml). 
Due to future planned federated usecases ([ODC-2994](https://issues.redhat.com/browse/ODC-2994)), a cluster admin should be able to declare multiple chart repositories.

#### Existing/Known In-Cluster Representations of Helm Chart Repositories

[Open Cluster Management project](https://github.com/open-cluster-management/multicloud-operators-subscription) introduced a notion of subscription for [a Helm chart](https://github.com/open-cluster-management/multicloud-operators-subscription/tree/master/examples/helmrepo-channel). A subscription is defined on the top of a channel of [type `HelmRepo`](https://github.com/open-cluster-management/multicloud-operators-subscription/blob/master/examples/helmrepo-channel/01-channel.yaml). Currently, chart repo configuration is provided in a separate `ConfigMap`, but the model could be extended to use the configuration provided in referred `HelmChartRepository` instance.

#### Introducing the HelmChartRepository API

The `HelmChartRepository` is a simple cluster-scoped API for letting users/admins define Helm Chart Repository configurations in a cluster. 

```yaml
apiVersion: helm.openshift.io/v1beta1
kind: HelmChartRepository
metadata:
  name: my-enterprise-chart-repo
spec:
  url: https://my.chart-repo.org/stable

  # optional and only needed for UI purposes
  displayName: myChartRepo

  # optional and only needed for UI purposes
  description: my private chart repo
```

An operator would watch for changes on them and reconfigure the chart repository proxy which is used to power the UI experience in OpenShift today.. 

Please note, the console backend already implements a few helm endpoints (including the chart proxy). In future, we plan  extract them into a separate service to make the functionality more modular, thereby decoupling the API contract from the scenario-specific implementations. 

Cluster admins would be able to easily manage `HelmChartRepositories` using the UI:

![Helm Cluster Configuration](assets/openshift-administration-cluster-settings.png)

or via CLI:

```shell
$ oc get helmchartrepositories
NAME          AGE
cluster       3h30m
```

In a case of multiple chart repositories, the console should be modified so that either allow:
* editing all chart repository instances at once through aggregated YAML document
* add/edit/removal of individual instances


Adding an additional chart repositories via CLI follows the usual k8s pattern:

```shell
$ cat <<EOF | oc apply -f -
apiVersion: helm.openshift.io/v1beta1
kind: HelmChartRepository
metadata:
  name: stable
spec:
  url: https://kubernetes-charts.storage.googleapis.com

  displayName: Public Helm stable charts

  description: Public Helm stable charts hosted on HelmHub
---
apiVersion: helm.openshift.io/v1beta1
kind: HelmChartRepository
metadata:
  name: incubator
spec:
  url: https://kubernetes-charts-incubator.storage.googleapis.com

  displayName: Public Helm charts in incubator state
EOF 

$ kubectl get helmchartrepositories
NAME          AGE
cluster       3h30m
stable        1m
incubator     1m
```

Chart repository proxy will use all configured chart repositories and deliver to the UI an aggregated index file. If needed by some future usecase, UI would be able read helm chart repository configuration and perhaps even talk to the individual chart repositories directly.

#### Taking the HelmChartRepository API upstream

##### Why

Enable admins to configure `Helm` chart repositories in the cluster which would be available for any user accessing the cluster, without neccessarily requiring the user to do a `helm repo add`.


#### How

1) We plan to use the [ChartRepository Config](https://github.com/helm/helm/blob/master/pkg/repo/chartrepo.go#L42) data structure in the proposed CRD, which is also used by the Helm CLI to list/add chart repositories.

2) Given that the `Helm` CLI manages chart repository configurations in the local filesystem only, we shall propose upstream changes in the [Helm CLI Repository](https://github.com/helm/helm) to optionally make the CLI aware of `HelmChartRepository` CRs in the current cluster context.

As a consequence, `helm repo list` would not only list local chart repositories from the local file system, it would also list chart repositories represented as Kubernetes objects in the cluster.


```shell
$ kubectl get helmchartrepositories
NAME          AGE
cluster       3h30m
stable        1m
incubator     1m

# repositories configured in the cluster.
$ helm repo list
NAME     	URL
cluster   http://my.chart-repo.org/stable
stable   	https://kubernetes-charts.storage.googleapis.com           
incubator	https://kubernetes-charts-incubator.storage.googleapis.com

# install a chart from the "stable" repo defined in the cluster
$ helm install mysql stable/mysql
```

3) Similar changes would be proposed to all `helm` application lifecycle commands to honour the presence of in-cluster representations of chart repository configurations.


#### Alternatives

#### 1. The configuration could be embedded into cluster-wide [`Console` config](https://github.com/openshift/api/blob/master/config/v1/types_console.go#L26)

Admins wouldn't be able to intuitively discover the operator config as a way to configure the Helm repository URLs. It becomes closely coupled with the console. Extracting Helm endpoints into a separate service would require moving the config as well.

#### 2. The configuration could be embedded into [`Console` operator config](https://github.com/openshift/api/blob/master/operator/v1/types_console.go#L26)

Conceptually, the Helm repository URL isn't really an operator configuration, hence this doesn't feel like the right place.
This approach would have similar issues with the previous alternative - admins wouldn't be able to intuitively discover the operator config as a way to configure the Helm repository URLs.

#### 3. OLM operator for Helm Configuration. 

Note, the Helm chart repository configuration today exists as a console configuration, which enables Console to proxy to the Helm chart repository URL. Moving it out of Console is outside the scope of this section. 

   * The default helm chart repository URL remains unchanged in the Console configuration.
   * Admin installs an OLM operator which only provides a `HelmChartRepository` cluster-scoped CRD
   * Admin creates a cluster-scoped CR. Note, this isn't very intuitive for the Admin.
   * Console-operator watches the new `HelmChartRepository` CR and reconciles.
   
Reflections on this approach:
* We get to avoid changes to `openshift/api`.
* However, Console operator would have to watch the `HelmChartRepository` CRD which it doesn't own.
* Creation of the cluster-scoped `HelmChartRepository` CR may not be very intuitive for the admin unless we show it in the Cluster Configuration UI in Console.
* Ideally, the operator should have been pre-installed in the cluster, but that isn't supported.

## How would the UI install charts

An endpoint that leverages the same Helm Golang APIs which the `helm install` command uses to install charts, will be introduced.

Here's how the control flow would look like:

1. The Console UI will create `POST` request containing appropriate JSON payload against `/api/helm/release` endpoint. 
2. The API handler for the given endpoint will, in turn talk to the API server (no Tiller in Helm3) using the user's authentication, while leveraging the Helm Golang API.

This is in-line with the "Console is a pretty kubectl" philosophy since Helm itself is a thin layer on top of kubectl.


![Helm Endpoints in Console Backend](../helm3/assets/helm-endpoints.svg)
