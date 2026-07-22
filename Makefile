.PHONY: build test check docker

build:
	go build -trimpath -o pagedrop ./cmd/pagedrop

test:
	go test -race ./...

check:
	gofmt -w cmd internal
	go vet ./...
	go test -race ./...

docker:
	docker build -t pagedrop:dev .
