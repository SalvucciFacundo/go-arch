package cmd

import (
	"fmt"
	"os"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"go-arch/internal/pkg/workspace"
)

// workspaceCmd is the parent command for workspace operations.
var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Run go-arch commands across multiple services in a monorepo",
	Long: `Workspace commands operate on a go-arch.workspace.yaml file at the
monorepo root that maps service names to paths. Services are processed
sequentially; a failing service is reported and the remaining services still
run (exit code is non-zero if any service failed).`,
}

func init() {
	RootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceUpgradeCmd)
	workspaceCmd.PersistentFlags().String("workspace", "", "path to go-arch.workspace.yaml (default: discover upward)")
	workspaceUpgradeCmd.Flags().Bool("yes", false, "apply upgrades (default: dry-run)")
}

// resolveWorkspace returns the workspace for the current command: the explicit
// --workspace flag wins, otherwise upward discovery from CWD.
func resolveWorkspace(flagPath string) (*workspace.Workspace, error) {
	if flagPath != "" {
		return workspace.Load(flagPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, oops.Code("workspace_invalid").Wrapf(err, "cannot resolve working directory")
	}
	path, err := workspace.Discover(cwd)
	if err != nil {
		return nil, err
	}
	return workspace.Load(path)
}

// requireWorkspace resolves the workspace and returns it, or an error that
// explains how to provide one (used by --service flag paths).
func requireWorkspace(flagPath string) (*workspace.Workspace, error) {
	ws, err := resolveWorkspace(flagPath)
	if err != nil {
		return nil, oops.
			Code("workspace_not_found").
			Hint("Pass --workspace <path> or run inside a monorepo with a go-arch.workspace.yaml").
			Wrapf(err, "the --service flag needs workspace context")
	}
	return ws, nil
}

// findService returns the named service or an instructive error.
func findService(ws *workspace.Workspace, name string) (*workspace.Service, error) {
	svc, ok := ws.Find(name)
	if !ok {
		return nil, oops.
			Code("service_not_found").
			Hint("Run 'go-arch workspace upgrade' to see available services").
			Errorf("service %q not found in workspace", name)
	}
	return svc, nil
}

// chdirService changes into the service's resolved directory and returns a
// restore function. The caller MUST defer the restore.
func chdirService(ws *workspace.Workspace, svc *workspace.Service) (func(), error) {
	dir := ws.ResolvePath(svc)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return nil, oops.
			Code("service_path_missing").
			Hint("Check the path declared in go-arch.workspace.yaml").
			Errorf("service %q path %s does not exist", svc.Name, dir)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		return nil, oops.Code("workspace_invalid").Wrapf(err, "cannot resolve working directory")
	}
	if err := os.Chdir(dir); err != nil {
		return nil, oops.Code("workspace_invalid").Wrapf(err, "cannot enter %s", dir)
	}
	return func() { _ = os.Chdir(oldWd) }, nil
}

// withService chdirs into a service, runs fn, and restores CWD + config.
func withService(ws *workspace.Workspace, name string, fn func() error) error {
	svc, err := findService(ws, name)
	if err != nil {
		return err
	}
	restoreWd, err := chdirService(ws, svc)
	if err != nil {
		return err
	}
	defer restoreWd()

	restoreCfg := loadServiceConfig()
	defer restoreCfg()

	return fn()
}

// printServiceSummary prints the per-service outcome and returns whether any failed.
func printServiceSummary(out interface{ Write([]byte) (int, error) }, results map[string]error) bool {
	anyFailed := false
	for _, svc := range sortedServiceNames(results) {
		if err := results[svc]; err != nil {
			anyFailed = true
			fmt.Fprintf(out, "%s %s: %v\n", ansiError(), svc, err)
		} else {
			fmt.Fprintf(out, "%s %s\n", ansiSuccess(), svc)
		}
	}
	return anyFailed
}
