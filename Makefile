GITCOMMIT := $(shell git rev-parse HEAD)
GITDATE := $(shell git show -s --format='%ct')
BUILDDATE := $(shell date)

LDFLAGSSTRING +=-X main.GitCommit=$(GITCOMMIT)
LDFLAGSSTRING +=-X main.GitDate=$(GITDATE)
LDFLAGSSTRING +=-X main.Meta=$(VERSION_META)
LDFLAGSSTRING +=-X 'main.BuildTime=$(BUILDDATE)'
LDFLAGS := -ldflags "$(LDFLAGSSTRING)"

es-node: build
	cp -r ethstorage/prover/snark_lib build/bin
	mkdir -p build/bin/snarkbuild

build:
	env GO111MODULE=on CGO_ENABLED=1 GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) go build -v $(LDFLAGS) -o build/bin/es-node -tags rapidsnark_asm ./cmd/es-node/

clean:
	rm -r build

test:
	go test -v ./...

lint:
	golangci-lint run -E goimports,sqlclosecheck,bodyclose,asciicheck,misspell,errorlint -e "errors.As" -e "errors.Is"


.PHONY: \
	es-node \
	build \
	clean \
	test \
	lint
