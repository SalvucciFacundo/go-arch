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

	"github.com/jinzhu/inflection"
)

// Templates is the embedded FS containing all templates.
//
//go:embed all:templates/*
var TemplatesFS embed.FS

type Engine struct {
	fs embed.FS
}

func NewEngine() *Engine {
	return &Engine{
		fs: TemplatesFS,
	}
}

func (e *Engine) Render(wr io.Writer, templatePath string, data interface{}) error {
	t, source, err := e.getTemplate(templatePath)
	if err != nil {
		return err
	}

	if source != "embedded" {
		fmt.Printf("🎨 Using custom template (%s): %s\n", source, templatePath)
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

	// 3. Embedded
	embeddedPath := filepath.Join("templates", templatePath)
	t, err := template.New(filepath.Base(templatePath)).Funcs(e.getFuncMap()).ParseFS(e.fs, embeddedPath)
	return t, "embedded", err
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
