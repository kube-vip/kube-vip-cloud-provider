#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly KIND=${KIND:-kind}

readonly LOAD_PREBUILT_IMAGE=${LOAD_PREBUILT_IMAGE:-false}
readonly CLUSTERNAME=${CLUSTERNAME:-kvcp-e2e}

readonly HERE=$(cd $(dirname $0) && pwd)
readonly REPO=$(cd ${HERE}/../.. && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::load::archive() {
    ${KIND} load image-archive \
        --name "${CLUSTERNAME}" \
        "$@"
}

kind::cluster::load::docker() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if ! kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME does not exist"
    exit 2
fi

if [ "${LOAD_PREBUILT_IMAGE}" = "true" ]; then
    kind::cluster::load::archive "$(ls ${REPO}/image/kube-vip-cloud-provider-*.tar)"
else
    # Build the current version of Kube-vip-cloud-provider.
    VERSION="v$$"
    make -C ${REPO} image-amd64-build-only REPOSITORY=ghcr.io/kube-vip DOCKERTAG=$VERSION

    # Also tag as main since test suites will use this tag unless overridden.
    docker tag ghcr.io/kube-vip/kube-vip-cloud-provider:${VERSION} ghcr.io/kube-vip/kube-vip-cloud-provider:main

    # Push the Kube-vip-cloud-provider build image into the cluster.
    kind::cluster::load::docker ghcr.io/kube-vip/kube-vip-cloud-provider:${VERSION}
    kind::cluster::load::docker ghcr.io/kube-vip/kube-vip-cloud-provider:main
fi
