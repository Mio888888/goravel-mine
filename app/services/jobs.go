package services

import "github.com/goravel/framework/contracts/queue"

func QueueJobs() []queue.Job {
	return []queue.Job{
		&OperationLogJob{},
		&QueueOutboxDispatchJob{},
	}
}
