package modulecatalog

import (
	"fmt"
	"strings"
)

func firstInvalidLifecycleCommand(commands string) (string, error) {
	for _, command := range normalizeLifecycleCommands(commands) {
		if err := validateLifecycleCommand(command); err != nil {
			return command, err
		}
	}
	return "", nil
}

func validateLifecycleCommand(command string) error {
	if isManualLifecycleCommand(command) {
		return manualLifecycleCommandError{Command: command}
	}
	if !isAllowedLifecycleCommand(command) {
		return fmt.Errorf("module lifecycle command not allowed: %s", command)
	}
	return nil
}

func normalizeLifecycleCommands(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	parts := strings.Split(command, "&&")
	commands := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "go run . artisan ")
		part = strings.TrimPrefix(part, "./artisan ")
		part = strings.TrimPrefix(part, "artisan ")
		if part = strings.TrimSpace(part); part != "" {
			commands = append(commands, part)
		}
	}
	return commands
}

func isManualLifecycleCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	return command == "manual" || strings.HasPrefix(command, "manual ")
}

func isAllowedLifecycleCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}
	name := strings.Fields(command)[0]
	switch name {
	case "module:manifest:check", "migrate", "migrate:rollback", "tenant:migrate", "db:seed",
		"reference-case:upgrade", "reference-case:rollback":
		return true
	default:
		return false
	}
}

type manualLifecycleCommandError struct {
	Command string
}

func (e manualLifecycleCommandError) Error() string {
	return "manual lifecycle step requires operator action: " + e.Command
}
