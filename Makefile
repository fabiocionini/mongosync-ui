# mongosync-ui — build automation
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help web build run dev release clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

web: ## Build the React web UI
	cd web && npm install --no-audit --no-fund && npm run build

build: web ## Build a binary for the current platform into ./dist
	mkdir -p dist
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/mongosync-ui ./cmd/mongosync-ui

run: build ## Build and run mongosync-ui
	./dist/mongosync-ui

dev: ## Run the Go server (UI must be built; use web/ npm run dev for live UI)
	go run ./cmd/mongosync-ui --open=false

release: ## Cross-compile release binaries for all platforms
	VERSION=$(VERSION) ./build.sh

clean: ## Remove build artifacts
	rm -rf dist web/dist/assets
	git checkout -- web/dist/index.html 2>/dev/null || true
