# Update Process

The Cluster Version Operator (CVO) runs in every cluster. CVO is in charge of performing updates to the cluster. It does this primarily by updating the manifests for all of the Second-Level Operators.

The Cluster Version Operator, like all operators, is driven by its corresponding Operator custom resources. This custom resource (i.e. clusterversion object) reports the next available updates considered by the CVO. CVO gets the next available update information from policy engine of OpenShift update service (OSUS). OSUS is part of the cluster version object. This allows the cluster updates to be driven both by the console, OC command line interface and by modifying the clusterversion object manually. Also clusterversion object can modified to direct the CVO to the policy engine API endpoint provided by any OSUS instance.



The series of steps that the Cluster Version Operator follows is detailed below:

1. CVO sleeps for a set duration of time plus some jitter.
2. CVO checks in to the upstream Policy Engine, downloading the latest update graph for the channel to which it’s subscribed.
3. CVO determines the next update(s) in the graph and writes them to the "available updates" field in its Operator custom resource.
    1. If there are no updates available, CVO goes back to step 1.
4. If automatic updates are enabled, CVO writes the newest update into the "desired update" field in its Operator custom resource.
5. CVO waits for the "desired update" field in its Operator custom resource to be set to something other than its current version.
6. CVO instructs the local container runtime to download the image specified in the "desired update" field.
7. CVO validates the digest in the downloaded image and verifies that it was signed by the private half of one of its hard coded keys.
    1. If the image is invalid, it is removed from the local system and CVO goes back to step 1.
8. CVO validates that the downloaded image can be applied to the currently running version by inspecting `release-metadata`.
    1. If the image cannot be applied, it is removed from the local system and CVO goes back to step 1.
9. CVO applies the deployment for itself, triggering Kubernetes to replace CVO with a newer version.
10. CVO applies the remainder of the deployments from the downloaded image, in order, triggering the SLOs to begin updating.
11. CVO waits for all of the SLOs to report that they are in a done state.
12. CVO goes back to step 1.