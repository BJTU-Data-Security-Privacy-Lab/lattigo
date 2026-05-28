# Toolchain Usage

本文档固定当前仓库的 Go 工具链使用方式，避免 A0-A6 测试中出现工具使用漂移。除非测试标准文档显式更新，后续测试都必须按本文档激活和验证 Go 环境。

## 1. 固定原则

本仓库使用 repo-local Go 工具链：

```text
.toolchain/go
```

`.toolchain/` 是本地工具链和缓存目录，已经通过 `.git/info/exclude` 排除，不加入 Git 追踪。

测试时不要直接依赖系统 `go`。每个新的 PowerShell 会话都必须先激活本仓库工具链，再执行 `go test`、`go test -json` 或 `go test -bench`。

## 2. 激活命令

在仓库根目录执行：

```powershell
. .\.toolchain\use-go.ps1
```

注意开头是点号加空格。这会把 Go 环境写入当前 PowerShell 会话，而不是启动子进程。

激活脚本设置：

```text
GOROOT      = <repo>\.toolchain\go
GOPATH      = <repo>\.toolchain\gopath
GOCACHE     = <repo>\.toolchain\go-cache
GOTOOLCHAIN = local
GOPROXY     = https://goproxy.cn,direct
GOSUMDB     = sum.golang.google.cn
GOMAXPROCS  = 1
PATH        = <repo>\.toolchain\go\bin;<previous PATH>
```

`GOTOOLCHAIN=local` 用于防止 Go 自动切换或下载其他工具链。`GOMAXPROCS=1` 是 A0-A6 latency/ablation 标准 lane 的固定设置。

## 3. 环境验证

激活后必须至少验证：

```powershell
go version
go env GOVERSION GOOS GOARCH GOMOD GOPATH GOCACHE GOROOT GOPROXY GOSUMDB GOTOOLCHAIN
$env:GOMAXPROCS
```

当前标准期望值：

```text
go version = go1.25.0 windows/amd64
GOVERSION  = go1.25.0
GOOS       = windows
GOARCH     = amd64
GOMOD      = <repo>\go.mod
GOPATH     = <repo>\.toolchain\gopath
GOCACHE    = <repo>\.toolchain\go-cache
GOROOT     = <repo>\.toolchain\go
GOPROXY    = https://goproxy.cn,direct
GOSUMDB    = sum.golang.google.cn
GOTOOLCHAIN= local
GOMAXPROCS = 1
```

如果 `go version` 显示系统 Go，或者 `GOROOT/GOPATH/GOCACHE` 不在 `.toolchain` 下，本次测试环境无效，必须重新激活。

## 4. 标准 quick lane

环境验证通过后，使用以下命令确认本机能够编译并运行 bootstrapping quick correctness lane：

```powershell
go test ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

当前已验证通过的结果形态：

```text
ok github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

后续 A0-A6 测试必须先通过 quick lane，再进入 long lane 或 benchmark lane。

## 5. 常见问题

### 5.1 `go` 命令不可识别

说明当前 shell 没有激活 repo-local toolchain。回到仓库根目录执行：

```powershell
. .\.toolchain\use-go.ps1
```

### 5.2 误用系统 Go

如果 `go version` 或 `go env GOROOT` 指向系统目录，说明 `PATH` 中系统 Go 优先级更高或没有激活脚本。重新执行激活命令，并确认：

```powershell
go env GOROOT
```

输出必须位于 `.toolchain\go`。

### 5.3 `proxy.golang.org` 下载超时

当前本机网络对 `proxy.golang.org` 可能超时。激活脚本已经为当前仓库设置：

```text
GOPROXY=https://goproxy.cn,direct
GOSUMDB=sum.golang.google.cn
```

不要把这个配置写入全局 Go 配置；它只应存在于本仓库激活后的 shell 会话中。

### 5.4 `.toolchain` 出现在 Git 状态中

标准状态中 `.toolchain/` 应显示为 ignored，不应显示为 untracked。验证命令：

```powershell
git check-ignore -v .toolchain .toolchain\go .toolchain\use-go.ps1
```

如果未被 ignore，需要确认 `.git/info/exclude` 中包含：

```text
.toolchain/
```

### 5.5 PowerShell 下 `-bkr.result-dir` 解析失败

A0-A6 专用 harness 使用自定义 Go test flag `-bkr.result-dir`。在 PowerShell
中必须给该 flag 名称加引号：

```powershell
go test ./circuits/ckks/bootstrapping `
  -run '^TestBootstrapKeyReuseA0_P0TinyNativeSingle$' `
  -count=1 `
  -timeout=30m `
  -args '-bkr.result-dir' docs/bootstrap_key_reuse/results/dev_a0_p0
```

不要写成未加引号的 `-bkr.result-dir`，否则 PowerShell 可能把它传成
`-bkr`，Go test 会报 `flag provided but not defined: -bkr`。

## 6. 禁止事项

- 不要把 `.toolchain/` 加入 Git 追踪。
- 不要在 A0-A6 标准测试中直接使用系统 `go`。
- 不要在未记录 `go version` 和 `go env` 的情况下提交测试结果。
- 不要把 throughput lane 的多核结果与 `GOMAXPROCS=1` 的 scalar latency 结果混合比较。
