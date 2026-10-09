package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xdearboy/prompp-vs-prometheus/internal/report"
)

func main() {
	var (
		root  = flag.String("root", "results", "results root")
		run   = flag.String("run", "", "run id, defaults to the newest directory")
		out   = flag.String("out", "", "markdown path, defaults to <root>/<run>/REPORT.md")
		dir   = flag.String("charts", "", "chart directory, defaults to <root>/<run>/charts")
		quiet = flag.Bool("quiet", false, "only print the output paths")
	)
	flag.Parse()

	runID := *run
	if runID == "" {
		newest, err := newestRun(*root)
		if err != nil {
			log.Fatal(err)
		}
		runID = newest
	}

	bundle, err := report.Load(*root, runID)
	if err != nil {
		log.Fatal(err)
	}
	bundle.AttachResources()

	markdownPath := *out
	if markdownPath == "" {
		markdownPath = filepath.Join(report.ResultsDir(*root, runID), "REPORT.md")
	}
	chartsDir := *dir
	if chartsDir == "" {
		chartsDir = filepath.Join(report.ResultsDir(*root, runID), "charts")
	}
	if err := bundle.WriteCharts(chartsDir); err != nil {
		log.Fatalf("charts: %v", err)
	}
	if err := report.WriteFile(markdownPath, bundle.Markdown()); err != nil {
		log.Fatalf("markdown: %v", err)
	}
	bundle.Lang = "ru"
	if err := report.WriteFile(strings.TrimSuffix(markdownPath, ".md")+".ru.md", bundle.Markdown()); err != nil {
		log.Fatalf("markdown: %v", err)
	}
	bundle.Lang = ""
	if !*quiet {
		fmt.Print(string(bundle.Markdown()))
	}
	fmt.Fprintf(os.Stderr, "report: %s\ncharts:  %s\n", markdownPath, chartsDir)
}

func newestRun(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	best := ""
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "charts" {
			continue
		}
		if best == "" || e.Name() > best {
			best = e.Name()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no runs under %s", root)
	}
	return best, nil
}
