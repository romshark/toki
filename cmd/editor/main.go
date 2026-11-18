package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/romshark/toki/editor"
	"github.com/romshark/toki/internal/cli"
	"github.com/romshark/toki/internal/log"
)

func main() {
	bundlePkg := flag.String("b", "tokibundle", "path to generated Go bundle package")
	flag.Parse()

	dir, _ := filepath.Abs(".")
	log.SetWriter(os.Stderr, false)

	a, s, err := editor.Setup(
		dir, *bundlePkg, cli.Version, os.Environ(),
		cli.CleanGenerated, cli.GenerateBundle,
		cli.ApplyChangesAndBuild, cli.RepairBundle,
		cli.RegenerateBundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setting up editor: %v\n", err)
		os.Exit(1)
	}

	if err := cli.RunHybridApp(a, s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
