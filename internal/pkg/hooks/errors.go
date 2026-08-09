package hooks

// oops error codes used by the hooks package.
// Consumers (cmd/) wrap these with context; the codes are set at source.
const (
	CodeUnknownHookType     = "unknown_hook_type"
	CodeInvalidHookConfig   = "invalid_hook_config"
	CodeHookFailed          = "hook_failed"
	CodeHookTimeout         = "hook_timeout"
	CodeHookCommandNotFound = "hook_command_not_found"
)
