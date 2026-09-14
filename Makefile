.PHONY: help test build clean zip release deploy

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)


FUNCTION_NAME := should-i-work
ZIP_FILE := bootstrap.zip

test: ## test golang code
	go test -v ./...

build: ## build lambda binary (bootstrap)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap .

zip: build ## zip lambda binary for deployment
	zip -j $(ZIP_FILE) bootstrap

clean: ## remove build artifacts
	rm -f bootstrap $(ZIP_FILE)

release: zip ## update lambda function code only (通常のリリースはこちら)
	aws lambda update-function-code \
		--function-name $(FUNCTION_NAME) \
		--zip-file fileb://$(ZIP_FILE)
	rm -f $(ZIP_FILE)

deploy: test release ## test -> build -> release
