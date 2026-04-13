// Command genexamplelarge generates a large example Toki project with many TIKs
// for testing the editor with realistic data volumes.
//
// It creates a self-contained Go module in the output directory with:
//   - A main.go and multiple texts_N.go files containing randomized
//     reader.String(...) calls using valid TIK syntax
//   - A minimal tokibundle/ stub that satisfies Go type-checking
//
// After writing the source files, it runs:
//  1. go get ./...       — resolve dependencies and populate go.sum
//  2. toki generate      — parse TIKs, create ARB catalogs and bundle code
//  3. go mod tidy        — add indirect deps introduced by the generated bundle
//
// The generated TIKs cover all placeholder types:
//   - plain literals, {text},
//   - {name} (with gender),
//   - {# plural},
//   - {date-*},
//   - {time-*},
//   - mixed combinations,
//   - multiple plurals,
//   - and [context]-prefixed labels.
//
// A deterministic seed (-seed flag) makes the output reproducible.
//
// Usage:
//
//	go run ./cmd/examplegen \
//		-tiks 10000 \
//		-locales en,de,fr,it,es,pt,nl,pl,cs,sv,da,ja \
//		-seed 42
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
)

//go:embed templates.json
var templatesJSON []byte

type tikPattern struct {
	TIK  string `json:"tik"`
	Desc string `json:"desc"`
}

type templateData struct {
	Literals        []string     `json:"literals"`
	TextPlaceholder []tikPattern `json:"textPlaceholder"`
	NamePlaceholder []tikPattern `json:"namePlaceholder"`
	CardinalPlural  []tikPattern `json:"cardinalPlural"`
	DateTime        []tikPattern `json:"dateTime"`
	NamePlusPlural  []tikPattern `json:"namePlusPlural"`
	MultiplePlurals []tikPattern `json:"multiplePlurals"`
	Contexts        []string     `json:"contexts"`
	ContextLabels   []string     `json:"contextLabels"`
}

var templates templateData

func init() {
	if err := json.Unmarshal(templatesJSON, &templates); err != nil {
		panic(fmt.Sprintf("parsing templates.json: %v", err))
	}
}

