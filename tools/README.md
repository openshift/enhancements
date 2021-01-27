# Enhancement Tools

## Open Enhancement Status Report

From the `tools` directory, run:

```console
go run ./report/main.go
```

There are command line options to control the range of time
scanned. Use the `-h` option to see the help.

You need to create a `~/.config/ocp-enhancements/config.yml`
containing a [personal access token](https://github.com/settings/tokens):

```yaml
github:
  token: "deadbeefdeadbeefdeadbeefdeadbeef"
```
