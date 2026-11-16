[<img src="docs/logo.svg" alt="IFC7 Logo" width="80">](https://ifc7.dev)

# [ifc7](https://ifc7.dev) cli tool

[![GitHub License](https://img.shields.io/github/license/ifc7/ifc)](LICENSE)
[![ifc7.dev](https://img.shields.io/badge/hub-ifc7.dev-00D27F)](https://ifc7.dev)

One simple tool for all your API and Schema needs.

Publish, discover, and share OpenAPI and JSON Schema interfaces on **[ifc7.dev](https://ifc7.dev)**.

---

# Installation

Prebuilt binaries for Linux and macOS (amd64, arm64) are attached to each [GitHub release](https://github.com/ifc7/ifc/releases):

```bash
version=0.1.0  # without the leading v
curl -sSfL "https://github.com/ifc7/ifc/releases/download/v${version}/ifc_${version}_linux_amd64.tar.gz" | tar -xz ifc
```

Each release also has a `checksums.txt` (sha256) for the archives.

Or, with Go 1.26.5 or later, `go install github.com/ifc7/ifc/cmd/ifc@latest`, or `make install` from a checkout, which builds and copies the `ifc` binary to your `$GOPATH/bin` directory.

### Releasing

Push a `v*` tag (e.g. `git tag v0.1.0 && git push origin v0.1.0`). The Release workflow runs lint and tests, then [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) builds the binaries and publishes the release. Check the config locally with `goreleaser release --snapshot --clean`.

---

# Quickstart

1. At the root of your project run `ifc init`. This will:
   - Create an `ifc.yaml` file defining the interfaces your project cares about
   - Create a `.ifc` folder with `manifest.json` for local interface metadata and revisions

2. Authenticate with the [ifc7.dev](https://ifc7.dev) hub:
   `ifc login`

3. Track a remote interface, for example:
   `ifc use ifc7.dev/i/interface-7/rest-api`

4. Fetch tracked interfaces into the local manifest:
   `ifc fetch`

   Revisions are stored in `.ifc/manifest.json`. For owned interfaces, write working-tree files from the fetched manifest with `ifc checkout`.

5. Own a local spec with `ifc add <path> <name>` (or `ifc scan` to find untracked files), then snapshot and publish:
   `ifc commit && ifc push`

---

# Commands

Use `ifc --help` for detailed explanations of commands.

`ifc init` - Create `ifc.yaml` and `.ifc/manifest.json` in the current directory.

`ifc login` - Authenticate with the [ifc7.dev](https://ifc7.dev) hub.

`ifc use <ref>` - Track a remote hub interface (`ifc7.dev/i/<owner>/<name>` or `interface_<id>`).

`ifc fetch` - Pull tracked interfaces from the hub into `.ifc/manifest.json`. Requires login. `--owned` fetches only owned interfaces with a ref.

`ifc add <path> <name>` - Track a local OpenAPI or JSON Schema file as an owned interface.

`ifc scan [path]` - Find untracked OpenAPI / JSON Schema files and add them as owned interfaces. Optional path limits the search to a subdirectory.

`ifc commit` - Record owned interface files into the local manifest. `--notes` sets the revision notes.

`ifc push [ref]` - Push committed owned interfaces to the hub. `--json` prints a machine-readable report.

`ifc status` - Show whether owned interface files differ from the local manifest.

`ifc diff <name|slug>` - Print a unified diff of an owned interface file vs the latest revision in the local manifest.

`ifc checkout [name|slug]...` - Update owned interface files on disk from the latest revision in the local manifest (use after `ifc fetch`; `--force` overwrites local edits).

`ifc lint [name|path]...` - Lint owned interfaces or specification files with the default linter plugin.

`ifc compare <before> <after>` - Compare two specifications for breaking changes with the default change-detector plugin.

## CI

In CI, `ifc` reads its credentials and hub from the environment and never prompts:

```bash
export IFC_TOKEN=<ifc7 access token>   # used instead of `ifc login`; e.g. an agent token
export IFC_HUB_URL=https://ifc7.dev    # optional; API at <IFC_HUB_URL>/api/v0/

ifc fetch --owned --non-interactive
ifc commit --non-interactive --notes "Describe the change"
ifc push --non-interactive --json > result.json
```

- `.ifc/` can stay gitignored: a missing manifest is treated as empty.
- `--non-interactive` is implied when `CI=true` or stdin is not a terminal. Owned interfaces without a `ref` are skipped with a warning; run `ifc push` interactively once to create them.
- `ifc push --json` prints one report to stdout listing each owned interface with `status` `pushed`, `up_to_date`, `skipped`, `failed` or `not_attempted`. Messages go to stderr.

## Coming Soon:

`ifc gen` - Generate code from an interface.

---

# Built on

ifc is built on these libraries:

- [libopenapi](https://github.com/pb33f/libopenapi) — parse and compare OpenAPI documents
- [vacuum](https://github.com/daveshanley/vacuum) — lint OpenAPI and JSON Schema
- [jsonschema](https://github.com/kaptinlin/jsonschema) — detect and validate JSON Schema

---

# Contributing

Issues and pull requests are welcome.
