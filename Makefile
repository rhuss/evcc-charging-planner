# Makefile
BINARY_NAME := charging-planner

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
	KO_DOCKER_REPO=ko.local ko build -B --tags latest --platform=all --image-refs ./image-refs.yaml .

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)