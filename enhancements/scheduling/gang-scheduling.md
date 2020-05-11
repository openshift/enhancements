---
title: gang-scheduling-in-kubernetes
authors:
  - "@ingvagabund"
reviewers:
  - "@damemi"
approvers:
  - "@soltysh"
creation-date: 2020-02-27
last-updated: 2020-02-27
status: provisional
see-also:
replaces:
superseded-by:
---

# Gang scheduling in Kubernetes

What is gang scheduling (or coscheduling) is described in [KEP proposal](https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/34-20180703-coscheduling.md).
Implementation of gang scheduling can be found under two repositories:
- https://github.com/kubernetes-sigs/kube-batch/ (implementing multi-tenant batch scheduling and resource sharing between prioritized queues)
- https://github.com/volcano-sh/volcano (extends and builds on top of batch scheduling framework from kube-batch)

Both repositories integrate [multi-level queue scheduling](https://en.wikipedia.org/wiki/Multilevel_queue) principle.
More about specific queue management implementation in [this doc](https://github.com/volcano-sh/volcano/blob/master/docs/design/queue/queue-state-management.md).

## Goals

1. Describe the current state of gang scheduling in Kubernetes.
2. Collect knowledge about its design, how it works, how to deploy it in OpenShift
3. Once we decide to integrate the feature within our portfolio, we don't need to revisit
and re-collect the same knowledge again.

## Summary

- The kube-batch scheduling framework is extendable through plugins and actions.
- Plugins allow to define various conditions (e.g. when a pod is considered evictable, when a queue is overused), priorities (e.g. defining order of jobs/queues processing), node scoring and other concepts. Providing building blocks for actions which implement various scheduling decision making logic.
- The framework provides default plugins such as `DRF` (focusing on fair job scoring), `Gang` (enforcing gang scheduling principles) or `Predicates`/`Priorities` (exposing predicates/priorities from the kube scheduler). With default actions such as `Allocate` (for scheduling group of pods as a single unit) or
`Preempt`/`Reclaim` (preempting group of pods wrt. multi-level queue and priority classes).
- All plugins and actions live under tiers which dictate in which order and how they are processed.
- The default plugins, actions and tiers are described in more detail [here](https://github.com/kubernetes-sigs/kube-batch/blob/master/doc/design/framework.md).
- Other documents describing design elements are available [here](https://github.com/kubernetes-sigs/kube-batch/tree/master/doc/design) and [here]( https://github.com/volcano-sh/volcano/tree/master/docs/design).
- Any consumer of the framework can write its own plugins and actions and thus extend/change the scheduling
decision making.

There's also an effort to integrate some functionality of kube-batch framework with [framework](https://kubernetes.io/docs/concepts/configuration/scheduling-framework/)
provided by the default kube-scheduler.


To learn more about the individual plugins and actions, check the code base under https://github.com/kubernetes-sigs/kube-batch/.

## Deploy and test

I am considering Vulcano as a referential implementation of the gang schediling for now.
The repository seems more active and alive than kube-batch.

Vulcano stack consists of (among other bits):
- Queue controller - managing lifecycle of queues (wrt. PodGroups/Jobs)
- Job controller - managing tasks of jobs and turning them into pods
- Admission controller - checking availability of a queue when creating a PodGroup/Job (through webhooks)
- CRDs for queue, jobs, etc.

To deploy the stack, edit and apply https://raw.githubusercontent.com/volcano-sh/volcano/master/installer/volcano-development.yaml:
- change port 443 to 6443
- `volcano-controllers` cluster role to extend RBAC rule for `jobs` with `jobs/finalizers`:
  ```
  - apiGroups:
    - batch.volcano.sh
    resources:
    - jobs
    - jobs/finalizers
    verbs:
    - get
    - list
    - watch
    - update
    - delete
  ```

Also notice the scheduler configuration:
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: volcano-scheduler-configmap
  namespace: volcano-system
data:
  volcano-scheduler.conf: |
    actions: "enqueue, allocate, backfill"
    tiers:
    - plugins:
      - name: priority
      - name: gang
      - name: conformance
    - plugins:
      - name: drf
      - name: predicates
      - name: proportion
      - name: nodeorder
      - name: binpack
```

The configuration enables three actions and two tiers of plugins.
With `gang` plugin enabled, the scheduler will require minimal number of replicas
to be schedulable for each job before job's state can be set to `Running`.

### Example job

Each job (`jobs.batch.volcano.sh` CRD) has a list of task categories, each task category with its own pod template and number of replicas.

You can use the following CR to see how the scheduler behaves (manifest borrowed from https://github.com/volcano-sh/volcano/blob/master/example/job.yaml):

```
apiVersion: batch.volcano.sh/v1alpha1
kind: Job
metadata:
  name: test-job
spec:
  minAvailable: 3
  schedulerName: volcano
  policies:
    - event: PodEvicted
      action: RestartJob
  maxRetry: 5
  queue: default
  tasks:
    - replicas: 6
      name: "default-nginx"
      template:
        metadata:
          name: web
        spec:
          containers:
            - image: nginx
              imagePullPolicy: IfNotPresent
              name: nginx
              resources:
                requests:
                  cpu: "1"
          restartPolicy: OnFailure
```

Notice the cpu request is set to `1` cpu and the minimal number of replicas is `3`.
In case your cluster does not have enough cpu resource to schedule at least 3 replicas,
the job will not change it's state to `Running` (due to gang scheduling minimal replicas constraint).

With sufficient cpu resource (e.g. setting cpu request to `150m`) you get:

```
$ oc get pods
NAME                       READY     STATUS     RESTARTS   AGE
test-job-default-nginx-0   0/1       OutOfcpu   0          6m3s
test-job-default-nginx-1   0/1       OutOfcpu   0          6m4s
test-job-default-nginx-2   1/1       Running    0          6m4s
test-job-default-nginx-3   1/1       Running    0          6m4s
test-job-default-nginx-4   0/1       OutOfcpu   0          6m3s
test-job-default-nginx-5   1/1       Running    0          6m4s
```

With insufficient cpu resource none of the pods gets to run:

```
$ oc get pods
NAME                       READY     STATUS    RESTARTS   AGE
test-job-default-nginx-0   0/1       Pending   0          9m38s
test-job-default-nginx-1   0/1       Pending   0          9m38s
test-job-default-nginx-2   0/1       Pending   0          9m38s
test-job-default-nginx-3   0/1       Pending   0          9m38s
test-job-default-nginx-4   0/1       Pending   0          9m38s
test-job-default-nginx-5   0/1       Pending   0          9m38s
```

### Scheduling cycle logs snippet

```
I0124 13:06:29.087366       1 cache.go:775] There are <1> Jobs, <1> Queues and <6> Nodes in total for scheduling.
I0124 13:06:29.087401       1 session.go:135] Open Session 55ab3cf8-3eaa-11ea-a61c-0a580a81020a with <1> Job and <1> Queues
I0124 13:06:29.088161       1 enqueue.go:55] Enter Enqueue ...
I0124 13:06:29.088181       1 enqueue.go:70] Added Queue <default> for Job <default/test-job>
I0124 13:06:29.088196       1 enqueue.go:87] Try to enqueue PodGroup to 0 Queues
I0124 13:06:29.088226       1 enqueue.go:134] Leaving Enqueue ...
I0124 13:06:29.088243       1 allocate.go:43] Enter Allocate ...
I0124 13:06:29.088261       1 allocate.go:94] Try to allocate resource to 1 Namespaces
I0124 13:06:29.088277       1 allocate.go:147] Try to allocate resource to Jobs in Namespace <default> Queue <default>
I0124 13:06:29.088306       1 allocate.go:172] Try to allocate resource to 6 tasks of Job <default/test-job>
I0124 13:06:29.088326       1 allocate.go:180] There are <6> nodes for Job <default/test-job>
I0124 13:06:29.088373       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-133-65.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 1668.00, memory 10753728512.00, hugepages-1Gi 0.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00>
I0124 13:06:29.088481       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-133-65.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-133-65.ec2.internal fit failed: node(s) had taints that the pod didn't tolerate
I0124 13:06:29.088540       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-136-163.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 468.00, memory 5477285888.00, hugepages-1Gi 0.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00>
I0124 13:06:29.088566       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-136-163.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-136-163.ec2.internal fit failed: node(s) resource fit failed
I0124 13:06:29.088579       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-146-126.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 148.00, memory 4210614272.00, hugepages-1Gi 0.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00>
I0124 13:06:29.088601       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-146-126.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-146-126.ec2.internal fit failed: node(s) resource fit failed
I0124 13:06:29.088613       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-154-53.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 1778.00, memory 11642920960.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00, hugepages-1Gi 0.00>
I0124 13:06:29.088638       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-154-53.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-154-53.ec2.internal fit failed: node(s) had taints that the pod didn't tolerate
I0124 13:06:29.088650       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-163-197.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 1768.00, memory 11540152320.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00, hugepages-1Gi 0.00>
I0124 13:06:29.088673       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-163-197.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-163-197.ec2.internal fit failed: node(s) had taints that the pod didn't tolerate
I0124 13:06:29.088685       1 scheduler_helper.go:87] Considering Task <default/test-job-default-nginx-2> on node <ip-10-0-167-126.ec2.internal>: <cpu 1000.00, memory 0.00> vs. <cpu 58.00, memory 4508401664.00, hugepages-2Mi 0.00, attachable-volumes-aws-ebs 39000.00, hugepages-1Gi 0.00>
I0124 13:06:29.088706       1 scheduler_helper.go:92] Predicates failed for task <default/test-job-default-nginx-2> on node <ip-10-0-167-126.ec2.internal>: task default/test-job-default-nginx-2 on node ip-10-0-167-126.ec2.internal fit failed: node(s) resource fit failed
I0124 13:06:29.088735       1 statement.go:312] Discarding operations ...
I0124 13:06:29.088753       1 allocate.go:147] Try to allocate resource to Jobs in Namespace <default> Queue <default>
I0124 13:06:29.088777       1 allocate.go:241] Leaving Allocate ...
I0124 13:06:29.088792       1 backfill.go:42] Enter Backfill ...
I0124 13:06:29.088805       1 backfill.go:91] Leaving Backfill ...
I0124 13:06:29.100766       1 session.go:154] Close Session 55ab3cf8-3eaa-11ea-a61c-0a580a81020a
```

Posting the entire snippet hear so you can see how the scheduler actually works.
Every time a scheduling cycle occurs, a session is opened (reading plugins and actions).
Then, based on specified configuration individual actions are triggered.
Starting with `Enqueue` action, followed with `Allocate` and `Backfill`.
`Allocate` action is responsible for scheduling job's tasks. As you can see
`default/test-job-default-nginx-2` pod can't be schedule due to insufficient resources
or taints not tolerated.

## What's next

Vulcano does not necessarily focus only on batch scheduling. It also wants to
[incorporate various topologies](https://github.com/volcano-sh/volcano/blob/master/docs/community/roadmap.md) (e.g. GPU) to improve allocation of resources.

It's also likely upstream will want to consume the gang scheduling feature as a [plugin](https://github.com/hex108/coscheduling-plugin)
in the scheduling framework instead of utilizing entire kube-batch code base.

Communities discussing gang scheduling:
- sig-scheduling
- wg-machine-learning
