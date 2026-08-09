package ui

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
)

type ProjectConfig struct {
	ProjectName          string `mapstructure:"project_name"`
	ModuleName           string `mapstructure:"module_name"`
	Architecture         string `mapstructure:"architecture"`
	DBDriver             string `mapstructure:"db_driver"`
	UseDocker            bool   `mapstructure:"use_docker"`
	UseObservability     bool   `mapstructure:"use_observability"`
	ObservabilityBackend string `mapstructure:"observability_backend"`
	UseGRPC              bool   `mapstructure:"use_grpc"`
	UseTemplHTMX         bool   `mapstructure:"use_templ_htmx"`
	Template             string `mapstructure:"template,omitempty"`
	GoArchVersion        string `mapstructure:"go_arch_version"`
}

func RunWizard() (*ProjectConfig, error) {
	fmt.Println("🚀 Welcome to the Go-Arch wizard")

	config := &ProjectConfig{}

	var mainQs = []*survey.Question{
		{
			Name: "ProjectName",
			Prompt: &survey.Input{
				Message: "Project name:",
				Default: "my-go-app",
			},
			Validate: survey.Required,
		},
		{
			Name: "ModuleName",
			Prompt: &survey.Input{
				Message: "Go module name:",
				Default: "github.com/user/app",
			},
		},
		{
			Name: "Architecture",
			Prompt: &survey.Select{
				Message: "Select the architecture:",
				Options: []string{"Minimalist", "Standard", "Hexagonal"},
				Default: "Standard",
			},
		},
		{
			Name: "DBDriver",
			Prompt: &survey.Select{
				Message: "Select the database driver:",
				Options: []string{"PostgreSQL", "MySQL", "MongoDB", "None"},
				Default: "None",
			},
		},
		{
			Name: "UseDocker",
			Prompt: &survey.Confirm{
				Message: "Include Docker configuration?",
				Default: true,
			},
		},
		{
			Name: "UseObservability",
			Prompt: &survey.Confirm{
				Message: "Enable Telemetry/Observability (OpenTelemetry)?",
				Default: false,
			},
		},
		{
			Name: "UseGRPC",
			Prompt: &survey.Confirm{
				Message: "Enable a gRPC server for Microservices?",
				Default: false,
			},
		},
		{
			Name: "UseTemplHTMX",
			Prompt: &survey.Confirm{
				Message: "Include templ + HTMX frontend?",
				Default: false,
			},
		},
	}

	err := survey.Ask(mainQs, config)
	if err != nil {
		return nil, err
	}

	if config.UseObservability {
		var obsQ = &survey.Select{
			Message: "Select the visualization tool:",
			Options: []string{"Console", "Jaeger", "Zipkin", "Prometheus", "SigNoz"},
			Default: "Console",
		}
		err = survey.AskOne(obsQ, &config.ObservabilityBackend)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}
