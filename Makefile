.PHONY: build test lint docker-build docker-push help

## build: compile the init-container binary
build:
	cd init-container && go build ./...

## test: run all unit tests
test:
	cd init-container && go test ./...

## lint: run golangci-lint
lint:
	cd init-container && golangci-lint run

## docker-build: build the init-container Docker image (dev tag)
docker-build:
	docker build -t platform-k8s-db-migrator/init-container:dev init-container/

## docker-push: push the init-container image to ECR (requires AWS auth)
docker-push:
	@echo "Ensure AWS credentials are configured and ECR_REGISTRY is set."
	docker push $(ECR_REGISTRY)/platform-k8s-db-migrator/init-container:dev

## help: list all targets with descriptions
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
