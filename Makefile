# Common flags passed into Go's linker.
KUBECLIVERSION ?= unknown # glide.yaml
GITVERSION ?= unknown # git tag -l
GITCOMMIT ?= $(shell git rev-parse HEAD)
DATE ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := "-s -w \
-X kolihub.io/koli/pkg/version.gitVersion=${GITVERSION} \
-X kolihub.io/koli/pkg/version.gitCommit=${GITCOMMIT} \
-X kolihub.io/koli/pkg/version.buildDate=${DATE}"

info:
	@echo "GITVERSION:       ${GITVERSION}"
	@echo "GITCOMMIT:        ${GITCOMMIT}"
	@echo "DATE:             ${DATE}"

build:
	mkdir -p ./build
	go build -ldflags ${LDFLAGS} -o build/koli-controller kolihub.io/koli/cmd/controller

.PHONY: build
