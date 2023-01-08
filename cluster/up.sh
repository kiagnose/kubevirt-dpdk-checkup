#!/bin/bash

set -ex

export KUBEVIRT_NUM_NODES=3
export KUBEVIRT_DEPLOY_CDI="false"
export DEPLOY_KUBEVIRT=${DEPLOY_KUBEVIRT:-true}

source ./cluster/cluster.sh

cluster::install

$(cluster::path)/cluster-up/up.sh

if [[ "${DEPLOY_KUBEVIRT}" = "true" ]]; then
	echo "Deploy KubeVirt latest stable release"
	./hack/deploy-kubevirt.sh
fi