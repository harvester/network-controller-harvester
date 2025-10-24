Harvester Network Controller
========
[![Build Status](https://drone-publish.rancher.io/api/badges/harvester/network-controller-harvester/status.svg)](https://drone-publish.rancher.io/harvester/network-controller-harvester)
[![Go Report Card](https://goreportcard.com/badge/github.com/harvester/network-controller-harvester)](https://goreportcard.com/report/github.com/harvester/network-controller-harvester)
[![Releases](https://img.shields.io/github/release/harvester/network-controller-harvester/all.svg)](https://github.com/harvester/network-controller-harvester/releases)

A network controller helps to manage the host network configuration of the [Harvester](https://github.com/harvester/harvester) cluster.

## How to Deploy
```
$ helm repo add harvester https://charts.harvesterhci.io
$ helm repo update
$ kubectl apply -f https://raw.githubusercontent.com/harvester/network-controller-harvester/master/manifests/dependency_crds
$ helm install harvester-network-controller harvester/harvester-network-controller
```
:::

Note: The `network-controller-harvester` assumes that it is running in Harvester clusters. The best practice is to [install](https://docs.harvesterhci.io/v1.6/install/index) a Harvester cluster, it rolls out everything automatically.

:::

## How to Contribute

General guide is on [Harvester Developer Guide](https://github.com/harvester/harvester/blob/master/DEVELOPER_GUIDE.md).

### Build

1. Run `make ci` on the source code

```
/go/src/github.com/harvester/network-controller-harvester$ make ci
```

1. A successful run will generate following container images.

```
REPOSITORY                                                                                           TAG                                         IMAGE ID       CREATED         SIZE
rancher/harvester-network-webhook                                                                    98a14e2b-amd64                              a71dc03c7968   41 hours ago    118MB
rancher/harvester-network-helper                                                                     98a14e2b-amd64                              1b64c83eb940   41 hours ago    102MB
rancher/harvester-network-controller                                                                 98a14e2b-amd64                              b5c8d3f6364e   41 hours ago    204MB
```

1. Push or upload the new images to the running cluster, replace them to the deployments and test your change.

### Add new CRDs

Run `make generate` to generate related codes.

```
/go/src/github.com/harvester/network-controller-harvester$ make generate
```

### Chart

The chart definition is managed on a central repo `https://github.com/harvester/charts`. Changes need to be sent to it.

https://github.com/harvester/charts/tree/master/charts/harvester-network-controller

For more information, see [Chart README](https://github.com/harvester/charts/blob/master/README.md).

## License
Copyright (c) 2025 [SUSE, LLC.](https://www.suse.com/)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
