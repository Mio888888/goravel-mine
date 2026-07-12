package modulecatalog

import "time"

type AdminRunRow struct {
	ID             uint64     `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	ModuleID       string     `json:"module_id"`
	Action         string     `json:"action"`
	FromVersion    string     `json:"from_version"`
	ToVersion      string     `json:"to_version"`
	Status         string     `json:"status"`
	DryRun         bool       `json:"dry_run"`
	Owner          string     `json:"owner"`
	Reason         string     `json:"reason"`
	Command        string     `json:"command"`
	Error          string     `json:"error"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      *time.Time `json:"created_at"`
}

type AdminStepRow struct {
	ID         uint64     `json:"id"`
	AttemptKey string     `json:"attempt_key"`
	RunKey     string     `json:"run_key"`
	ModuleID   string     `json:"module_id"`
	Action     string     `json:"action"`
	StepName   string     `json:"step_name"`
	Command    string     `json:"command"`
	Status     string     `json:"status"`
	Stdout     string     `json:"stdout"`
	Stderr     string     `json:"stderr"`
	Error      string     `json:"error"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  *time.Time `json:"created_at"`
}

type AdminLockRow struct {
	ID        uint64     `json:"id"`
	Key       string     `json:"key"`
	Owner     string     `json:"owner"`
	RunKey    string     `json:"run_key"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type AdminStateDiffItem struct {
	ModuleID         string `json:"module_id"`
	Name             string `json:"name"`
	ManifestVersion  string `json:"manifest_version"`
	PersistedVersion string `json:"persisted_version"`
	ManifestEnabled  bool   `json:"manifest_enabled"`
	PersistedEnabled bool   `json:"persisted_enabled"`
	PersistedStatus  string `json:"persisted_status"`
	LastAction       string `json:"last_action"`
	Drift            string `json:"drift"`
}

type AdminLockReleasePayload struct {
	Key          string `json:"key"`
	DryRun       bool   `json:"dry_run"`
	ConfirmToken string `json:"confirm_token"`
	ReAuthToken  string `json:"reauth_token"`
	ApprovalID   string `json:"approval_id"`
	OperatorID   uint64 `json:"-"`
}

type AdminLockReleaseResult struct {
	DryRun   bool           `json:"dry_run"`
	Released []AdminLockRow `json:"released"`
}

type AdminExecutePayload struct {
	Action       string `json:"action"`
	ModuleID     string `json:"module_id"`
	Execute      bool   `json:"execute"`
	Owner        string `json:"owner"`
	Reason       string `json:"reason"`
	ConfirmToken string `json:"confirm_token"`
	ReAuthToken  string `json:"reauth_token"`
	ApprovalID   string `json:"approval_id"`
	OperatorID   uint64 `json:"-"`
}
