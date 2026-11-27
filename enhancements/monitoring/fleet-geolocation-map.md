---
title: fleet-geolocation-map
authors:
  - "@sradco"
reviewers:
  - "@jacobbaungard"
  - "@jgbernalp"
approvers:
  - "@jacobbaungard"
  - "@jgbernalp"
api-approvers:
  - "None"
creation-date: 2026-06-15
last-updated: 2026-06-15
tracking-link:
  - https://github.com/perses/perses/issues/4132
see-also:
  - "/enhancements/monitoring/alerts-ui-management.md"
---
# Fleet Geolocation Map

## Summary

This proposal introduces a geographic map visualization for managed clusters on the hub alerting overview page, allowing fleet administrators to see their clusters plotted on a world map as dots — colored by health status (green/yellow/red) and sized by configurable metrics (node count, CPUs, memory, VMs, alert count).

The map view is an alternative view mode alongside the Fleet Health Heatmap, toggled from the same page. Both views show the same clusters and health data but with different representations: the heatmap for density/priority scanning, the map for spatial/geographic context.

It adds a `geolocation` label convention for ManagedCluster resources. The Perses geomap panel resolves location codes to geographic coordinates using a built-in gazetteer of countries and states — no server-side coordinate resolution or additional hub resources required.

This feature complements the Fleet Health Heatmap (planned as part of the multi-cluster alerting overview) by adding a geographic dimension — administrators can see not just *which* clusters have issues, but *where* those clusters are physically located.

## Motivation

Fleet administrators managing clusters across multiple data centers, cloud regions, and edge locations need spatial awareness of their infrastructure. Questions like "are all my European clusters healthy?" or "which region has the most critical alerts?" are currently answered by reading text labels and mentally mapping them to geography.

For daily SRE incident response, the primary tools remain the Fleet Health Heatmap and alert list - these are optimized for scanning severity and prioritizing action. The map view complements them for scenarios where spatial context provides value that a list or grouping cannot:

- **Large edge/distributed fleets** (50–500+ sites): region-grouped lists become unwieldy at scale; a map provides an instant visual inventory.
- **Blast radius assessment**: determining whether an incident is region-wide or isolated to a single site is immediate on a map.
- **Stakeholder and NOC communication**: non-technical stakeholders need to grasp fleet status visually during incidents or planning.
- **Regional pattern detection**: correlated failures across geographically adjacent sites (shared ISP, power grid, weather events) are visible as spatial clusters on a map but non-obvious in a flat list.

### User Stories

1. As a fleet administrator, I want to see all my managed clusters plotted on a world map as dots colored by health status (green/yellow/red) and sized by cluster scale (nodes, CPUs, VMs), so I can identify regional issues and their blast radius at a glance.
2. As a fleet administrator, I want to toggle between the heatmap view and the map view on the alerting overview page, so I can choose the representation that best fits my current task.
3. As a fleet administrator, I want to assign a location to a cluster by selecting a country or state (not typing coordinates), so the setup process is quick and intuitive.
4. As an SRE, I want to set cluster locations via CLI/GitOps using a simple label (e.g., `geolocation=IE`), so it fits my automation workflow.
5. As a fleet administrator, I want clusters in cloud environments to have their location auto-derived from the cloud provider region when possible, so I don't have to manually label every cluster.
6. As a namespace owner, I want to filter the fleet map by my clusters/namespaces, so I can see the geographic distribution of my workloads.
7. As a fleet administrator, I want to change what the dot size represents (nodes, CPUs, memory, VMs, alert count) via a selector, so I can analyze different aspects of my fleet geographically.

### Goals

1. Define a `geolocation` label convention for ManagedCluster resources that is human-friendly and CLI/GitOps-compatible.
2. Deliver a map view panel in the Perses alerting overview page showing clusters as dots on a world map — colored by health status, sized by configurable metrics (nodes, CPUs, memory, VMs, alerts).
3. Provide the map view as an alternative view mode alongside the Fleet Health Heatmap (toggle between heatmap and map).
4. Ensure the Perses geomap panel includes a built-in gazetteer of countries and states for coordinate resolution.
5. Define the UX for a location picker widget in the ACM Console (for interactive use).

### Non-Goals

- Replacing the Fleet Health Heatmap. The geomap is a complementary view mode — heatmap for density/priority, map for spatial context. Users toggle between them.
- Tracking real-time geographic movement of clusters (this is static location assignment).
- Providing sub-building-level precision. Country/state-level granularity is sufficient.
- Server-side coordinate resolution. The panel handles coordinate lookup internally.

