package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/xdearboy/prompp-vs-prometheus/internal/report"
)

func main() {
	root := flag.String("root", "results", "directory holding the run directories")
	out := flag.String("out", "", "markdown path, defaults to <root>/AGGREGATE.md")
	flag.Parse()

	entries, err := os.ReadDir(*root)
	if err != nil {
		log.Fatal(err)
	}
	var runs []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(*root, e.Name(), "REPORT.md")); err == nil {
			runs = append(runs, e.Name())
		}
	}
	if len(runs) == 0 {
		log.Fatalf("no finished runs under %s", *root)
	}
	sort.Strings(runs)

	body, err := report.Aggregate(*root, runs)
	if err != nil {
		log.Fatal(err)
	}
	path := *out
	if path == "" {
		path = filepath.Join(*root, "AGGREGATE.md")
	}
	if err := report.WriteFile(path, body); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d runs, %s\n", len(runs), path)
}
