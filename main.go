package main

import (
	"os"
	"time"

	"github.com/romshark/toki/internal/app"
)

func main() {
	r, exitCode := app.Run(os.Args, os.Stderr, os.Stdout, time.Now())
	r.Print()
	os.Exit(exitCode)
}