## Proposal

### Workflow Description

**Fleet administrator** is a human user responsible for managing clusters across the fleet.

1. The fleet administrator selects a location from the ACM Console cluster import/edit form — a dropdown listing countries and states/provinces.
2. Alternatively, for GitOps workflows, the administrator sets the `geolocation` label directly in the ManagedCluster manifest (e.g., `geolocation: IE`).
3. The `acm_managed_cluster_labels` metric exposes the label.
4. The Perses geomap panel on the alerting overview page renders the cluster as a dot on the world map, colored by health status and sized by the selected metric.
5. The administrator toggles between heatmap and map views to assess fleet health spatially.

For cloud-provisioned clusters (Phase 3), the geolocation is auto-derived from the `topology.kubernetes.io/region` label — no manual action required.

### API Extensions

This enhancement does not introduce any new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers. It uses an existing Kubernetes label on the ManagedCluster resource.

### Design Principles

**Core principle:** The admin provides a location code (country or state); the panel handles coordinate resolution internally. The admin never types or sees latitude/longitude values.

This approach prioritizes:
- **Simplicity:** A single label with a recognizable code value. No additional hub resources required beyond the label itself.
- **Separation of concerns:** The user-facing label is decoupled from the visualization data (coordinates). Coordinate resolution is a panel-internal concern, not a data pipeline concern.
- **Graceful degradation:** Clusters without a location label, or with a label value not recognized by the panel's gazetteer, simply do not appear on the map — no errors, no broken views.

### Architecture

The feature has two layers:

1. **Label layer** — a single `geolocation` label on ManagedCluster resources (user-facing).
2. **Visualization layer** — Perses geomap panel with a built-in gazetteer that resolves location codes to coordinates at render time.

#### Data Flow

```text
ManagedCluster
labels:
  geolocation: "IE"
        │
        ▼
┌───────────────────────────┐
│  acm_managed_cluster_labels  │
│  (existing metric, already   │
│   exposes ManagedCluster     │
│   labels including geolocation) │
└─────────────┬─────────────┘
              │
              ▼
┌───────────────────────────┐
│  Perses Geomap Panel         │
│                              │
│  Built-in gazetteer:         │
│    IE → 53.14, -7.69        │
│    US.VA → 37.43, -78.66    │
│    DE → 51.16, 10.45        │
│    SG → 1.35, 103.82        │
│                              │
│  Resolves geolocation label  │
│  to coordinates at render    │
│  time. Plots dot on map.     │
│                              │
│  - Color: health status      │
│  - Size: selected metric     │
└───────────────────────────┘
```

### Label Convention

A single label on the ManagedCluster resource:

- **Key:** `geolocation`
- **Value:** A location code matching an entry in the panel's built-in gazetteer
- **Supported code types:**
  - Country codes (ISO 3166-1 alpha-2): `IE`, `DE`, `US`, `SG`, `JP`
  - State/province codes (dot-qualified): `US.VA`, `US.CA`, `US.TX`, `DE.HE`
- **Validation:** 1–63 characters, must conform to Kubernetes label value rules (`[a-z0-9A-Z][-a-z0-9A-Z_.]*`)

The label value is resolved by the panel's built-in gazetteer to geographic coordinates at render time. Full location names (e.g., "Ireland", "Virginia, United States") are shown in the tooltip on hover/click.

Multiple clusters can share the same `geolocation` value. They will cluster together on the map (same coordinates), which is correct — they are co-located in the same country/state/region.

#### Relationship with `region` Label

The `geolocation` label is the authoritative source for map placement. The existing `region` label (often set by cloud providers or admins for organizational grouping) is not used directly for map visualization.

However, in Phase 3 (auto-derivation), if `geolocation` is NOT set but a cloud provider region is known, the controller translates the cloud region to the corresponding country/state code (e.g., `us-east-1` → `US.VA`) and sets that as the `geolocation` value. A manually set `geolocation` always takes precedence.

### Visualization Specification

The fleet map is a **view mode toggle** alongside the Fleet Health Heatmap on the alerting overview page. Both views show the same data (clusters + health) but with different representations:
- **Heatmap view:** Grid/treemap — optimized for scanning health at scale, weighted by priority.
- **Map view:** Geographic — optimized for spatial awareness, regional pattern detection, and stakeholder communication.

