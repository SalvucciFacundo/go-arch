package scaffold

// UpgradeOption configures optional behavior for Upgrade.
type UpgradeOption func(*upgradeConfig)

// upgradeConfig holds the resolved options for an Upgrade call.
type upgradeConfig struct {
	resolver Resolver
	root     string
}

// WithResolver injects a pack Resolver for re-rendering pack-sourced entries.
// When nil or omitted, pack-sourced entries are not resolved (the entry is
// classified as PROTECTED).
func WithResolver(r Resolver) UpgradeOption {
	return func(uc *upgradeConfig) {
		uc.resolver = r
	}
}

// WithRoot sets the project root used for manifest loading and file operations.
// When omitted, the root is "." (ADR-7: CWD is the project root). Workspace
// commands may pass an explicit root to operate from a monorepo directory.
func WithRoot(root string) UpgradeOption {
	return func(uc *upgradeConfig) {
		uc.root = root
	}
}
