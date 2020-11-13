---
title: catalog-categories
authors:
  - "@jerolimov"
reviewers:
  - "@spadgett"
  - "@cvogt"
  - "@rohitkrai03"
approvers:
  - "@spadgett"
  - "@cvogt"
creation-date: 2020-10-22
last-updated: 2020-11-06
status: implementable
---

# Customize Developer Catalog Categories

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OpenShift Console provides a "Developer Catalog" which enables the user to create apps and services based on Operator backed services, helm charts, templates, etc.

Cluster admins can provide additional catalog items by installing operators or adding helm repositories. But the filter categories (Languages, Databases, Middleware, CI/CD) are hard coded. This enhancement enable cluster admin to customize these category filters.

## Motivation

Customers who provide their own catalog items would like to customize the category filters such that they better represent their catalog items and make it easier for developers to navigate the catalog.

### Goals

- Add configuration options to allow cluster admins to customize Developer Catalog categories.
- Provide a list of default values to the admin

### Non-Goals

- Hiding categories based on the available catalog items.
- Introducing a UI to configure this categories without a yaml editor.

## Proposal

### Extending the Console CRD

Extend the existing `operator.openshift.io/v1` / `Console` CRD. It provides already a `spec.customization` area where we can add a list of available categories.

Each category could have a list of subcategories which enables the Developer Catalog to build a 2 level tree based on this data:

```
├── Customer Name
│   ├── Project/Team A
│   ├── Project/Team B
│   └── ...
├── Languages
│   ├── Java
│   ├── JavaScript
│   └── ...
├── Databases
├── Middleware
│   ├── Integration
│   ├── Process Automation
│   └── ...
└── ...
```

Each Category and Subcategory conforms to the following schema:

- `id: string`, identifier used in the URL to enable deeplinking in console
- `label: string`, category display label
- `tags: string[]`, the list of item tags this category will match

Categories on the first level contains also a list of subcategories:

- `subcategories:` a list of child categories

If the user selects a top-level category the tags of the top-level category and all subcategories are merged, so that the parent category contains all catalog items of all subcategories.

So the final yaml should look like this one:

```yaml
apiVersion: operator.openshift.io/v1
kind: Console
metadata:
  annotations:
    release.openshift.io/create-only: 'true'
  name: cluster
  ...
spec:
  customization:
    brand: online
    developerCatalog:
      categories:
      - id: languages
        label: Languages
        tags: [go]
        subcategories:
        - id: java
          label: Java
          tags: [java]
        - id: javascript
          tags: [javascript, nodejs, js]
  ...
```

### Provide a code snippet in YAML editor

The admin needs to know the current default categories to customize them. We don't want to provide the default categories in the CR so that we can change these defaults later without migrating the customer data.

To provide the default values close to the editor, the console will provide a code snippet in the sidebar of the YAML editor when editing the `Console` operator config.

### Upgrade process

Keeping the default categories in the console makes upgrades and downgrades easier:

*Upgrade to 4.7:*

- Developer Catalog category customization are not supported before this version and the default categories will be displayed.
- The admin can customize the Developer Catalog categories by editing the `Console` operator config. To start with the latest default categories, the admin needs to insert the "Default Catalog Categories" snippet from the sidebar. Afterwards adding, changing and removing these categories is possible.

*Downgrading to 4.6 or earlier:*

- The Developer Catalog will display the set of hard coded categories.

*Upgrade to 4.8 or later:*

- If there are no Developer Catalog category customizations, the latest default categories will be displayed.
- If there are Developer Catalog category customizations, the upgrade process will not alter the saved configuration.
- To restore the default list of categories the admin needs just to remove the `customization.developerCatalog.categories` entry from the `Console` operator config.
- To check the new defaults, adding one of them to the own customization or removing dropped defaults requires the admin to add the latest code snippet again via the yaml editor.

### User Stories

#### Story 1

As an admin, I want to customize (add, update or remove) some of the Developer Catalog categories. The admin needs to know the current list of categories.

#### Story 2

As a user, I want to use the configured catalog categories in the Developer Catalog.

### Risks and Mitigations

**Risk**: How we handle downgrades and upgrades

**Mitigation**: We keep the default values as part of the console code, so that without customization the console always shows the latest catalog categories. If new categories are added or old defaults are removed, the admin needs to merge the new defaults with their customization manually. See upgrade process above. The benefit of this is that we will not modify an already customized category list.

## Open Questions

- [ ] Is the `Console` operator config the best option, or should we create a new `ConsoleCategory` CRD instead? Pro and con: Operators can provide CRs
- [x] ~~How can we provide a default list. Can we automatically create it when there are currently no `developerCatalog` categories defined? So the admin can restore the default value by removing the current configuration?~~ We will provide a code snippet in the sidebar of the YAML editor to provide the latest default list.

### Test Plan

Testing will be carried out with the usual Console unit and e2e test suites.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

As long as the admin does not customize Developer Catalog categories, the new defaults (static part of console) will be shown automatically of the latest console version.

When there is a customization configured, the Developer Catalog will not be affected by any changes in the default categories.

### Version Skew Strategy

None, console is the only consumer of this configuration.

## Implementation History

None

## Drawbacks

- As soon as the admin customizes the Developer Catalog categories, a cluster update will not add new default categories or drop existing categories.

## Alternatives

- Stay with non-configable static categories.
- Introduce a new CRD.