#### Marker Rendering

Each cluster appears as a **dot (circle marker)** on the map:

| Visual property | Data source | Encoding |
| --------------- | ----------- | -------- |
| **Position** | `geolocation` label resolved by the panel's built-in gazetteer | Dot placement on map |
| **Color** | Cluster health calculated from alerts classification (components and impact layer) | Green = healthy, Yellow = warning, Red = critical |
| **Size** | Configurable metric — selectable from a dropdown | Scaled by: node count, CPU count, memory capacity, VM count, alert count, or pod count |

The size metric is selectable via a dashboard variable or UI control, allowing the user to switch perspectives:
- "Show me clusters sized by node count" — larger dots = bigger clusters
- "Show me clusters sized by VM count" — useful for virtualization-focused teams
- "Show me clusters sized by alert count" — highlights noisy clusters

#### Tooltip / Click-through

Hovering or clicking a cluster marker shows:
- Cluster name
- Full location name (resolved from code, e.g., "Ireland", "Virginia, United States")
- Health status (healthy / warning / critical)
- Current size metric value (e.g., "24 nodes", "128 CPUs", "48 VMs")
- Link to drill down into the cluster's component health view

#### Co-located Clusters

When multiple clusters share the same `geolocation` value, each cluster is rendered as its own individual dot (preserving per-cluster size and color). The dots are offset slightly around the location center point so they remain visually distinguishable without overlapping.

- Each cluster retains its own size (based on the selected metric) and color (based on health status).
- The offset is deterministic (based on cluster index) so dots do not shift between refreshes.
- This allows administrators to immediately identify which specific cluster at a location is largest or most critical — supporting prioritization without requiring interaction.

### Perses Geomap Panel

The primary target for this visualization is the **Perses-based alerting overview page** (the multi-cluster console). Perses does not currently ship a native geomap panel plugin.

#### Upstream Dependency

