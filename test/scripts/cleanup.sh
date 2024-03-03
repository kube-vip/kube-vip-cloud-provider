#! /usr/bin/env bash

set -o pipefail
set -o errexit
set -o nounset

readonly KIND=${KIND:-kind}
readonly CLUSTERNAME=${CLUSTERNAME:-kvcp-e2e}

kind::cluster::delete() {
    ${KIND} delete cluster --name "${CLUSTERNAME}"
}

# Delete existing kind cluster.
kind::cluster::delete
