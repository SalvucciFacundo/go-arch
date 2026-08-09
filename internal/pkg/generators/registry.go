package generators

import "github.com/samber/oops"

// BuiltinFunc is the signature for a builtin generator function.
type BuiltinFunc func(g Generator, args map[string]any) ([]Record, error)

// BuiltinRegistry is a map of builtin generator names → their
// implementation functions. v2.0 ships with an empty registry as a
// scaffold for future builtins.
var BuiltinRegistry = make(map[string]BuiltinFunc)

// Register adds a builtin generator to the global registry.
func Register(name string, fn BuiltinFunc) {
	BuiltinRegistry[name] = fn
}

// Lookup returns the builtin function for name, or an unknown_builtin
// error listing all registered builtin names.
func Lookup(name string) (BuiltinFunc, error) {
	fn, ok := BuiltinRegistry[name]
	if !ok {
		names := make([]string, 0, len(BuiltinRegistry))
		for n := range BuiltinRegistry {
			names = append(names, n)
		}
		return nil, oops.
			Code(CodeUnknownBuiltin).
			Errorf("unknown builtin %q; registered builtins: %v", name, names)
	}
	return fn, nil
}
