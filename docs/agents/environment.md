# Environment and Toolchain

This project uses a repository-local portable Go toolchain.

## Go

- Version: `go1.25.0 windows/amd64`
- Toolchain root: `.tools/go1.25.0`
- Activation script: `.tools/use-go.ps1`

The activation script configures the current PowerShell session with project-local Go paths:

```text
GOROOT=.tools/go1.25.0
GOPATH=.tools/gopath
GOBIN=.tools/gobin
GOMODCACHE=.tools/gomodcache
GOCACHE=.tools/gocache
```

The `.tools/` directory stores the local toolchain, downloaded archive, module cache, and build cache.
Treat it as local environment state rather than source content.
