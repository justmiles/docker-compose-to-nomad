package converter

import (
	_ "crypto/sha512"
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"
)

func newProject() types.Project {
	return types.Project{
		Services: types.Services{},
		Networks: types.Networks{},
		Volumes:  types.Volumes{},
		Secrets:  types.Secrets{},
		Configs:  types.Configs{},
	}
}

func Test_ProjectFromString(t *testing.T) {

	// setup tests

	// test 1
	input1 := `
    version: "3.9"
    services:
      redis:
        image: "redis:alpine"
  `

	output1 := newProject()
	output1.Services = append(output1.Services, types.ServiceConfig{
		Name:        "redis",
		Image:       "redis:alpine",
		Scale:       1,
		Environment: types.MappingWithEquals{},
	})

	// test2
	input2 := `
  services:
    db:
      # this is a test comment
      image: mariadb:10.6.4-focal
      command: '--default-authentication-plugin=mysql_native_password'
      volumes:
        - db_data:/var/lib/mysql
      restart: always
      environment:
        - MYSQL_ROOT_PASSWORD=somewordpress
        - MYSQL_DATABASE=wordpress
        - MYSQL_USER=wordpress
        - MYSQL_PASSWORD=wordpress
      expose:
        - 3306
        - 33060
    wordpress:
      image: wordpress:latest
      ports:
        - 80:80
      restart: always
      environment:
        - WORDPRESS_DB_HOST=db
        - WORDPRESS_DB_USER=wordpress
        - WORDPRESS_DB_PASSWORD=wordpress
        - WORDPRESS_DB_NAME=wordpress
  volumes:
    db_data:
  `

	output2 := newProject()
	output2.Services = append(output2.Services, types.ServiceConfig{
		Name:  "db",
		Image: "mariadb:10.6.4-focal",
		Scale: 1,
		Environment: types.MappingWithEquals{
			"MYSQL_ROOT_PASSWORD": stringptr("somewordpress"),
			"MYSQL_DATABASE":      stringptr("wordpress"),
			"MYSQL_USER":          stringptr("wordpress"),
			"MYSQL_PASSWORD":      stringptr("wordpress"),
		},
		Command: types.ShellCommand{
			"--default-authentication-plugin=mysql_native_password",
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "volume",
				Source: "db_data",
				Target: "/var/lib/mysql",
				Volume: &types.ServiceVolumeVolume{},
			},
		},
		Restart: "always",
		Expose: types.StringOrNumberList{
			"3306",
			"33060",
		},
	})

	output2.Services = append(output2.Services, types.ServiceConfig{
		Name:  "wordpress",
		Image: "wordpress:latest",
		Scale: 1,
		Environment: types.MappingWithEquals{
			"WORDPRESS_DB_HOST":     stringptr("db"),
			"WORDPRESS_DB_USER":     stringptr("wordpress"),
			"WORDPRESS_DB_PASSWORD": stringptr("wordpress"),
			"WORDPRESS_DB_NAME":     stringptr("wordpress"),
		},
		Restart: "always",
		Ports: []types.ServicePortConfig{
			{
				Mode:      "ingress",
				Target:    80,
				Published: "80",
				Protocol:  "tcp",
			},
		},
	})
	output2.Volumes["db_data"] = types.VolumeConfig{}

	// execute tests
	tests := []struct {
		name    string
		input   string
		want    *types.Project
		wantErr bool
	}{
		{
			name:  "basic compose file",
			input: input1,
			want:  &output1,
		},
		{
			name:  "complex compose file",
			input: input2,
			want:  &output2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProjectFromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProjectFromString() error = %v", err)
				return
			}

			gotJson, err := got.Services.MarshalJSON()
			if err != nil {
				t.Errorf("ProjectFromString(): could not marshal json %v", err)
			}
			wantJson, err := tt.want.Services.MarshalJSON()
			if err != nil {
				t.Errorf("ProjectFromString(): could not marshal json %v", err)
			}

			if !reflect.DeepEqual(gotJson, wantJson) {
				t.Errorf("ProjectFromString() services do not match: = %v, want %v", string(gotJson), string(wantJson))
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProjectFromString() = %v, want %v", string(gotJson), string(wantJson))
			}
		})
	}
}

func stringptr(s string) *string {
	return &s
}
