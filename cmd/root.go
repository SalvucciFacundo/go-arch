package cmd

import (
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"os"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var RootCmd = &cobra.Command{
	Use:           "go-arch",
	Short:         "A CLI tool to scaffold Go projects with clean architecture",
	Long:          `go-arch is a CLI library for Go that empowers developers to create projects with hexagonal or minimalist architecture.`,
	SilenceUsage:  true, // Samber Standard: We handle error display
	SilenceErrors: true, // Samber Standard: Avoid duplicate error logs
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		// Wrap error with context and report through UI
		ui.Fatal(oops.
			Code("root_execution_failed").
			Hint("Check your flags or config file").
			Wrapf(err, "Failed to execute the main command"))
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.go-arch.yaml)")

	// Bind persistent flags to viper
	_ = viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			ui.Fatal(oops.
				Code("home_dir_failed").
				Wrapf(err, "Could not find the user home directory"))
		}
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		ui.Success(fmt.Sprintf("Using config file: %s", viper.ConfigFileUsed()))
	}
}
