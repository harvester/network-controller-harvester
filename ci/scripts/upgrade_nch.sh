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

pushd $TOP_DIR
# cleanup first
rm -rf harvester-node-disk-manager*
rm -rf ndm-override.yaml

cp -r ../deploy/charts/harvester-node-disk-manager harvester-node-disk-manager
cp ../ci/charts/ndm-override.yaml ndm-override.yaml

target_img=$(yq -e .image.repository ndm-override.yaml)
echo "upgrade target image: ${target_img}, upgrading ..."
$HELM upgrade -f $TOP_DIR/ndm-override.yaml harvester-node-disk-manager harvester-node-disk-manager/ -n harvester-system

sleep 30 # wait 30 seconds for ndm respawn pods

# check image
pod_name=$(kubectl get pods -n harvester-system |grep Running |grep -v webhook |grep ^harvester-node-disk-manager|head -n1 |awk '{print $1}')
container_img=$(kubectl get pods ${pod_name} -n harvester-system -o yaml |yq -e .spec.containers[0].image |tr ":" \n)
yaml_img=$(yq -e .image.repository ndm-override.yaml)
if grep -q ${yaml_img} <<< ${container_img}; then
  echo "Image is equal: ${yaml_img}"
else
  echo "Image is non-equal, container: ${container_img}, yaml file: ${yaml_img}"
  exit 1
fi
echo "harvester-node-disk-manager upgrade successfully!"
popd