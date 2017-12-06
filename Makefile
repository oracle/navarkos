PROJECT_NAME := navarkos
PKG_LIST=$(shell go list ./... | grep -v '/vendor/')
VERSION?=$(shell git describe --tags --dirty --always)
LDFLAGS=-X main.Version=${VERSION}
ifndef DOCKER_REGISTRY
  DOCKER_REGISTRY := docker.io
endif
IMAGE_TAG:=${DOCKER_REGISTRY}/${PROJECT_NAME}:${VERSION}

.PHONY: all
all: build-docker

build-local:
	rm -rf bin
	mkdir -p $(dir $@)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/linux/navarkos -installsuffix cgo -ldflags "$(LDFLAGS)" ./cmd/...

build-image: build-local
	docker build . -f Dockerfile -t $(IMAGE_TAG) --build-arg http_proxy=$(http_proxy) --build-arg https_proxy=$(https_proxy) --build-arg no_proxy=$(no_proxy)

build-docker:
	rm -rf bin
	docker build . -f Dockerfile.multistage -t $(IMAGE_TAG) --build-arg ldflags="${LDFLAGS}" --build-arg http_proxy=$(http_proxy) --build-arg https_proxy=$(https_proxy) --build-arg no_proxy=$(no_proxy)

docker-push: build-local
	docker build . -f Dockerfile -t  $(IMAGE_TAG)
	docker push $(IMAGE_TAG)

coverage: ## Generate global code coverage report
	./tools/coverage.sh;

coverhtml: ## Generate global code coverage report in HTML
	./tools/coverage.sh html;

check:
	gofmt -s -l $(shell find . -name '*.go' | grep -v -E '(./vendor)')
	go vet ${PKG_LIST}
	go test -v ${PKG_LIST}

