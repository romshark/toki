---
suggested: true
---

# Installation

## Homebrew

On macOS, with [Homebrew](https://brew.sh):

```sh
brew install --cask romshark/tools/toki
```

## Go

With Go 1.27.4 or later:

```sh
go install github.com/romshark/toki@latest
```

Or skip the install and run `go run github.com/romshark/toki@latest` in place of `toki`.

## Release binaries

The [releases](https://github.com/romshark/toki/releases) hold binaries for Linux, macOS and Windows.

## The desktop editor

`toki edit` opens the editor as a desktop app in builds with cgo: Homebrew's, the macOS release binaries, and `go install` or `go run` where a C compiler is available. The other builds, such as the release binaries for Linux and Windows, serve the editor over HTTP with [`-server`](../3-reference/1-cli.md#edit).

> [!IMPORTANT]
> On Linux, a build with cgo needs the development packages of GTK 4 and WebKitGTK 6.0, as `libgtk-4-dev` and `libwebkitgtk-6.0-dev` on Debian and Ubuntu. Without them, `go install` and `go run` fail. Build without cgo instead, and serve the editor with `-server`:
>
> ```sh
> CGO_ENABLED=0 go install github.com/romshark/toki@latest
> ```

## Check the installation

```sh
toki version
```
