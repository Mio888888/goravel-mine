package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const reportSchema = "resilience-report/v1"

var requiredScenarios = []string{
	"multi-tenant-connection-scale", "casbin-large-policy", "redis-short-outage", "queue-backlog",
	"db-pool-exhaustion", "concurrent-migration", "eight-hour-soak", "restore-rto-rpo",
}

type Config struct {
	Schema    string     `yaml:"schema"`
	Scenarios []Scenario `yaml:"scenarios"`
}

type Scenario struct {
	Name       string             `yaml:"name"`
	Profiles   []string           `yaml:"profiles"`
	Setup      string             `yaml:"setup"`
	Action     string             `yaml:"action"`
	Assertions string             `yaml:"assertions"`
	Cleanup    string             `yaml:"cleanup"`
	Timeout    string             `yaml:"timeout"`
	Thresholds map[string]float64 `yaml:"thresholds"`
	Evidence   []string           `yaml:"evidence"`
}

type ScenarioResult struct {
	Name        string             `json:"name"`
	Status      string             `json:"status"`
	StartedAt   time.Time          `json:"started_at"`
	CompletedAt time.Time          `json:"completed_at"`
	Thresholds  map[string]float64 `json:"thresholds"`
	Evidence    []string           `json:"evidence"`
	Error       string             `json:"error,omitempty"`
}

type Report struct {
	Schema      string           `json:"schema"`
	Profile     string           `json:"profile"`
	GeneratedAt time.Time        `json:"generated_at"`
	Status      string           `json:"status"`
	Scenarios   []ScenarioResult `json:"scenarios"`
}

type commandRunner func(context.Context, string) error

func main() {
	configPath := flag.String("config", "tests/resilience/scenarios.yaml", "scenario configuration")
	profile := flag.String("profile", "quick", "quick or soak")
	output := flag.String("output", "artifacts/resilience/resilience-report.json", "report output")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	if err := validateConfig(config); err != nil {
		fatal(err)
	}
	report := runProfile(config, *profile, shellCommand)
	if err := writeReport(*output, report); err != nil {
		fatal(err)
	}
	if report.Status != "passed" {
		os.Exit(1)
	}
}

func loadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateConfig(config Config) error {
	if config.Schema != "resilience-scenarios/v1" {
		return errors.New("invalid resilience scenario schema")
	}
	seen := make(map[string]bool)
	for _, scenario := range config.Scenarios {
		seen[scenario.Name] = true
		if strings.TrimSpace(scenario.Action) == "" || strings.TrimSpace(scenario.Assertions) == "" {
			return fmt.Errorf("scenario %s requires action and assertions", scenario.Name)
		}
		if strings.TrimSpace(scenario.Cleanup) == "" {
			return fmt.Errorf("scenario %s requires cleanup", scenario.Name)
		}
		if _, err := time.ParseDuration(scenario.Timeout); err != nil {
			return fmt.Errorf("scenario %s has invalid timeout", scenario.Name)
		}
		if len(scenario.Thresholds) == 0 {
			return fmt.Errorf("scenario %s requires thresholds", scenario.Name)
		}
		if len(scenario.Evidence) == 0 {
			return fmt.Errorf("scenario %s requires evidence collectors", scenario.Name)
		}
	}
	for _, name := range requiredScenarios {
		if !seen[name] {
			return fmt.Errorf("missing required scenario %s", name)
		}
	}
	return nil
}

func runProfile(config Config, profile string, runner commandRunner) Report {
	report := Report{Schema: reportSchema, Profile: profile, GeneratedAt: time.Now().UTC(), Status: "passed"}
	for _, scenario := range config.Scenarios {
		if !contains(scenario.Profiles, profile) {
			continue
		}
		result := runScenario(scenario, runner)
		report.Scenarios = append(report.Scenarios, result)
		if result.Status != "passed" {
			report.Status = "failed"
		}
	}
	if len(report.Scenarios) == 0 {
		report.Status = "failed"
	}
	return report
}

func runScenario(scenario Scenario, runner commandRunner) ScenarioResult {
	result := ScenarioResult{Name: scenario.Name, Status: "passed", StartedAt: time.Now().UTC(), Thresholds: scenario.Thresholds, Evidence: scenario.Evidence}
	timeout, _ := time.ParseDuration(scenario.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	commands := []string{scenario.Setup, scenario.Action, scenario.Assertions}
	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := runner(ctx, command); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			break
		}
	}
	cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cleanupCancel()
	if err := runner(cleanupContext, scenario.Cleanup); err != nil {
		result.Status = "failed"
		result.Error = strings.TrimSpace(result.Error + "; cleanup: " + err.Error())
	}
	if result.Status == "passed" {
		for _, path := range scenario.Evidence {
			info, err := os.Stat(path)
			if err != nil || info.ModTime().Before(result.StartedAt.Add(-time.Second)) {
				result.Status = "failed"
				result.Error = "missing or stale evidence: " + path
				break
			}
		}
	}
	result.CompletedAt = time.Now().UTC()
	return result
}

func shellCommand(ctx context.Context, command string) error {
	cmd := exec.CommandContext(ctx, "bash", "-o", "pipefail", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeReport(path string, report Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