This feature depends on the Perses project implementing a geomap panel type. An upstream issue has been filed: [perses/perses#4132](https://github.com/perses/perses/issues/4132). The requested capabilities are:

- **Input:** PromQL query returning data with a location code field (the `geolocation` label value) plus value fields for sizing and health.
- **Gazetteer:** Built-in reference of countries and states/provinces with coordinates. The panel resolves location codes to lat/lon internally.
- **Rendering:** Circle markers positioned by resolved coordinates, colored by configurable thresholds, sized by a value field.
- **Interactivity:** Zoom, pan, tooltip on hover, click-through links.
- **Basemap:** Bundled PMTiles world basemap (~50MB, zoom levels 0–6) for offline capability. No external tile server required.
- **Theming:** Respects Perses dark/light mode.
- **Custom locations:** Optionally, allow panel configuration to add custom location entries for sites not in the built-in gazetteer.

The delivery of this feature is gated on the upstream Perses geomap panel being available.

### ACM Console Location Picker (Phase 2)

The cluster import/edit form in the ACM Console adds a location field:

1. A dropdown or autocomplete input listing countries and states/provinces. The list is a static bundled reference (ISO 3166 countries and states/provinces) — the same set of codes recognized by the panel's gazetteer.
2. When the user selects a location:
   - The `geolocation` label is set on the ManagedCluster resource with the corresponding code (e.g., `IE`, `US.VA`).
3. Optional: A small map preview showing the selected location for visual confirmation.

For CLI/GitOps users, setting the label manually (`oc label managedcluster my-cluster geolocation=IE`) works directly — the panel resolves the coordinates.

### Auto-Derivation from Cloud Provider Regions (Phase 3)

For cloud-provisioned clusters, the geolocation can be auto-derived:

1. The MCOA addon (or a hub controller) reads the `topology.kubernetes.io/region` label from spoke nodes (available via `ManagedClusterInfo` status or node metrics).
2. The controller translates the cloud region to the corresponding country/state code (e.g., `us-east-1` → `US.VA`, `eu-west-1` → `IE`).
3. If the ManagedCluster does not already have a `geolocation` label, the controller sets it to the resolved country/state code.

**Precedence:** Manual label always wins. Auto-derivation only fills in clusters without an explicit `geolocation` label.

### Integration with Existing Infrastructure

This feature builds on existing hub infrastructure:

- **`acm_managed_cluster_labels` metric:** Already exposes ManagedCluster labels (including `geolocation` once set). The panel reads this metric directly — no new metric required.
- **Label allowlist:** The `geolocation` label is added to the existing `observability-managed-cluster-label-allowlist` ConfigMap, making it discoverable via the proxy's `acm_label_names` metric and available for dashboard filtering.
- **Fleet Health Heatmap:** The heatmap can optionally group tiles by `geolocation` (nesting clusters by country/region), complementing the map view.

### RBAC and Access Control

This feature inherits the existing RBAC model with no new access control mechanisms required:

- **Metric visibility:** The `acm_managed_cluster_labels` metric is served through the `rbac-query-proxy`, which already filters results by user permissions. Users only see map markers for clusters they have access to.
- **Privacy:** Cluster physical locations are opt-in. No geolocation data is collected or exposed unless an admin explicitly applies the `geolocation` label. Organizations that consider data center locations sensitive can choose not to use this feature.

### Topology Considerations

#### Hypershift / Hosted Control Planes
Hosted clusters inherit the location of their management cluster's data plane nodes unless explicitly overridden. The `geolocation` label on the ManagedCluster resource is independent of control plane placement.

#### Standalone Clusters

Fully supported. The `geolocation` label can be applied to any ManagedCluster regardless of topology.

#### Single-node Deployments or MicroShift

Edge sites and single-node deployments are a primary use case for geographic visualization. Each edge cluster gets its own `geolocation` value representing its physical site or the nearest state/region. No additional resource consumption — the label is evaluated on the hub, not the spoke.

MicroShift clusters registered as ManagedClusters are supported identically.

#### OpenShift Kubernetes Engine

This feature is part of the ACM hub alerting overview. It does not depend on features excluded from OKE — any cluster topology visible to the hub can participate.

#### Map Tiles
The map panel ships with a **bundled world basemap** (~50MB, zoom levels 0–6) using the PMTiles format — a single-file tile archive that MapLibre GL JS reads directly without any external tile server or internet access. This is the default for all environments (connected and air-gapped alike).

- Zero external dependencies. No tile server, no internet access, no configuration required.
- The bundled PMTiles file contains a world basemap at zoom levels 0–6 (continent/country outlines, major labels, water bodies) — sufficient for fleet-level geographic context.
- ~50MB is comparable to a typical Go binary and acceptable for container image size.
- The map works identically in connected and air-gapped environments.

### Implementation Details/Notes/Constraints

Delivery is phased to decouple the data model from the upstream Perses panel availability:

**Phase 1: Data Model Foundation**
- File upstream Perses issue requesting a native geomap panel type with built-in gazetteer (done: [perses/perses#4132](https://github.com/perses/perses/issues/4132)).
- Add `geolocation` to the seeded label allowlist.
- Document the labeling convention (supported codes, examples).

Phase 1 delivers the data model (a single label) independently of the Perses panel. Low initial adoption is expected — this phase establishes the foundation so integration is immediate once the upstream panel is available.

**Phase 2: Perses Map Integration + ACM Console Location Picker**
- Integrate the Perses geomap panel (once available upstream) as a view mode toggle on the alerting overview page.
- Add location selector widget to cluster import/edit form in the ACM Console.
- Add "Clusters needing location" indicator (clusters without `geolocation` label).

**Phase 3: Auto-Derivation**
- Implement controller that reads spoke `topology.kubernetes.io/region` and auto-sets `geolocation`.
- Only fills clusters without an explicit label (manual always wins).

### Risks and Mitigations

- **Low adoption if setup is cumbersome:** Mitigated by requiring only a single label with standard codes. Cloud clusters require zero effort after Phase 3 auto-derivation.
- **Privacy concerns (data center locations):** Mitigated by making the feature entirely opt-in. No geolocation data is collected or exposed unless the admin explicitly labels clusters.
- **Map tile dependency:** Eliminated. The panel bundles a PMTiles world basemap — no external tile server or internet access required in any environment.
- **Upstream Perses panel dependency:** The geomap panel with built-in gazetteer is not yet available in Perses. Delivery timeline is not under our control. Mitigated by filing the upstream issue early with clear requirements.
- **Gazetteer coverage:** Custom on-prem sites may not map cleanly to a country or state code. Mitigated by supporting custom location entries in the panel configuration, and by the ACM Console location picker providing a selection UX.

### Drawbacks

The primary drawback is the upstream dependency on the Perses geomap panel. Until [perses/perses#4132](https://github.com/perses/perses/issues/4132) is implemented, only the data model (Phase 1) can be delivered. The visualization requires the upstream panel to be available.

The bundled PMTiles basemap adds ~50MB to the container image. This is acceptable given that it eliminates all external tile server dependencies and enables air-gapped operation.

## Alternatives (Not Implemented)

- **Grouping/filtering alerts by region instead of a map:** A simpler approach — display clusters in a list or table grouped by region/location. This provides the same "which region has issues?" answer without a map panel. However, it does not scale well for large edge fleets (50–500+ sites), does not communicate blast radius visually, and is less effective for stakeholder/NOC communication. The `geolocation` label enables both approaches: grouping/filtering works immediately in Phase 1, and the map view adds spatial context in Phase 2 for teams that benefit from it.
- **Raw latitude/longitude labels on ManagedCluster:** Rejected. Poor UX — admins don't know data center coordinates.
- **Server-side coordinate resolution via ConfigMap:** Rejected. Adds unnecessary hub-side complexity (ConfigMap lifecycle, operator reconciliation, synthetic metric generation) when the panel can resolve coordinates internally from a built-in gazetteer.
- **City-level granularity:** Not required. Country/state/region level is sufficient for fleet overview and aligns with established map panel patterns.
- **Embedding coordinates directly in the ManagedCluster label value (e.g., `53.35_-6.26`):** Rejected. Unreadable, error-prone, not human-friendly for CLI use or GitOps review.
- **Using the existing `region` label as-is:** Rejected. Cloud provider regions are opaque strings without standardized coordinate resolution. Auto-derivation (Phase 3) translates them to country/state codes instead.

## Test Plan

- **Unit tests:**
  - Label validation: geolocation values conform to Kubernetes label value rules.
  - Auto-derivation controller: correctly sets `geolocation` from topology labels; does not overwrite existing labels.
- **Integration tests:**
  - End-to-end: label a ManagedCluster with `geolocation`, verify the label appears in `acm_managed_cluster_labels` metric.
  - Label allowlist: verify `geolocation` is discoverable via `acm_label_names`.
  - Graceful degradation: clusters without geolocation label are absent from the map without error.
- **E2E / Manual validation:**
  - Perses map view panel renders circle markers at correct positions for clusters with geolocation labels.
  - Health color coding (green/yellow/red) reflects current alert state.
  - Dot size scales correctly with the selected metric (nodes, CPUs, memory, VMs, alerts).
  - Size metric selector switches the scaling in real time.
  - Co-located clusters group at lower zoom levels and separate on zoom-in.
  - Tooltip shows cluster name, full location name, health, and metric value.
  - View mode toggle switches between heatmap and map without data loss.
  - Unrecognized geolocation values (not in gazetteer) are gracefully excluded.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Phase 1 complete: `geolocation` label in allowlist, documented convention.
- Perses geomap panel available upstream and integrated.
- End-to-end map rendering validated for clusters with geolocation labels.
- Gather feedback from fleet administrators on label UX and map usability.

### Tech Preview -> GA

- Phase 2 complete: ACM Console location picker integrated.
- Phase 3 complete: auto-derivation controller for cloud regions.
- Load testing with 1000+ managed clusters on the map.
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

Not applicable. This is a new feature with no deprecation path planned.

## Upgrade / Downgrade Strategy

No upgrade or downgrade concerns. The `geolocation` label is a standard Kubernetes label — it persists across upgrades and is ignored if the map panel is not present. Removing the panel (downgrade) simply removes the map view; the label remains harmless on the ManagedCluster resource.

## Version Skew Strategy

No version skew concerns. The feature involves a label on ManagedCluster (hub-side) and a Perses panel rendering on the hub console. There is no spoke-side component until Phase 3 (auto-derivation controller), which only writes a label — compatible with any spoke version.

## Operational Aspects of API Extensions

This enhancement does not introduce CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers. No operational impact on existing SLIs.

## Support Procedures

- **Feature not working (clusters not appearing on map):** Verify the ManagedCluster has a `geolocation` label with a valid code. Check that the label appears in the `acm_managed_cluster_labels` metric via the Thanos querier.
- **Unrecognized location code:** The panel gracefully excludes clusters with codes not in its gazetteer. Check the label value against the documented list of supported ISO 3166 codes.
- **Disabling the feature:** Remove the `geolocation` label from ManagedClusters. The map view will show no markers. No cluster health impact.
