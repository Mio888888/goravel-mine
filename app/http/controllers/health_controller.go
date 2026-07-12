package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
)

type HealthController struct{}

func NewHealthController() *HealthController {
	return &HealthController{}
}

func (r *HealthController) Live(ctx contractshttp.Context) contractshttp.Response {
	return healthJSON(ctx, http.StatusOK, "live", nil)
}

func (r *HealthController) Ready(ctx contractshttp.Context) contractshttp.Response {
	deps := map[string]string{
		"database": checkDatabase(ctx.Context()),
		"cache":    checkCache(ctx.Context()),
	}
	status := http.StatusOK
	for _, depStatus := range deps {
		if depStatus != "ok" {
			status = http.StatusServiceUnavailable
			break
		}
	}

	return healthJSON(ctx, status, "ready", deps)
}

func checkDatabase(parent context.Context) string {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	db, err := facades.Orm().WithContext(ctx).DB()
	if err != nil {
		return "error"
	}
	if err := db.PingContext(ctx); err != nil {
		return "error"
	}
	return "ok"
}

func checkCache(parent context.Context) string {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	key := "health:ready:" + randomSuffix()
	cache := facades.Cache().WithContext(ctx)
	if err := cache.Put(key, "ok", 5*time.Second); err != nil {
		return "error"
	}
	if cache.GetString(key) != "ok" {
		return "error"
	}
	cache.Forget(key)
	return "ok"
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

func healthJSON(ctx contractshttp.Context, status int, check string, deps map[string]string) contractshttp.Response {
	body := map[string]any{
		"status": "ok",
		"check":  check,
	}
	if status != http.StatusOK {
		body["status"] = "unavailable"
	}
	if deps != nil {
		body["dependencies"] = deps
	}
	return ctx.Response().Json(status, body)
}
