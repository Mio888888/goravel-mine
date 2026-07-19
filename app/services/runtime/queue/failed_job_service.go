package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	"github.com/goravel/framework/support/carbon"

	"goravel/app/facades"
	"goravel/app/http/request"
)

type QueueFailedJobFilters struct {
	Connection string   `json:"connection"`
	Queue      string   `json:"queue"`
	UUIDs      []string `json:"uuids"`
}

type QueueFailedJobRow struct {
	UUID       string `json:"uuid"`
	Connection string `json:"connection"`
	Queue      string `json:"queue"`
	Signature  string `json:"signature"`
	FailedAt   string `json:"failed_at"`
}

type QueueFailedJobRetryResult struct {
	Retried int `json:"retried"`
}

type QueueFailedJobDeleteResult struct {
	Deleted int64 `json:"deleted"`
}

type QueueFailer interface {
	All() ([]queueFailedJob, error)
	Get(connection, queue string, uuids []string) ([]queueFailedJob, error)
}

type QueueFailedJob interface {
	UUID() string
	Connection() string
	Queue() string
	Signature() string
	FailedAtString() string
	Retry() error
}

type queueFailer = QueueFailer

type queueFailedJob = QueueFailedJob

type queueFailedJobRecord struct {
	FailedAt   *carbon.DateTime `gorm:"column:failed_at"`
	UUID       string           `gorm:"column:uuid"`
	Connection string           `gorm:"column:connection"`
	Queue      string           `gorm:"column:queue"`
	Payload    string           `gorm:"column:payload"`
	ID         uint             `gorm:"column:id"`
}

type goravelQueueFailer struct {
	failer contractsqueue.Failer
}

type goravelFailedJob struct {
	job contractsqueue.FailedJob
}

type QueueFailedJobService struct {
	ctx    context.Context
	failer queueFailer
}

func NewQueueFailedJobService() *QueueFailedJobService {
	return NewQueueFailedJobServiceWithFailer(goravelQueueFailer{failer: facades.Queue().Failer()})
}

func NewQueueFailedJobServiceWithFailer(failer queueFailer) *QueueFailedJobService {
	return &QueueFailedJobService{failer: failer}
}

func (s *QueueFailedJobService) WithContext(ctx context.Context) *QueueFailedJobService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *QueueFailedJobService) List(filters QueueFailedJobFilters, page, pageSize int) (request.PageResult[QueueFailedJobRow], error) {
	query := s.failedJobQuery(filters).OrderByDesc("id")
	result, err := request.Paginate[queueFailedJobRecord](query, page, pageSize)
	if err != nil {
		return request.PageResult[QueueFailedJobRow]{}, err
	}
	return request.PageResult[QueueFailedJobRow]{List: queueFailedJobRecordRows(result.List), Total: result.Total}, nil
}

func (s *QueueFailedJobService) Retry(filters QueueFailedJobFilters) (QueueFailedJobRetryResult, error) {
	uuids := filters.uuids()
	if len(uuids) == 0 {
		return QueueFailedJobRetryResult{}, BusinessError{Message: "请选择要重试的失败任务"}
	}
	jobs, err := s.failer.Get(filters.connection(), filters.queue(), uuids)
	if err != nil {
		return QueueFailedJobRetryResult{}, err
	}
	if len(jobs) == 0 {
		return QueueFailedJobRetryResult{}, BusinessError{Message: "未找到可重试的失败任务"}
	}
	for _, job := range jobs {
		if err := job.Retry(); err != nil {
			return QueueFailedJobRetryResult{}, err
		}
	}
	return QueueFailedJobRetryResult{Retried: len(jobs)}, nil
}

