package main

import (
	_ "crypto/sha512"
	"fmt"
	"syscall/js"

	"github.com/justmiles/docker-compose-to-nomad/cmd/converter"
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

	project, err := converter.InputToComposeProject(input)
	if err != nil {
		fmt.Println(err)
		return output
	}

	output = "Services:"
	for _, service := range project.Services {
		fmt.Println(service.Name)
		output = fmt.Sprintf("%s\n- %s", output, service.Name)
	}

	return output
}
