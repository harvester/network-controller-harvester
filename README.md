Harvester Network Controller
========
[![Build Status](https://drone-publish.rancher.io/api/badges/harvester/network-controller-harvester/status.svg)](https://drone-publish.rancher.io/harvester/network-controller-harvester)
[![Go Report Card](https://goreportcard.com/badge/github.com/harvester/network-controller-harvester)](https://goreportcard.com/report/github.com/harvester/network-controller-harvester)
[![Releases](https://img.shields.io/github/release/harvester/network-controller-harvester/all.svg)](https://github.com/harvester/network-controller-harvester/releases)

A network controller helps to manage the host network configuration of the [Harvester](https://github.com/harvester/harvester) cluster.

## How to deploy
```
$ helm repo add harvester https://charts.harvesterhci.io
$ helm repo update
$ kubectl apply -f https://raw.githubusercontent.com/harvester/network-controller-harvester/master/manifests/dependency_crds
$ helm install harvester-network-controller harvester/harvester-network-controller
```

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
