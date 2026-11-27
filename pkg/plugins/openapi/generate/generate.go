// Package generate implements the default OpenAPI code-generation plugin. It
// wraps github.com/ifc7/oapi-codegen to produce a Go package with models, a
// strict net/http server and a client.
package generate

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ifc7/oapi-codegen/v2/pkg/codegen"
	"github.com/ifc7/oapi-codegen/v2/pkg/engine"
	"github.com/ifc7/oapi-codegen/v2/pkg/engine/libopenapi"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// PluginID identifies SDKs produced by this plugin.
const PluginID = "openapi-go@v1"

// DefaultGoVersion is the go directive written when package.details.goVersion is unset.
const DefaultGoVersion = "1.24"

// Generated file names, one per oapi-codegen generator.
const (
	TypesFile  = "types.gen.go"
	ServerFile = "server.gen.go"
	ClientFile = "client.gen.go"
	GoModFile  = "go.mod"
)

// ErrExternalRef is returned (wrapped) when the document references another document.
// Following external references is not supported yet.
var ErrExternalRef = errors.New("external $ref is not supported")

// requiredModules pins the modules generated code may import, by module path.
var requiredModules = map[string]string{
	"github.com/oapi-codegen/runtime":  "v1.8.0",
	"github.com/google/uuid":           "v1.6.0",
	"github.com/oapi-codegen/nullable": "v1.1.0",
}

// codegenMu serializes oapi-codegen, which keeps package-level state while generating.
var codegenMu sync.Mutex

var goIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Plugin is the default OpenAPI to Go generator.
type Plugin struct{}

// ID returns the plugin identifier.
func (Plugin) ID() string { return PluginID }

// Generate produces types.gen.go, server.gen.go, client.gen.go and a go.mod
// from an OpenAPI document. The go.mod's module is package.name and it
// requires the modules the generated code imports.
func (Plugin) Generate(input contract.GenerationInput) (contract.GenerationOutput, error) {
	if input.InterfaceType != contract.InterfaceTypeOpenAPI {
		return contract.GenerationOutput{}, fmt.Errorf("openapi generate: unsupported interface type %q", input.InterfaceType)
	}
	if input.Target.Language != contract.SdkLanguageGo {
		return contract.GenerationOutput{}, fmt.Errorf("openapi generate: unsupported language %q", input.Target.Language)
	}
	if input.Package.Name == "" {
		return contract.GenerationOutput{}, errors.New("openapi generate: package.name is required")
	}
	pkgName := detailString(input.Package.Details, "packageName")
	if pkgName == "" {
		pkgName = defaultPackageName(input.Package.Name)
	}
	if !goIdentifier.MatchString(pkgName) {
		return contract.GenerationOutput{}, fmt.Errorf("openapi generate: package name %q is not a Go identifier", pkgName)
	}
	goVersion := detailString(input.Package.Details, "goVersion")
	if goVersion == "" {
		goVersion = DefaultGoVersion
	}
	spec, err := input.Document.Decode()
	if err != nil {
		return contract.GenerationOutput{}, err
	}
	if err := checkLocalRefs(spec); err != nil {
		return contract.GenerationOutput{}, err
	}

	targets := []struct {
		path string
		gen  codegen.GenerateOptions
	}{
		{TypesFile, codegen.GenerateOptions{Models: true}},
		{ServerFile, codegen.GenerateOptions{Server: true}},
		{ClientFile, codegen.GenerateOptions{Client: true}},
	}
	files := make([]contract.GeneratedFile, 0, len(targets)+1)
	var raw strings.Builder
	imports := map[string]bool{}
	for _, t := range targets {
		code, err := generateFile(spec, codegen.Configuration{PackageName: pkgName, Generate: t.gen})
		if err != nil {
			return contract.GenerationOutput{Raw: raw.String()}, fmt.Errorf("generate %s: %w", t.path, err)
		}
		if strings.TrimSpace(code) == "" {
			fmt.Fprintf(&raw, "skipped %s: nothing to generate\n", t.path)
			continue
		}
		if err := collectImports(t.path, code, imports); err != nil {
			return contract.GenerationOutput{Raw: raw.String()}, err
		}
		files = append(files, contract.GeneratedFile{Path: t.path, Contents: code})
		fmt.Fprintf(&raw, "wrote %s (%d bytes)\n", t.path, len(code))
	}
	gomod, requires, err := goMod(input.Package.Name, goVersion, imports)
	if err != nil {
		return contract.GenerationOutput{Raw: raw.String()}, err
	}
	files = append(files, contract.GeneratedFile{Path: GoModFile, Contents: gomod})
	fmt.Fprintf(&raw, "wrote %s (module %s)\n", GoModFile, input.Package.Name)
	return contract.GenerationOutput{
		Files: files,
		Raw:   raw.String(),
		Extra: map[string]any{"packageName": pkgName, "requires": requires},
	}, nil
}