func main() {
	var (
		numTIKs    = flag.Int("tiks", 10_000, "number of TIKs to generate")
		localesRaw = flag.String("locales",
			"en,de,fr,it,es,pt,nl,pl,cs,sv,da,ja",
			"comma-separated list of locales")
		defaultLoc = flag.String("default", "en", "default locale")
		outDir     = flag.String("out", "examples/largecodebase", "output directory")
		seed       = flag.Uint64("seed", 42, "random seed (0 = time-based)")
		bundlePkg  = flag.String("bundle", "tokibundle", "bundle package name")
	)
	flag.Parse()

	locales := strings.Split(*localesRaw, ",")
	for i := range locales {
		locales[i] = strings.TrimSpace(locales[i])
	}

	var rng *rand.Rand
	if *seed != 0 {
		rng = rand.New(rand.NewPCG(*seed, *seed))
	} else {
		s := uint64(time.Now().UnixNano())
		rng = rand.New(rand.NewPCG(s, s))
	}

	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving output dir: %v\n", err)
		os.Exit(1)
	}

	// Clean output directory from previous runs.
	if err := os.RemoveAll(absOut); err != nil {
		fmt.Fprintf(os.Stderr, "error cleaning output dir: %v\n", err)
		os.Exit(1)
	}
	bundleDir := filepath.Join(absOut, *bundlePkg)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating dirs: %v\n", err)
		os.Exit(1)
	}

	// Step 1: Write go.mod and a dummy main.go (no tokibundle import).
	if err := writeGoMod(absOut); err != nil {
		fmt.Fprintf(os.Stderr, "error writing go.mod: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(absOut, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing dummy main.go: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "head.txt"),
		[]byte("Generated example project\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing head.txt: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Pre-add dependencies that toki's generated bundle needs.
	fmt.Println("Adding bundle dependencies...")
	if err := run(absOut, "go", "get",
		"github.com/go-playground/locales@v0.14.1",
		"golang.org/x/text@latest",
	); err != nil {
		fmt.Fprintf(os.Stderr, "go get failed: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Bootstrap the bundle with toki generate (creates bundle_gen.go + ARBs).
	tokiArgs := []string{"generate", "-l", *defaultLoc, "-b", *bundlePkg}
	for _, loc := range locales {
		if loc != *defaultLoc {
			tokiArgs = append(tokiArgs, "-t", loc)
		}
	}
	fmt.Println("Bootstrapping bundle with toki generate...")
	if err := run(absOut, "toki", tokiArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "toki generate (bootstrap) failed: %v\n", err)
		os.Exit(1)
	}

	// Step 4: Resolve deps now that the bundle exists.
	fmt.Println("Running go get...")
	if err := run(absOut, "go", "get", "./..."); err != nil {
		fmt.Fprintf(os.Stderr, "go get failed: %v\n", err)
		os.Exit(1)
	}

	// Step 5: Generate the real source files with TIK usages.
	tiks := generateTIKs(rng, *numTIKs)
	if err := writeGoSources(absOut, *bundlePkg, tiks); err != nil {
		fmt.Fprintf(os.Stderr, "error writing Go sources: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %d TIKs in %s\n", len(tiks), absOut)

	// Step 6: Run toki generate again to discover TIKs and update ARBs.
	fmt.Println("Running toki generate...")
	if err := run(absOut, "toki", tokiArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "toki generate failed: %v\n", err)
		os.Exit(1)
	}

	// Step 7: Final go mod tidy.
	fmt.Println("Running go mod tidy...")
	if err := run(absOut, "go", "mod", "tidy"); err != nil {
		fmt.Fprintf(os.Stderr, "go mod tidy failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Done: %d TIKs across %d locales in %s\n",
		len(tiks), len(locales), absOut)
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	return cmd.Run()
}

type tikEntry struct {
	TIK     string // raw TIK string
	ID      string // msg + xxhash64 hex
	Comment string // description
	Params  []tikParam
}

type tikParam struct {
	Name string
	Type string // "String", "num", "DateTime"
	Desc string
	ICU  string // how it appears in ICU (e.g., "{var0}")
}

func tikID(tik string) string {
	h := xxhash.New()
	_, _ = h.WriteString(tik)
	return fmt.Sprintf("msg%016x", h.Sum64())
}

// tikGenerator is a function that produces a random TIK from a template category.
type tikGenerator func(rng *rand.Rand) (tik, comment string, params []tikParam)

// tikGenerators returns the generator functions, one per template category.
func tikGenerators() []tikGenerator {
	return []tikGenerator{
		// Simple literal text.
		func(rng *rand.Rand) (string, string, []tikParam) {
			s := templates.Literals[rng.IntN(len(templates.Literals))]
			return s, fmt.Sprintf("UI label: %s", s), nil
		},
		// {text} placeholder (plain string arg).
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.TextPlaceholder[rng.IntN(len(templates.TextPlaceholder))]
			return p.TIK, p.Desc, []tikParam{
				{Name: "var0", Type: "String", Desc: "arbitrary string"},
			}
		},
		// {name} placeholder (text-with-gender, requires tokibundle.String).
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.NamePlaceholder[rng.IntN(len(templates.NamePlaceholder))]
			return p.TIK, p.Desc, []tikParam{
				{
					Name: "var0",
					Type: "StringGender",
					Desc: "arbitrary string with gender information",
				},
			}
		},
		// Cardinal plural {# items}.
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.CardinalPlural[rng.IntN(len(templates.CardinalPlural))]
			return p.TIK, p.Desc, []tikParam{
				{Name: "var0", Type: "num", Desc: "cardinal plural"},
			}
		},
		// Date/time placeholders.
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.DateTime[rng.IntN(len(templates.DateTime))]
			params := []tikParam{
				{Name: "var0", Type: "DateTime", Desc: "date"},
			}
			if strings.Contains(p.TIK, "{time-") {
				params = append(params, tikParam{
					Name: "var1",
					Type: "DateTime",
					Desc: "time",
				})
			}
			return p.TIK, p.Desc, params
		},
		// Mixed: {name} + {# plural}.
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.NamePlusPlural[rng.IntN(len(templates.NamePlusPlural))]
			return p.TIK, p.Desc, []tikParam{
				{
					Name: "var0",
					Type: "StringGender",
					Desc: "arbitrary string with gender information",
				},
				{
					Name: "var1",
					Type: "num",
					Desc: "cardinal plural",
				},
			}
		},
		// Multiple plurals.
		func(rng *rand.Rand) (string, string, []tikParam) {
			p := templates.MultiplePlurals[rng.IntN(len(templates.MultiplePlurals))]
			return p.TIK, p.Desc, []tikParam{
				{Name: "var0", Type: "num", Desc: "cardinal plural"},
				{Name: "var1", Type: "num", Desc: "cardinal plural"},
			}
		},
		// [context] prefix.
		func(rng *rand.Rand) (string, string, []tikParam) {
			ctx := templates.Contexts[rng.IntN(len(templates.Contexts))]
			label := templates.ContextLabels[rng.IntN(len(templates.ContextLabels))]
			tik := fmt.Sprintf("[%s] %s", ctx, label)
			return tik, fmt.Sprintf("Context: %s — Label: %s", ctx, label), nil
		},
	}
}

