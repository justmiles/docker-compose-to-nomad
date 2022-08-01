package converter

import (
	_ "crypto/sha512"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"

	_ "github.com/hashicorp/nomad/jobspec2"
	"github.com/rodaine/hclencoder"
)

func NomadJobFromComposeProject(project *types.Project) (Job, error) {
	job := Job{
		Name:        "myjob",
		Datacenters: []string{"dc1", "dc2"},
		Type:        "service",
		TaskGroups:  []*TaskGroup{},
	}

	for _, service := range project.Services {

		config := make(map[string]interface{})
		config["image"] = service.Image
		config["volumes"] = service.Volumes
		config["ports"] = service.Ports

		job.TaskGroups = append(job.TaskGroups, &TaskGroup{
			Name:  service.Name,
			Count: service.Scale,
			Tasks: []*Task{
				{
					Name:   service.Name,
					Driver: "docker",
					User:   service.User,
					Config: config,
					// Env: TODO,
					Resources: &Resources{
						CPU:         int(service.CPUS),
						Cores:       int(service.CPUCount),
						MemoryMB:    int(service.MemReservation),
						MemoryMaxMB: int(service.MemLimit),
					},
				},
			},
			Services: []*Service{
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

func (job *Job) MarshalHCL() (string, error) {
	// type Job struct {
	// 	Stop                     bool                            `hcl:"stop" hcle:"omitempty"`
	// 	Region                   string                          `hcl:"region" hcle:"omitempty"`
	// 	Namespace                string                          `hcle:"omitempty"`
	// 	ID                       string                          `hcle:"omitempty"`
	// 	ParentID                 string                          `hcle:"omitempty"`
	// 	Name                     string                          `hcl:",key" hcle:"omitempty"`
	// 	Type                     string                          `hcl:"type" hcle:"omitempty"`
	// 	Priority                 int                             `hcle:"omitempty"`
	// 	AllAtOnce                bool                            `hcle:"omitempty"`
	// 	Datacenters              []string                        `hcl:"datacenters" hcle:"omitempty"`
	// 	Constraints              []*structs.Constraint           `hcle:"omitempty"`
	// 	Affinities               []*structs.Affinity             `hcle:"omitempty"`
	// 	Spreads                  []*structs.Spread               `hcle:"omitempty"`
	// 	TaskGroups               []*structs.TaskGroup            `hcl:"group,squash" hcle:"omitempty"`
	// 	Update                   structs.UpdateStrategy          `hcle:"omitempty"`
	// 	Multiregion              *structs.Multiregion            `hcle:"omitempty"`
	// 	Periodic                 *structs.PeriodicConfig         `hcle:"omitempty"`
	// 	ParameterizedJob         *structs.ParameterizedJobConfig `hcle:"omitempty"`
	// 	Dispatched               bool                            `hcle:"omitempty"`
	// 	DispatchIdempotencyToken string                          `hcle:"omitempty"`
	// 	Payload                  []byte                          `hcle:"omitempty"`
	// 	Meta                     map[string]string               `hcle:"omitempty"`
	// 	ConsulToken              string                          `hcle:"omitempty"`
	// 	ConsulNamespace          string                          `hcle:"omitempty"`
	// 	VaultToken               string                          `hcle:"omitempty"`
	// 	VaultNamespace           string                          `hcle:"omitempty"`
	// 	NomadTokenID             string                          `hcle:"omitempty"`
	// 	Status                   string                          `hcle:"omitempty"`
	// 	StatusDescription        string                          `hcle:"omitempty"`
	// 	Stable                   bool                            `hcle:"omitempty"`
	// 	Version                  uint64                          `hcle:"omitempty"`
	// 	SubmitTime               int64                           `hcle:"omitempty"`
	// 	CreateIndex              uint64                          `hcle:"omitempty"`
	// 	ModifyIndex              uint64                          `hcle:"omitempty"`
	// 	JobModifyIndex           uint64                          `hcle:"omitempty"`
	// }

	var jobSpec = JobSpec{
		Job: *job,
	}

	hclstr, err := hclencoder.Encode(jobSpec)
	if err != nil {
		return "", err
	}

	return string(hclstr), nil
}

// func JobToHCL(job structs.Job) (string, error) {

// 	test := HclJobSpec{
// 		Job: job,
// 	}

// 	hclstr, err := hclencoder.Encode(test)
// 	if err != nil {
// 		return "", err
// 	}

// 	return string(hclstr), nil
// }
