# Docker Compose to Nomad

This project provides a tool to convert Docker Compose YAML configurations into Nomad HCL (HashiCorp Configuration Language) job specifications. It is written in Go and compiled to WebAssembly (WASM) to run directly in a web browser.

## [Check it out here.](https://justmiles.github.io/docker-compose-to-nomad)

## How it Works

The Go application parses a Docker Compose YAML file and transforms its services, ports, volumes, environment variables, and other configurations into the equivalent Nomad job, group, and task structures. The WebAssembly module exposes a function that can be called from JavaScript in the browser to perform this conversion.

## Features

The converter currently supports the following Docker Compose elements:

- `version`
- `services`:
  - `image`
  - `ports` (host:container and container only)
  - `environment` (key-value pairs)
  - `volumes` (named volumes, host path bindings, read-only option)
  - `command` (string or list)
  - `entrypoint` (string or list)
  - `restart` (maps to Nomad restart policies: `always`, `unless-stopped`, `on-failure`, `no`)
  - `deploy`:
    - `replicas` (maps to group `count`)
- `volumes` (top-level, basic recognition for context but primary mapping is done per-service)

## Getting Started

### Prerequisites

- Go (for compiling to WASM)
- Make (for using the Makefile targets)
- `ran` (a simple static file server, can be installed with `go install github.com/m3ng9i/ran/cmd/ran@latest`)
- `watchexec` (optional, for auto-reloading on changes, e.g., `cargo install watchexec-cli` or via package manager)

### Building

1.  **Build the WebAssembly binary:**

    ```bash
    make build
    ```

    This compiles `main.go` into `static/main.wasm`.

2.  **Copy WASM JavaScript support file:**
    ```bash
    make assets
    ```
    This copies `wasm_exec.js` from your Go installation to the `static/` directory.

### Running Locally

1.  **Start the development server:**

    ```bash
    make dev
    ```

    This will build the WASM, ensure assets are present, and then serve the `static/` directory on `http://localhost:8090`.

2.  **For auto-reloading on code changes:**
    ```bash
    make dev-reload
    ```
    This uses `watchexec` to monitor Go files and automatically rebuild and restart the server.

Open `http://localhost:8090` in your browser to use the converter.

### Deployment

The `Makefile` includes a `deploy` target:

```bash
make deploy
```

This target clones a separate `deployment` branch from the GitHub repository into a `dist/` directory, copies the built static assets into it, commits, and pushes. This suggests a Git-based deployment workflow, likely to a static hosting service like GitHub Pages.

## Usage in the Browser

The application (`static/index.html`) provides a user interface to paste Docker Compose YAML and get the converted Nomad HCL. The core conversion logic is exposed via a JavaScript function `golangConvertToNomad(yamlString)` which returns a Promise that resolves with the HCL string or rejects with an error.

## Input / Output

- **Input:** Docker Compose YAML string.
- **Output:** Nomad HCL string.

## Limitations

- This tool handles a subset of Docker Compose features. Complex configurations, networks, dependencies beyond basic service definitions, and other advanced options may not be fully translated or supported.
- Error handling for invalid Docker Compose syntax is basic.
- Nomad job specifications can be complex; the generated HCL is a best-effort conversion and may require manual adjustments for specific use cases or advanced Nomad features.
