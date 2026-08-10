package cmd

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// sortedServiceNames returns the workspace service names in declaration order.
func sortedServiceNames(results map[string]error) []string {
	names := make([]string, 0, len(results))
	for n := range results {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// loadServiceConfig snapshots the current viper config state, reloads the
// service's .go-arch.yaml (if present) for the current directory, and returns
// a restore function. Reloads are best-effort: a missing file is not fatal.
//
// The service is expected to be the current working directory (callers chdir
// first). Single-project flows never call this — it is opt-in for workspace
// and --service commands only.
func loadServiceConfig() func() {
	prev := viper.ConfigFileUsed()

	// Reload from the current directory (the service root).
	viper.Reset()
	viper.SetConfigName(".go-arch")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		// Best-effort: no service config, fall through with empty state.
		viper.Reset()
	}

	restore := func() {
		viper.Reset()
		if prev != "" {
			viper.SetConfigFile(prev)
			_ = viper.ReadInConfig()
			return
		}
		viper.SetConfigName(".go-arch")
		viper.AddConfigPath(".")
		_ = viper.ReadInConfig()
	}
	return restore
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// resolveServicePath joins a workspace dir with a service's relative path.
func resolveServicePath(wsDir, svcPath string) string {
	return filepath.Join(wsDir, filepath.FromSlash(svcPath))
}

// addServiceFlag registers the --service flag on a command.
func addServiceFlag(cmd *cobra.Command) {
	cmd.Flags().String("service", "", "service name from the workspace to operate on")
}

// getServiceFlag returns the --service flag value.
func getServiceFlag(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("service")
	return v
}
