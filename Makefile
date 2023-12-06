GITCOMMIT := $(shell git rev-parse HEAD)
GITDATE := $(shell git show -s --format='%ct')
BUILDDATE := $(shell date)

LDFLAGSSTRING +=-X main.GitCommit=$(GITCOMMIT)
LDFLAGSSTRING +=-X main.GitDate=$(GITDATE)
LDFLAGSSTRING +=-X main.Meta=$(VERSION_META)
LDFLAGSSTRING +=-X 'main.BuildTime=$(BUILDDATE)'
LDFLAGS := -ldflags "$(LDFLAGSSTRING)"

es-node:
	env GO111MODULE=on GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) go build -v $(LDFLAGS) -o build/bin/es-node ./cmd/es-node/
	cp -r ethstorage/prover/snarkjs build/bin
	mkdir -p build/bin/snarkbuild

clean:
	rm build -r

test:
	go test -v ./...

lint:
	golangci-lint run -E goimports,sqlclosecheck,bodyclose,asciicheck,misspell,errorlint -e "errors.As" -e "errors.Is"


.PHONY: \
	es-node \
	clean \
	test \
	lint