func generateTIKs(rng *rand.Rand, n int) []tikEntry {
	generators := tikGenerators()
	seen := make(map[string]bool)
	entries := make([]tikEntry, 0, n)

	for len(entries) < n {
		gen := generators[rng.IntN(len(generators))]
		tik, comment, params := gen(rng)

		// Ensure uniqueness by appending index if needed.
		origTIK := tik
		for seen[tik] {
			tik = fmt.Sprintf("%s (%d)", origTIK, rng.IntN(10000))
		}
		seen[tik] = true

		entries = append(entries, tikEntry{
			TIK:     tik,
			ID:      tikID(tik),
			Comment: comment,
			Params:  params,
		})
	}
	return entries
}

func writeGoSources(dir, bundlePkg string, tiks []tikEntry) error {
	// Split TIKs across multiple files for realism.
	filesCount := max(1, len(tiks)/100)
	for i := range filesCount {
		start := i * len(tiks) / filesCount
		end := (i + 1) * len(tiks) / filesCount
		chunk := tiks[start:end]

		needsTime := false
		for _, t := range chunk {
			for _, p := range t.Params {
				if p.Type == "DateTime" {
					needsTime = true
					break
				}
			}
			if needsTime {
				break
			}
		}

		var b strings.Builder
		b.WriteString("package main\n\n")
		if needsTime {
			fmt.Fprintf(&b,
				"import (\n\t\"%s/%s\"\n\t\"time\"\n)\n\n", "largecodebase", bundlePkg)
		} else {
			fmt.Fprintf(&b, "import \"%s/%s\"\n\n", "largecodebase", bundlePkg)
		}
		fmt.Fprintf(&b, "func texts%d(r tokibundle.Reader) {\n", i)

		for _, t := range chunk {
			if t.Comment != "" {
				fmt.Fprintf(&b, "\t// %s\n", t.Comment)
			}
			args := tikGoArgs(t)
			fmt.Fprintf(&b, "\t_ = r.String(%q%s)\n", t.TIK, args)
		}
		b.WriteString("}\n")

		filename := filepath.Join(dir, fmt.Sprintf("texts_%d.go", i))
		if err := os.WriteFile(filename, []byte(b.String()), 0o644); err != nil {
			return err
		}
	}

	// Write main.go.
	var b strings.Builder
	b.WriteString("package main\n\n")
	fmt.Fprintf(&b,
		"import (\n\t\"%s/%s\"\n\t\"golang.org/x/text/language\"\n)\n\n",
		"largecodebase", bundlePkg)
	b.WriteString("func main() {\n")
	b.WriteString("\tr, _ := tokibundle.Match(language.English)\n")
	for i := range filesCount {
		fmt.Fprintf(&b, "\ttexts%d(r)\n", i)
	}
	b.WriteString("}\n")

	return os.WriteFile(filepath.Join(dir, "main.go"), []byte(b.String()), 0o644)
}

func tikGoArgs(t tikEntry) string {
	if len(t.Params) == 0 {
		return ""
	}
	var args []string
	for _, p := range t.Params {
		switch p.Type {
		case "String":
			args = append(args, `"example"`)
		case "StringGender":
			args = append(args,
				`tokibundle.String{Value: "Example", Gender: tokibundle.GenderNeutral}`)
		case "num":
			args = append(args, "42")
		case "DateTime":
			args = append(args, "time.Now()")
		default:
			args = append(args, `"?"`)
		}
	}
	return ", " + strings.Join(args, ", ")
}

func writeGoMod(dir string) error {
	return run(dir, "go", "mod", "init", "largecodebase")
}
