build: pkg/generated
	go build

pkg/generated:
	go generate

run: build
	kubectl apply -f manifests/daemonset.yaml
	./harvester-network-controller
