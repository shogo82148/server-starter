help: ## Show this text.
	# https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: all
all: build

.PHONY: build
build:
	mkdir -p dist
	go build -o dist/start_server ./cmd/start_server/main.go

.PHONY: test
test: ## Run test.
	go test -v -race ./...
	go vet ./...

.PHONY: clean
clean: ## Remove built files.
	-rm -rf dist
