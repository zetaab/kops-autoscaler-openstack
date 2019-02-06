BINARY_NAME := kops-autoscaler-openstack
IMAGE := jesseh/$(BINARY_NAME)
.PHONY: test build_linux_amd64 build build-image

test:
	golint -set_exit_status pkg/...
	golint -set_exit_status cmd/...
	./.gofmt.sh

build_linux_amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -v -i -o $(BINARY_NAME) ./cmd

build:
	rm -rf bin/$(BINARY_NAME)
	go build -v -i -o bin/$(BINARY_NAME) ./cmd

build-image:
	rm -rf bin/linux/
	mkdir -p bin/linux
	GOOS=linux GOARCH=amd64 go build -v -i -o bin/linux/$(BINARY_NAME) ./cmd
	docker build -t $(IMAGE):latest .
