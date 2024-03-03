#! /usr/bin/env bash

# make-kind-cluster.sh: build a kind cluster.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly NODEIMAGE=${NODEIMAGE:-"kindest/node:v1.29.2@sha256:51a1434a5397193442f0be2a297b488b6c919ce8a3931be0ce822606ea5ca245"}
readonly CLUSTERNAME=${CLUSTERNAME:-kvcp-e2e}
readonly WAITTIME=${WAITTIME:-5m}

readonly HERE=$(cd "$(dirname "$0")" && pwd)
readonly REPO=$(cd "${HERE}/../.." && pwd)

kind::cluster::exists() {
    ${KIND} get clusters | grep -q "$1"
}

kind::cluster::create() {
    ${KIND} create cluster \
        --name "${CLUSTERNAME}" \
        --image "${NODEIMAGE}" \
        --wait "${WAITTIME}"
}

kind::cluster::load() {
    ${KIND} load docker-image \
        --name "${CLUSTERNAME}" \
        "$@"
}

if kind::cluster::exists "$CLUSTERNAME" ; then
    echo "cluster $CLUSTERNAME already exists"
    echo exit 2
fi

# Create a fresh kind cluster.
if ! kind::cluster::exists "$CLUSTERNAME" ; then
  kind::cluster::create

  # Print the k8s version for verification.
  ${KUBECTL} version
fi
