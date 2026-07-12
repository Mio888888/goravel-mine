package services

import (
	"time"

	"goravel/app/models"
)

func nowTimestamps() models.Timestamps {
	now := time.Now()
	return models.Timestamps{CreatedAt: now, UpdatedAt: now}
}
