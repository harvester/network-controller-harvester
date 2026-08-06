#!/bin/bash -e

COMMIT=$(git rev-parse --short HEAD)
IMAGE=ttl.sh/network-controller-harvester-${COMMIT}
IMAGE_WEBHOOK=ttl.sh/node-disk-manager-webhook-${COMMIT}

yq e -i ".image.repository = \"${IMAGE}\"" ci/charts/nch-override.yaml
yq e -i ".image.tag = \"1h\"" ci/charts/nch-override.yaml 
yq e -i ".webhook.image.repository = \"${IMAGE_WEBHOOK}\"" ci/charts/nch-override.yaml
yq e -i ".webhook.image.tag = \"1h\"" ci/charts/nch-override.yaml 