package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/facades"
	"goravel/app/models"
)

type tenantDataExportPayload struct {
	RunID    uint64            `json:"run_id"`
	TenantID uint64            `json:"tenant_id"`
	Dataset  string            `json:"dataset"`
	Format   string            `json:"format"`
	Filters  map[string]string `json:"filters"`
}

type tenantExportUser struct {
	ID        uint64    `gorm:"column:id" json:"id"`
	Username  string    `gorm:"column:username" json:"username"`
	Nickname  string    `gorm:"column:nickname" json:"nickname"`
	Email     string    `gorm:"column:email" json:"email"`
	Phone     string    `gorm:"column:phone" json:"phone"`
	Status    int8      `gorm:"column:status" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func init() {
	RegisterQueueOutboxHandler(tenantDataExportTopic, handleTenantDataExportEvent)
}

func handleTenantDataExportEvent(ctx context.Context, event QueueOutboxEvent) error {
	var payload tenantDataExportPayload
	if json.Unmarshal([]byte(event.Payload), &payload) != nil || payload.RunID == 0 || payload.TenantID == 0 {
		return ErrTenantExportInvalid
	}
	err := NewTenantDataExportWorker().WithContext(ctx).Handle(payload)
	if err != nil {
		markTenantExportFailed(ctx, payload.RunID, err)
	}
	return err
}

type TenantDataExportWorker struct {
	ctx   context.Context
	store TenantExportArtifactStore
}

func NewTenantDataExportWorker() *TenantDataExportWorker {
	return &TenantDataExportWorker{store: configuredTenantExportArtifactStore(context.Background())}
}

func (w *TenantDataExportWorker) WithContext(ctx context.Context) *TenantDataExportWorker {
	clone := *w
	clone.ctx = contextOrBackground(ctx)
	if clone.store == nil {
		clone.store = configuredTenantExportArtifactStore(clone.ctx)
	}
	return &clone
}

func (w *TenantDataExportWorker) Handle(payload tenantDataExportPayload) error {
	run, err := NewTenantDataExportService().WithContext(w.ctx).Status(payload.TenantID, payload.RunID)
	if err != nil || run.Status == models.TenantGovernanceRunStatusCompleted {
		return err
	}
	repository := NewTenantGovernanceRunRepository().WithContext(w.ctx)
	if run.Status == models.TenantGovernanceRunStatusPending {
		if err := repository.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""); err != nil {
			return err
		}
		run.Status = models.TenantGovernanceRunStatusRunning
	}
	if run.Status == models.TenantGovernanceRunStatusFailed {
		if err := resumeTenantExportRun(w.ctx, run.ID); err != nil {
			return err
		}
		run.Status = models.TenantGovernanceRunStatusRunning
	}
	if run.Status == models.TenantGovernanceRunStatusArtifactWritten {
		return repository.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, "")
	}
	if run.Status != models.TenantGovernanceRunStatusRunning || w.store == nil {
		return ErrTenantExportInvalid
	}
	tenant, err := NewTenantService().WithContext(w.ctx).FindByID(payload.TenantID)
	if err != nil {
		return err
	}
	plain, err := exportTenantDataset(w.ctx, tenant, payload)
	if err != nil {
		return err
	}
	encrypted, err := facades.Crypt().EncryptString(string(plain))
	if err != nil {
		return err
	}
	artifact, err := w.store.WriteImmutable(w.ctx, run, []byte(encrypted))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = repository.AttachEvidence(run, TenantGovernanceEvidenceInput{
		URI: artifact.URI, ObjectVersion: artifact.ObjectVersion, SHA256: artifact.SHA256,
		VerifiedAt: now, ExpiresAt: now.Add(24 * time.Hour),
		Metadata: map[string]any{"dataset": payload.Dataset, "format": payload.Format, "encrypted": true},
	})
	if err != nil && !errors.Is(err, ErrTenantGovernanceEvidenceExists) {
		return err
	}
	return repository.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, "")
}

func exportTenantDataset(ctx context.Context, tenant Tenant, payload tenantDataExportPayload) ([]byte, error) {
	if payload.Dataset != "users" || !tenantExportFormatAllowed(payload.Format) {
		return nil, ErrTenantExportInvalid
	}
	rows := make([]tenantExportUser, 0)
	query := OrmForConnectionWithContext(ctx, TenantConnectionName(tenant)).Query().Table("user").
		Select("id", "username", "nickname", "email", "phone", "status", "created_at").OrderBy("id")
	if status := payload.Filters["status"]; status != "" {
		query = query.Where("status", status)
	}
	if err := query.Get(&rows); err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
		return nil, err
	}
	if payload.Format == "csv" {
		return tenantExportCSV(rows)
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func tenantExportCSV(rows []tenantExportUser) ([]byte, error) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"id", "username", "nickname", "email", "phone", "status", "created_at"})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatUint(row.ID, 10), safeTenantExportCSVCell(row.Username), safeTenantExportCSVCell(row.Nickname),
			safeTenantExportCSVCell(row.Email), safeTenantExportCSVCell(row.Phone), strconv.Itoa(int(row.Status)), row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}

func safeTenantExportCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " ")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	default:
		return value
	}
}

func markTenantExportFailed(ctx context.Context, runID uint64, runErr error) {
	if runID == 0 || runErr == nil {
		return
	}
	now := time.Now().UTC()
	_, _ = OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table((models.TenantGovernanceRun{}).TableName()).
		Where("id", runID).Where("kind", models.TenantGovernanceRunKindExport).
		WhereIn("status", []any{models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning}).
		Update(map[string]any{"status": models.TenantGovernanceRunStatusFailed, "error": runErr.Error(), "finished_at": now, "updated_at": now})
}

func resumeTenantExportRun(ctx context.Context, runID uint64) error {
	now := time.Now().UTC()
	result, err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table((models.TenantGovernanceRun{}).TableName()).
		Where("id", runID).Where("kind", models.TenantGovernanceRunKindExport).Where("status", models.TenantGovernanceRunStatusFailed).
		Update(map[string]any{"status": models.TenantGovernanceRunStatusRunning, "error": "", "finished_at": nil, "started_at": now, "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}
