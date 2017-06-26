SHORT_NAME ?= koli

include versioning.mk

REPO_PATH := kolihub.io/${SHORT_NAME}
DEV_ENV_IMAGE := quay.io/koli/go-dev:v0.3.0
DEV_ENV_WORK_DIR := /go/src/${REPO_PATH}
DEV_ENV_PREFIX := docker run --rm -v ${CURDIR}:${DEV_ENV_WORK_DIR} -w ${DEV_ENV_WORK_DIR}
DEV_ENV_CMD := ${DEV_ENV_PREFIX} ${DEV_ENV_IMAGE}

BINARY_DEST_DIR := rootfs/usr/bin

# # It's necessary to set this because some environments don't link sh -> bash.
SHELL := /bin/bash

# Common flags passed into Go's linker.
GOTEST := go test --race -v
KUBECLIVERSION ?= unknown # glide.yaml
GITCOMMIT ?= $(shell git rev-parse HEAD)
DATE ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := "-s -w \
-X kolihub.io/koli/pkg/version.gitVersion=${VERSION} \
-X kolihub.io/koli/pkg/version.gitCommit=${GITCOMMIT} \
-X kolihub.io/koli/pkg/version.buildDate=${DATE}"

GOOS ?= linux
GOARCH ?= amd64

build-controller:
	mkdir -p ${BINARY_DEST_DIR}
	go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_DIR}/koli-controller cmd/controller/controller-manager.go

build-mutator: clean
	mkdir -p ${BINARY_DEST_DIR}
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_DIR}/k8smutator cmd/mutator/main.go
	
build:
	mkdir -p ${BINARY_DEST_DIR}
	${DEV_ENV_CMD} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_DIR}/koli-controller cmd/controller/controller-manager.go
	${DEV_ENV_CMD} upx -9 ${BINARY_DEST_DIR}/koli-controller

docker-build:
	docker build --rm -t ${IMAGE} rootfs
	docker tag ${IMAGE} ${MUTABLE_IMAGE}

docker-build-mutator:
	docker build -f rootfs/Dockerfile.mutator --rm -t ${IMAGE} rootfs
	docker tag ${IMAGE} ${MUTABLE_IMAGE}


clean:
	rm -f ${BINARY_DEST_DIR}/*

test-unit:
	${GOTEST} ./pkg/...

.PHONY: build docker-build
