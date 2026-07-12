package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/goravel/framework/contracts/console/command"
	"github.com/stretchr/testify/require"

	"goravel/app/modulecatalog"
	"goravel/tests/testsupport"
)

type governanceCommand interface {
	Signature() string
	Description() string
	Extend() command.Extend
}

type cliContract struct {
	Signature   string         `json:"signature"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Flags       []flagContract `json:"flags"`
}

type flagContract struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  any    `json:"default,omitempty"`
}

func TestModuleGovernanceCLIContract(t *testing.T) {
	contracts := make([]cliContract, 0, 5)
	for _, item := range []governanceCommand{
		&ModuleCompatibilityExportCommand{},
		&ModuleLifecycleCommand{},
		&ModuleManifestExportCommand{},
		&ModulePlanCommand{},
		&ModuleStateCommand{},
	} {
		extend := item.Extend()
		contracts = append(contracts, cliContract{
			Signature:   item.Signature(),
			Description: item.Description(),
			Category:    extend.Category,
			Flags:       governanceFlags(t, extend.Flags),
		})
	}

	requireGovernanceCLIGoldenJSON(t, "module-governance-cli", contracts)
}

func TestLifecycleResultWriter(t *testing.T) {
	output, err := captureLifecycleWriterOutput(modulecatalog.LifecycleResult{
		Action: modulecatalog.LifecycleActionUpgrade,
		DryRun: true,
		Items: []modulecatalog.LifecycleResultItem{{
			ModuleID:       "platform-rbac",
			Name:           "Platform RBAC",
			Action:         modulecatalog.LifecycleActionUpgrade,
			Status:         modulecatalog.LifecycleStatusPlanned,
			Command:        "go run . artisan migrate",
			IdempotencyKey: "platform-rbac:upgrade",
		}},
	})
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(output, "\n"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &payload))
	require.Equal(t, []string{"action", "dry_run", "items"}, sortedMapKeys(payload))

	items, ok := payload["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Equal(t, []string{"action", "command", "idempotency_key", "module_id", "name", "status"},
		sortedMapKeys(items[0].(map[string]any)))
}

func governanceFlags(t *testing.T, flags []command.Flag) []flagContract {
	t.Helper()

	items := make([]flagContract, 0, len(flags))
	for _, flag := range flags {
		switch typed := flag.(type) {
		case *command.StringFlag:
			item := flagContract{Name: typed.Name, Type: typed.Type(), Required: typed.Required}
			if typed.Value != "" {
				item.Default = typed.Value
			}
			items = append(items, item)
		case *command.BoolFlag:
			item := flagContract{Name: typed.Name, Type: typed.Type(), Required: typed.Required}
			if typed.Value {
				item.Default = typed.Value
			}
			items = append(items, item)
		default:
			t.Fatalf("unsupported flag type %T", flag)
		}
	}
	return items
}

func captureLifecycleWriterOutput(result modulecatalog.LifecycleResult) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	stdout := os.Stdout
	os.Stdout = writer
	writeErr := writeLifecycleResult(result)
	_ = writer.Close()
	os.Stdout = stdout
	if writeErr != nil {
		return "", writeErr
	}

	payload, err := io.ReadAll(reader)
	return string(payload), err
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func requireGovernanceCLIGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.json")
	testsupport.RequireGoldenJSON(t, path, value)
}
