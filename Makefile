.PHONY: build test lint docker-build docker-push help

## build: compile the init-container binary
build:
	cd init-container && go build ./...

## test: run all unit tests
test:
	cd init-container && go test -v -race -count=1 ./...

## lint: run golangci-lint
lint:
	cd init-container && golangci-lint run

## docker-build: build the init-container Docker image (dev tag)
docker-build:
	docker build -t platform-k8s-db-migrator/init-container:dev init-container/

## docker-push: push the init-container image to Docker Hub (requires DOCKERHUB_USERNAME and DOCKERHUB_TOKEN env vars)
docker-push:
	@echo "Ensure DOCKERHUB_USERNAME and DOCKERHUB_TOKEN are set and you are logged in (docker login)."
	docker push hashvelani/platform-k8s-db-migrator:dev

## help: list all targets with descriptions
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