func (s *QueueFailedJobService) Delete(filters QueueFailedJobFilters) (QueueFailedJobDeleteResult, error) {
	uuids := filters.uuids()
	if len(uuids) == 0 {
		return QueueFailedJobDeleteResult{}, BusinessError{Message: "请选择要丢弃的失败任务"}
	}
	query := s.failedJobQuery(QueueFailedJobFilters{
		Connection: filters.Connection,
		Queue:      filters.Queue,
		UUIDs:      uuids,
	})
	result, err := query.Delete()
	if err != nil {
		return QueueFailedJobDeleteResult{}, err
	}
	return QueueFailedJobDeleteResult{Deleted: result.RowsAffected}, nil
}

func (s *QueueFailedJobService) failedJobQuery(filters QueueFailedJobFilters) contractsorm.Query {
	failedDatabase := facades.Config().GetString("queue.failed.database")
	failedTable := facades.Config().GetString("queue.failed.table", "failed_jobs")
	orm := OrmForConnectionWithContext(s.ctx, failedDatabase)
	query := orm.Query().Table(failedTable)
	if connection := filters.connection(); connection != "" {
		query = query.Where("connection", connection)
	}
	if queue := filters.queue(); queue != "" {
		query = query.Where("queue", queue)
	}
	if uuids := filters.uuids(); len(uuids) > 0 {
		query = query.WhereIn("uuid", stringAny(uuids))
	}
	return query
}

func (f goravelQueueFailer) All() ([]queueFailedJob, error) {
	jobs, err := f.failer.All()
	if err != nil {
		return nil, err
	}
	return wrapFailedJobs(jobs), nil
}

func (f goravelQueueFailer) Get(connection, queue string, uuids []string) ([]queueFailedJob, error) {
	jobs, err := f.failer.Get(connection, queue, uuids)
	if err != nil {
		return nil, err
	}
	return wrapFailedJobs(jobs), nil
}

func wrapFailedJobs(jobs []contractsqueue.FailedJob) []queueFailedJob {
	rows := make([]queueFailedJob, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, goravelFailedJob{job: job})
	}
	return rows
}

func (j goravelFailedJob) UUID() string       { return j.job.UUID() }
func (j goravelFailedJob) Connection() string { return j.job.Connection() }
func (j goravelFailedJob) Queue() string      { return j.job.Queue() }
func (j goravelFailedJob) Signature() string  { return j.job.Signature() }
func (j goravelFailedJob) Retry() error       { return j.job.Retry() }
func (j goravelFailedJob) FailedAtString() string {
	if j.job.FailedAt() == nil {
		return ""
	}
	return fmt.Sprint(j.job.FailedAt())
}

func queueFailedJobRows(jobs []queueFailedJob) []QueueFailedJobRow {
	rows := make([]QueueFailedJobRow, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, QueueFailedJobRow{
			UUID:       job.UUID(),
			Connection: job.Connection(),
			Queue:      job.Queue(),
			Signature:  job.Signature(),
			FailedAt:   job.FailedAtString(),
		})
	}
	return rows
}

func queueFailedJobRecordRows(records []queueFailedJobRecord) []QueueFailedJobRow {
	rows := make([]QueueFailedJobRow, 0, len(records))
	for _, record := range records {
		failedAt := ""
		if record.FailedAt != nil {
			failedAt = fmt.Sprint(record.FailedAt)
		}
		rows = append(rows, QueueFailedJobRow{
			UUID:       record.UUID,
			Connection: record.Connection,
			Queue:      record.Queue,
			Signature:  queueFailedJobPayloadSignature(record.Payload),
			FailedAt:   failedAt,
		})
	}
	return rows
}

func queueFailedJobPayloadSignature(payload string) string {
	var data struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return ""
	}
	return data.Signature
}

func (f QueueFailedJobFilters) connection() string {
	return strings.TrimSpace(f.Connection)
}

func (f QueueFailedJobFilters) queue() string {
	return strings.TrimSpace(f.Queue)
}

func (f QueueFailedJobFilters) uuids() []string {
	out := make([]string, 0, len(f.UUIDs))
	for _, uuid := range f.UUIDs {
		uuid = strings.TrimSpace(uuid)
		if uuid != "" {
			out = append(out, uuid)
		}
	}
	return out
}
