package security

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

var ErrSecurityAuditPruneMode = errors.New("choose exactly one of --dry-run or --execute")

type SecurityAuditPruneInput struct {
	DryRun        bool
	Execute       bool
	Scope         string
	RetentionDays int
	Format        string
	PlanID        string
	ProofFile     string
	EvidenceStdin bool
	ArchiveOutput string
}

type SecurityAuditPruneCore struct {
	plans     *services.AuditPrunePlanService
	executor  *services.AuditPruneExecutor
	readStdin func() ([]byte, error)
	readProof func(string) (services.AuditPruneWORMProof, error)
}

func NewSecurityAuditPruneCore() *SecurityAuditPruneCore {
	return &SecurityAuditPruneCore{
		plans:     services.NewAuditPrunePlanService(),
		executor:  services.NewAuditPruneExecutor(),
		readStdin: readSecurityAuditPruneEvidence,
		readProof: services.ReadAuditPruneProofFile,
	}
}

func (c *SecurityAuditPruneCore) WithContext(ctx context.Context) *SecurityAuditPruneCore {
	clone := *c
	clone.plans = clone.plans.WithContext(ctx)
	clone.executor = clone.executor.WithContext(ctx)
	return &clone
}

func (c *SecurityAuditPruneCore) Run(input SecurityAuditPruneInput) (any, error) {
	if err := validateSecurityAuditPruneInput(input); err != nil {
		return nil, err
	}
	if !input.Execute {
		input.DryRun = true
	}
	if input.DryRun {
		plan, err := c.plans.Create(services.AuditPrunePlanOptions{Scope: input.Scope, RetentionDays: input.RetentionDays})
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(input.ArchiveOutput) != "" {
			if err := writeAuditPruneArchive(input.ArchiveOutput, services.AuditPruneArchiveManifestForPlan(plan, time.Time{}, time.Time{})); err != nil {
				return nil, err
			}
		}
		return plan, nil
	}
	proof, err := c.readProof(input.ProofFile)
	if err != nil {
		return nil, err
	}
	payload, err := c.readStdin()
	if err != nil {
		return nil, err
	}
	var evidence services.AuditPruneEvidence
	if err := json.Unmarshal(payload, &evidence); err != nil {
		return nil, err
	}
	return c.executor.Execute(input.PlanID, proof, evidence)
}

func validateSecurityAuditPruneInput(input SecurityAuditPruneInput) error {
	if input.DryRun && input.Execute {
		return ErrSecurityAuditPruneMode
	}
	if !input.Execute {
		return nil
	}
	if strings.TrimSpace(input.PlanID) == "" || strings.TrimSpace(input.ProofFile) == "" || !input.EvidenceStdin {
		return services.ErrAuditPruneEvidenceRequired
	}
	return nil
}

func readSecurityAuditPruneEvidence() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

type SecurityAuditPruneCommand struct{}

func (r *SecurityAuditPruneCommand) Signature() string {
	return "security:audit-prune"
}

func (r *SecurityAuditPruneCommand) Description() string {
	return "Create a WORM-backed audit prune plan or execute a verified plan"
}

func (r *SecurityAuditPruneCommand) Extend() command.Extend {
	return command.Extend{
		Category: "security",
		Flags: []command.Flag{
			&command.BoolFlag{Name: "dry-run", Usage: "Persist a prune plan without deleting rows"},
			&command.BoolFlag{Name: "execute", Usage: "Execute a persisted plan after proof and stdin evidence validation"},
			&command.StringFlag{Name: "scope", Usage: "all, platform, or tenant:<code>", Value: "all"},
			&command.IntFlag{Name: "retention-days", Usage: "Fallback retention days when no tenant governance policy applies"},
			&command.StringFlag{Name: "format", Usage: "Output format: json", Value: "json"},
			&command.StringFlag{Name: "plan-id", Usage: "Persisted prune plan id"},
			&command.StringFlag{Name: "proof-file", Usage: "Verified WORM proof JSON file"},
			&command.BoolFlag{Name: "evidence-stdin", Usage: "Read sensitive operation evidence JSON from stdin"},
			&command.StringFlag{Name: "archive-output", Usage: "Write upload-ready archive manifest during dry-run"},
		},
	}
}

func (r *SecurityAuditPruneCommand) Handle(ctx console.Context) error {
	input := SecurityAuditPruneInput{
		DryRun: ctx.OptionBool("dry-run"), Execute: ctx.OptionBool("execute"), Scope: ctx.Option("scope"),
		RetentionDays: ctx.OptionInt("retention-days"), Format: ctx.Option("format"), PlanID: ctx.Option("plan-id"),
		ProofFile: ctx.Option("proof-file"), EvidenceStdin: ctx.OptionBool("evidence-stdin"),
		ArchiveOutput: ctx.Option("archive-output"),
	}
	if !input.Execute {
		input.DryRun = true
	}
	result, err := NewSecurityAuditPruneCore().WithContext(context.Background()).Run(input)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Line(string(payload))
	return nil
}

func writeAuditPruneArchive(path string, manifest services.AuditPruneArchiveManifest) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".audit-prune-archive-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
