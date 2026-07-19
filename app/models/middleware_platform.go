package models

import "time"

type MiddlewareAdapter struct {
	ID                uint64     `gorm:"column:id;primaryKey" json:"id"`
	AdapterKey        string     `gorm:"column:adapter_key" json:"adapter_key"`
	Name              string     `gorm:"column:name" json:"name"`
	AdapterType       string     `gorm:"column:adapter_type" json:"adapter_type"`
	Connection        string     `gorm:"column:connection" json:"connection"`
	Capabilities      JSONMap    `gorm:"column:capabilities;type:jsonb" json:"capabilities"`
	ConfigEncrypted   string     `gorm:"column:config_encrypted" json:"-"`
	ConfigFingerprint string     `gorm:"column:config_fingerprint" json:"config_fingerprint,omitempty"`
	Enabled           bool       `gorm:"column:enabled" json:"enabled"`
	HealthStatus      string     `gorm:"column:health_status" json:"health_status"`
	LastCheckedAt     *time.Time `gorm:"column:last_checked_at" json:"last_checked_at"`
	Version           int        `gorm:"column:version" json:"version"`
	AuditColumns
	Timestamps
	Configured bool `gorm:"-" json:"configured"`
}

func (MiddlewareAdapter) TableName() string {
	return "middleware_adapter"
}

type MessageRoute struct {
	ID               uint64     `gorm:"column:id;primaryKey" json:"id"`
	Name             string     `gorm:"column:name" json:"name"`
	MessageType      string     `gorm:"column:message_type" json:"message_type"`
	AdapterID        uint64     `gorm:"column:adapter_id" json:"adapter_id"`
	Destination      string     `gorm:"column:destination" json:"destination"`
	ConsumptionMode  string     `gorm:"column:consumption_mode" json:"consumption_mode"`
	ConsumerGroup    string     `gorm:"column:consumer_group" json:"consumer_group"`
	Concurrency      int        `gorm:"column:concurrency" json:"concurrency"`
	OrderingEnabled  bool       `gorm:"column:ordering_enabled" json:"ordering_enabled"`
	RetryPolicy      JSONMap    `gorm:"column:retry_policy;type:jsonb" json:"retry_policy"`
	DeadLetterPolicy JSONMap    `gorm:"column:dead_letter_policy;type:jsonb" json:"dead_letter_policy"`
	Status           string     `gorm:"column:status" json:"status"`
	Enabled          bool       `gorm:"column:enabled" json:"enabled"`
	Version          int        `gorm:"column:version" json:"version"`
	PublishedAt      *time.Time `gorm:"column:published_at" json:"published_at"`
	AuditColumns
	Timestamps
	Adapter *MiddlewareAdapter `gorm:"-" json:"adapter,omitempty"`
}

func (MessageRoute) TableName() string {
	return "message_route"
}

type MessageDelivery struct {
	ID               uint64     `gorm:"column:id;primaryKey" json:"id"`
	MessageID        string     `gorm:"column:message_id" json:"message_id"`
	MessageType      string     `gorm:"column:message_type" json:"message_type"`
	ConsumerKey      string     `gorm:"column:consumer_key" json:"consumer_key"`
	RouteID          uint64     `gorm:"column:route_id" json:"route_id"`
	AdapterID        uint64     `gorm:"column:adapter_id" json:"adapter_id"`
	Status           string     `gorm:"column:status" json:"status"`
	Attempt          int        `gorm:"column:attempt" json:"attempt"`
	ReceivedAt       *time.Time `gorm:"column:received_at" json:"received_at"`
	FinishedAt       *time.Time `gorm:"column:finished_at" json:"finished_at"`
	DurationMS       int        `gorm:"column:duration_ms" json:"duration_ms"`
	CorrelationID    string     `gorm:"column:correlation_id" json:"correlation_id"`
	ExternalPosition string     `gorm:"column:external_position" json:"external_position"`
	ErrorSummary     string     `gorm:"column:error_summary" json:"error_summary"`
	Timestamps
}

func (MessageDelivery) TableName() string {
	return "message_delivery"
}

type MessageDeadLetter struct {
	ID                uint64     `gorm:"column:id;primaryKey" json:"id"`
	MessageID         string     `gorm:"column:message_id" json:"message_id"`
	MessageType       string     `gorm:"column:message_type" json:"message_type"`
	ConsumerKey       string     `gorm:"column:consumer_key" json:"consumer_key"`
	RouteID           uint64     `gorm:"column:route_id" json:"route_id"`
	AdapterID         uint64     `gorm:"column:adapter_id" json:"adapter_id"`
	Envelope          JSONMap    `gorm:"column:envelope;type:jsonb" json:"envelope,omitempty"`
	EnvelopeEncrypted string     `gorm:"column:envelope_encrypted" json:"-"`
	FailureClass      string     `gorm:"column:failure_class" json:"failure_class"`
	ErrorSummary      string     `gorm:"column:error_summary" json:"error_summary"`
	FirstFailedAt     *time.Time `gorm:"column:first_failed_at" json:"first_failed_at"`
	LastFailedAt      *time.Time `gorm:"column:last_failed_at" json:"last_failed_at"`
	ReplayCount       int        `gorm:"column:replay_count" json:"replay_count"`
	ResolutionStatus  string     `gorm:"column:resolution_status" json:"resolution_status"`
	ResolvedBy        uint64     `gorm:"column:resolved_by" json:"resolved_by"`
	ResolvedAt        *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	Timestamps
}

func (MessageDeadLetter) TableName() string {
	return "message_dead_letter"
}

type ProtectionRuleSet struct {
	ID               uint64     `gorm:"column:id;primaryKey" json:"id"`
	Name             string     `gorm:"column:name" json:"name"`
	Scope            string     `gorm:"column:scope" json:"scope"`
	ResourcePattern  string     `gorm:"column:resource_pattern" json:"resource_pattern"`
	Rules            JSONMap    `gorm:"column:rules;type:jsonb" json:"rules"`
	Status           string     `gorm:"column:status" json:"status"`
	Enabled          bool       `gorm:"column:enabled" json:"enabled"`
	Version          int        `gorm:"column:version" json:"version"`
	PublishedVersion int        `gorm:"column:published_version" json:"published_version"`
	PublishedAt      *time.Time `gorm:"column:published_at" json:"published_at"`
	AuditColumns
	Timestamps
}

func (ProtectionRuleSet) TableName() string {
	return "protection_rule_set"
}

type ProtectionRuleVersion struct {
	ID              uint64    `gorm:"column:id;primaryKey" json:"id"`
	RuleSetID       uint64    `gorm:"column:rule_set_id" json:"rule_set_id"`
	Version         int       `gorm:"column:version" json:"version"`
	Name            string    `gorm:"column:name" json:"name"`
	Scope           string    `gorm:"column:scope" json:"scope"`
	ResourcePattern string    `gorm:"column:resource_pattern" json:"resource_pattern"`
	Rules           JSONMap   `gorm:"column:rules;type:jsonb" json:"rules"`
	Enabled         bool      `gorm:"column:enabled" json:"enabled"`
	PublishedBy     uint64    `gorm:"column:published_by" json:"published_by"`
	PublishedAt     time.Time `gorm:"column:published_at" json:"published_at"`
	Timestamps
}

func (ProtectionRuleVersion) TableName() string {
	return "protection_rule_version"
}
