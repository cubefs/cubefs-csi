#!/usr/bin/env bash

# /*
# Copyright 2025 The Kubernetes Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#     http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# */

set -o errexit
set -o nounset
set -o pipefail

# shellcheck disable=SC2155
export CWD=$(pwd)
function cleanup {
    exit_code=$?
    if [ "$ARTIFACTS" != "" ] && [ "$exit_code" -ne 0 ]
    then
        collect_k8s_logs
        if [ "$USE_EXISTING_CUBEFS_CLUSTER" == 'false' ]
        then
            collect_cubefs_logs
        fi
    fi

    $KUBECTL delete -f "$CWD/deploy/storageclass-external.yaml" || true
    
    if [ "$USE_EXISTING_KUBE_CLUSTER" == 'false' ]
    then
        $KIND delete cluster --name "$KIND_CLUSTER_NAME"
    fi

    if [ "$USE_EXISTING_CUBEFS_CLUSTER" == 'false' ]
    then
        docker ps --filter "label=com.docker.compose.project" --format "{{.ID}}" | xargs docker stop
        docker network rm docker_extnetwork
        sudo rm -rf "$CWD/cubefs"
    fi
}
function startup {
    if [ "$USE_EXISTING_CUBEFS_CLUSTER" == 'false' ]
    then
        git clone https://github.com/cubefs/cubefs.git && cd cubefs
        git checkout -b "$CUBEFS_RELEASE_BRANCH" "remotes/origin/$CUBEFS_RELEASE_BRANCH" -f
        new_tag=$(git describe --tags --abbrev=0)
        git checkout -b "$new_tag" "tags/$new_tag" -f
        docker/run_docker.sh --build
        docker/run_docker.sh --monitor
        docker/run_docker.sh --server
        cd ..
    fi

    if [ "$USE_EXISTING_KUBE_CLUSTER" == 'false' ]
    then
        $KIND create cluster --name "$KIND_CLUSTER_NAME" --image "$E2E_KIND_NODE_VERSION" --config ./hack/kind-config.yaml
        if [ "$USE_EXISTING_CUBEFS_CLUSTER" == 'false' ]
        then
            # allow cubefs csi driver to access the cubeFS cluster which is deployed via docker-compose.
            # See https://github.com/cubefs/cubefs/blob/7a116351bfc950d0e01484a2a59f28beb202786c/docker/docker-compose.yml#L59
            docker network connect docker_extnetwork "$KIND_CLUSTER_NAME-control-plane"
        fi
        # allow csi driver to run on all nodes
        $KUBECTL label nodes --all component.cubefs.io/csi=enabled
    fi
}
function kind_load {
    $KIND load docker-image "$IMAGE_TAG" --name "$KIND_CLUSTER_NAME"
}
function deploy {    
    $KUBECTL apply -f "$CWD/deploy/csi-rbac.yaml"
    $YQ eval "(.spec.template.spec.containers[] | select(.name == \"cfs-driver\")).image = \"$IMAGE_TAG\"" "$CWD/deploy/csi-controller-deployment.yaml" | $KUBECTL apply -f -
    $YQ eval "(.spec.template.spec.containers[] | select(.name == \"cfs-driver\")).image = \"$IMAGE_TAG\"" "$CWD/deploy/csi-node-daemonset.yaml" | $KUBECTL apply -f -
}
function run_test {
    $KUBECTL apply -f "$CWD/deploy/storageclass-external.yaml"

    $GINKGO -p -focus='External.Storage' \
       -skip='\[Feature:|\[Disruptive\]|\[Serial\]' \
       "$KUBE_E2E_BINARY" \
       -- \
       -storage.testdriver="$CWD/hack/cubefs-testdriver.yaml" \
       -kubeconfig="$HOME/.kube/config"
}
function collect_k8s_logs {
    $KUBECTL get pod -n cubefs -o json | $JQ -r '.items[] | "\(.metadata.name) \(.spec.nodeName)"' | while read pod node; do
        echo "Processing pod: $pod on node: $node"
        mkdir -p "$$ARTIFACTS/$node/$pod"
        $KUBECTL cp -c cfs-driver "cubefs/${pod}:/cfs/." "$ARTIFACTS/$node/$pod" || echo "Failed to copy from $pod, skipping"
    done
}

function collect_cubefs_logs {
    containers=$(docker ps --filter "label=com.docker.compose.project" --format "{{.Names}}")
    if [ -z "$containers" ]; then
        echo "No running docker-compose containers found."
        return
    fi

    for container in $containers; do
        echo "Processing container: $container"

        dest="$ARTIFACTS/cubefs/$container"
        mkdir -p "$dest"

        echo "Copying from $container:/cfs to $dest"
        mkdir -p "$dest/log"
        if ! docker cp "$container:/cfs/log" "$dest/log"; then
            echo "❌ Failed to copy from $container, skipping..."
        else
            echo "✅ Successfully log copied from $container"
        fi
        mkdir -p "$dest/conf"
        if ! docker cp "$container:/cfs/conf" "$dest/conf"; then
            echo "❌ Failed to copy from $container, skipping..."
        else
            echo "✅ Successfully conf copied from $container"
        fi
    done

    echo "All containers processed. Artifacts saved under $ARTIFACTS" 
}

trap cleanup EXIT
startup
kind_load
deploy
run_test