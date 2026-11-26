package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
	"github.com/ifc7/ifc/pkg/governance/findings"
	"github.com/ifc7/ifc/pkg/plugins/collection"
)

// exitCollectionInvalid is the exit status when a collection has errors (or,
// with --strict, warnings).
const exitCollectionInvalid = 2

var collectionCmd = &cobra.Command{
	Use:   "collection",
	Short: "Work with collections: Arazzo, Postman, JSON Schema Test Suite and OpenCollection",
}

var collectionValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a collection the way the hub does when a version is created",
	Long: `
Validate a collection source file offline, with the same checks the hub runs when a collection
version is created. Nothing is fetched: Arazzo source descriptions and remote $refs are not loaded.

Formats (detected from the file, or set with --format):
  ARAZZO_1_0              Arazzo 1.0 workflows, linted with Vacuum's recommended Arazzo rules
  POSTMAN_2_1             Postman Collection v2.1
  JSON_SCHEMA_TEST_SUITE  JSON Schema Test Suite files; each test's data is checked against its schema
  OPEN_COLLECTION_1_0     OpenCollection 1.0, against the pinned OpenCollection schema

--interface takes the bound specification (an owned interface name or a file path). With it,
Arazzo steps and Postman and OpenCollection requests are checked against the OpenAPI operations,
and a test suite's schemas can $ref the interface schema by its $id. These checks are warnings.

Exit status: 0 when there are no errors (with --strict, no findings at all), 2 when there are,
1 for any other error.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		src, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		format := collection.Format(strings.ToUpper(collectionOpts.format))
		if format == "" {
			f, ok := collection.Detect(src)
			if !ok {
				return fmt.Errorf("can't tell the collection format of %s: pass --format", args[0])
			}
			format = f
		} else if !slices.Contains(collection.Formats(), format) {
			return fmt.Errorf("unknown --format %q: use one of %v", collectionOpts.format, collection.Formats())
		}
		var iface []byte
		if collectionOpts.iface != "" {
			path := collectionOpts.iface
			if proj, err := project.Load(); err == nil {
				if path, err = proj.ResolveComparePath(path); err != nil {
					return fmt.Errorf("interface: %w", err)
				}
			}
			if iface, err = os.ReadFile(path); err != nil {
				return fmt.Errorf("interface: %w", err)
			}
		}
		res, err := collection.Validate(cmd.Context(), collection.Input{Format: format, Source: src, Interface: iface})
		if err != nil {
			return err
		}
		failed := !res.Valid() || collectionOpts.strict && len(res.Findings) > 0
		if collectionOpts.asJSON {
			out := map[string]any{"file": args[0], "format": format, "validator": res.Validator, "valid": res.Valid(), "findings": res.Findings}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				return err
			}
		} else {
			fmt.Printf("%s %s\n", ui.KeyHints("file:     "), args[0])
			fmt.Printf("%s %s\n", ui.KeyHints("format:   "), format)
			fmt.Printf("%s %s\n", ui.KeyHints("validator:"), res.Validator)
			fmt.Printf("%s %d errors, %d warnings\n", ui.KeyHints("result:   "), len(res.Errors()), len(res.Warnings()))
			if len(res.Findings) > 0 {
				fmt.Printf("\n%s", findings.RenderText(res.Findings))
			}
		}
		if failed {
			return &ExitError{Code: exitCollectionInvalid, Err: fmt.Errorf("%s is not a valid %s collection", args[0], format)}
		}
		return nil
	},
}

var collectionOpts struct {
	format string
	iface  string
	asJSON bool
	strict bool
}

func init() {
	f := collectionValidateCmd.Flags()
	f.StringVar(&collectionOpts.format, "format", "", "collection format (default: detected from the file)")
	f.StringVar(&collectionOpts.iface, "interface", "", "bound interface: owned interface name or specification file, for cross-checks")
	f.BoolVar(&collectionOpts.asJSON, "json", false, "print a JSON report to stdout")
	f.BoolVar(&collectionOpts.strict, "strict", false, "also fail on warnings")
	collectionCmd.AddCommand(collectionValidateCmd)
	rootCmd.AddCommand(collectionCmd)
}
