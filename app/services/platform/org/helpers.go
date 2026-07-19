package org

import (
	"time"

	"github.com/goravel/framework/support/collect"

	"goravel/app/models"
	"goravel/app/support/idutil"
)

func nowTimestamps() models.Timestamps {
	now := time.Now()
	return models.Timestamps{CreatedAt: now, UpdatedAt: now}
}

func payloadIDs(values []any, key string) []uint64 {
	return idutil.PayloadIDs(values, key)
}

func uint64Any(values []uint64) []any {
	return collect.Map(values, func(value uint64, _ int) any {
		return value
	})
}
