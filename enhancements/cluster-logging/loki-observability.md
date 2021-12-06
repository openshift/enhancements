---
title: LokiStack Observability
authors:
  - "@ronensc"
reviewers:
approvers:
creation-date: 2021-10-21
last-updated: 2021-10-21
status: []
see-also: []
replaces: []
superseded-by: []
---

# LokiStack Observability

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement adds observability capabilities to Loki operator in the form of Prometheus rules and alerts.
These rules and alerts are used to identify abnormal conditions and update operators.

## Motivation

### Goals

* List the alerts and rules that could identify abnormal conditions.

### Non-Goals

## Proposal

### User Stories

* As an SRE, I want alerts to inform me that LokiStack needs my attention.

### API Extensions

### Implementation Details

The operational alerts are used to send alerts from Loki deployment indicating some level of unhealthy or misbehavior of
the Loki Deployment. The alerts are based on metrics consumed from Prometheus into alert manager, evaluated and when
fired, pushed to notify human operators.

Loki Alerts can be categorized as follows:

| Category | Description |
|----------|-------------|
| **Loki System** | One or more of the Loki components is misbehaving; this can be due to any of Loki or OCP system that is not working as expected |
| **Tenant workload** | Customer (tenant) sending (writing) workload or (reading) query logs and/or acting against the system in some anomalous behavior |
| **Storage** | The storage system that Loki works with is not working as expected. This includes internal storage and external S3 storage |

Loki alerts can be of various severities:

| Severity | Description |
| -------- | ----------- |
| **Critical** | For alerting current and impending disaster situations. These alerts page an SRE. The situation should warrant waking someone in the middle of the night. |
| **Warning** | The vast majority of alerts should use the severity. Issues at the warning level should be addressed in a timely manner, but don't pose an immediate threat to the operation of the cluster as a whole. |
| **Info** | Info level alerts represent situations an administrator should be aware of, but that don't necessarily require any action. Use these sparingly, and consider instead reporting this information via Kubernetes events. |

See also: [Alerting Consistency](/enhancements/monitoring/alerting-consistency.md)
#### Alerts

Some alerts are based on metrics exposed by Loki and some on recording rules, which are metrics created based on other metrics.

See also [Defining recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/)

Alerts summary table:

