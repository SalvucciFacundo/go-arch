package workspace

// oops error codes for the workspace package.
const (
	// CodeWorkspaceNotFound — no workspace file found by flag or discovery.
	CodeWorkspaceNotFound = "workspace_not_found"
	// CodeWorkspaceInvalid — workspace file failed schema/validation.
	CodeWorkspaceInvalid = "workspace_invalid"
	// CodeServiceNotFound — --service named a service not in the workspace.
	CodeServiceNotFound = "service_not_found"
	// CodeServicePathMissing — a service's declared path does not exist on disk.
	CodeServicePathMissing = "service_path_missing"
	// CodeServiceDuplicate — two services share a name.
	CodeServiceDuplicate = "service_duplicate"
	// CodeServiceNoManifest — a service lacks a manifest; legacy fallback applies.
	CodeServiceNoManifest = "service_no_manifest"
)
