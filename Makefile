GITCOMMIT := $(shell git rev-parse HEAD)
GITDATE := $(shell git show -s --format='%ct')
VERSION := v0.1.1
VERSION_META := dev

LDFLAGSSTRING +=-X main.GitCommit=$(GITCOMMIT)
LDFLAGSSTRING +=-X main.GitDate=$(GITDATE)
LDFLAGSSTRING +=-X main.Version=$(VERSION)
LDFLAGSSTRING +=-X main.Meta=$(VERSION_META)
LDFLAGS := -ldflags "$(LDFLAGSSTRING)"

es-node:
	env GO111MODULE=on GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) go build -v $(LDFLAGS) -o ./cmd/es-node/es-node ./cmd/es-node/

clean:
	rm ./cmd/es-node/es-node

test:
	go test -v ./...

lint:
	golangci-lint run -E goimports,sqlclosecheck,bodyclose,asciicheck,misspell,errorlint -e "errors.As" -e "errors.Is"


.PHONY: \
	es-node \
	clean \
	test \
	lint
