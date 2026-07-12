package modulecatalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"goravel/app/modules"
)

const (
	LifecycleActionInstall   = "install"
	LifecycleActionUpgrade   = "upgrade"
	LifecycleActionRollback  = "rollback"
	LifecycleActionUninstall = "uninstall"

	LifecycleStatusPlanned                = "planned"
	LifecycleStatusRunning                = "running"
	LifecycleStatusSucceeded              = "succeeded"
	LifecycleStatusFailed                 = "failed"
	LifecycleStatusSkipped                = "skipped"
	LifecycleStatusLockBlocked            = "lock_blocked"
	LifecycleStatusManualRequired         = "manual_required"
	LifecycleStatusReconciliationRequired = "reconciliation_required"
)

const maxLifecycleOutput = 8192

const defaultLifecycleRunnerCancelGrace = 25 * time.Millisecond

type LifecycleOptions struct {
	ModuleID                 string
	Execute                  bool
	Owner                    string
	Reason                   string
	ConfirmToken             string
	ReAuthToken              string
	ApprovalID               string
	securityGate             func(context.Context, func() error) error
	emergencyRemovalApproved bool
}

type LifecycleResult struct {
	Action string                `json:"action"`
	DryRun bool                  `json:"dry_run"`
	Owner  string                `json:"owner,omitempty"`
	Reason string                `json:"reason,omitempty"`
	Items  []LifecycleResultItem `json:"items"`
}

type LifecycleResultItem struct {
	ModuleID         string `json:"module_id"`
	Name             string `json:"name"`
	Action           string `json:"action"`
	Status           string `json:"status"`
	Skipped          bool   `json:"skipped,omitempty"`
	Command          string `json:"command"`
	DestructiveCheck string `json:"destructive_check,omitempty"`
	IdempotencyKey   string `json:"idempotency_key"`
	Error            string `json:"error,omitempty"`
}

type LifecycleService struct {
	registry          modules.Registry
	repository        LifecycleRepository
	lockManager       LifecycleLockManager
	commandRunner     LifecycleCommandRunner
	clock             LifecycleClock
	lockTTL           time.Duration
	lockRenewInterval time.Duration
	commandTimeout    time.Duration
	runnerCancelGrace time.Duration
}

type LifecycleLock struct {
	Key       string
	Owner     string
	RunKey    string
	Acquired  bool
	ExpiresAt time.Time
}

type LifecycleRunRecord struct {
	IdempotencyKey string
	ModuleID       string
	Action         string
	FromVersion    string
	ToVersion      string
	Status         string
	DryRun         bool
	Owner          string
	Reason         string
	Command        string
	Plan           string
	Error          string
	started        time.Time
	finished       time.Time
}

type LifecycleStepRecord struct {
	AttemptKey string
	RunKey     string
	ModuleID   string
	Action     string
	StepName   string
	Command    string
	Status     string
	Stdout     string
	Stderr     string
	Error      string
	Started    time.Time
	Finished   time.Time
}

type LifecycleStateRecord struct {
	ModuleID       string
	Name           string
	Version        string
	TargetVersion  string
	Status         string
	Enabled        bool
	Owner          string
	DisabledReason string
	LastAction     string
	LastRunKey     string
	LastError      string
	Metadata       string
	updatedAt      time.Time
}

func NewLifecycleService(registry modules.Registry) *LifecycleService {
	store := NewDBLifecycleStore("")
	return &LifecycleService{
		registry:          registry,
		repository:        store,
		lockManager:       store,
		commandRunner:     artisanLifecycleCommandRunner{},
		clock:             systemLifecycleClock{},
		lockTTL:           5 * time.Minute,
		lockRenewInterval: time.Minute,
		commandTimeout:    10 * time.Minute,
		runnerCancelGrace: defaultLifecycleRunnerCancelGrace,
	}
}

func (s *LifecycleService) SetRunnerForTest(runner func(context.Context, string) error) {
	if runner != nil {
		s.commandRunner = lifecycleCommandRunnerFunc(runner)
	}
}

func (s *LifecycleService) Execute(ctx context.Context, action string, opts LifecycleOptions) (LifecycleResult, error) {
	ctx = contextOrBackground(ctx)
	action = strings.TrimSpace(action)
	if !validLifecycleAction(action) {
		return LifecycleResult{}, fmt.Errorf("unsupported lifecycle action: %s", action)
	}
	opts.Owner = strings.TrimSpace(opts.Owner)
	opts.Reason = strings.TrimSpace(opts.Reason)
	if opts.Execute && (opts.Owner == "" || opts.Reason == "") {
		return LifecycleResult{}, errors.New("module lifecycle execute requires owner and reason")
	}
	planner := newLifecyclePlannerWithReplacements(s.registry.LifecycleStates(), s.registry.ReplacementPlans())
	planner.emergencyRemovalApproved = opts.emergencyRemovalApproved
	if err := planner.validateVersionConstraints(); err != nil {
		return LifecycleResult{}, err
	}

	plan, err := planner.plan(action, opts.ModuleID)
	if err != nil {
		return LifecycleResult{}, err
	}
	if opts.Execute && len(plan.items) > 1 {
		if err := planner.validateBatch(plan); err != nil {
			return LifecycleResult{}, err
		}
	}
	return s.executeLifecyclePlan(ctx, action, opts, plan)
}

func (s *LifecycleService) executeLifecyclePlan(ctx context.Context, action string, opts LifecycleOptions, plan lifecyclePlan) (LifecycleResult, error) {
	result := LifecycleResult{
		Action: action,
		DryRun: !opts.Execute,
		Owner:  opts.Owner,
		Reason: opts.Reason,
	}
	executedCommands := map[string]struct{}{}
	securityGate := lifecycleSecurityGate{callback: opts.securityGate}
	executor := newLifecycleExecutor(s)
	for position, planned := range plan.items {
		state := *planned.state
		item := planned.resultDTO()
		if !state.Enabled {
			item.Status = LifecycleStatusSkipped
			item.Skipped = true
			item.Error = firstLifecycleNonEmpty(state.Reason, "module disabled")
			result.Items = append(result.Items, item)
			continue
		}
		if !opts.Execute {
			item.Status = LifecycleStatusPlanned
			result.Items = append(result.Items, item)
			continue
		}
		executed, err := executor.executeItem(lifecycleItemExecution{
			ctx: ctx, state: state, opts: opts, item: item,
			executedCommands: executedCommands, securityGate: &securityGate,
		})
		result.Items = append(result.Items, executed)
		if err != nil {
			result, err = executor.rollbackReplacement(ctx, opts, plan, position, result, err, executedCommands, &securityGate)
			return result, err
		}
	}

	return result, nil
}

func (e lifecycleExecutor) rollbackReplacement(
	ctx context.Context,
	opts LifecycleOptions,
	plan lifecyclePlan,
	position int,
	result LifecycleResult,
	cause error,
	executedCommands map[string]struct{},
	securityGate *lifecycleSecurityGate,
) (LifecycleResult, error) {
	if plan.rollback == nil || position < plan.rollbackAfter {
		return result, cause
	}
	var runningErr lifecycleCommandStillRunningError
	if errors.As(cause, &runningErr) {
		return result, cause
	}
	rollback := *plan.rollback
	item, rollbackErr := e.executeItem(lifecycleItemExecution{
		ctx: ctx, state: *rollback.state, opts: opts, item: rollback.resultDTO(),
		executedCommands: executedCommands, securityGate: securityGate,
	})
	result.Items = append(result.Items, item)
	return result, errors.Join(cause, rollbackErr)
}
