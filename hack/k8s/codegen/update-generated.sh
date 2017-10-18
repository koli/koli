
#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

DOCKER_REPO_ROOT="/go/src/kolihub.io/koli"
IMAGE=${IMAGE:-"gcr.io/coreos-k8s-scale-testing/codegen"}

docker run --rm \
  -v "$PWD":"$DOCKER_REPO_ROOT" \
  -w "$DOCKER_REPO_ROOT" \
  "$IMAGE" \
  "./hack/k8s/codegen/codegen.sh" \
  "deepcopy-gen" \
  "kolihub.io/koli/pkg/generated" \
  "kolihub.io/koli/pkg/apis" \
  "core:v1alpha1" \
  --go-header-file "./hack/k8s/codegen/boilerplate.go.txt" \
  $@
