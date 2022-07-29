package main

import (
	_ "crypto/sha512"
	"fmt"
	"syscall/js"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

const testyaml = `
version: "3.9"
services:
  web:
    build: .
    ports:
      - "8000:5000"
  redis:
    image: "redis:alpine"
`

func main() {
	done := make(chan struct{}, 0)
	js.Global().Set("wasmHash", js.FuncOf(hash))
	<-done
}

func hash(this js.Value, args []js.Value) interface{} {
	var output string

	// TODO: get from multi-line args[0].String() instead of hardcoded above
	project, err := inputToComposeProject(testyaml)
	if err != nil {
		fmt.Println(err)
	}

	output = "Services:"
	for _, service := range project.Services {
		fmt.Println(service.Name)
		output = fmt.Sprintf("%s<br />- %s", output, service.Name)
	}

	return output
}

func inputToComposeProject(input string) (*types.Project, error) {
	configDetails := types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			types.ConfigFile{
				Content: []byte(input),
			},
		},
	}

	loadOption := func(options *loader.Options) {
		options.SkipNormalization = true
	}

	return loader.Load(configDetails, loadOption)
}
