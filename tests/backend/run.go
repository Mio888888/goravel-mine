package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const packageTestsRoot = "tests/backend/_packages"

type overlay struct {
	Replace map[string]string `json:"Replace"`
}

func main() {
	root, err := repositoryRoot()
	if err != nil {
		exitError(err)
	}
	overlayPath, cleanup, err := writeOverlay(root)
	if err != nil {
		exitError(err)
	}
	defer cleanup()

	testArgs := os.Args[1:]
	if len(testArgs) > 0 && testArgs[0] == "--" {
		testArgs = testArgs[1:]
	}
	if len(testArgs) == 0 {
		testArgs = []string{"./..."}
	}
	args := []string{"test", "-overlay", overlayPath}
	args = append(args, testArgs...)
	command := exec.Command("go", args...)
	command.Dir = root
	command.Env = append(
		os.Environ(),
		"GORAVEL_REPOSITORY_ROOT="+root,
		"GORAVEL_TEST_OVERLAY=1",
	)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		exitError(err)
	}
}

func repositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("go.mod not found")
		}
		current = parent
	}
}

func writeOverlay(root string) (string, func(), error) {
	sourceRoot := filepath.Join(root, filepath.FromSlash(packageTestsRoot))
	replacements := make(map[string]string)
	err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if _, err := os.Stat(filepath.Dir(target)); err != nil {
			return fmt.Errorf("overlay target package missing for %s: %w", filepath.ToSlash(relative), err)
		}
		replacements[target] = path
		return nil
	})
	if err != nil {
		return "", func() {}, err
	}
	if len(replacements) == 0 {
		return "", func() {}, errors.New("no centralized package tests found")
	}

	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, key := range keys {
		ordered[key] = replacements[key]
	}

	payload, err := json.MarshalIndent(overlay{Replace: ordered}, "", "  ")
	if err != nil {
		return "", func() {}, err
	}
	file, err := os.CreateTemp("", "goravel-go-test-overlay-*.json")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return file.Name(), cleanup, nil
}

func exitError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
