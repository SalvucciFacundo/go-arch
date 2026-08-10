package generators

// oops error codes used by the generators package.
// Defined as plain string constants — this package MUST NOT import packs
// to avoid an import cycle (packs → generators → template).
const (
	CodeUnknownStepType             = "unknown_step_type"
	CodeRecipePathEscape            = "recipe_path_escape"
	CodeUnknownBuiltin              = "unknown_builtin"
	CodeGeneratorStepFailed         = "generator_step_failed"
	CodeGeneratorTemplateNotFound   = "generator_template_not_found"
	CodeGeneratorRunSkippedTrust    = "generator_run_skipped_trust"
	CodeGeneratorPromptUnresolvable = "generator_prompt_unresolvable"
	CodeMissingGeneratorArgument    = "missing_generator_argument"
	CodeUnknownGenerator            = "unknown_generator"
	CodePackNotInstalled            = "pack_not_installed"
	CodeInvalidPackManifest         = "invalid_pack_manifest"
)
