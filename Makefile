# Makefile
BINARY_NAME := evcc-charging-planner
IMAGE := "docker.io/rhuss/evcc-charging-planner:latest"

build: init
	go build -o $(BINARY_NAME) main.go

.PHONY: init
init:
	go mod tidy

.PHONY: test
test:
	go test

.PHONY: image
image:
	-podman manifest rm $(IMAGE)
	podman manifest create --amend $(IMAGE)
	podman build --platform "linux/amd64" --manifest $(IMAGE) .
	podman build --platform "linux/arm64/v8" --manifest $(IMAGE) .
	podman manifest push $(IMAGE)

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
