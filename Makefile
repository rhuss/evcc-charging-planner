# Makefile
BINARY_NAME := evcc-charging-planner

build: init
	go build -o $(BINARY_NAME) main.go

.PHONY: init
init:
	go mod tidy

.PHONY: test
test:
	go test

.PHONY: container
container:
	KO_DOCKER_REPO=docker.io ko build -B --tags latest --push --platform=all --image-refs ./image-refs.yaml .

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
