#!/bin/bash -e

TOP_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/" &> /dev/null && pwd )"

ensure_command() {
  local cmd=$1
  if ! which $cmd &> /dev/null; then
    echo 1
    return
  fi
  echo 0
}

ensure_network_controller_ready() {
  while [ true ]; do
    running_num=$(kubectl get pods -n harvester-system |grep ^network-controller |grep Running |awk '{print $3}' |wc -l)
    if [[ $running_num -eq ${cluster_nodes} ]]; then
      echo "network-controller pods are ready!"
      break
    fi
    echo "check network-controller failure."
    exit 1
  done
}

if [ ! -f $TOP_DIR/kubeconfig ]; then
  echo "kubeconfig does not exist. Please create cluster first."
  echo "Maybe try new_cluster.sh"
  exit 1
fi
echo $TOP_DIR/kubeconfig
export KUBECONFIG=$TOP_DIR/kubeconfig

if [[ $(ensure_command helm) -eq 1 ]]; then
  echo "no helm, try to curl..."
  curl -O https://get.helm.sh/helm-v3.9.4-linux-amd64.tar.gz
  tar -zxvf helm-v3.9.4-linux-amd64.tar.gz
  HELM=$TOP_DIR/linux-amd64/helm
  $HELM version
else
  echo "Get helm, version info as below"
  HELM=$(which helm)
  $HELM version
fi

cluster_nodes=$(yq -e e '.cluster_size' $TOP_DIR/settings.yaml)
echo "cluster nodes: $cluster_nodes"
ensure_network_controller_ready

pushd $TOP_DIR
cat >> nch-override.yaml.default << 'EOF'
autoProvisionFilter: [/dev/sd*]
EOF

if [ ! -f nch-override.yaml ]; then
  mv nch-override.yaml.default nch-override.yaml
fi

cp -r ../deploy/charts/harvester-node-disk-manager harvester-network-cntroller

target_img=$(yq -e .image.repository nch-override.yaml)
echo "install target image: ${target_img}"
$HELM install -f $TOP_DIR/nch-override.yaml harvester-network-cntroller ./harvester-network-cntroller --create-namespace -n harvester-system

popd