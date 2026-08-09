package scaffold

// UpgradeOption configures optional behavior for Upgrade.
type UpgradeOption func(*upgradeConfig)

// upgradeConfig holds the resolved options for an Upgrade call.
type upgradeConfig struct {
	resolver Resolver
}

// WithResolver injects a pack Resolver for re-rendering pack-sourced entries.
// When nil or omitted, pack-sourced entries are not resolved (the entry is
// classified as PROTECTED).
func WithResolver(r Resolver) UpgradeOption {
	return func(uc *upgradeConfig) {
		uc.resolver = r
	}
}