// generateFile loads spec and runs one oapi-codegen generator. The document is loaded for each
// run because generation prunes and filters it in place.
func generateFile(spec []byte, cfg codegen.Configuration) (string, error) {
	codegenMu.Lock()
	defer codegenMu.Unlock()
	doc, err := libopenapi.Engine{}.LoadData(spec, "", engine.LoadOptions{})
	if err != nil {
		return "", err
	}
	return codegen.Generate(doc, cfg)
}

// checkLocalRefs fails when any $ref points outside the document. It runs before the document is
// loaded so no file or URL is ever read.
func checkLocalRefs(spec []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(spec, &root); err != nil {
		return fmt.Errorf("parse specification: %w", err)
	}
	var external []string
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if k.Value == "$ref" && v.Kind == yaml.ScalarNode && !strings.HasPrefix(v.Value, "#") {
					external = append(external, v.Value)
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	if len(external) == 0 {
		return nil
	}
	slices.Sort(external)
	external = slices.Compact(external)
	return fmt.Errorf("%w: %s", ErrExternalRef, strings.Join(external, ", "))
}

// collectImports adds the import paths of a generated Go file to imports.
func collectImports(path, code string, imports map[string]bool) error {
	f, err := parser.ParseFile(token.NewFileSet(), path, code, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("parse generated %s: %w", path, err)
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		imports[p] = true
	}
	return nil
}

// goMod writes the go.mod for the generated package, requiring the pinned version of each
// non-standard module the code imports.
func goMod(modulePath, goVersion string, imports map[string]bool) (string, map[string]string, error) {
	mf := &modfile.File{}
	if err := mf.AddModuleStmt(modulePath); err != nil {
		return "", nil, fmt.Errorf("go.mod module: %w", err)
	}
	if err := mf.AddGoStmt(goVersion); err != nil {
		return "", nil, fmt.Errorf("go.mod go version: %w", err)
	}
	requires := map[string]string{}
	for imp := range imports {
		first, _, _ := strings.Cut(imp, "/")
		if !strings.Contains(first, ".") {
			continue // standard library
		}
		mod, ok := moduleFor(imp)
		if !ok {
			return "", nil, fmt.Errorf("generated code imports %s, which has no pinned module version", imp)
		}
		requires[mod] = requiredModules[mod]
	}
	mods := make([]string, 0, len(requires))
	for m := range requires {
		mods = append(mods, m)
	}
	slices.Sort(mods)
	for _, m := range mods {
		if err := mf.AddRequire(m, requires[m]); err != nil {
			return "", nil, fmt.Errorf("go.mod require %s: %w", m, err)
		}
	}
	mf.Cleanup()
	b, err := mf.Format()
	if err != nil {
		return "", nil, fmt.Errorf("format go.mod: %w", err)
	}
	return string(b), requires, nil
}

func moduleFor(imp string) (string, bool) {
	for mod := range requiredModules {
		if imp == mod || strings.HasPrefix(imp, mod+"/") {
			return mod, true
		}
	}
	return "", false
}

func detailString(details map[string]any, key string) string {
	s, _ := details[key].(string)
	return s
}

// defaultPackageName derives a package identifier from a module path: the last element that is
// not a /vN suffix, lowercased, with characters other than letters and digits removed.
func defaultPackageName(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && isMajorSuffix(name) {
		name = parts[len(parts)-2]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "sdk" + out
	}
	return out
}

func isMajorSuffix(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	n, err := strconv.Atoi(s[1:])
	return err == nil && n >= 2
}
