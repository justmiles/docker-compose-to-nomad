package converter

import (
	_ "crypto/sha512"
	"fmt"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

func InputToComposeProject(input string) (*types.Project, error) {
	fmt.Println(input)
	configDetails := types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			{
				Content: []byte(input),
			},
		},
	}

	loadOption := func(options *loader.Options) {
		options.SkipNormalization = true
	}

	project, err := loader.Load(configDetails, loadOption)
	if err != nil {
		return nil, err
	}

	for i, service := range project.Services {
		if service.Scale == 0 {
			project.Services[i].Scale = 1
		}
	}

	return project, nil
}
