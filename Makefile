all: test build

test:
	@echo "Running Go tests..."
	go test ./internal/converter/...

build:
	@echo "Building WASM binary..."
	GOOS=js GOARCH=wasm go build -o static/main.wasm cmd/wasm/main.go

assets:
	cp "`go env GOROOT`/lib/wasm/wasm_exec.js" ./static

dev: build
	ran -p 8090 -r static

dev-reload:
	watchexec -r -e go -- make dev

.ONESHELL:
deploy: assets build
	rm -rf dist
	git clone --no-checkout --branch deployment git@github.com:justmiles/docker-compose-to-nomad.git dist
	cd dist
	rsync -avz --delete ../static/* .
	git add -A
	git commit -a -m 'deployment'
	git push -u origin deployment
