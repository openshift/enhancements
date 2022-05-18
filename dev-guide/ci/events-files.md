# Openshift CI produced files related to events

To aid in debugging, the Openshift CI (prow) produces various files related to events.  The files
reside in the gather-must-gather and gather-extra subdirectories in the artifacts storage area.
You will see the "Artifacts" link in the upper right of a prow job (for examples, click on any
prow job from the [main prow page](https://prow.ci.openshift.org/).

This document provides details on where the files are located and what they contain.

Throughout this document, we provide links to actual files as examples but realize that after some
period of time, prow jobs are purged and the files will no longer be present. To get a recent/valid
link goto prow, change the search pattern to match a prow job you're interested in
(e.g.,  [link for e2e-aws jobs](https://prow.ci.openshift.org/?job=periodic*e2e-aws&state=success)),
and follow a similar path shown in the example links.

Artifacts will be in a path with a URL of the form: `.../{job-name}/{run-id}/artifacts/{job-slug}/{step-name}/artifacts/`.
For must-gather output, it will be in `.../{job-name}/{run-id}/artifacts/{job-slug}/{step-name}/artifacts/gather-must-gather`.
For gather-extra output, it will be in `.../{job-name}/{run-id}/artifacts/{job-slug}/{step-name}/artifacts/gather-extra`.

## Events files

* event-filter.html
  * included in the output of a `oc adm must-gather`
  * contains events from namespaces gathered by `oc adm must-gather` plus events that exist in the kube-apiserver at the time `oc adm must-gather` was run.
    Namespaces not listed as relatedResources for clusteroperators will be missing.
  * located in the prow job stored artifacts at `.../gather-must-gather/artifacts/` and contained in the must-gather.tar
    file in that same location
  * [Example](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/gather-must-gather/artifacts/event-filter.html)
  * Tip: wait several seconds for the html page to finish rendering so you can use the text entry boxes to search

* events.json (same as `oc_cmds/events` mentioned below except as json)
  * generated via `oc get events --all-namespaces -o json` (see [source code](https://github.com/openshift/release/blob/f5017d5136a740a4186477b02bed70047ade200b/ci-operator/step-registry/gather/extra/gather-extra-commands.sh#L61))
  * located in the prow job stored artifacts at `.../gather-extra/aftifacts/`
  * [Example](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/gather-extra/artifacts/events.json)

* events (same as `gather-extra/artifacts/events.json` except not json)
  * generated via `oc get events --all-namespaces` (see [source code](https://github.com/openshift/release/blob/f5017d5136a740a4186477b02bed70047ade200b/ci-operator/step-registry/gather/extra/gather-extra-commands.sh#L62)).  This file has all the events present in the apiserver at the time it was generated.
  * located in the prow job stored artifacts at `.../gahter-extra/artifacts/oc_cmds/events`
  * [Example](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/gather-extra/artifacts/oc_cmds/events)

* resource-events_(date)-(number).zip
  * located in the prow job stored artifacts at `.../<test_name>/openshift-e2e-test/artifacts/junit`
  * There will be one file per invocation of `openshift-tests`
  * Contains a watch of all events including "delete" related events
  * These contain kube-apiserver events
  * [Example1](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/openshift-e2e-test/artifacts/junit/resource-events_20220321-040307.zip), [Example2](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/openshift-e2e-test/artifacts/junit/resource-events_20220321-052002.zip)
  * Here's a sample of the contents:

```bash
    ./default:
    total 332
    -rw-rw-r-- 1 dperique dperique 339013 Dec 31  1979 events.json

    ./e2e-check-for-dns-availability-1546:
    total 68
    -rw-rw-r-- 1 dperique dperique 67899 Dec 31  1979 events.json

    ./e2e-k8s-service-load-balancer-with-pdb-new-485:
    total 88
    -rw-rw-r-- 1 dperique dperique 86468 Dec 31  1979 events.json
```

* e2e-events_(date)-(number).json
  * located in the prow job stored artifacts at `.../artifcts/<test_name>/openshift-e2e-test/artifacts/junit`; there are two of them
  * these are monitor events, not API events, produced by `monitor` which is part of the openshift-tests invocation. They do not have the same info as the API events.
  * The first one is from the initial installation
  * The second one is from the upgrade (if initial installation fails, this one will not be present)
  * [Example1](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/openshift-e2e-test/artifacts/junit/e2e-events_20220321-040307.json), [Example2](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-gcp-ovn-upgrade/1505747388038385664/artifacts/e2e-gcp-ovn-upgrade/openshift-e2e-test/artifacts/junit/e2e-events_20220321-052002.json)

* core/events.yaml
  * located in the must-gather.tar at `.../namespaces/(aNamespace)/core/events.yaml`; there is one per namespace.
  * produced by [must-gather](https://github.com/openshift/must-gather/) using `oc adm inspect`.

* We talked about events related to [Devan's PR](https://github.com/openshift/origin/pull/26862) which we can document
  more later
  * These events are Monitor events (received by a Watcher)
