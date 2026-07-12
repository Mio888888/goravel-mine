package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
)

const (
	reportVersion    = "v1"
	largestFileLimit = 10
	cloneSampleLimit = 10
)

type report struct {
	Version         string        `json:"version"`
	Scope           scopeReport   `json:"scope"`
	Groups          []groupReport `json:"groups"`
	Files           int           `json:"files"`
	Lines           int           `json:"lines"`
	CloneBlocks     int           `json:"clone_blocks"`
	DuplicatedLines int           `json:"duplicated_lines"`
	LargestFiles    []fileSummary `json:"largest_files"`
	CloneSamples    []cloneSample `json:"clone_samples"`
}

type scopeReport struct {
	Root       string   `json:"root"`
	Extensions []string `json:"extensions"`
	Excludes   []string `json:"excludes"`
}

type groupReport struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
	Lines int    `json:"lines"`
}

type fileSummary struct {
	Path  string `json:"path"`
	Group string `json:"group"`
	Lines int    `json:"lines"`
}

type cloneSample struct {
	Lines     int             `json:"lines"`
	Chars     int             `json:"chars"`
	Locations []cloneLocation `json:"locations"`
}

type cloneLocation struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type options struct {
	root   string
	format string
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		exitWithError(err)
	}
	result, err := buildReport(opts.root)
	if err != nil {
		exitWithError(err)
	}
	if err := writeReport(os.Stdout, result, opts.format); err != nil {
		exitWithError(err)
	}
}

func parseOptions() (options, error) {
	opts := options{}
	flag.StringVar(&opts.root, "root", ".", "scan root")
	flag.StringVar(&opts.format, "format", "json", "output format")
	flag.Parse()
	if opts.format != "json" {
		return options{}, errors.New("only --format json is supported")
	}
	return opts, nil
}

func buildReport(root string) (report, error) {
	scopedFiles, err := collectScopedFiles(root)
	if err != nil {
		return report{}, err
	}
	prepared, groups, files, lines, largest, err := collectMetrics(scopedFiles)
	if err != nil {
		return report{}, err
	}
	blocks, duplicated, samples := detectClones(prepared)
	return report{
		Version:         reportVersion,
		Scope:           newScopeReport(root),
		Groups:          groups,
		Files:           files,
		Lines:           lines,
		CloneBlocks:     blocks,
		DuplicatedLines: duplicated,
		LargestFiles:    largest,
		CloneSamples:    samples,
	}, nil
}

func writeReport(file *os.File, result report, format string) error {
	if format != "json" {
		return errors.New("unsupported report format")
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
