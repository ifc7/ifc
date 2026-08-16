[<img src="docs/logo.svg" alt="IFC7 Logo" width="80">](https://ifc7.dev)

# [ifc7](https://ifc7.dev) cli tool

[![GitHub License](https://img.shields.io/github/license/ifc7/ifc)](LICENSE)
[![ifc7.dev](https://img.shields.io/badge/hub-ifc7.dev-00D27F)](https://ifc7.dev)

One simple tool for all your API and Schema needs.

Publish, discover, and share OpenAPI and JSON Schema interfaces on **[ifc7.dev](https://ifc7.dev)**.

---

# Installation

With Go 1.26.5 or later installed, `make install` will build and copy the `ifc` binary to your `$GOPATH/bin` directory.

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

`ifc fetch` - Pull tracked interfaces from the hub into `.ifc/manifest.json`. Requires login.

`ifc add <path> <name>` - Track a local OpenAPI or JSON Schema file as an owned interface.

`ifc scan [path]` - Find untracked OpenAPI / JSON Schema files and add them as owned interfaces. Optional path limits the search to a subdirectory.

`ifc commit` - Record owned interface files into the local manifest.

`ifc push [ref]` - Push committed owned interfaces to the hub.

`ifc status` - Show whether owned interface files differ from the local manifest.

`ifc diff <name|slug>` - Print a unified diff of an owned interface file vs the latest revision in the local manifest.

`ifc checkout [name|slug]...` - Update owned interface files on disk from the latest revision in the local manifest (use after `ifc fetch`; `--force` overwrites local edits).

`ifc lint [name|path]...` - Lint owned interfaces or specification files with the default linter plugin.

`ifc compare <before> <after>` - Compare two specifications for breaking changes with the default change-detector plugin.

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