| Alert | Category | Severity | Summary |
| ----- | -------- | -------- | ----------- |
| [LokiRequestErrors](#LokiRequestErrors) | Loki System | Critical | At least 10% of requests are responded by 5xx server errors |
| [LokiRequestPanics](#LokiRequestPanics) | Loki System | Critical | A panic was triggered |
| [LokiRequestLatency](#LokiRequestLatency) | Loki System | Critical | The 99th percentile is experiencing high latency (higher than 1 second) |
| [LokiTenantRateLimit](#LokiTenantRateLimit) | Tenant Workload | Warning | A tenant is experiencing rate limiting |
| [LokiCPUHigh (Warning)](#LokiCPUHigh  (Warning)) | Loki System | Warning | Loki's CPU usage is high (above 75% for 5 minutes) |
| [LokiCPUHigh (Critical)](#LokiCPUHigh (Critical)) | Loki System | Critical | Loki's CPU usage is high (above 95% for 5 minutes) |
| [LokiMemoryHigh (Warning)](#LokiMemoryHigh (Warning)) | Loki System | Warning | Loki's memory usage is high (above 75% of the configured limit for 5 minutes) |
| [LokiMemoryHigh (Critical)](#LokiMemoryHigh (Critical)) | Loki System | Critical | Loki's memory usage is high (above 95% of the configured limit for 5 minutes) |
| [LokiStorageSlowWrite](#LokiStorageSlowWrite) | Storage | Warning | The 99th percentile is experiencing slow write |
| [LokiStorageSlowRead](#LokiStorageSlowRead) | Storage | Warning | The 99th percentile is experiencing slow read |
| [LokiStorageFreeSpaceLow](#LokiStorageFreeSpaceLow) | Storage | Warning | Loki's storage is running out of space. |
| [LokiWritePathHighLoad](#LokiWritePathHighLoad) | Loki System | Warning | The write path of Loki is loaded |
| [LokiReadPathHighLoad](#LokiReadPathHighLoad) | Loki System | Warning | The read path of Loki is loaded |


##### LokiRequestErrors

| Key | Value |
| --- | ----- |
| **Summary** | At least 10% of requests are responded by 5xx server errors. |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Critical |
| **Steps to recovery** | Check the component logs for additional context |

query:
```PromQL
for: 15m
expr: |
    sum(
        rate(
            loki_request_duration_seconds_count{status_code=~"5.."}[1m]
        )
    ) by (namespace, job, route)
    /
    sum(
        rate(
            loki_request_duration_seconds_count[1m]
        )
    ) by (namespace, job, route)
    * 100
    > 10
```

References:
* https://monitoring.mixins.dev/loki/
* https://github.com/rhobs/configuration/blob/main/docs/sop/observatorium.md#lokirequesterrors

##### LokiRequestPanics

| Key | Value |
| --- | ----- |
| **Summary** | A panic was triggered. |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Critical |
| **Steps to recovery** |  |

query:
```PromQL
expr: |
    sum(
        increase(
            loki_panic_total[10m]
        )
    ) by (namespace, job)
    > 0
```

Note: `loki_panic_total` metric is implemented but not documented.
* https://github.com/grafana/loki/blob/a046d7926122cef1b619d89bfcf15403e14e0de3/pkg/util/server/recovery.go#L19-L23
* https://grafana.com/docs/loki/latest/operations/observability/

References:
* https://monitoring.mixins.dev/loki/
* https://github.com/rhobs/configuration/blob/main/docs/sop/observatorium.md#lokirequestpanics

##### LokiRequestLatency

| Key | Value |
| --- | ----- |
| **Summary** | The 99th percentile is experiencing high latency (higher than 1 second). |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Critical |
| **Steps to recovery** |  |

query:
```PromQL
for: 15m
expr: |
    namespace_job_route:loki_request_duration_seconds:99quantile{route!~"(?i).*tail.*"}
    > 1
```

underline recording rule to calculate the 99th percentile:
```PromQL
record: namespace_job_route:loki_request_duration_seconds:99quantile
expr: |
    histogram_quantile(0.99, 
        sum(
            rate(
                loki_request_duration_seconds_bucket[1m]
            )
        )
        by (le, namespace, job, route)
    )
```

References:
* https://monitoring.mixins.dev/loki/
* https://github.com/rhobs/configuration/blob/main/docs/sop/observatorium.md#lokirequestlatency

##### LokiTenantRateLimit

| Key | Value |
| --- | ----- |
| **Summary** | A tenant is experiencing rate limiting. The number of discarded logs per tenant and reason over the last 30 minutes is above 100. |
| **Description** |  |
| **Category** | Tenant Workload |
| **Severity** | Warning |
| **Steps to recovery** | - Inspect the tenant ID for the ingester limits panel. <br> - Contact the tenant administrator to adapt the client configuration. |

query:
```PromQL
for: 15m
expr: |
    sum (
        sum_over_time(
            rate(
                loki_discarded_samples_total{namespace="observatorium-logs-production"}[1m]
            )[30m:1m]
        )
    ) by (namespace, tenant, reason)
    > 100
```

Note 1: `loki_discarded_samples_total` metric is implemented but not documented
* https://github.com/grafana/loki/blob/25163470b0b861d5c539b66a4c5613e46666e0c8/pkg/validation/validate.go#L98-L105
* https://grafana.com/docs/loki/latest/operations/observability/

Note 2: `loki_discarded_samples_total` couldn't be found in Prometheus after installing LokiStack through Loki Operator.
It probably starts to be exposed once it has a value.


References:
* https://github.com/rhobs/configuration/blob/main/docs/sop/observatorium.md#lokitenantratelimitwarning


##### LokiCPUHigh (Warning)
| Key | Value |
| --- | ----- |
| **Summary** | Loki's CPU usage is high (above 75% for 5 minutes) |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Warning |
| **Steps to recovery** | Consider scaling out the component |

query:
```PromQL
for: 5m
expr: |
    sum(
        rate(
            container_cpu_usage_seconds_total{namespace="openshift-logging", container=~"loki-.*"}[5m]
        )
    ) by (namespace, container)
    /
    sum(
        kube_pod_container_resource_limits{namespace="openshift-logging", resource="cpu"}
    ) by (namespace, container)
    * 100
    > 75
```

**Note**: Currently, no limits are configured in loki-operator

##### LokiCPUHigh (Critical)
| Key | Value |
| --- | ----- |
| **Summary** | Loki's CPU usage is high (above 95% for 5 minutes) |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Critical |
| **Steps to recovery** |  |

query:
```PromQL
for: 5m
expr: |
    sum(
        rate(
            container_cpu_usage_seconds_total{namespace="openshift-logging", container=~"loki-.*"}[5m]
        )
    ) by (namespace, container)
    /
    sum(
        kube_pod_container_resource_limits{namespace="openshift-logging", resource="cpu"}
    ) by (namespace, container)
    * 100
    > 95
```

**Note**: Currently, no limits are configured in loki-operator

##### LokiMemoryHigh (Warning)
| Key | Value |
| --- | ----- |
| **Summary** | Loki's memory usage is high (above 75% of the configured limit for 5 minutes) |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Warning |
| **Steps to recovery** | Consider scaling out the component |

query:
```PromQL
for: 5m
expr: |
    sum(container_memory_working_set_bytes{namespace="openshift-logging", container=~"loki-.*"}) by (namespace, container)
    /
    sum(kube_pod_container_resource_limits{namespace="openshift-logging", resource="memory"}) by (namespace, container)
    * 100
    > 75
```

**Note**: Currently, no limits are configured in loki-operator

##### LokiMemoryHigh (Critical)
| Key | Value |
| --- | ----- |
| **Summary** | Loki's memory usage is high (above 95% of the configured limit for 5 minutes) |
| **Description** |  |
| **Category** | Loki System |
| **Severity** | Critical |
| **Steps to recovery** |  |

query:
```PromQL
for: 5m
expr: |
    sum(container_memory_working_set_bytes{namespace="openshift-logging", container=~"loki-.*"}) by (namespace, container)
    /
    sum(kube_pod_container_resource_limits{namespace="openshift-logging", resource="memory"}) by (namespace, container)
    * 100
    > 95
```

**Note**: Currently, no limits are configured in loki-operator

##### LokiStorageSlowWrite
| Key | Value |
| --- | ----- |
| **Summary** | The 99th percentile is experiencing slow write |
| **Description** | The 99th percentile of storage write is higher than 1 second |
| **Category** | Storage |
| **Severity** | Warning |
| **Steps to recovery** |  |

query:
```PromQL
for: 5m
expr: |
    histogram_quantile(.99, 
        sum(
            rate(
                loki_boltdb_shipper_request_duration_seconds_bucket{namespace="openshift-logging", operation="WRITE"}[5m]
            )
        ) by (le, namespace, operation)
    )
    > 1
```

##### LokiStorageSlowRead
| Key | Value |
| --- | ----- |
| **Summary** | The 99th percentile is experiencing slow read |
| **Description** | The 99th percentile of storage read is higher than 1 second |
| **Category** | Storage |
| **Severity** | Warning |
| **Steps to recovery** |  |

query:
```PromQL
for: 5m
expr: |
    histogram_quantile(.99, 
        sum(
            rate(
                loki_boltdb_shipper_request_duration_seconds_bucket{namespace="openshift-logging", operation="QUERY"}[5m]
            )
        ) by (le, namespace, operation)
    )
    > 1
```

##### LokiStorageFreeSpaceLow
| Key | Value |
| --- | ----- |
| **Summary** | Loki's storage is running out of space |
| **Description** |  |
| **Category** | Storage |
| **Severity** | Warning |
| **Steps to recovery** | Consider increasing the storage space |

query:
```PromQL
TODO: This is a bit tricky since we don't have a metric for the free storage space.
```

##### LokiWritePathHighLoad
| Key | Value |
| --- | ----- |
| **Summary** | The write path of Loki is loaded |
| **Description** | The QpS (query per second) reaches the benchmark value of LokiStack t-shirt size. |
| **Category** | Loki System |
| **Severity** | Warning |
| **Steps to recovery** | Consider scaling out the distributors and the ingesters |

query:
```PromQL
for: 5m
expr: |
    sum(
        rate(
            loki_distributor_bytes_received_total{namespace="openshift-logging"}[5m]
        )
    ) by (namespace) 
    >
    # Threshold per t-shirt size:
    # 500 GBpD = 500 * (1000*1000*1000) / (24*60*60) BpS
    # 2 TBpD = 2 * (1000*1000*1000*1000) / (24*60*60) BpS
    $threshold
```

##### LokiReadPathHighLoad
| Key | Value |
| --- | ----- |
| **Summary** | The read path of Loki is loaded |
| **Description** | The QpS (query per second) reaches the benchmark value of LokiStack t-shirt size. |
| **Category** | Loki System |
| **Severity** | Warning |
| **Steps to recovery** | Consider scaling out the query frontends and the queriers |

query:
```PromQL
for: 5m
expr: |
    sum(
        rate(
            loki_request_duration_seconds_count{
                namespace="openshift-logging",
                job="query-frontend",
                # HTTP & GRPC routes
                route=~"loki_api_v1_series|api_prom_series|api_prom_query|api_prom_label|api_prom_label_name_values|loki_api_v1_query|loki_api_v1_query_range|loki_api_v1_labels|loki_api_v1_label_name_values|/logproto.Querier/Query|/logproto.Querier/Label|/logproto.Querier/Series|/logproto.Querier/QuerySample|/logproto.Querier/GetChunkIDs",
            }[5m]
        )
    ) by (namespace) 
    >
    # Threshold per t-shirt size:
    #   Small: 50
    #   Medium: 100
    $threshold
```


#### Assumptions

#### Security

### Risks and Mitigations

## Design Details

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
