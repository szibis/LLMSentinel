package evidence

import (
	"flag"
	"fmt"
	"io"
)

type artifactFlags []string

func (a *artifactFlags) String() string     { return fmt.Sprint([]string(*a)) }
func (a *artifactFlags) Set(s string) error { *a = append(*a, s); return nil }

// Run creates or verifies bounded synthetic CI evidence. It does not contact
// providers, capture clients, or inspect private lab contents.
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "regression" {
		return runRegression(args[1:], out, stderr)
	}
	if len(args) == 0 || (args[0] != "create" && args[0] != "verify") {
		fmt.Fprintln(stderr, "usage: evidence create|verify --root DIR --manifest FILE; evidence regression --events FILE --report FILE --source-revision SHA --control-revision SHA (synthetic CI evidence only)")
		return 2
	}
	fs := flag.NewFlagSet("evidence "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "synthetic artifact directory")
	manifest := fs.String("manifest", "", "manifest file")
	source := fs.String("source-revision", "", "full source Git object ID")
	control := fs.String("control-revision", "", "full control Git object ID")
	runtime := fs.String("runtime-revision", "", "full runtime Git object ID, unknown if omitted")
	model := fs.String("model", "", "observed model identifier, unknown if omitted")
	client := fs.String("client", "", "observed client identifier, unknown if omitted")
	var artifacts artifactFlags
	fs.Var(&artifacts, "artifact", "relative synthetic artifact path (repeat)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *root == "" || *manifest == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "root and manifest are required; positional arguments are not accepted")
		return 2
	}
	var err error
	if args[0] == "create" {
		p := Provenance{SourceRevision: *source, ControlRevision: *control}
		if *runtime != "" {
			p.RuntimeRevision = runtime
		}
		if *model != "" {
			p.Model = model
		}
		if *client != "" {
			p.Client = client
		}
		var m Manifest
		m, err = Generate(*root, artifacts, p)
		if err == nil {
			err = WriteManifest(*manifest, m)
		}
	} else {
		var m Manifest
		m, err = ReadManifest(*manifest)
		if err == nil {
			err = Verify(*root, m)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "evidence:", err)
		return 1
	}
	fmt.Fprintln(out, "evidence", args[0], "verified:", *manifest)
	return 0
}
