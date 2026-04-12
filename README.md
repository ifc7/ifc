![IFC7 Logo](docs/logo.svg)
# [Ifc]

---
![GitHub License](https://img.shields.io/github/license/ifc7/ifc)


One simple tool for all your API and Schema needs.

---

# Installation

With Go 1.26 installed, `make install` will build and copy the `ifc` binary to your `$GOPATH/bin` directory.

---

# Quickstart

1. At the root of your project run `ifc init --interactive`. This will:
   - Create an `ifc.yaml` file defining the interfaces your project cares about
   - Create a `.ifc` folder for storing local copies of interfaces
   - Add the appropriate lines to your `.gitignore` file, if needed
   - Scan your repo for interfaces to manage

2. Use an existing interface in your project, for example:
`ifc use ifc7.dev/orgs/ifc7/restapi`

3. Now fetch the latest copies of interfaces you are using:
`ifc fetch`
You will have a local copy of the external interface you are using in `.ifc/ifc7.dev/orgs/ifc7/restapi/latest.yaml`

4. If you have an account at `ifc7.dev`, you can push local copies of interfaces you own to the hub:
`ifc login && ifc push`

---

# Commands

Use `ifc --help` for detailed explanations of commands.

`ifc init` - Initialize your repository with an `ifc.yaml` file.

`ifc login` - Get credentials for the `ifc7.dev` interface hub.

`ifc fetch` - Fetch copies of external interfaces you are tracking from a remote hub.

`ifc commit` - Commit changes to local interface manifest.

`ifc push` - Push the latest copies of interfaces managed by this project to a remote hub.

`ifc use` - Use an externally owned interface in your project.

`ifc add` - Add a locally owned interface to your project.

## Coming Soon:

`ifc compare` - Compare revisions of an interface for breaking changes.

`ifc gen` - Generate code from an interface.

---

# Contributing

