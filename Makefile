SHORT_NAME ?= koli

include versioning.mk

REPO_PATH := kolihub.io/${SHORT_NAME}
DEV_ENV_IMAGE := quay.io/koli/go-dev:v0.3.0
DEV_ENV_WORK_DIR := /go/src/${REPO_PATH}
DEV_ENV_PREFIX := docker run --rm -v ${CURDIR}:${DEV_ENV_WORK_DIR} -w ${DEV_ENV_WORK_DIR}
DEV_ENV_CMD := ${DEV_ENV_PREFIX} ${DEV_ENV_IMAGE}

BINARY_DEST_DIR := rootfs/usr/bin
ROOTFS := rootfs
BINARY_DEST_CONTROLLER_DIR := ${ROOTFS}/controller/usr/bin
BINARY_DEST_GITSTEP_DIR := ${ROOTFS}/gitstep/usr/bin
BINARY_DEST_MUTATOR_DIR := ${ROOTFS}/mutator/usr/bin

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

build-git:
	rm -f ${BINARY_DEST_GITSTEP_DIR}/*
	mkdir -p ${BINARY_DEST_GITSTEP_DIR}
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_GITSTEP_DIR}/gitserver cmd/gitserver/gitserver.go
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_GITSTEP_DIR}/gitreceive cmd/gitreceive/gitreceive.go
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_GITSTEP_DIR}/gitapi cmd/gitapi/gitapi.go

build-controller:
	rm -f ${BINARY_DEST_CONTROLLER_DIR}/*
	mkdir -p ${BINARY_DEST_CONTROLLER_DIR}
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_CONTROLLER_DIR}/koli-controller cmd/controller/controller-manager.go

build-mutator:
	rm -f ${BINARY_DEST_MUTATOR_DIR}/*
	mkdir -p ${BINARY_DEST_MUTATOR_DIR}
	env GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags ${LDFLAGS} -o ${BINARY_DEST_MUTATOR_DIR}/k8smutator cmd/mutator/main.go

build: build-git build-controller build-mutator

docker-build-gitstep:
	docker build -f ${ROOTFS}/gitstep/Dockerfile --rm -t ${KOLI_REGISTRY}${IMAGE_PREFIX}/gitstep:${VERSION} ${ROOTFS}/gitstep

docker-build-mutator:
	docker build -f ${ROOTFS}/mutator/Dockerfile --rm -t ${KOLI_REGISTRY}${IMAGE_PREFIX}/k8s-mutator:${VERSION} ${ROOTFS}/mutator

docker-build-controller:
	docker build -f ${ROOTFS}/controller/Dockerfile --rm -t ${KOLI_REGISTRY}${IMAGE_PREFIX}/koli-controller:${VERSION} ${ROOTFS}/controller

docker-build: docker-build-gitstep docker-build-mutator docker-build-controller

test:
	${GOTEST} ./pkg/...

.PHONY: build docker-build test

