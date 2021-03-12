# Enhancement Tools

## Configuring

You need to create a `~/.config/ocp-enhancements/config.yml`
containing a [personal access token](https://github.com/settings/tokens):

```yaml
github:
  token: "deadbeefdeadbeefdeadbeefdeadbeef"
```

## Open Enhancement Status Report

From the `tools` directory, run:

```console
go run ./main.go report
```

There are command line options to control the range of time
scanned. Use the `-h` option to see the help.

## Reviewer Stats

From the `tools` directory, run:

```console
go run ./main.go reviewers
```

To see reviewer contributions on PRs in the last 31 days.

There are command line options to control the number of days and which
repository to scan. Use the `-h` option to see the help.

It is common for bot accounts to comment a lot on PRs. To ignore those
comments, use the `--ignore` flag, passing the name of the account to
ignore. The option can be repeated. For example

```console
go run ./main.go reviewers --ignore openshift-ci-robot
```

The list of accounts to ignore can also be included in the
configuration file, like this.

```yaml
github:
  token: "deadbeefdeadbeefdeadbeefdeadbeef"
reviewers:
  ignore:
    - openshift-ci-robot
```
