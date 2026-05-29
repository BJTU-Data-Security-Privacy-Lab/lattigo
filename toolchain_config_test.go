package lattigo

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestToolchainConfigurationIsConsistent(t *testing.T) {
	goMod := readConfigFile(t, "go.mod")

	goVersion := findGoDirective(t, goMod, "go")
	toolchain := findGoDirective(t, goMod, "toolchain")
	if !strings.HasPrefix(toolchain, "go") {
		t.Fatalf("go.mod toolchain directive must use a Go toolchain name, got %q", toolchain)
	}

	toolchainVersion := strings.TrimPrefix(toolchain, "go")
	goLanguageVersion := majorMinorVersion(t, goVersion)
	if toolchainLanguageVersion := majorMinorVersion(t, toolchainVersion); toolchainLanguageVersion != goLanguageVersion {
		t.Fatalf("go.mod toolchain %q should stay on Go language version %q", toolchain, goLanguageVersion)
	}

	makefile := readConfigFile(t, "Makefile")
	requireConfigContains(t, makefile, "Makefile", "staticcheck -go "+goLanguageVersion+" -checks all ./...")
	requireConfigContains(t, makefile, "Makefile", "GO_BIN ?= $(shell go env GOPATH)/bin")
	requireConfigContains(t, makefile, "Makefile", "export PATH := $(GO_BIN):$(PATH)")
	requireConfigContains(t, makefile, "Makefile", "GOIMPORTS_VERSION ?= ")
	requireConfigContains(t, makefile, "Makefile", "STATICCHECK_VERSION ?= ")
	requireConfigContains(t, makefile, "Makefile", "GOVULNCHECK_VERSION ?= ")
	requireConfigContains(t, makefile, "Makefile", "GOSEC_VERSION ?= ")
	if strings.Contains(makefile, "@latest") {
		t.Fatalf("Makefile must pin tool install versions instead of using @latest")
	}

	ciWorkflow := readConfigFile(t, ".github/workflows/ci.yml")
	requireConfigContains(t, ciWorkflow, ".github/workflows/ci.yml", "go-version: '"+toolchainVersion+"'")
	requireConfigContains(t, ciWorkflow, ".github/workflows/ci.yml", `"`+toolchainVersion+`"`)
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(contents)
}

func findGoDirective(t *testing.T, contents, directive string) string {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(directive) + `\s+(\S+)$`)
	match := pattern.FindStringSubmatch(contents)
	if match == nil {
		t.Fatalf("go.mod is missing %q directive", directive)
	}

	return match[1]
}

func majorMinorVersion(t *testing.T, version string) string {
	t.Helper()

	pattern := regexp.MustCompile(`^(\d+\.\d+)(?:\.\d+)?$`)
	match := pattern.FindStringSubmatch(version)
	if match == nil {
		t.Fatalf("cannot parse Go version %q", version)
	}

	return match[1]
}

func requireConfigContains(t *testing.T, contents, path, want string) {
	t.Helper()

	if !strings.Contains(contents, want) {
		t.Fatalf("%s should contain %q", path, want)
	}
}
