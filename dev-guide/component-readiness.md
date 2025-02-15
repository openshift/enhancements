tl;dr, We can only ship an OCP release when Component Readiness is green.
There are only two ways to make it green: 
fix the regression or 
get BU to acknowledge and accept the regression.

The [Component Readiness dashboard](https://sippy.dptools.openshift.org/sippy-ng/component_readiness/main) allows our organization to
conclude with statistical certainty when our product has regressed.  
Moreover, nearly all issues reported by Component Readiness reflect direct negative impact for customers.
While Component Readiness gives us new insight into regressions, this builds on our longstanding policy that regressions are blocker bugs.

Starting in 4.16 (and aspirationally in 4.15), we will use Component Readiness to drive our release decisions:
- Component Readiness identified regressions are either blocker bugs or have merged PRs changing the acceptable level of reliability.
- OpenShift Program team will delay releases for unaccepted regressions.

These changes will allow us to better understand and adjust to the real-world impact of regressions, 
give a clear target for release quality (zero unaccepted statistical regressions), 
and provide a transparent record of our decisions around the release timing/quality tradeoff.

This improvement comes with a new responsibility for our management team: 
identified regressions must be accepted by engineering leadership (Nick Stielau, David Eads) and BU leadership (Kirsten Newcomer).

After written approval in the Jira bug representing the regression, the component team would reduce the acceptable reliability
for a particular capability in OCP.

Consider [openshift/sippy#1991](https://github.com/openshift/sippy/pull/1991).
Once merged, this change will allow `operator conditions network`, with the
specified set of variants, to go from 100% reliable to 66% reliable for the
4.17 release. It will, however, maintain the 4.16 100% baseline when 4.17 is the basis.
The format clearly exposes:
1. What is changing
2. What the previous reliability was
3. What the new reality is
4. Why we are allowing the regression

This "regression allowance" is due one week before the minor release date.  By
that time anything simple low risk should have long since merged and anything
risky must wait until an errata.  It also provides sufficient time for
discussion and ensures that we can release with the level of quality that we've
agreed to.
