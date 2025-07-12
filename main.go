//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go
//go:generate /bin/bash scripts/generate-manifest
//go:generate goimports -w ./pkg/apis
//go:generate goimports -w ./pkg/generated

package main
