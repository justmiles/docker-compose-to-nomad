package main

import (
	_ "crypto/sha512"
	"fmt"
	"syscall/js"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

func main() {
	done := make(chan struct{}, 0)
	js.Global().Set("wasmHash", js.FuncOf(hash))
	<-done
}

func hash(this js.Value, args []js.Value) interface{} {
	var output string
	if len(args) == 0 {
		return output
	}
	input := args[0].String()

	project, err := inputToComposeProject(input)
	if err != nil {
		fmt.Println(err)
		return output
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
