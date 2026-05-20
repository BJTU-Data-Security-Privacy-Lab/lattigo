# Tool Usage

Run commands from the repository root unless noted otherwise.

## Activate Go

```powershell
. .\.tools\use-go.ps1
```

## Verify Environment

```powershell
go version
go env GOROOT GOPATH GOBIN GOMODCACHE GOCACHE
```

Expected Go version:

```text
go version go1.25.0 windows/amd64
```

## Common Go Commands

Run a small compile-only check for the RLWE package:

```powershell
go test -mod=readonly ./core/rlwe -run '^$'
```

Run the full test suite:

```powershell
go test -mod=readonly ./...
```

## Temporary Module Proxy

If `proxy.golang.org` is unreachable from the current network, set a temporary proxy only for the current shell session:

```powershell
$env:GOPROXY='https://goproxy.cn,direct'
```

Do not persist this value into project configuration unless explicitly requested.
