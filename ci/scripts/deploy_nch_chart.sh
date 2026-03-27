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
    echo "check network-controller pods failure."
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

pushd $TOP_DIR

cat >> nch-override.yaml.default << 'EOF'
autoProvisionFilter: [/dev/sd*]
EOF

if [ ! -f nch-override.yaml ]; then
  mv nch-override.yaml.default nch-override.yaml
fi

#$HELM pull harvester-network-controller --repo https://charts.harvesterhci.io --untar
#$HELM install -f $TOP_DIR/nch-override.yaml harvester-network-controller ./harvester-network-controller --create-namespace -n harvester-system

$HELM repo add harvester https://charts.harvesterhci.io
$HELM repo update
kubectl apply -f https://raw.githubusercontent.com/harvester/network-controller-harvester/master/manifests/dependency_crds
$HELM install harvester-network-controller harvester/harvester-network-controller

cluster_nodes=$(yq -e e '.cluster_size' $TOP_DIR/settings.yaml)
echo "cluster nodes: $cluster_nodes"
ensure_network_controller_ready

popd