package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/romshark/toki/editor"
	intapp "github.com/romshark/toki/internal/app"
	"github.com/romshark/toki/internal/log"
)

func main() {
	server := flag.String("server", "localhost:8080", "server host address")
	bundlePkg := flag.String("b", "tokibundle", "path to generated Go bundle package")
	dirFlag := flag.String("dir", "", "project directory (defaults to current directory)")
	flag.Parse()

	dir := *dirFlag
	if dir == "" {
		dir, _ = filepath.Abs(".")
	} else {
		dir, _ = filepath.Abs(dir)
	}
	log.SetWriter(os.Stderr, false)

	_, s := editor.Setup(dir, *bundlePkg, intapp.Version, os.Environ(), intapp.CleanGenerated, intapp.GenerateBundle)
	os.Exit(editor.RunServer(s, *server))
}
