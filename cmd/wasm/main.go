//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/justmiles/docker-compose-to-nomad/internal/converter"
)

// convert wraps the internal converter's ConvertToNomadHCL function for JS interop.
// It takes a YAML string input and returns a Promise that resolves with the HCL string
// or rejects with an error.
func convert(this js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		// It's good practice to return a JS Error object
		errorConstructor := js.Global().Get("Error")
		return errorConstructor.New("Invalid number of arguments. Expected 1 (yamlInput string).")
	}
	yamlInput := args[0].String()

	// Create a Promise
	handler := js.FuncOf(func(this js.Value, pArgs []js.Value) interface{} {
		resolve := pArgs[0]
		reject := pArgs[1]

		go func() {
			hclOutput, err := converter.ConvertToNomadHCL(yamlInput)
			if err != nil {
				errorConstructor := js.Global().Get("Error")
				// It's important to pass a JS Error object to reject
				errorObject := errorConstructor.New(err.Error())
				reject.Invoke(errorObject)
			} else {
				resolve.Invoke(hclOutput)
			}
		}()
		return nil // Required for js.FuncOf if the Go function doesn't return a value for the JS caller
	})

	promiseConstructor := js.Global().Get("Promise")
	return promiseConstructor.New(handler)
}

func main() {
	c := make(chan struct{}, 0)
	fmt.Println("Go WASM Initialized (compose2nomad)")
	js.Global().Set("convertToNomad", js.FuncOf(convert))
	<-c // Keep the program alive
}