---
title: cluster-fleet-evaluation
authors:
  - "@aravindhp"
reviewers:
  - "@openshift/openshift-patch-managers"
approvers:
  - "@openshift/openshift-patch-managers"
creation-date: 2022-08-16
last-updated: 2022-08-16
---

# Cluster Fleet Evaluation

## Problem Statement

OpenShift components sometimes need to introduce a change that could result in
highly visible alerts, breakages or upgrade failures on user clusters. In
scenarios like this it will be useful for the component owner to assess how many
customers would be affected before rolling out the change and based on that
feedback decide on the level of investment required to properly inform the
customer base, or change implementation details.

## Solution

Rather than introducing the change, add just the detection logic to the operator
for the change and associate it with a
[ClusterStatusConditionTypes](https://github.com/openshift/api/blob/cc0db1116639638254e87a564902833f1ee006d5/config/v1/types_cluster_operator.go#L145)
called [EvaluationConditionDetected](https://github.com/openshift/api/pull/1250).
If the
[ConditionStatus](https://github.com/openshift/api/blob/cc0db1116639638254e87a564902833f1ee006d5/config/v1/types_cluster_operator.go#L124)
associated with this type is
[ConditionTrue](https://github.com/openshift/api/blob/cc0db1116639638254e87a564902833f1ee006d5/config/v1/types_cluster_operator.go#L107)
and includes the
[Reason]([Reason](https://github.com/openshift/api/blob/cc0db1116639638254e87a564902833f1ee006d5/config/v1/types_cluster_operator.go#L133))
you are expecting, it will indicate that the change would have affected that
cluster. However, if the _ConditionStatus_ is _false_, or it's _true_ but the
_Reason_ does not include the _Reason_ you are expecting, the change doesn't
affect the cluster.  You can join multiple _Reason_ to test multiple scenarios.

Now release the detection logic to master and
[backport](https://docs.google.com/document/d/1PC87sSFa_zGCk95kXDW-wrVxnlgBmkHqpOgQnd4bbUw/edit)
it to a N-1 Z stream release where N is the current release under development.
The Z stream backport is useful as it allows the team to gather the data without
going through a full release cycle and waiting for customers to upgrade to newer
minor versions. However, we still have to wait for them to upgrade to a patch
level that includes the new info gathering logic.

How far back the backports should be done is subjective and the teams
should aim to backport as far back as possible so as to collect a reasonable
amount data that allows them to reach a conclusion. This decision will be
largely based on what versions the bulk of the fleet of clusters are running
and the nature of the problem such as whether or not the problem domain
is likely influenced by cluster age or the version it was installed with. For
instance clusters running 4.8 today, 12 months after GA, are disproportionately
older and installed on older versions than those which were running 4.8 in
the first 90 days after GA.

Once the Z streams have been released the component developers can [interrogate
the telemetry data](#interacting-with-telemetry) to figure out the prevalence of
the scenario that the change will introduce. Allow sufficient time for customers
to upgrade to the Z stream release. The upgrade data is also available in
telemetry.

At this point the team needs to validate that the telemetry data matches their
expectations and make a decision if they still plan to move forward with the
change. In particular the team needs to measure the level of impact the change
has. If the percentage of impact is very low then it might be preferable to
contact the affected customers. Alternatively if the percentage of impact is
very high, it would be preferable that the team re-evaluate the change and
investigate if there is a way to automate the fix process in a seamless manner.

If the component team decides to move forward, they will then be responsible for
writing a [Knowledge Centered Support](https://source.redhat.com/groups/public/cee-kcs-program/cee_kcs_program_wiki/kcs_solutions_content_standard_v20)
(KCS) document providing sufficient information describing the scenario and how
to resolve it. Refer to the [KCS section](#writing-a-kcs-document) for more
information.

Once the KCS article/solution is ready and the list of customers are available,
the component team can alert the affected customers using the methods described
in the [Alerting customers](#alerting-customers) reference section. Teams
should make every attempt to use all methods of alerting customers (this ensures
contact with customers is made), but are free to drop options if they feel using
that method may not reach the proper audience or may be too expensive.

**Note**: Before the team goes down this path it will be useful to discuss the
outcome and expectations with their respective OpenShift staff engineer, product
manager and PLM contact. PLM contacts are listed under *column S* in the
[OCP Team Member Tracking sheet](https://docs.google.com/spreadsheets/d/1M4C41fX2J1nBXhqPdtwd8UP4RAx98NA4ByIUv-0Z0Ds/edit#gid=1382138347).
In particular the team should set their expectations for action they intend to
take (abort the change, add automated migration, contact individual customers,
etc) for a given impact percentage. This avoids post hoc rationalizing a
decision if the data that comes back is unexpected.

## Reference

### Interacting with telemetry

#### Pre-requisites

To access the telemetry data please follow
[these instructions](https://help.datahub.redhat.com/docs/interacting-with-telemetry-data).
Once you have access, you will be able to perform queries against the data
collected from customer clusters.

#### Running queries

You can query the telemetry data either using the
[Thanos](https://telemeter-lts.datahub.redhat.com/graph) or
[Grafana](https://telemeter-lts-dashboards.datahub.redhat.com) interfaces. For
example, if you have introduced a new _ConditionStatus/Reason_ called _FooBar_,
it can be queried using:

```sql
count by (_id, ebs_account) (
  cluster_operator_conditions{name="component-name",condition="ClusterFleetEvaluation",reason="FooBar"}
  * on (_id) group_left (ebs_account)
  topk by (_id) (1, id_version_ebs_account_internal:cluster_subscribed)
)
```

This will return a list of clusters and the associated customers. The above
query is just an example and the team is free to create their own custom
queries.

### Writing a KCS document

[KCS](https://source.redhat.com/groups/public/cee-kcs-program/cee_kcs_program_wiki/kcs_solutions_content_standard_v20)
docs are split into solutions and articles.
[Solutions](https://access.redhat.com/node/add/kcs-solution) explain breakages
and how to fix them and are transactional in nature. The scenario presented here
would typically result in a KCS solution.
[Articles](https://access.redhat.com/node/add/kcs-article) are targeted towards
highlighting features / explanations that are outside the scope of the OpenShift
documentation. If you are new to writing a KCS doc, you can reach out to 
[Albert Myles](amyles@redhat.comto) who can connect you with a KCS coach.

### Alerting customers

Once the component team has collected the telemetry data and the list of
affected customers, they can alert them using any combination of the methods
listed below. This should only be done if the impact of the change being
introduced is very low. For help in facilitating use of any of these methods you
can reach out to your PLM contact. The way to choose method(s) are subjective to
the scope and impact of the change and should ideally be done in collaboration
with your respective OpenShift staff engineer, product manager and PLM contact.

In order of preference, in what teams should choose:
1. [Insight rule notification](https://access.redhat.com/labs/proactiveissuestracker/) -
   These appear on cloud.redhat.com and on cluster when customers are logged in)
    1. [Insights Rule Contribution wiki](https://source.redhat.com/groups/public/insights-rule-contribution/insights_rule_contribution_wiki/insights_rule_contribution_main) 
    2. Contact [Jan Holecek](mailto:jholecek@redhat.com) for questions.
2. [Technical Topic Torrent](https://source.redhat.com/groups/public/t3) (T3) -
   This is a communication to TAM/CSM’s (GCS) and GCS customers; delivered via
   email.
    1. Read the
       [T3 FAQ](https://source.redhat.com/groups/public/t3/technical_topic_torrent_wiki/t3_frequently_asked_questions_faq)
       for more details.
    2. Contact [gcs-tam-ocp@redhat.com](mailto:gcs-tam-ocp@redhat.com) if you
       have questions.
3. [Email Campaigns to affected customers](https://docs.google.com/document/d/11ZSX5HYG_-KPC-I4zuygK2BSBow58WnG0lQkTyNgMYI/edit) -
   This is direct email contact with customers
    1. While this should ideally be viewed as a last option, history has
       indicated that this is more effective 90% of time over customer portal
       banners.
    2. Contact [Will Wang](mailto:wiwang@redhat.com) for questions.
4. [Customer Portal Banners](https://docs.google.com/document/d/1rOU3aYNvW90dTPGoFJKA2RMtj65atXxsVoI3FLdIPeA/edit) -
   This will reach customers visiting our access.redhat.com (Customer Portal)
   web properties (IE: Docs, Product Pages, Downloads, etc)
    1. The linked document above is used to request the banner. The resulting
       banner can live on any page (negotiated with the Customer Portal Team) on
       the Red Hat Customer Portal.
       [Here](https://docs.google.com/presentation/d/1Tul10Enf35dsqHQdJrf5diX0yTLgC15_IgIvRYYeZ1s/edit#slide=id.g129b5bff778_0_27)
       is an example.
    2. **Note**: This option can be targeted to ‘customers’, ‘web properties’ on
       the customer portal (IE: docs, or product pages, etc).
    3. **Note**: The SRE and Services teams also status pages available to them
       that could also be potentially used to inform customers based on the
       scenario.
    4. Contact [Jacquelene Booker](mailto:jbooker@redhat.com) for questions.

In addition to the above methods, the team has an option to backport a
ClusterOperator _Upgradable=False_ guard in certain scenarios. This is not a
preferable but in situations where the percentage of impacted clusters is small
and the impact of upgrading without addressing the situation is high, it might
be applicable.
