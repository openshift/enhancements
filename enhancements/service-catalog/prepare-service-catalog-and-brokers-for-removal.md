---
title: prepare-service-catalog-and-brokers-for-removal
authors:
  - "@jmrodri"
reviewers:
  - "@dmesser"
  - "@joelanford"
approvers:
  - TBD
creation-date: 2019-09-12
last-updated: 2019-09-23
status: implementable
see-also:
replaces:
superseded-by:
---

# prepare-service-catalog-and-brokers-for-removal

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Operators became the strategy in OpenShift 4.x for customers to manage their
applications in the cluster. As this technology has improved, the need for
having a Service Catalog backed by Service Brokers is no longer needed. As an
enterprise product, we need to deprecate the catalog and brokers, inform the
users of this deprecation, and then finally remove them from the product in
OpenShift 4.4. This should allow plenty of time to migrate any broker related
items the customer may have had to Operators.

## Motivation

As our strategy shifted from Service Brokers to Operators, customers and
partners need sufficient time to shift their strategy towards Operators as well.

### Goals

Service Catalog, Service Brokers and their associated operators need to be
deprecated and removed by OpenShift 4.4.

Alert users that the above have been deprecated and will be removed in a
future OpenShift release.

### Non-Goals

Not in scope for 4.3, is writing the preflight-check for CVO.

## Proposal

### User Stories

#### Story 1

The cluster-svcat-apiserver-operator will notify users and admins via alerts in
prometheus.

Create 2 gauges called service_catalog_apiserver_installed and
service_catalog_apiserver_operator_installed.

#### Story 2

The cluster-svcat-controller-manager-operator will notify users and admins via
alerts in prometheus.

Create 2 gauges called service_catalog_installed and
service_catalog_controller_manager_operator_installed

#### Story 3

The ansible-service-broker-operator will notify users and admins via alerts in
prometheus.

Create 2 gauges called ansible_service_broker_installed and
ansible_service_broker_operator_installed. Possibly create a new ServiceMonitor.

#### Story 4

The template-service-broker-operator will notify users and admins via alerts in
prometheus.

Create 2 gauges called template_service_broker_installed and
template_service_broker_operator_installed. Possibly create a new
ServiceMonitor.

#### Story 5

Verify the prometheus alerting manager is enabled, otherwise, enable it. For
each of the operators add alerting code.

The operators will need a new dependency.
[alerting client](https://github.com/prometheus/alertmanager/blob/master/client/client.go)

### Implementation Details/Notes/Constraints [optional]

The 4 operators, Service Catalog Controller Manager operator (svcat-cm-op),
Service Catalog APIServer operator (svcat-apiserver-op), the Ansible Service
Broker Operator (ASBO) and the Template Service Broker Operator (TSBO), will
need a metric to tell folks when any of these are in use.

Each of the 4 operators will need to create a ServiceMonitor, and expose 2 new
metrics: a gauge indicating their operand is installed and a gauge indicating
the operator itself is installed. If not installed, the gauge will have a 0
value.

#### cluster-svcat-apiserver-operator
  - already has a ServiceMonitor
  - need to create a new Gauge
    - name: "service_catalog_apiserver_installed"
    - help: "Indicates whether the service catalog apiserver is installed"
  - need to create a new Gauge
    - name: "service_catalog_apiserver_operator_installed"
    - help: "Indicates whether the service catalog apiserver operator is
      installed"

#### cluster-svcat-controller-manager-operator
  - already has a ServiceMonitor
  - need to create a new Gauge
    - name: "service_catalog_installed"
    - help: "Indicates whether the service catalog is installed"
  - need to create a new Gauge
    - name: "service_catalog_controller_manager_operator_installed"
    - help: "Indicates whether the service catalog controller manager operator
      is installed"

#### ansible-service-broker-operator
  - has a ServiceMonitor configured for the broker
    - can the operator use it? if not, need to create its own monitor
  - need to create a new Gauge
    - name: "ansible_service_broker_installed"
    - help: "Indicates whether the ansible service broker is installed"
  - need to create a new Gauge
    - name: "ansible_service_broker_operator_installed"
    - help: "Indicates whether the ansible service broker operator is installed"

#### template-service-broker-operator
  - could not determine if this operator has a ServiceMonitor configured.
    If not, need to create its own monitor
  - need to create a new Gauge
    - name: "template_service_broker_installed"
    - help: "Indicates whether the template service broker is installed"
  - need to create a new Gauge
    - name: "template_service_broker_operator_installed"
    - help: "Indicates whether the template service broker operator is installed"

#### other resources
  - [alerting client](https://github.com/prometheus/alertmanager/blob/master/client/client.go)
  - [Alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
  - [Alertmanager](https://prometheus.io/docs/alerting/alertmanager/)
  - [Metric types](https://prometheus.io/docs/concepts/metric_types/)
  - [cluster-svcat-controller-manager-operator](https://github.com/openshift/cluster-svcat-controller-manager-operator/)
  - [cluster-svcat-apiserver-operator](https://github.com/openshift/cluster-svcat-apiserver-operator)
  - [template-service-broker-operator](https://github.com/openshift/template-service-broker-operator/)
  - [ansible-service-broker-operator](https://github.com/openshift/ansible-service-broker/tree/master/operator)

### Risks and Mitigations

TBD

## Design Details

### Test Plan

#### cluster-svcat-apiserver-operator
  - already has unit tests
  - add e2e test to test observing cluster config
  - add e2e test to test alert is sent when operator is enabled
  - add e2e test to test alert is sent when apiserver is enabled
  - add e2e test to test apiservice is removed when state changed to Removed

#### cluster-svcat-controller-manager-operator
  - already has unit tests
  - add e2e test to test observing cluster config
  - add e2e test to test alert is sent when operator is enabled
  - add e2e test to test controller-manager is removed when state changed
    to Removed

#### ansible-service-broker-operator
  - already has unit tests
  - seems to have e2e tests already
  - update e2e tests to test alert is sent when operator is enabled
  - update e2e tests to test alert is sent when broker is enabled

#### template-service-broker-operator
  - TBD, it will have some tests just not sure what is already there.
  - add/update e2e tests to test alert is sent when operator is enabled
  - add/update e2e tests to test alert is sent when broker is enabled

### Graduation Criteria

The Service Catalog and Brokers have reached their deprecation stage of the
graduation criteria.

##### Removing a deprecated feature

- In 4.2, the Service Catalog and Brokers APIs were marked as deprecated in the
  documentation:

  INSERT LINK TO 4.2 DOCS
  [PR tracking release notes](https://github.com/openshift/openshift-docs/issues/16327#issuecomment-533383688)

- In 4.3, we will alert users that the Service Catalog and Brokers are being
  used despite being deprecated.

- In 4.4, Service Catalog and Brokers will be removed from shipping.

### Upgrade / Downgrade Strategy

Clusters will be blocked from upgrading if Service Catalog and/or Service
Brokers are enabled. They must be removed before cluster upgrades may proceed.

### Version Skew Strategy

N/A

## Implementation History

- 20190917 - Added deprecated section.
- 20190916 - Initial proposal to remove service catalog and brokers

## Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

N/A
