.PHONY: build test run lint

BINARY := bin/hexlet-go-crawler
CMD := ./cmd/hexlet-go-crawler

build:
	@mkdir -p bin
	go build -o $(BINARY) $(CMD)

test: lint
	go test -race -count=1 ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	golangci-lint run ./...

run:
	@./bin/run.sh $(URL)
