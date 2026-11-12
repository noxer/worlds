WEB_OUT ?= build/web

.PHONY: run test web serve

run:
	go run ./cmd/worlds

test:
	go vet ./...
	go test ./...

# Browser build: build/web contains everything a static web server needs.
web:
	mkdir -p $(WEB_OUT)
	GOOS=js GOARCH=wasm go build -o $(WEB_OUT)/worlds.wasm ./cmd/worlds
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WEB_OUT)/
	cp web/index.html $(WEB_OUT)/

serve: web
	python3 -m http.server --directory $(WEB_OUT) 8080
