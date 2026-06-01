.PHONY: build test run lint

BINARY := bin/hexlet-go-crawler
CMD := ./cmd/hexlet-go-crawler

build:
	@mkdir -p bin
	go build -o $(BINARY) $(CMD)

test: lint
	go test -race -count=1 ./...

GOLANGCI_LINT_VERSION := v2.12.2

lint:
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	golangci-lint run ./...

run:
	@./bin/run.sh $(URL)
