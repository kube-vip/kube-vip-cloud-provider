#! /usr/bin/env bash

# make-kind-cluster.sh: build a kind cluster.

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}

readonly NODEIMAGE=${NODEIMAGE:-"kindest/node:v1.33.0@sha256:02f73d6ae3f11ad5d543f16736a2cb2a63a300ad60e81dac22099b0b04784a4e"}
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
