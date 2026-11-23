# Contributing

First of all, thank you for your interest in escli! 🎉

There are many ways to help, and you don't have to write code to make a
difference.

## Help with issues

You can:

* Report a bug you found.
* Suggest a new feature or an improvement.
* Help other people with the questions in their issues.

Before you open an issue, check that it is not already reported.

When you report a bug, include:

* The escli version (`escli version`).
* Your platform, for example macOS on Apple silicon.
* The command you ran, and its output.

> [!WARNING]
> Remove API keys, cluster addresses you don't want to share, and any other
> secrets from the output **before** you post it.

## Development

### Requirements

You need [Go](https://go.dev/dl/). The version is in [`go.mod`](go.mod).

To scan for secrets with `make secrets`, you also need
[gitleaks](https://github.com/gitleaks/gitleaks).

### Get the code

Fork the repository, then clone your fork:

```console
$ git clone https://github.com/<your-username>/escli.git
$ cd escli
```

### Build it

```console
$ make build
```

That gives you `bin/escli`. Try it:

```console
$ bin/escli --help
```

### Check it

Before you open a pull request, run the linter and the tests:

```console
$ make check
```

And scan for secrets:

```console
$ make secrets
```

> [!TIP]
> `make check` runs the tests with the race detector, so it needs a working C
> compiler. `go test ./...` runs them without it.

### All the commands

| Command | What it does |
|---|---|
| `make build` | Builds `bin/escli` for your machine. |
| `make install` | Installs escli into `$(go env GOPATH)/bin`. |
| `make test` | Runs the tests with the race detector. |
| `make lint` | Runs golangci-lint, at the version pinned in the `Makefile`. |
| `make check` | Runs the linter and the tests. |
| `make secrets` | Scans the repository for secrets with gitleaks. |
| `make snapshot` | Builds the release archives for every platform into `dist/` (needs GoReleaser). |

### Project layout

| Path | Contents |
|---|---|
| `cmd/escli` | `main` |
| `internal/cli` | The commands (cobra). |
| `internal/es` | The HTTP client, error messages and API key formats. |
| `internal/endpoint` | Converts a Kibana URL or Cloud ID to an Elasticsearch URL. |
| `internal/config` | Profiles and the configuration file. |
| `internal/secret` | Keychain storage, with a file fallback. |
| `skills/escli` | The skill for AI agents. |

## Pull requests

For a small fix, go ahead and open a pull request. 🚀

For anything bigger, please open an issue first, so that we can agree on the
approach before you spend your time on it.

1. Create a branch from `main`.
2. Make your change, and add tests for it.
3. Run `make check` and `make secrets`.
4. Open a pull request that explains **what** the change does and **why**.

> [!IMPORTANT]
> escli is read-only by design. I won't accept a change that lets a command
> write to Elasticsearch without `--allow-write`.

## Releases

I make the releases. To release, I push a tag:

```console
$ git tag v0.1.0
$ git push origin v0.1.0
```

The release workflow then runs GoReleaser, which builds the binaries and
creates the GitHub release with the archives and checksums.

## License

By contributing, you agree that your contributions are licensed under the
[MIT license](LICENSE).
