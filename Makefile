SHORT_NAME ?= koli

include versioning.mk

REPO_PATH := kolihub.io/${SHORT_NAME}
DEV_ENV_IMAGE := quay.io/koli/go-dev:v0.3.0
DEV_ENV_WORK_DIR := /go/src/${REPO_PATH}
DEV_ENV_PREFIX := docker run --rm -v ${CURDIR}:${DEV_ENV_WORK_DIR} -w ${DEV_ENV_WORK_DIR}
DEV_ENV_CMD := ${DEV_ENV_PREFIX} ${DEV_ENV_IMAGE}

BINARY_DEST_DIR := rootfs/usr/bin

# Common flags passed into Go's linker.
GOTEST := go test --race -v
KUBECLIVERSION ?= unknown # glide.yaml
GITCOMMIT ?= $(shell git rev-parse HEAD)
DATE ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := "-s -w \
-X kolihub.io/koli/pkg/version.gitVersion=${VERSION} \
-X kolihub.io/koli/pkg/version.gitCommit=${GITCOMMIT} \
-X kolihub.io/koli/pkg/version.buildDate=${DATE}"

build-local:
	export SHELL=/bin/sh
	mkdir -p ${BINARY_DEST_DIR}
	go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_DIR}/koli-controller cmd/controller/controller-manager.go
	
build:
	export SHELL=/bin/bash
	mkdir -p ${BINARY_DEST_DIR}
	${DEV_ENV_CMD} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_DIR}/koli-controller cmd/controller/controller-manager.go
	${DEV_ENV_CMD} upx -9 ${BINARY_DEST_DIR}/koli-controller

docker-build:
	docker build --rm -t ${IMAGE} rootfs
	docker tag ${IMAGE} ${MUTABLE_IMAGE}

.PHONY: build docker-build
