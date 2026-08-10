package cmd

import (
	"fmt"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/pkg/workspace"
)

// workspaceUpgradeCmd upgrades every service in the workspace in order.
var workspaceUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade all services in the workspace",
	Long: `Upgrades every service declared in go-arch.workspace.yaml, in declaration
order. Each service is processed with the standard upgrade logic (plan,
dry-run by default; --yes applies). A failing service is reported and the
remaining services still run; the command exits non-zero if any failed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		yes, _ := cmd.Flags().GetBool("yes")

		ws, err := resolveWorkspace(wsFlag)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		results := make(map[string]error, len(ws.Services))

		for _, svc := range ws.Services {
			err := upgradeOneService(ws, &svc, yes, out)
			results[svc.Name] = err
		}

		fmt.Fprintln(out)
		fmt.Fprintln(out, "Workspace upgrade summary:")
		anyFailed := printServiceSummary(out, results)
		if anyFailed {
			return oops.Code("workspace_upgrade_failed").Errorf("one or more services failed to upgrade")
		}
		return nil
	},
}

// upgradeOneService runs the full standard upgrade logic for a single service:
// chdir, config reload, Upgrade (with pack resolver), plan display, and apply
// under --yes (with surgical version write).
func upgradeOneService(ws *workspace.Workspace, svc *workspace.Service, yes bool, out interface{ Write([]byte) (int, error) }) error {
	restoreWd, err := chdirService(ws, svc)
	if err != nil {
		return err
	}
	defer restoreWd()
	restoreCfg := loadServiceConfig()
	defer restoreCfg()

	projectName := viper.GetString("project_name")
	if projectName == "" {
		// No config at all: legacy service (no manifest). Report and fall back.
		return oops.
			Code("service_no_manifest").
			Hint("Run 'go-arch setup' inside the service to initialize it").
			Errorf("service %q has no manifest", svc.Name)
	}
	cfg := configFromViper(projectName)

	plan, err := scaffold.Upgrade(cfg,
		scaffold.WithResolver(scaffold.DefaultResolver{}),
		scaffold.WithRoot("."),
	)
	if err != nil {
		return oops.Code("upgrade_failed").Wrapf(err, "service %q upgrade classification failed", svc.Name)
	}

	fmt.Fprintf(out, "%s %s\n", ansiInfo(), svc.Name)
	displayPlan(out, plan)

	if !yes {
		return nil // dry-run by default (batch mode never prompts interactively)
	}

	if plan.CountBy(scaffold.ClassUpgradable) == 0 {
		fmt.Fprintf(out, "%s All files are up to date.\n", ansiSuccess())
		return nil
	}

	if _, err := plan.Apply(); err != nil {
		return oops.Code("upgrade_apply_failed").Wrapf(err, "service %q apply failed", svc.Name)
	}

	// Surgical version write (ADR-4), non-fatal.
	configPath := ".go-arch.yaml"
	if err := scaffold.WriteVersionField(configPath, Version); err != nil {
		fmt.Fprintf(out, "%s Could not update go_arch_version: %v\n", ansiWarning(), err)
	}
	return nil
}
