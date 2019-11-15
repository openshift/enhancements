---
title: user-provided-price-sheets
authors:
  - "@chancez"
reviewers:
  - @tflannag
  - "@emilym1"
approvers:
  - "@operator-framework/team-metering"
creation-date: 2019-11-06
last-updated: 2019-11-06
status: implementable
---

# user-provided-price-sheets

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

TBD

## Summary

User-provided price sheets is a way for users to define custom price rates for calculating cost reports on resource reservations or usage.
It is an opt-in feature that would require the user to provide a configuration that defines the prices and rates they desire.

## Motivation

Currently metering only supports calculating costs using AWS billing data, but does not support calculating costs for other clouds nor on-premise solutions.
User-provided price sheets would enable metering to expand it's functionality to those who wish to define their own prices for clouds and environments which are not yet supported.

Additionally, by creating the concept of price sheets, we must define our cost report queries to work against a well-defined API.
Instead of writing a cost ReportQuery per cloud provider, we can write queries that will work against any price sheet.
This would allow us, and users to programmatically generate price sheets, more easily unlocking support cost reports on different clouds and on-premise environments.

### Goals

- Allow users to define their own prices for resources, eg: $0.015 per core hour.
- Enable better use of cost [ReportQueries][reportqueries] by standardizing cost calculations to use price sheet Presto tables.

### Non-Goals

- Perfect ease-of use. Price sheets builds upon multiple core concepts already available in Metering. We can accomplish everything we need with existing functionality, but it may not be as easy as it could be. We will add higher level abstractions to hide the lower level details once the concept is proven.
- Adding support for new clouds. Supporting user-provided price sheets is the primary goal, and the secondary goal is ensuring cloud bills could be used to generate a price sheet.

## Proposal

Currently, cost calculations for AWS are done by determining the total cluster cost for the reporting period by looking at the AWS billing data, using node metrics to determine the nodes active during the reporting period.
This total cluster cost was then split up by pod, or namespace by calculating the percentage of usage or request for the pod/namespaces in terms of CPU or memory.
This percentage was then multiplied by the cost for the reporting period to get the total cost for a pod or namespace.

The price sheet model changes how we calculate cost, but actually provides more granularity and actually translates to the same concepts as above but with a bit more complexity as a result of the flexibility.

Today, with AWS, to calculate cost, we do cost reporting by multiplign usage by a price for rate of usage.
In a similar fashion, we propose to do this for other environments by means of a price sheet.
The primary difference is that instead of doing this over the entire reporting period, we do it per pricing period in the billing report, and the calculation is done by calculating the price per pricing period and summing the results of that over the entire reporting period.

Users can define price sheets using currently available functionality.
Price sheets are enabled by adding new default ReportQueries that reference price sheet tables in calculating costs and having user define price sheets using the ReportDataSource and PrestoTable resources.

At a high level this means we need to do the following:

- Define a Presto table schema that supports both user-provided prices and calculated prices from cloud billing data.
- Create built in [ReportQuery][reportqueries] and [ReportDataSource][reportdatasources] that can create price sheet table using AWS billing data.
- Create built in [ReportQueries][reportqueries] that calculate costs using a price sheet specified as a [Report input][report-input].

### User Stories [optional]

- As a member of the operations team, I want to create a Report that let's me calculate costs with any price sheet.
- As a member of the operations team, I want to be able to define my own prices to be used with Reports.

### Implementation Details/Notes/Constraints [optional]


#### Price Sheet Table Schema

The Presto table schema is defining a set of column names and their data types that all price sheet Presto database tables will have, enabling ReportQueries designed to use price sheets to work with any of them interchangeably.

The price sheet will define 7 columns.

- `resource varchar` is the type of resource, such as CPU or memory.
- `resource_unit varchar` is the unit of the resource. For CPU it might be cores or millicores and for memory it could be bytes, gigabytes, etc.
- `time_unit varchar` is the unit of time that the resource is billed at. For example 'hour' or 'minute'.
- `price double` is the price the resource is billed at, in terms of resources used in terms of `resource_unit` per `time_unit`.
- `currency varchar` is the currency the price is in.
- `price_start_date timestamp` is when this price starts
- `price_end_date timestamp` is when this price ends

There are a few properties that each row must obey to ensure correct results:
- Each row is expected to be a non-overlapping period of time specified by `price_start_dateand` and `price_end_date`, also known as the "price period".
- For each price period, there is only 1 row for a particular value of `resource`. This means one currency, one price, one `resource_unit`, one `time_unit` per price period per resource.
- For a given `resource`, the correct `unit` is specified. E.g.: for `resource=cpu`, `resource_unit` must be either `cores` or `millicores`, and must not be specified in different units within the same price period.


#### Initial design

