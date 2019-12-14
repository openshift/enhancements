# Developer Catalog Helm 3 Roadmap

Helm is a Kubernetes package manager.  Helm 3.0 is a major release of helm which brings in a rich set of features and 
addresses major security concerns around tiller.  Red hat Openshift wants to bring Helm based content support to Openshift 4.4 Developer Catalog along with Operators to strengthen the helm based ecosystem.  

This document lists use cases to bring Helm install and management experience to Openshift Developer Catalog.  The goal is to start with a single cluster scenarios and expand to multicluster scenarios in the future.  


### Use Cases 

#### Developer Catalog Helm Install
- As Cloud Admin/Operator, I want to view, filter and search helm charts from Developer Catalog
  - As Cloud Admin/Operator I want to view available helm charts with chart name, repository name, short description, icon etc
  - As Cloud Admin/Operator, I want to filter helm charts by different Categories
  - As Cloud Admin/Operator, I want to filter helm charts by classification (Beta, tech preview etc), architecture (intel, power, z), repositories, qualification (certified, non certified contents)
  - As Cloud Admin/Operator, I want to filter certified and non certified contents with badges
- As Cloud Admin/Operator, I want get an overview of selected helm chart from Developer Catalog
  - As Cloud Admin/Operator, I want get details from helm chart Readme.md file. This includes introduction of the helm chart, resources required, Chart details, prerequisites, Security Policies, Limitations , installation instructions, configuration details etc.
  - As Cloud Admin/Operator, I want to view available versions of selected helm chart and get details of selected version
  - As Cloud Admin/Operator, I want to view licenses attached with my helm chart
- As Cloud Admin, Operator, I want to configure values and deploy selected helm chart on Single Cluster
  - As Cloud Admin/Operator, I want to configure name of the release, namespace to deploy helm chart
  - As Cloud Admin/Operator, I want to view description, tooltip of chart configuration parameters
  - As Cloud Admin/Operator, I want to view all chart configuration parameters with default values
  - As Cloud Admin/Operator, I want to understand required and non required parameters to deploy helm chart
  - As Cloud Admin/Operator, I want to get validation errors before deploying a chart
  - As Cloud Admin/Operator , I want to get notification if chart deployment is successful or failed with detailed message
- As Cloud Admin/Operator, I want to configure values and deploy selected helm chart on one or more remote Clusters
  - As Cloud Admin/Operator, I want to select namespaces and clusters I have access and deploy helm chart
  - As Cloud Admin/Operator, I want to get validation errors before installing a chart
  - As Cloud Admin/Operator, I want to get notification of successful and failed deployments on selected clusters and namespaces

#### Developer Catalog day 2 management of deployed helm releases
- As Cloud Admin/Operator/editor/viewer,  I want to view and manage deployed releases on single or IBM Multicloud managed Hub Cluster
  - As Cloud Admin/Operator/editor/viewer, I want to view all deployed releases in my namespaces
  - As Cloud Admin/editor, I want to delete a release and all resources created by that release in my namespaces
  - As Cloud Admin/editor, I want to view if new version of helm chart is available to upgrade a release
  - As Cloud Admin/editor, I want to upgrade a release to a new version. I want to have option to keep existing configuration or overwrite values during upgrade.
  - As Cloud Admin/Operator/editor/viewer, I want to be able to get history the helm release
  - As Cloud Admin/editor, I want to be able to rollback a release to stable version in case of any upgrade failures
 - As Cloud Admin/editor, I want to get cluster and namespace information where release is deployed
 - As Cloud Admin/Operator/editor/viewer, I want to get details and status of Kubernetes resources deployed by the release
    - As Cloud Admin/Operator/editor/viewer, I want view release details such as release name , version, history, release notes, chart name etc.
    - As Cloud Admin/Operator/editor/viewer, I want get details of all deployed kubenetes resources for release
    - As Cloud Admin/Operator/editor/viewer, I want to view details of any particular deployed kubenetes resource
 - As Cloud Admin/Operator/editor/viewer, I want to view launch links and navigate to launch link endpoints exposed by services, ingresses and routes.
 - As Cloud Admin/Operator/editor/viewer, I want to view logs of release and resources created by a release


#### Content Management
  - As Cloud Admin, I want to configure/create a repository url or docker registry url where helm charts are hosted
  - As Cloud Admin, I want to delete a repository or docker registry url configuration
  - As Cloud Admin, I want to enforce namespace scoped access control on helm repository and helm charts
  - As Cloud Admin, I want to sync each repository or docker registry new contents are changed


### Reference
- Basic architecture documentation from helm community is given below.
https://github.com/helm/community/blob/master/helm-v3/000-helm-v3.md
