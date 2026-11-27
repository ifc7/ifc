package commands

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

var generateCmd = &cobra.Command{
	Use:   "generate <name|path>",
	Short: "Generate an SDK from an interface specification",
	Long: `
Generate SDK code from an OpenAPI specification with the default generator plugin, the
same one the hub uses for published SDKs.

Arguments:
  name|path  Owned interface name from ifc.yaml, or a file path
             File paths work even without an ifc.yaml project

Go (the only language so far) writes three files to the output directory:
  types.gen.go   models for the specification's schemas
  server.gen.go  a strict net/http server interface and handler
  client.gen.go  an HTTP client

With --module, a go.mod for that module path is written too. Without it the code is
meant for a package inside an existing module: add the modules listed after the run
to that module's go.mod (go mod tidy does it).

JSON Schema interfaces and specifications with external $refs are not supported yet.
`,
	Args: cobra.ExactArgs(1),
	RunE: runGenerate,
}

type generateFlags struct {
	lang    string
	out     string
	module  string
	pkg     string
	version string
}

var generateOpts generateFlags

func runGenerate(cmd *cobra.Command, args []string) error {
	f := generateOpts
	target := args[0]
	if proj, err := project.Load(); err == nil {
		if target, err = proj.ResolveComparePath(target); err != nil {
			return err
		}
	}
	lang, err := sdkLanguage(f.lang)
	if err != nil {
		return err
	}
	outDir := f.out
	if outDir == "" {
		base := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
		outDir = filepath.Join("gen", base)
	}
	res, err := project.GenerateFile(target, project.GenerateOptions{
		Language: lang,
		Module:   f.module,
		Package:  f.pkg,
		Version:  f.version,
		OutDir:   outDir,
	})
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "%s %s\n", ui.KeyHints("source: "), res.Source)
	_, _ = fmt.Fprintf(w, "%s %s\n", ui.KeyHints("plugin: "), res.PluginID)
	_, _ = fmt.Fprintf(w, "%s %s\n", ui.KeyHints("package:"), res.Package)
	_, _ = fmt.Fprintf(w, "%s %s\n", ui.KeyHints("output: "), res.OutDir)
	for _, p := range res.Written {
		_, _ = fmt.Fprintf(w, "  %s\n", ui.Apply(ui.Success, p))
	}
	if f.module == "" && len(res.Requires) > 0 {
		mods := make([]string, 0, len(res.Requires))
		for m := range res.Requires {
			mods = append(mods, m)
		}
		slices.Sort(mods)
		_, _ = fmt.Fprintf(w, "%s\n", ui.KeyHints("requires:"))
		for _, m := range mods {
			_, _ = fmt.Fprintf(w, "  %s %s\n", m, res.Requires[m])
		}
	}
	return nil
}

func sdkLanguage(s string) (contract.SdkLanguage, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "go", "golang":
		return contract.SdkLanguageGo, nil
	default:
		return "", fmt.Errorf("--lang %q is not supported; supported: go", s)
	}
}

func init() {
	f := generateCmd.Flags()
	f.StringVar(&generateOpts.lang, "lang", "go", "SDK language: go")
	f.StringVarP(&generateOpts.out, "out", "o", "", "output directory (default gen/<spec file name>)")
	f.StringVar(&generateOpts.module, "module", "", "Go module path; writes a go.mod when set")
	f.StringVar(&generateOpts.pkg, "package", "", "Go package name (default from the output directory)")
	f.StringVar(&generateOpts.version, "version", "", "version recorded in the generated code (default v0.0.0)")
	rootCmd.AddCommand(generateCmd)
}
