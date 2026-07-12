package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildReportIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixture{"app/modules/alpha.go", fixtureAlpha})
	writeFixture(t, root, fixture{"app/modulecatalog/beta.go", fixtureBeta})
	writeFixture(t, root, fixture{"app/modules/gamma.go", fixtureGamma})
	writeFixture(t, root, fixture{"app/modules/alpha_test.go", fixtureAlphaTest})
	writeFixture(t, root, fixture{"tests/unit/module_catalog_service_test.go", fixtureUnitTest})
	writeFixture(t, root, fixture{"MineAdmin-web/src/modules/base/api/platformModuleLifecycle.ts", fixtureFrontend})
	writeFixture(t, root, fixture{"app/modules/testdata/ignored.go", fixtureIgnored})

	first, err := buildReport(root)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	second, err := buildReport(root)
	if err != nil {
		t.Fatalf("buildReport() second error = %v", err)
	}

	firstJSON := mustJSON(t, first)
	secondJSON := mustJSON(t, second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("report JSON differs between runs\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	if first.CloneBlocks != 1 {
		t.Fatalf("CloneBlocks = %d, want 1", first.CloneBlocks)
	}
	if first.DuplicatedLines != 16 {
		t.Fatalf("DuplicatedLines = %d, want 16", first.DuplicatedLines)
	}
	if first.Files != 6 {
		t.Fatalf("Files = %d, want 6", first.Files)
	}
	assertGroupCount(t, first, groupExpectation{"backend", 3})
	assertGroupCount(t, first, groupExpectation{"frontend", 1})
	assertGroupCount(t, first, groupExpectation{"tests", 2})
	if len(first.CloneSamples) != 1 {
		t.Fatalf("len(CloneSamples) = %d, want 1", len(first.CloneSamples))
	}
	sample := first.CloneSamples[0]
	if len(sample.Locations) != 2 {
		t.Fatalf("len(sample.Locations) = %d, want 2", len(sample.Locations))
	}
	if sample.Locations[0].Path != "app/modulecatalog/beta.go" {
		t.Fatalf("first clone path = %q, want app/modulecatalog/beta.go", sample.Locations[0].Path)
	}
	if sample.Locations[1].Path != "app/modules/alpha.go" {
		t.Fatalf("second clone path = %q, want app/modules/alpha.go", sample.Locations[1].Path)
	}
}

func TestBuildReportMergesAdjacentCloneWindows(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixture{"app/modules/delta.go", adjacentFixture("delta")})
	writeFixture(t, root, fixture{"app/modulecatalog/epsilon.go", adjacentFixture("epsilon")})

	result, err := buildReport(root)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	if result.CloneBlocks != 1 {
		t.Fatalf("CloneBlocks = %d, want one merged block", result.CloneBlocks)
	}
	if result.DuplicatedLines != 20 {
		t.Fatalf("DuplicatedLines = %d, want 20 normalized source lines", result.DuplicatedLines)
	}
	if result.CloneSamples[0].Lines != 10 {
		t.Fatalf("clone sample lines = %d, want 10", result.CloneSamples[0].Lines)
	}
}

type groupExpectation struct {
	Name  string
	Files int
}

func assertGroupCount(t *testing.T, report report, expected groupExpectation) {
	t.Helper()
	for _, group := range report.Groups {
		if group.Name == expected.Name {
			if group.Files != expected.Files {
				t.Fatalf("%s files = %d, want %d", expected.Name, group.Files, expected.Files)
			}
			return
		}
	}
	t.Fatalf("group %q not found", expected.Name)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	return append(data, '\n')
}

type fixture struct {
	Path    string
	Content string
}

func writeFixture(t *testing.T, root string, item fixture) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(item.Path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(item.Content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

const fixtureAlpha = `package modules

import (
    "fmt"
)

func alpha() {
    moduleID := "module-governance"
    action := "install"
    owner := "platform-admin"
    reason := "baseline-freeze"
    summary := moduleID + action
    details := owner + reason
    output := summary + details
    fmt.Println(output)
}
`

const fixtureBeta = `package modulecatalog

import "fmt"

func beta() {
    moduleID := "module-governance"
    /* ignored comment
       ignored block content
    */
    action := "install"
    owner := "platform-admin"
    reason := "baseline-freeze"
    summary := moduleID + action
    details := owner + reason
    output := summary + details
    fmt.Println(output)
}
`

const fixtureGamma = `package modules

func gamma() {
    moduleID := "module-governance"
    action := "rollback"
    owner := "platform-admin"
    reason := "baseline-freeze"
    summary := moduleID + action
    details := owner + reason
    output := summary + details
    println(output)
}
`

const fixtureAlphaTest = `package modules

func TestAlpha(t *testing.T) {
    t.Helper()
}
`

const fixtureUnitTest = `package unit

func TestModuleCatalogService(t *testing.T) {
    t.Helper()
}
`

const fixtureFrontend = `export async function fetchLifecycleState() {
  return Promise.resolve({ status: "ok" })
}
`

const fixtureIgnored = `package ignored

func fixture() {
    moduleID := "module-governance"
    action := "install"
    owner := "platform-admin"
    reason := "baseline-freeze"
    summary := moduleID + action
    details := owner + reason
    output := summary + details
    println(output)
}
`

func adjacentFixture(name string) string {
	return `package fixture

func ` + name + `() {
    moduleID := "module-governance"
    action := "install"
    owner := "platform-admin"
    reason := "baseline-freeze"
    summary := moduleID + action
    details := owner + reason
    output := summary + details
    audit := output + owner
    checksum := audit + reason
    println(checksum)
}
`
}
