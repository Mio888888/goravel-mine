package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	ReplacementPhasePrepare        ReplacementPhase = "prepare"
	ReplacementPhaseDualRun        ReplacementPhase = "dual_run"
	ReplacementPhaseCutover        ReplacementPhase = "cutover"
	ReplacementPhaseRollbackWindow ReplacementPhase = "rollback_window"
	ReplacementPhaseRetired        ReplacementPhase = "retired"
)

type ReplacementPhase string

type ReplacementPlan struct {
	FromModule          string             `json:"from_module"`
	ToModule            string             `json:"to_module"`
	DataMigration       string             `json:"data_migration"`
	ConfigMigration     string             `json:"config_migration"`
	PermissionMapping   string             `json:"permission_mapping"`
	Validation          string             `json:"validation"`
	Cutover             string             `json:"cutover"`
	Rollback            string             `json:"rollback"`
	AnnouncedAt         time.Time          `json:"announced_at"`
	EndOfSupport        time.Time          `json:"end_of_support"`
	RemovalVersion      string             `json:"removal_version"`
	Phases              []ReplacementPhase `json:"phases"`
	CommandPolicyHashes map[string]string  `json:"command_policy_hashes"`
}

type ReplacementPlanProvider interface {
	ReplacementPlan() ReplacementPlan
}

type ReplacementCommand struct {
	Phase      ReplacementPhase `json:"phase"`
	Name       string           `json:"name"`
	Command    string           `json:"command"`
	PolicyHash string           `json:"policy_hash"`
}

func (r Registry) ReplacementPlans() map[string]ReplacementPlan {
	plans := make(map[string]ReplacementPlan)
	for _, module := range r.kernel.sourceModules() {
		plan, ok := moduleReplacementPlan(module)
		if ok {
			plans[module.ID()] = plan
		}
	}
	return plans
}

func moduleReplacementPlan(module Module) (ReplacementPlan, bool) {
	provider, ok := module.(ReplacementPlanProvider)
	if !ok {
		return ReplacementPlan{}, false
	}
	return provider.ReplacementPlan(), true
}

func (p ReplacementPlan) Validate(moduleID, replacementID string, now time.Time) error {
	if strings.TrimSpace(p.FromModule) != strings.TrimSpace(moduleID) || strings.TrimSpace(p.ToModule) != strings.TrimSpace(replacementID) {
		return fmt.Errorf("replacement plan module binding is invalid")
	}
	if !packageVersionPattern.MatchString(strings.TrimSpace(p.RemovalVersion)) {
		return fmt.Errorf("replacement plan removal version is invalid")
	}
	if p.AnnouncedAt.IsZero() || p.EndOfSupport.IsZero() || !p.EndOfSupport.After(p.AnnouncedAt) {
		return fmt.Errorf("replacement plan end of support is invalid")
	}
	if err := p.validatePhases(); err != nil {
		return err
	}
	return p.validateCommandHashes()
}

func (p ReplacementPlan) validatePhases() error {
	required := []ReplacementPhase{ReplacementPhasePrepare, ReplacementPhaseDualRun, ReplacementPhaseCutover, ReplacementPhaseRollbackWindow, ReplacementPhaseRetired}
	if len(p.Phases) != len(required) {
		return fmt.Errorf("replacement plan phases are incomplete")
	}
	for position, phase := range required {
		if p.Phases[position] != phase {
			return fmt.Errorf("replacement plan phases must follow prepare, dual_run, cutover, rollback_window, retired")
		}
	}
	return nil
}

func (p ReplacementPlan) validateCommandHashes() error {
	commands := p.commands()
	if len(p.CommandPolicyHashes) != len(commands) {
		return fmt.Errorf("replacement plan command policy hashes are incomplete")
	}
	for name, command := range commands {
		if strings.TrimSpace(command) == "" || p.CommandPolicyHashes[name] != replacementCommandPolicyHash(command) {
			return fmt.Errorf("replacement plan command policy hash mismatch: %s", name)
		}
	}
	return nil
}

func (p ReplacementPlan) commands() map[string]string {
	return map[string]string{
		"data_migration": p.DataMigration, "config_migration": p.ConfigMigration,
		"permission_mapping": p.PermissionMapping, "validation": p.Validation,
		"cutover": p.Cutover, "rollback": p.Rollback,
	}
}

func (p ReplacementPlan) CommandHashes() map[string]string {
	commands := p.commands()
	keys := make([]string, 0, len(commands))
	for name := range commands {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	hashes := make(map[string]string, len(commands))
	for _, name := range keys {
		hashes[name] = replacementCommandPolicyHash(commands[name])
	}
	return hashes
}

func (p ReplacementPlan) ForwardCommands() []ReplacementCommand {
	return []ReplacementCommand{
		p.replacementCommand(ReplacementPhasePrepare, "data_migration", p.DataMigration),
		p.replacementCommand(ReplacementPhasePrepare, "config_migration", p.ConfigMigration),
		p.replacementCommand(ReplacementPhaseDualRun, "permission_mapping", p.PermissionMapping),
		p.replacementCommand(ReplacementPhaseDualRun, "validation", p.Validation),
		p.replacementCommand(ReplacementPhaseCutover, "cutover", p.Cutover),
	}
}

func (p ReplacementPlan) RollbackCommand() ReplacementCommand {
	return p.replacementCommand(ReplacementPhaseRollbackWindow, "rollback", p.Rollback)
}

func (p ReplacementPlan) replacementCommand(phase ReplacementPhase, name, command string) ReplacementCommand {
	return ReplacementCommand{Phase: phase, Name: name, Command: command, PolicyHash: p.CommandPolicyHashes[name]}
}

func (p ReplacementPlan) CanRemove(now time.Time, emergencyApproved bool) error {
	if emergencyApproved || !now.Before(p.EndOfSupport) {
		return nil
	}
	return fmt.Errorf("replacement removal is blocked before end of support: %s", p.EndOfSupport.UTC().Format(time.RFC3339))
}

func replacementCommandPolicyHash(command string) string {
	canonical := strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:])
}
