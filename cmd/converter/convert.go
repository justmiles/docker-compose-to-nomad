package converter

import (
	_ "crypto/sha512"
	"fmt"
	"log"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"

	_ "github.com/hashicorp/nomad/jobspec2"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/rodaine/hclencoder"
)

func NomadJobFromComposeProject(project *types.Project) (structs.Job, error) {
	job := structs.Job{
		Datacenters: []string{"dc1", "dc2"},
		Type:        "service",
		TaskGroups:  []*structs.TaskGroup{},
	}

	for _, service := range project.Services {
		fmt.Println(service)
		config := make(map[string]interface{})
		config["image"] = service.Image
		config["volumes"] = service.Volumes
		config["ports"] = service.Ports

		job.TaskGroups = append(job.TaskGroups, &structs.TaskGroup{
			Name:  service.Name,
			Count: service.Scale,
			Tasks: []*structs.Task{
				{
					Name:   service.Name,
					Driver: "docker",
					User:   service.User,
					Config: config,
					// Env: TODO,
					Resources: &structs.Resources{
						CPU:         int(service.CPUS),
						Cores:       int(service.CPUCount),
						MemoryMB:    int(service.MemReservation),
						MemoryMaxMB: int(service.MemLimit),
					},
				},
			},
			Services: []*structs.Service{
				{
					Name:     service.Name,
					TaskName: service.Name,
				},
			},
		})
	}

	return job, nil
}

func ProjectFromString(input string) (*types.Project, error) {
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

type HclJobSpec struct {
	Job structs.Job `hcl:"job"`
}

func chris() {

	test := HclJobSpec{
		Job: structs.Job{
			Datacenters: []string{"dc1", "dc2"},
			Type:        "service",
			TaskGroups: []*structs.TaskGroup{
				{
					Name: "mytask",
				},
			},
		},
	}

	hclstr, err := hclencoder.Encode(test)
	if err != nil {
		log.Fatal("unable to encode: ", err)
	}

	fmt.Println(string(hclstr))

}
