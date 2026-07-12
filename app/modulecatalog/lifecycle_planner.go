package modulecatalog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"goravel/app/modules"
)

type lifecyclePlanner struct {
	states                   []modules.ModuleState
	replacements             map[string]modules.ReplacementPlan
	now                      func() time.Time
	emergencyRemovalApproved bool
}

type lifecyclePlan struct {
	action        string
	items         []lifecyclePlanItem
	rollback      *lifecyclePlanItem
	rollbackAfter int
}

type lifecyclePlanItem struct {
	state            *modules.ModuleState
	action           string
	command          string
	destructiveCheck string
	idempotencyKey   string
}

func newLifecyclePlanner(states []modules.ModuleState) lifecyclePlanner {
	return newLifecyclePlannerWithReplacements(states, nil)
}

func newLifecyclePlannerWithReplacements(states []modules.ModuleState, replacements map[string]modules.ReplacementPlan) lifecyclePlanner {
	return lifecyclePlanner{states: states, replacements: replacements, now: time.Now}
}

func validLifecycleAction(action string) bool {
	switch action {
	case LifecycleActionInstall, LifecycleActionUpgrade, LifecycleActionRollback, LifecycleActionUninstall:
		return true
	default:
		return false
	}
}

func (p lifecyclePlanner) plan(action string, moduleID string) (lifecyclePlan, error) {
	if !validLifecycleAction(action) {
		return lifecyclePlan{}, fmt.Errorf("unsupported lifecycle action: %s", action)
	}
	states, err := p.orderedStates(action, moduleID)
	if err != nil {
		return lifecyclePlan{}, err
	}
	items := make([]lifecyclePlanItem, 0, len(states))
	for _, state := range states {
		items = append(items, newLifecyclePlanItem(state, action))
	}
	if action == LifecycleActionUninstall && strings.TrimSpace(moduleID) != "" {
		return p.replacementRemovalPlan(moduleID, items)
	}
	return lifecyclePlan{action: action, items: items}, nil
}

func (p lifecyclePlanner) replacementRemovalPlan(moduleID string, items []lifecyclePlanItem) (lifecyclePlan, error) {
	plan, ok := p.replacements[strings.TrimSpace(moduleID)]
	if !ok {
		return lifecyclePlan{action: LifecycleActionUninstall, items: items}, nil
	}
	now := time.Now
	if p.now != nil {
		now = p.now
	}
	if err := plan.Validate(moduleID, plan.ToModule, now()); err != nil {
		return lifecyclePlan{}, err
	}
	if err := plan.CanRemove(now(), p.emergencyRemovalApproved); err != nil {
		return lifecyclePlan{}, err
	}
	for index := range p.states {
		if p.states[index].ID == plan.ToModule {
			target := newLifecyclePlanItem(&p.states[index], LifecycleActionInstall)
			replacementItems, rollback, cutoverIndex := replacementLifecycleItems(&p.states[index], plan)
			ordered := append([]lifecyclePlanItem{target}, replacementItems...)
			ordered = append(ordered, items...)
			return lifecyclePlan{action: LifecycleActionUninstall, items: ordered, rollback: &rollback, rollbackAfter: 1 + cutoverIndex}, nil
		}
	}
	return lifecyclePlan{}, fmt.Errorf("replacement target module not found: %s", plan.ToModule)
}

func replacementLifecycleItems(state *modules.ModuleState, plan modules.ReplacementPlan) ([]lifecyclePlanItem, lifecyclePlanItem, int) {
	commands := plan.ForwardCommands()
	items := make([]lifecyclePlanItem, 0, len(commands))
	cutoverIndex := -1
	for _, command := range commands {
		action := "replacement:" + string(command.Phase) + ":" + command.Name
		items = append(items, lifecyclePlanItem{
			state: state, action: action, command: command.Command,
			idempotencyKey: lifecycleIdempotencyKey(action, state.ID, state.Metadata.Version),
		})
		if command.Phase == modules.ReplacementPhaseCutover {
			cutoverIndex = len(items) - 1
		}
	}
	rollbackCommand := plan.RollbackCommand()
	rollbackAction := "replacement:" + string(rollbackCommand.Phase) + ":" + rollbackCommand.Name
	rollback := lifecyclePlanItem{
		state: state, action: rollbackAction, command: rollbackCommand.Command,
		idempotencyKey: lifecycleIdempotencyKey(rollbackAction, state.ID, state.Metadata.Version),
	}
	return items, rollback, cutoverIndex
}

