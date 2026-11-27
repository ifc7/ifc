package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
	"github.com/ifc7/ifc/pkg/plugins/openapi/generate"
	"github.com/ifc7/ifc/pkg/plugins/runner"
)

// GenerateOptions configures a local SDK generation run.
type GenerateOptions struct {
	// Language is the SDK language. Empty means Go.
	Language contract.SdkLanguage
	// Module is the Go module path. Empty means the code is generated into an existing module:
	// no go.mod is written.
	Module string
	// Package is the Go package clause. Empty derives it from OutDir.
	Package string
	// Version is passed to the generator as the package version. Empty means v0.0.0.
	Version string
	// OutDir is the directory the files are written to.
	OutDir string
}

// GenerateResult is the outcome of a local generation run.
type GenerateResult struct {
	Source   string
	PluginID string
	Package  string
	OutDir   string
	// Written are the paths written, relative to OutDir.
	Written []string
	// Requires are the modules the generated code needs, with their versions.
	Requires map[string]string
	Output   contract.GenerationOutput
}

// GenerateFile runs the default generator on a specification file and writes the result to
// opts.OutDir.
func GenerateFile(path string, opts GenerateOptions) (GenerateResult, error) {
	if opts.OutDir == "" {
		return GenerateResult{}, errors.New("an output directory is required")
	}
	raw, typ, format, err := readSpecFile(path)
	if err != nil {
		return GenerateResult{}, err
	}
	lang := opts.Language
	if lang == "" {
		lang = contract.SdkLanguageGo
	}
	pkg := opts.Package
	if pkg == "" {
		abs, err := filepath.Abs(opts.OutDir)
		if err != nil {
			return GenerateResult{}, err
		}
		pkg = packageNameFor(filepath.Base(abs))
	}
	module := opts.Module
	if module == "" {
		// The generator needs a module path; the go.mod it writes is dropped below.
		module = "local/" + pkg
	}
	version := opts.Version
	if version == "" {
		version = "v0.0.0"
	}
	out, pluginID, err := runner.DefaultRegistry().Generate(contract.GenerationInput{
		InterfaceType: toPluginType(typ),
		Document:      contract.NewSpecificationDocument(raw, format),
		Target:        contract.GenerationTarget{Language: lang},
		Package: contract.GenerationPackage{
			Name:    module,
			Version: version,
			Details: map[string]any{"packageName": pkg},
		},
	})
	if err != nil {
		return GenerateResult{}, err
	}
	res := GenerateResult{Source: path, PluginID: pluginID, Package: pkg, OutDir: opts.OutDir, Output: out}
	if req, ok := out.Extra["requires"].(map[string]string); ok {
		res.Requires = req
	}
	files := out.Files
	if opts.Module == "" {
		files = withoutFile(files, generate.GoModFile)
	}
	if res.Written, err = writeGenerated(opts.OutDir, files); err != nil {
		return res, err
	}
	return res, nil
}

func withoutFile(files []contract.GeneratedFile, path string) []contract.GeneratedFile {
	out := make([]contract.GeneratedFile, 0, len(files))
	for _, f := range files {
		if f.Path != path {
			out = append(out, f)
		}
	}
	return out
}

// writeGenerated writes files under dir, refusing paths that are absolute or leave it.
func writeGenerated(dir string, files []contract.GeneratedFile) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	written := make([]string, 0, len(files))
	for _, f := range files {
		clean := filepath.Clean(filepath.FromSlash(f.Path))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return written, fmt.Errorf("generated path %q leaves the output directory", f.Path)
		}
		dst := filepath.Join(dir, clean)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(dst, []byte(f.Contents), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", dst, err)
		}
		written = append(written, f.Path)
	}
	return written, nil
}

// packageNameFor turns a directory name into a Go package identifier.
func packageNameFor(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "sdk" + s
	}
	return s
}
