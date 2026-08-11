package template

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"

	"github.com/jinzhu/inflection"
	"github.com/samber/oops"
)

// CodeGeneratorTemplateNotFound is returned when RenderPackOnly cannot find a
// template inside the pack's templates/ directory. Defined in the template
// package (not generators) to avoid import cycles: generators → template.
const CodeGeneratorTemplateNotFound = "generator_template_not_found"

// Templates is the embedded FS containing all templates.
//
//go:embed all:templates/*
var TemplatesFS embed.FS

// SourceKind identifies which source provided a resolved template or binary.
type SourceKind int

const (
	SourceLocal    SourceKind = iota // .go-arch/templates/
	SourceGlobal                     // ~/.go-arch/templates/
	SourcePack                       // ~/.go-arch/packs/<name>@<ver>/templates/
	SourceEmbedded                   // embedded FS
)

// ResolvedSource represents a resolved binary file with a source kind and
// a read function. The Read closure unifies disk paths and embedded FS reads
// so callers don't need to know the source.
type ResolvedSource struct {
	Kind SourceKind
	Read func() ([]byte, error)
}

// EngineOption is a functional option for Engine.
type EngineOption func(*Engine)

// WithPacksDir sets the packs installation directory.
func WithPacksDir(dir string) EngineOption {
	return func(e *Engine) {
		e.packsDir = dir
	}
}

// WithPack activates pack-scoped template resolution.
func WithPack(name, version string) EngineOption {
	return func(e *Engine) {
		e.packName = name
		e.packVersion = version
	}
}

type Engine struct {
	fs          embed.FS
	packsDir    string
	packName    string
	packVersion string
}

// NewEngine creates a new template engine. Optional EngineOption functions
// can configure pack directory and active pack. With no options, the engine
// uses the default 3-step chain (local > global > embedded).
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		fs: TemplatesFS,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Render renders a template to wr, printing a notice to stdout when a custom
// (local or global) template override is used. Delegates to RenderTo with
// quiet=false to preserve existing behavior.
func (e *Engine) Render(wr io.Writer, templatePath string, data interface{}) error {
	return e.RenderTo(wr, templatePath, data, false)
}

// RenderTo is like Render but accepts a quiet flag to suppress the
// "Using custom template" print to stdout. Use quiet=true during upgrade
// or MCP calls to avoid corrupting JSON-RPC.
func (e *Engine) RenderTo(wr io.Writer, templatePath string, data interface{}, quiet bool) error {
	t, source, err := e.getTemplate(templatePath)
	if err != nil {
		return err
	}

	if !quiet && source != "embedded" {
		fmt.Fprintf(ui.Out, "🎨 Using custom template (%s): %s\n", source, templatePath)
	}

	return t.Execute(wr, data)
}

func (e *Engine) getTemplate(templatePath string) (*template.Template, string, error) {
	// 1. Local
	localPath := filepath.Join(".go-arch", "templates", templatePath)
	if _, err := os.Stat(localPath); err == nil {
		t, err := template.New(filepath.Base(templatePath)).Funcs(e.getFuncMap()).ParseFiles(localPath)
		return t, "local", err
	}

	// 2. Global
	home, errHome := os.UserHomeDir()
	if errHome == nil {
		globalPath := filepath.Join(home, ".go-arch", "templates", templatePath)
		if _, err := os.Stat(globalPath); err == nil {
			t, err := template.New(filepath.Base(templatePath)).Funcs(e.getFuncMap()).ParseFiles(globalPath)
			return t, "global", err
		}
	}

	// 3. Pack (only when a pack is configured via options)
	if e.packName != "" {
		packPath := filepath.Join(e.packsDir, e.packName+"@"+e.packVersion, "templates", templatePath)
		if _, err := os.Stat(packPath); err == nil {
			t, err := template.New(filepath.Base(templatePath)).Funcs(e.getFuncMap()).ParseFiles(packPath)
			return t, "pack:" + e.packName + "@" + e.packVersion, err
		}
	}

	// 4. Embedded
	embeddedPath := filepath.Join("templates", templatePath)
	t, err := template.New(filepath.Base(templatePath)).Funcs(e.getFuncMap()).ParseFS(e.fs, embeddedPath)
	return t, "embedded", err
}

// ResolveBinary resolves a binary asset path through the same 4-step chain
// as templates (local > global > pack > embedded). Returns a ResolvedSource
// whose Read closure reads the actual bytes from the winning source.
func (e *Engine) ResolveBinary(path string) (ResolvedSource, error) {
	// 1. Local
	localPath := filepath.Join(".go-arch", path)
	if _, err := os.Stat(localPath); err == nil {
		return ResolvedSource{
			Kind: SourceLocal,
			Read: func() ([]byte, error) {
				return os.ReadFile(localPath)
			},
		}, nil
	}

	// 2. Global
	home, errHome := os.UserHomeDir()
	if errHome == nil {
		globalPath := filepath.Join(home, ".go-arch", path)
		if _, err := os.Stat(globalPath); err == nil {
			return ResolvedSource{
				Kind: SourceGlobal,
				Read: func() ([]byte, error) {
					return os.ReadFile(globalPath)
				},
			}, nil
		}
	}

	// 3. Pack
	if e.packName != "" {
		packPath := filepath.Join(e.packsDir, e.packName+"@"+e.packVersion, path)
		if _, err := os.Stat(packPath); err == nil {
			return ResolvedSource{
				Kind: SourcePack,
				Read: func() ([]byte, error) {
					return os.ReadFile(packPath)
				},
			}, nil
		}
	}

	// 4. Embedded
	return ResolvedSource{
		Kind: SourceEmbedded,
		Read: func() ([]byte, error) {
			return e.fs.ReadFile(filepath.Join("templates", path))
		},
	}, nil
}

// RenderPackOnly renders a template sourced exclusively from the pack's
// templates/ directory. Unlike RenderTo, it does NOT fall back to local,
// global, or embedded chains — the pack template is the only valid source.
//
// Returns an error with code CodeGeneratorTemplateNotFound when the
// template does not exist in the pack's templates/ directory.
func (e *Engine) RenderPackOnly(wr io.Writer, packDir, from string, data interface{}) error {
	tmplPath := filepath.Join(packDir, "templates", from)

	info, err := os.Stat(tmplPath)
	if os.IsNotExist(err) {
		return oops.
			Code(CodeGeneratorTemplateNotFound).
			Errorf("generator template not found: %q (in pack templates/)", from)
	}
	if err != nil {
		return oops.
			Code(CodeGeneratorTemplateNotFound).
			Wrapf(err, "cannot access template %q", from)
	}
	if info.IsDir() {
		return oops.
			Code(CodeGeneratorTemplateNotFound).
			Errorf("template path %q is a directory", from)
	}

	tmpl := template.New(filepath.Base(from)).Funcs(e.getFuncMap())
	parsed, err := tmpl.ParseFiles(tmplPath)
	if err != nil {
		return oops.
			Code(CodeGeneratorTemplateNotFound).
			Wrapf(err, "failed to parse template %q", from)
	}
	return parsed.Execute(wr, data)
}

func (e *Engine) getFuncMap() template.FuncMap {
	return template.FuncMap{
		"now": func() string {
			return time.Now().Format("2006-01-02 15:04:05")
		},
		"lower":  strings.ToLower,
		"upper":  strings.ToUpper,
		"plural": inflection.Plural,
		"title": func(s string) string {
			if len(s) == 0 {
				return ""
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
	}
}
