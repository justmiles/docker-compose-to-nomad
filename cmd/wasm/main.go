package main

import (
	_ "crypto/sha512"
	"fmt"
	"log"
	"syscall/js"

	"github.com/justmiles/docker-compose-to-nomad/cmd/converter"
	"github.com/rodaine/hclencoder"
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

	project, err := converter.ProjectFromString(input)
	if err != nil {
		fmt.Println(err)
		return output
	}

	job, err := converter.NomadJobFromComposeProject(project)
	if err != nil {
		fmt.Println(err)
		return output
	}

	hclstr, err := hclencoder.Encode(job)
	if err != nil {
		log.Fatal("unable to encode: ", err)
	}

	return hclstr
}