To start, we will provide an example of a ReportDataSource and ReportQuery users can use to create a price sheet (details in ReportDataSources) and a set of ReportQueries for calculating cost using a price sheet ReportDataSource.
Once we are satisfied with the results, we can enable creating these resource as part of the Metering install by configuring any necessary options in the MeteringConfig.
Before proceeding with full support for the feature, we will likely leverage an "unsupported features" flag in the MeteringConfig that must be set to in order install to the price sheet resources, otherwise the user can manually create the resources if they wish to test the functionality before it's GA.

#### Price sheet ReportDataSource and ReportQuery

A ReportDataSource in the context of price sheets serves the purpose of creating and/or exposing a PrestoTable to Reports, ReportQueries and other ReportDataSources to enable accessing the price sheet from the reporting sub-system.
To support for user-provided price sheets, we will start by having users use a ReportDataSource to either create a view or table for price sheets.
This will be done by specifying a ReportQuery to use that defines the SQL for creating the table with whatever contents so long as it meets the criteria listed above.

In addition to writing a ReportQuery for creating a user-defined price sheet, we also need to write a ReportQuery that can be used to create a price sheet from the AWS ReportDataSource tables we already have.

Today a ReportDataSource can be used to create a view, or it can be used to reference an existing PrestoTable resource, so we will need to update it to support creating a table or we can start with just using views.
Creating a table from a ReportQuery directly can be done the same way as a view but by setting a spec.view to false on the PrestoTable resource to create a table instead of a view, all that is needed is updating ReportDataSources to have a field for creating a table instead of a view, and to set the corresponding option.

#### Cost ReportQueries

In addition to ReportQueries which will be used to generate a table or view containing a user's prices, we need to write new ReportQueries that calculate cost using a price sheet table.
Currently the only queries we have for calculating cost are the AWS ones, which calculate it directly from the billing table data.

Calculating price is very similar to how we calculate usage in our existing (non-cost) pod and namespace queries for usage/request for CPU or memory.
The new ReportQueries for calculating cost can be adapted from the existing ReportQueries we have for usage/request and then modeled to calculate the cost by JOINing against the price table to get the price and doing the price calculation.

### Risks and Mitigations

TBD

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).


- [ ] run Reports using a custom price sheet with static values and verify results.
- [ ] run Reports using an AWS price sheet and verify results. This likely requires using fixtures for the billing data to verify results.

### Graduation Criteria

##### Dev Preview -> Tech Preview

Dev preview will be based on documentation and reference examples user can take and use with Metering today.

Going to tech preview will be done by adding support for enabling and creating a price sheets related resources by specifying an option in the MeteringConfig.
During tech preview, this option will be behind a "unsupport features" flag in the MeteringConfig CRD.

##### Tech Preview -> GA

- There is a supported configuration option in the MeteringConfig CRD for enabling the price sheet functionality.
- Improved user experience. Users can define a price sheet with static prices and times in their `MeteringConfig` which will create all the ReportDataSources and PrestoTable resources.
- Improved e2e testing
- Sufficient time for feedback

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Upgrades are dealt with by the Metering operator and OLM.
During dev preview this feature leverages only existing functionality through documentation, and tech preview requires enabling an unsupported feature flag which can be used to prevent upgrades from tech preview to GA if we determine we cannot safely upgrade.

Upgrades following GA would primarily be a matter of updating the price sheet related Metering resources (ReportDataSources and ReportQueries) which is no different from how we upgrade any other resource today (eg services/deployments or existing Metering resources like ReportDataSources).

Downgrade for Metering in this case isn't supported by OLM, but if the user wished to manually downgrade, then the Metering components would continue to function, and the new price sheet functionality would likely continue to work in most cases.
In the case that it no longer works, the impact would be limited to periodic retries for the resources until they're deleted manually by the user.

### Version Skew Strategy

## Implementation History

- Research spikes have been done to determine:
  - The desired schema for price sheets and queries to produce them
  - SQL queries for calculating cost based on namespace memory usage using price sheets
  - SQL queries to translate the raw AWS billing report tables into a price sheet conformant schema using a view

## Drawbacks

## Alternatives

Since we are starting by leveraging existing functionality with the plans for abstractions that hide some of these details away, the main alternative would be to either add no higher level abstraction or build it into the higher level abstraction instead.

We will explore whether or not we users should be required to create their own ReportDataSource or ReportQueries for creating a basic price sheet and if providing a simpler configuration through a MeteringConfig is actually necessary.

It is also possible some of that functionality could be implemented as a more specialized version of ReportDataSource rather than using the existing methods, but we believe that any improvements that could be made here could be equally applied to the existing functionality as well.
For example, ins

## Infrastructure Needed [optional]

n/a

[reports]: https://github.com/operator-framework/operator-metering/blob/master/Documentation/reports.md
[report-input]: https://github.com/operator-framework/operator-metering/blob/master/Documentation/reports.md#inputs
[reportqueries]: https://github.com/operator-framework/operator-metering/blob/master/Documentation/reportqueries.md
[reportdatasources]: https://github.com/operator-framework/operator-metering/blob/master/Documentation/reportdatasources.md
