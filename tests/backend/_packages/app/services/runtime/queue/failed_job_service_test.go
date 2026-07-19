package queue

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueueFailedJobFiltersDropsEmptyUUIDs(t *testing.T) {
	filters := QueueFailedJobFilters{
		Connection: " database ",
		Queue:      " default ",
		UUIDs:      []string{"", " job-a ", "job-b"},
	}

	require.Equal(t, "database", filters.connection())
	require.Equal(t, "default", filters.queue())
	require.Equal(t, []string{"job-a", "job-b"}, filters.uuids())
}

func TestRetryFailedJobsRejectsEmptyUUIDs(t *testing.T) {
	service := NewQueueFailedJobServiceWithFailer(nil)

	_, err := service.Retry(QueueFailedJobFilters{})

	require.Error(t, err)
	require.Equal(t, "请选择要重试的失败任务", err.Error())
}

func TestRetryFailedJobsReturnsRetryCount(t *testing.T) {
	service := NewQueueFailedJobServiceWithFailer(fakeQueueFailer{
		jobs: []queueFailedJob{
			fakeFailedJob{uuid: "job-a"},
			fakeFailedJob{uuid: "job-b"},
		},
	})

	result, err := service.Retry(QueueFailedJobFilters{UUIDs: []string{"job-a", "job-b"}})

	require.NoError(t, err)
	require.Equal(t, 2, result.Retried)
}

func TestRetryFailedJobsStopsOnRetryError(t *testing.T) {
	service := NewQueueFailedJobServiceWithFailer(fakeQueueFailer{
		jobs: []queueFailedJob{
			fakeFailedJob{uuid: "job-a", err: errors.New("push failed")},
		},
	})

	_, err := service.Retry(QueueFailedJobFilters{UUIDs: []string{"job-a"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "push failed")
}

type fakeQueueFailer struct {
	jobs []queueFailedJob
	err  error
}

func (f fakeQueueFailer) Get(connection, queue string, uuids []string) ([]queueFailedJob, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.jobs, nil
}

func (f fakeQueueFailer) All() ([]queueFailedJob, error) {
	return f.jobs, f.err
}

type fakeFailedJob struct {
	uuid string
	err  error
}

func (f fakeFailedJob) UUID() string       { return f.uuid }
func (f fakeFailedJob) Connection() string { return "database" }
func (f fakeFailedJob) Queue() string      { return "default" }
func (f fakeFailedJob) Signature() string  { return "operation_log" }
func (f fakeFailedJob) FailedAtString() string {
	return "2026-07-06 12:00:00"
}
func (f fakeFailedJob) Retry() error { return f.err }