func (p lifecyclePlanner) validateVersionConstraints() error {
	return validateLifecycleVersionConstraints(p.states)
}

func (p lifecyclePlanner) validateBatch(plan lifecyclePlan) error {
	for _, item := range plan.items {
		if !item.state.Enabled {
			continue
		}
		if err := validateLifecycleBatchItem(item); err != nil {
			return err
		}
	}
	return nil
}

func validateLifecycleBatchItem(item lifecyclePlanItem) error {
	for _, commands := range []string{item.destructiveCheck, item.command} {
		command, err := firstInvalidLifecycleCommand(commands)
		if err == nil {
			continue
		}
		var manualErr manualLifecycleCommandError
		if errors.As(err, &manualErr) {
			return fmt.Errorf("module lifecycle batch contains manual step for %s: %w", item.state.ID, err)
		}
		return fmt.Errorf("module lifecycle batch command not allowed for %s: %s", item.state.ID, command)
	}
	return nil
}

func (p lifecyclePlanner) orderedStates(action string, moduleID string) ([]*modules.ModuleState, error) {
	moduleID = strings.TrimSpace(moduleID)
	if moduleID != "" {
		return p.filteredState(moduleID, reversesLifecycleOrder(action))
	}
	items := make([]*modules.ModuleState, 0, len(p.states))
	if reversesLifecycleOrder(action) {
		for index := len(p.states) - 1; index >= 0; index-- {
			items = append(items, &p.states[index])
		}
		return items, nil
	}
	for index := range p.states {
		items = append(items, &p.states[index])
	}
	return items, nil
}

func (p lifecyclePlanner) filteredState(moduleID string, reverse bool) ([]*modules.ModuleState, error) {
	if reverse {
		for index := len(p.states) - 1; index >= 0; index-- {
			if p.states[index].ID == moduleID {
				return []*modules.ModuleState{&p.states[index]}, nil
			}
		}
		return nil, fmt.Errorf("module not found: %s", moduleID)
	}
	for index := range p.states {
		if p.states[index].ID == moduleID {
			return []*modules.ModuleState{&p.states[index]}, nil
		}
	}
	return nil, fmt.Errorf("module not found: %s", moduleID)
}

func reversesLifecycleOrder(action string) bool {
	return action == LifecycleActionRollback || action == LifecycleActionUninstall
}

func newLifecyclePlanItem(state *modules.ModuleState, action string) lifecyclePlanItem {
	return lifecyclePlanItem{
		state:            state,
		action:           action,
		command:          lifecycleCommand(state.Metadata.Lifecycle, action),
		destructiveCheck: state.Metadata.Lifecycle.DestructiveCheck,
		idempotencyKey:   lifecycleIdempotencyKey(action, state.ID, state.Metadata.Version),
	}
}

func (item lifecyclePlanItem) planDTO() LifecyclePlanItem {
	return LifecyclePlanItem{
		ID: item.state.ID, Name: item.state.Metadata.Name, Action: item.action,
		Enabled: item.state.Enabled, Reason: item.state.Reason, Command: item.command,
		DestructiveCheck:     item.destructiveCheck,
		RequiresRestart:      item.state.Metadata.Lifecycle.RequiresRestart,
		SupportsHotDisable:   item.state.Metadata.Lifecycle.SupportsHotDisable,
		BreakingChangePolicy: item.state.Metadata.Lifecycle.BreakingChangePolicy,
	}
}

func (item lifecyclePlanItem) resultDTO() LifecycleResultItem {
	return LifecycleResultItem{
		ModuleID: item.state.ID, Name: item.state.Metadata.Name, Action: item.action,
		Command: item.command, DestructiveCheck: item.destructiveCheck, IdempotencyKey: item.idempotencyKey,
	}
}

func lifecycleCommand(lifecycle modules.Lifecycle, action string) string {
	switch action {
	case LifecycleActionInstall:
		return lifecycle.Install
	case LifecycleActionUpgrade:
		return lifecycle.Upgrade
	case LifecycleActionRollback:
		return lifecycle.Rollback
	case LifecycleActionUninstall:
		return lifecycle.Uninstall
	default:
		return ""
	}
}

func lifecycleIdempotencyKey(action, moduleID, targetVersion string) string {
	return strings.Join([]string{
		strings.TrimSpace(action), strings.TrimSpace(moduleID), strings.TrimSpace(targetVersion),
	}, ":")
}
