.PHONY: build test check docker

build:
	go build -trimpath -o seol ./cmd/seol

test:
	go test -race ./...

check:
	gofmt -w cmd internal
	go vet ./...
	go test -race ./...

docker:
	docker build -t seol:dev .
