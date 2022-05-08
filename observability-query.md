---
title: observability-query-language
authors:
  - alanconway
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:https://github.com/grafana/tempo/pull/1378
  - TBD
creation-date: 2022-05-06
last-updated: 2022-05-06
tracking-link:
  - https://issues.redhat.com/browse/LOG-2580
see-also:
replaces:
superseded-by:
---

# Observability Query Language

## Summary

Different observability signal types may be stored in different stores, with different query languages.
In order to correlate different signal types, a "correlation service" must query multiple stores.
This proposal describes a common query language (ObsQL) that can be mapped to the native query language of each store.
This is important to separate the handling of different query languages from the logic of correlating signals.

## Motivation

There are several signal types that we would like to be able correlate including logs, metrics, traces and k8s events.
Each signal type can have a different store, with its own query language. For example:

- Logs: Loki ([LogQL](https://grafana.com/docs/loki/latest/logql/)), Elasticsearch ([ES Query](https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl.html))
- Metrics: Prometheus ([PromQL](https://prometheus.io/docs/prometheus/luerying/basics/))
- Traces: Tempo ([TraceQL](https://github.com/grafana/tempo/pull/1378)), Jaeger ([NoSQL](https://www.jaegertracing.io/docs/1.33/features/#multiple-storage-backends))
- K8s Events: K8s API server ([K8s Selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/))

To correlate different signals, a "correlation service" must query multiple stores for multiple signal sets.

A common query language separates correlation logic from the details of handling multiple query languages.
A translation library from ObsQL to each back-end query language will translate observability queries into native queries.

### User Stories

Some of these stories are for the Openshift developers, this API is important for internal use.

#### The cluster logging team want a common log exploration API for Loki and Elasticsearch

A single API for retrieving logs regardless of the store technology will enable:

- Console views for logs.
- Programmatic access for users to log data.
- Direct access to node log files via the k8s API (like `oc logs`) with a consistent API.

#### As cluster admin, I want command line access to aggregated logs

Currently `oc logs` can only tail one or more pod logs directly.
With a suitable query language, we can extend `oc logs` to do flexible queries on stored aggregated logs.
This will allow admins to write custom scripts to automate complex log grepping tasks.

#### As implementer of a correlation service, I want uniform access to varied signal stores

A common query language will enable access to diverse signal stores.
This is essential to building a future correlation service.

### Goals

- Written specification of the syntax and semantics of the observability query language ObsQL.
- Translation libraries from ObsQL to (at least) LogQL, Elastic Query, K8s selectors.
- Tests for the translation libraries on a representative set of queries.

### Non-Goals

This proposal describes the query language and translation libraries.
The following are separate goals, not discussed here:

- Log exploration API
- Extensions to `oc logs`
- Meta-data translation libraries
- Correlation service

## Proposal

### ObsQL Specification

``` ebnf
letter = "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J" | "K" | "L" | "M" | "N"
       | "O" | "P" | "Q" | "R" | "S" | "T" | "U" | "V" | "W" | "X" | "Y" | "Z" | "a" | "b"
       | "c" | "d" | "e" | "f" | "g" | "h" | "i" | "j" | "k" | "l" | "m" | "n" | "o" | "p"
       | "q" | "r" | "s" | "t" | "u" | "v" | "w" | "x" | "y" | "z" ;
digit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
string = '"' { unicode character } '"'
number =

FIXME HERE https://en.wikipedia.org/wiki/Extended_Backus%E2%80%93Naur_form

name = ( letter | "_" ), { letter | digit | "_" } ;
value = string | number
operator = "=" | "!=" | "=~" | "!~" |
predicate = name , operator , value | "not", predicate | "(" , expression , ")"

exp→term {OR term};

term→predicate {AND predicate};

predicate→LPAREN exp RPAREN;

query = term , { "," , term }

```

 2

Here is an EBNF grammar with precedence of operators enforced.

We support multiple value types which are automatically inferred from the query input.

    String is double quoted or backticked such as "200" or `us-central1`.
    Duration is a sequence of decimal numbers, each with optional fraction and a unit suffix, such as “300ms”, “1.5h” or “2h45m”. Valid time units are “ns”, “us” (or “µs”), “ms”, “s”, “m”, “h”.
    Number are floating-point number (64bits), such as250, 89.923.
    Bytes is a sequence of decimal numbers, each with optional fraction and a unit suffix, such as “42MB”, “1.5Kib” or “20b”. Valid bytes units are “b”, “kib”, “kb”, “mib”, “mb”, “gib”, “gb”, “tib”, “tb”, “pib”, “pb”, “eib”, “eb”.

NOTE:
- choice (enum, k8s in) via regexp
- regexp syntax: golang https://github.com/google/re2/wiki/Syntax
- no booleans, all terms are ANDed. For OR issue multiple queries (?)
  - loki has OR in filter exprs - AND of OR

TODO
- review inclusion of ":" vs "."
- what about utf8 labels?
- is this any use for metrics?

### LogQL translation
NOTE:
- PromQL and LogQL distinguish "label" from "field name". ObsQL does not, and calls them both "name"
### Elastic Query translation
### K8s Selector translation

### API Extensions

None.

### Risks and Mitigations

TODO

### Drawbacks

- duplication of existing query languages
- similar but different to existing query languages, may cause confusion
- may be hard to balance portability with usability:
  - a 'least common denominator' QL may lack flexibility
  - a 'fully featured' QL may be difficult to translate to some back-end QLs

## Design Details

### Open Questions

TODO

### Test Plan

Collect some common queries against existing stores.
Invent some complex queries to exercise the translators.

Automated tests should
- translate to native QL
- execute a query against test data (using native APIs)
- validate the correct data set is returned (using native APIs)

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

TODO

### Operational Aspects of API Extensions

None

## Implementation History

None

## Alternatives

None known.

Any attempt to correlate signal types must include code to query multiple stores.
This seems like the best way to factor that code and simplify the correlation logic.

