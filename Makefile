build:
	GOOS=js GOARCH=wasm go build -o static/main.wasm main.go

assets:
	cp "`go env GOROOT`/lib/wasm/wasm_exec.js" ./static

dev: build
	ran -p 8090 -r static

dev-reload:
	watchexec -r -e go -- make dev

test:
	go test -timeout 30s github.com/justmiles/docker-compose-to-nomad/cmd/wasm

deploy:
	git branch -D deployment
	git checkout --orphan deployment
	git reset
	rsync -a static/* .
	git add *.html *.wasm *.js *.css
	git commit -m 'deployment'
	git push -u origin deployment --force
