# Toolchain

## Rule

`go.mod` defines the Go language and toolchain version. The `Makefile` defines
the pinned auxiliary tools required for build verification and prepends the Go
bin directory to `PATH` so those tools are discoverable after installation.

## Why

Unpinned tool installs such as `@latest` can silently select newer toolchains
and produce different results across local runs, CI, and future agent sessions.
Missing `$(go env GOPATH)/bin` in `PATH` can also make installed tools invisible
to `make checks`.

## Required Tools

Install with:

```bash
make get_tools
```

The Makefile currently installs:

- `goimports` from `golang.org/x/tools/cmd/goimports`
- `staticcheck` from `honnef.co/go/tools/cmd/staticcheck`
- `govulncheck` from `golang.org/x/vuln/cmd/govulncheck`
- `gosec` from `github.com/securego/gosec/v2/cmd/gosec`

Version pins live in the Makefile variables:

- `GOIMPORTS_VERSION`
- `STATICCHECK_VERSION`
- `GOVULNCHECK_VERSION`
- `GOSEC_VERSION`

## How To Verify

Use writable Go and tool caches when sandbox defaults are read-only:

```bash
GOCACHE=/tmp/lattigo-gocache XDG_CACHE_HOME=/tmp/lattigo-xdg-cache go test . -run TestToolchainConfigurationIsConsistent -count=1
make check_tools
GOCACHE=/tmp/lattigo-gocache XDG_CACHE_HOME=/tmp/lattigo-xdg-cache make checks
```

If `make checks` fails because a tool is missing, run `make get_tools` and then
retry `make checks`.

## Applies To

- `go.mod`
- `Makefile`
- `.github/workflows/ci.yml`
- local and CI build/check workflows
