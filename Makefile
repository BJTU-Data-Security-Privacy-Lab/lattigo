.DEFAULT_GOAL := test

GO_BIN ?= $(shell go env GOPATH)/bin
export PATH := $(GO_BIN):$(PATH)

GOIMPORTS_VERSION ?= v0.45.0
STATICCHECK_VERSION ?= 2025.1.1
GOVULNCHECK_VERSION ?= v1.3.0
GOSEC_VERSION ?= v2.26.1

.PHONY: test_gotest
test_gotest:
	go clean -testcache
	go test -timeout=0 ./...

.PHONY: checks
checks: check_tools
	@echo Checking correct formatting of files
	
	@FMTOUT=$$(go fmt ./...); \
	if [ -z $$FMTOUT ]; then\
        echo "go fmt: OK";\
	else \
		echo "go fmt: problems in files:";\
		echo $$FMTOUT;\
		false;\
    fi

	@if GOVETOUT=$$(go vet ./... 2>&1); then\
        echo "go vet: OK";\
	else \
		echo "go vet: problems in files:";\
		echo "$$GOVETOUT";\
		false;\
    fi

	@if GOIMPORTSOUT=$$(goimports -l . 2>&1); then\
		if [ -z "$$GOIMPORTSOUT" ]; then\
        echo "goimports: OK";\
		else \
			echo "goimports: problems in files:";\
			echo "$$GOIMPORTSOUT";\
			false;\
		fi;\
	else \
		echo "goimports: problems in files:";\
		echo "$$GOIMPORTSOUT";\
		false;\
	fi
	
	@if STATICCHECKOUT=$$(staticcheck -go 1.25 -checks all ./... 2>&1); then\
		if [ -z "$$STATICCHECKOUT" ]; then\
        echo "staticcheck: OK";\
		else \
			echo "staticcheck: problems in files:";\
			echo "$$STATICCHECKOUT";\
			false;\
		fi;\
	else \
		echo "staticcheck: problems in files:";\
		echo "$$STATICCHECKOUT";\
		false;\
	fi

	@if GOVULNCHECKOUT=$$(govulncheck ./... 2>&1); then\
		if echo "$$GOVULNCHECKOUT" | grep -q "No vulnerabilities found"; then\
			echo "govulncheck: OK";\
		else \
			echo "govulncheck:" >&2;\
			echo "$$GOVULNCHECKOUT" >&2;\
			false;\
		fi;\
	else \
		echo "govulncheck:" >&2;\
		echo "$$GOVULNCHECKOUT" >&2;\
		false;\
	fi

	@if GOSECOUT=$$(gosec -quiet -exclude=G602 ./... 2>&1); then\
		if [ -z "$$GOSECOUT" ]; then\
		echo "gosec: OK";\
		else \
			echo "gosec: problems in files:";\
			echo "$$GOSECOUT";\
			false;\
		fi;\
	else \
		echo "gosec: problems in files:";\
		echo "$$GOSECOUT";\
		false;\
	fi
	
	@echo Checking all local changes are committed
	go mod tidy
	out=`git status --porcelain`; echo "$$out"; [ -z "$$out" ]

.PHONY: test
test: test_gotest

.PHONY: ci_test
ci_test: checks test_gotest

EXECUTABLES = goimports staticcheck govulncheck gosec
.PHONY: get_tools
get_tools:
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

.PHONY: check_tools
check_tools:
	@missing=0; \
	for exec in $(EXECUTABLES); do \
		if ! command -v $$exec >/dev/null 2>&1; then \
			echo "\"$$exec not found in PATH, consider running \`make get_tools\`.\"" >&2; \
			missing=1; \
		fi; \
	done; \
	[ $$missing -eq 0 ]
