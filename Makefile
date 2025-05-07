build:
	GOOS=js GOARCH=wasm go build -o static/main.wasm main.go

assets:
	cp "`go env GOROOT`/lib/wasm/wasm_exec.js" ./static

dev: build
	ran -p 8090 -r static

dev-reload:
	watchexec -r -e go -- make dev

deploy: assets build
  git clone --single-branch --branch deployment git@github.com:justmiles/docker-compose-to-nomad.git dist
	cd dist
	rsync -avz --delete ../static/* .
	git commit -am 'deployment'
	git push -u origin deployment
