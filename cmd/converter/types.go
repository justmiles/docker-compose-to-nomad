package converter

type JobSpec struct {
	Job `hcl:"job"`
}

type Job struct {
	Name        string       `hcl:",key" hcle:"omitempty"`
	Type        string       `hcl:"type" hcle:"omitempty"`
	Datacenters []string     `hcl:"datacenters" hcle:"omitempty"`
	TaskGroups  []*TaskGroup `hcl:"group,squash" hcle:"omitempty"`
}

type TaskGroup struct {
	Name     string     `hcl:",key"`
	Count    int        `hcl:"count" hcle:"omitempty"`
	Tasks    []*Task    `hcl:"task,squash" hcle:"omitempty"`
	Services []*Service `hcl:"service,squash" hcle:"omitempty"`

	// EphemeralDisk             *structs.EphemeralDisk            `hcle:"omitempty"`
	// Networks                  structs.Networks                  `hcle:"omitempty"`
	// Volumes                   map[string]*structs.VolumeRequest `hcle:"omitempty"`

}

type Service struct {
	Name     string `hcl:"name,key" hcle:"omitempty"`
	TaskName string `hcl:"task_name" hcle:"omitempty"`
	Port     string `hcl:"port" hcle:"omitempty"`
}

type Task struct {
	Name   string                 `hcl:"name"`
	Driver string                 `hcl:"driver" hcle:"omitempty"`
	User   string                 `hcl:"user" hcle:"omitempty"`
	Config map[string]interface{} `hcl:"config" hcle:"omitempty"`
	// TODO: env
	*Resources `hcl:"resources,squash"`
}

type Resources struct {
	CPU         int `hcl:"cpu" hcle:"omitempty"`
	Cores       int `hcl:"cores" hcle:"omitempty"`
	MemoryMB    int `hcl:"memory" hcle:"omitempty"`
	MemoryMaxMB int `hcl:"memory_max" hcle:"omitempty"`
}
