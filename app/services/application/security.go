package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	frameworkcache "github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"
	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/support/digest"
	"goravel/app/support/idutil"
	"goravel/app/support/redaction"
	"goravel/app/support/safehttp"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source: enterprise_security_control_service.go
var (
	ErrReAuthRequired            = errors.New("sensitive operation requires valid re-auth token")
	ErrApprovalRequired          = errors.New("permission change requires approved approval record")
	ErrApprovalSelfApproved      = errors.New("permission change approver must differ from requester")
	ErrSensitiveOperationBinding = errors.New("sensitive operation approval binding is invalid")
	ErrWORMProofRequired         = errors.New("audit prune requires WORM archive proof")
	ErrCSPUnsafeInline           = errors.New("csp must not allow unsafe-inline")
	ErrCSPNonceHashRequired      = errors.New("csp script-src requires nonce or sha hash")
)

const (
	enterpriseReAuthTokenPrefix = "security:reauth:"
	enterpriseReAuthMinLockTTL  = 5 * time.Second
)

var enterpriseSecurityCache = func() contractscache.Driver {
	return facades.Cache()
}

var enterpriseSecurityNow = time.Now

var enterpriseSecurityHasApprovalBindingColumns = hasEnterpriseSecurityApprovalBindingColumns

type EnterpriseSecurityControlService struct {
	approvals      map[string]PermissionApprovalRequest
	policyRegistry *SensitiveOperationPolicyRegistry
	planProvider   SensitiveOperationPlanProvider
	mu             sync.Mutex
}

type ReAuthTokenClaims struct {
	UserID    uint64
	TenantID  uint64
	Operation string
	Resource  string
	ExpiresAt time.Time
}

type SensitiveOperationRequest struct {
	UserID      uint64
	TenantID    uint64
	Operation   string
	Resource    string
	ReAuthToken string
}

type PermissionApprovalRequest struct {
	RequesterID   uint64
	ApproverID    uint64
	TenantID      uint64
	PolicyKey     string
	BindingDigest string
	Scope         string
	Resource      string
	Before        []string
	After         []string
	Reason        string
	Status        string
	UsedAt        time.Time
	ExpiresAt     time.Time
}

type PlatformReAuthRequest struct {
	UserID    uint64
	Password  string
	MFACode   string
	Operation string
	Resource  string
}

type TenantReAuthRequest struct {
	Tenant    Tenant
	UserID    uint64
	Password  string
	MFACode   string
	Operation string
	Resource  string
}

type ReAuthTokenResult struct {
	ReAuthToken string    `json:"reauth_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type PlatformApprovalCreateRequest struct {
	RequesterID uint64
	TenantID    uint64
	PolicyKey   string
	Scope       string
	Resource    string
	Reason      string
	Before      []string
	After       []string
	ExpiresAt   time.Time
}

type PlatformApprovalApproveRequest struct {
	ApprovalID string
	ApproverID uint64
	TenantID   uint64
	ExpiresAt  time.Time
}

type PermissionApprovalRecord struct {
	ApprovalID    string    `json:"approval_id"`
	RequesterID   uint64    `json:"requester_id"`
	ApproverID    uint64    `json:"approver_id"`
	TenantID      uint64    `json:"tenant_id"`
	PolicyKey     string    `json:"policy_key"`
	BindingDigest string    `json:"binding_digest"`
	Scope         string    `json:"scope"`
	Resource      string    `json:"resource"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
	UsedAt        time.Time `json:"used_at,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
}

type permissionApprovalRow struct {
	ApprovalID     string     `gorm:"column:approval_id"`
	RequesterID    uint64     `gorm:"column:requester_id"`
	ApproverID     uint64     `gorm:"column:approver_id"`
	TenantID       uint64     `gorm:"column:tenant_id"`
	PolicyKey      string     `gorm:"column:policy_key"`
	BindingDigest  string     `gorm:"column:binding_digest"`
	Scope          string     `gorm:"column:scope"`
	Resource       string     `gorm:"column:resource"`
	Status         string     `gorm:"column:status"`
	Reason         string     `gorm:"column:reason"`
	BeforeSnapshot string     `gorm:"column:before_snapshot"`
	AfterSnapshot  string     `gorm:"column:after_snapshot"`
	UsedAt         *time.Time `gorm:"column:used_at"`
	ExpiresAt      *time.Time `gorm:"column:expires_at"`
}

type AuditPruneProof struct {
	ArchiveURI string
	Digest     string
	WindowFrom time.Time
	WindowTo   time.Time
	VerifiedAt time.Time
}

func NewEnterpriseSecurityControlService() *EnterpriseSecurityControlService {
	return sharedEnterpriseSecurityControl
}

var sharedEnterpriseSecurityControl = &EnterpriseSecurityControlService{
	approvals: map[string]PermissionApprovalRequest{},
}

func ResetEnterpriseSecurityControlForTest() {
	sharedEnterpriseSecurityControl.mu.Lock()
	defer sharedEnterpriseSecurityControl.mu.Unlock()
	sharedEnterpriseSecurityControl.approvals = map[string]PermissionApprovalRequest{}
}

func UseEnterpriseSecurityMemoryCacheForTest() func() {
	cache := newEnterpriseSecurityMemoryCache()
	original := enterpriseSecurityCache
	enterpriseSecurityCache = func() contractscache.Driver { return cache }
	return func() {
		enterpriseSecurityCache = original
	}
}

func SetEnterpriseSecurityNowForTest(now func() time.Time) func() {
	original := enterpriseSecurityNow
	enterpriseSecurityNow = now
	return func() {
		enterpriseSecurityNow = original
	}
}

func SetSensitiveOperationPlanProviderForTest(prepare func(
	context.Context,
	string,
	uint64,
	uint64,
	SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error)) func() {
	original := sharedEnterpriseSecurityControl.planProvider
	if prepare == nil {
		sharedEnterpriseSecurityControl.planProvider = nil
	} else {
		sharedEnterpriseSecurityControl.planProvider = sensitiveOperationPlanProviderFunc(prepare)
	}
	return func() {
		sharedEnterpriseSecurityControl.planProvider = original
	}
}

func newEnterpriseSecurityMemoryCache() contractscache.Driver {
	cache, err := frameworkcache.NewMemory(enterpriseSecurityMemoryCacheConfig{})
	if err != nil {
		panic(err)
	}
	return cache
}

type enterpriseSecurityMemoryCacheConfig struct{}

func (enterpriseSecurityMemoryCacheConfig) Env(string, ...any) any { return nil }

func (enterpriseSecurityMemoryCacheConfig) EnvString(string, ...string) string { return "" }

func (enterpriseSecurityMemoryCacheConfig) EnvBool(string, ...bool) bool { return false }

func (enterpriseSecurityMemoryCacheConfig) Add(string, any) {}

func (enterpriseSecurityMemoryCacheConfig) Get(string, ...any) any { return nil }

func (enterpriseSecurityMemoryCacheConfig) GetString(string, ...string) string { return "" }

func (enterpriseSecurityMemoryCacheConfig) GetInt(string, ...int) int { return 0 }

func (enterpriseSecurityMemoryCacheConfig) GetBool(string, ...bool) bool { return false }

func (enterpriseSecurityMemoryCacheConfig) GetDuration(string, ...time.Duration) time.Duration {
	return 0
}

func (enterpriseSecurityMemoryCacheConfig) UnmarshalKey(string, any) error { return nil }

func (s *EnterpriseSecurityControlService) IssueReAuthToken(claims ReAuthTokenClaims) (string, error) {
	if claims.ExpiresAt.IsZero() {
		claims.ExpiresAt = time.Now().Add(5 * time.Minute)
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	token := sha256Hex([]byte(strings.Join([]string{
		hex.EncodeToString(nonce),
		claims.Operation,
		claims.Resource,
		claims.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}, ":")))
	ttl := time.Until(claims.ExpiresAt)
	if ttl <= 0 {
		return "", ErrReAuthRequired
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	if err := enterpriseSecurityCache().Put(enterpriseReAuthTokenKey(token), string(raw), ttl); err != nil {
		return "", err
	}
	return token, nil
}

func (s *EnterpriseSecurityControlService) IssuePlatformReAuthToken(ctx context.Context, req PlatformReAuthRequest) (ReAuthTokenResult, error) {
	req.Operation = strings.TrimSpace(req.Operation)
	req.Resource = strings.TrimSpace(req.Resource)
	if req.UserID == 0 || strings.TrimSpace(req.Password) == "" || req.Operation == "" || req.Resource == "" {
		return ReAuthTokenResult{}, BusinessError{Message: "二次认证参数不完整"}
	}
	registry := s.sensitiveOperationPolicyRegistry()
	if policy, ok := registry.Policy(req.Operation); ok {
		plan, err := s.canonicalPlanProvider().Prepare(ctx, policy.PolicyKey, req.UserID, 0, SensitiveOperationPlanSelector{Resource: req.Resource})
		if err != nil {
			return ReAuthTokenResult{}, err
		}
		req.Operation = plan.Scope
		req.Resource = plan.Resource
	}
	var user models.User
	err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("platform_user").
		Where("id", req.UserID).
		First(&user)
	if err != nil {
		return ReAuthTokenResult{}, ErrInvalidCredentials
	}
	if user.Status == 2 {
		return ReAuthTokenResult{}, ErrUserDisabled
	}
	if !passwordHashMatches(user.Password, req.Password) {
		return ReAuthTokenResult{}, ErrInvalidCredentials
	}
	mfa := NewPlatformMFAService().WithContext(ctx)
	if mfa.Enabled(user.ID) {
		if err := mfa.Verify(user.ID, req.MFACode); err != nil {
			return ReAuthTokenResult{}, err
		}
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	token, err := s.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    user.ID,
		Operation: req.Operation,
		Resource:  req.Resource,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return ReAuthTokenResult{}, err
	}
	return ReAuthTokenResult{ReAuthToken: token, ExpiresAt: expiresAt}, nil
}

func (s *EnterpriseSecurityControlService) IssueTenantReAuthToken(ctx context.Context, req TenantReAuthRequest) (ReAuthTokenResult, error) {
	req.Operation = strings.TrimSpace(req.Operation)
	req.Resource = strings.TrimSpace(req.Resource)
	if req.Tenant.ID == 0 || req.UserID == 0 || strings.TrimSpace(req.Password) == "" || req.Operation == "" || req.Resource == "" {
		return ReAuthTokenResult{}, BusinessError{Message: "二次认证参数不完整"}
	}
	policy, ok := s.sensitiveOperationPolicyRegistry().Policy(req.Operation)
	if !ok || policy.TenantPermission == "" {
		return ReAuthTokenResult{}, ErrSensitiveOperationPolicy
	}
	plan, err := s.canonicalPlanProvider().Prepare(ctx, policy.PolicyKey, req.UserID, req.Tenant.ID, SensitiveOperationPlanSelector{Resource: req.Resource})
	if err != nil {
		return ReAuthTokenResult{}, err
	}
	var user models.User
	err = OrmForConnectionWithContext(contextOrBackground(ctx), TenantConnectionName(req.Tenant)).
		Query().Table("user").Where("id", req.UserID).First(&user)
	if err != nil || user.Status == 2 || !passwordHashMatches(user.Password, req.Password) {
		return ReAuthTokenResult{}, ErrInvalidCredentials
	}
	mfa := NewMFAServiceForTenant(req.Tenant).WithContext(ctx)
	if mfa.Enabled(user.ID) {
		if err := mfa.Verify(user.ID, req.MFACode); err != nil {
			return ReAuthTokenResult{}, err
		}
	}
	expiresAt := enterpriseSecurityNow().Add(5 * time.Minute)
	token, err := s.IssueReAuthToken(ReAuthTokenClaims{
		UserID: user.ID, TenantID: req.Tenant.ID, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: expiresAt,
	})
	if err != nil {
		return ReAuthTokenResult{}, err
	}
	return ReAuthTokenResult{ReAuthToken: token, ExpiresAt: expiresAt}, nil
}

func (s *EnterpriseSecurityControlService) RequireSensitiveOperation(req SensitiveOperationRequest) error {
	return s.requireSensitiveOperation(req, nil)
}

func (s *EnterpriseSecurityControlService) RequireSensitiveOperationWithApproval(
	ctx context.Context,
	req SensitiveOperationRequest,
	approvalID string,
	requesterID uint64,
	scope string,
	resource string,
) error {
	return s.requireSensitiveOperation(req, func() error {
		return s.RequireRegisteredPermissionApproval(ctx, approvalID, requesterID, scope, resource)
	})
}

func (s *EnterpriseSecurityControlService) ExecuteSensitiveOperation(req SensitiveOperationRequest, operation func() error) error {
	return s.withLockedReAuthToken(req, func(cache contractscache.Driver, key string, raw string, claims ReAuthTokenClaims) error {
		if !cache.Forget(key) {
			return ErrReAuthRequired
		}
		if err := operation(); err != nil {
			return joinRestoreError(err, restoreReAuthToken(cache, key, raw, claims.ExpiresAt))
		}
		return nil
	})
}

func (s *EnterpriseSecurityControlService) ExecuteSensitiveOperationNoRestore(
	req SensitiveOperationRequest,
	consume func() error,
	operation func() error,
) error {
	return s.withLockedReAuthToken(req, func(cache contractscache.Driver, key string, raw string, claims ReAuthTokenClaims) error {
		if !cache.Forget(key) {
			return ErrReAuthRequired
		}
		if consume != nil {
			if err := consume(); err != nil {
				return joinRestoreError(err, restoreReAuthToken(cache, key, raw, claims.ExpiresAt))
			}
		}
		return operation()
	})
}

func (s *EnterpriseSecurityControlService) ExecuteSensitiveOperationWithApproval(
	ctx context.Context,
	req SensitiveOperationRequest,
	approvalID string,
	requesterID uint64,
	scope string,
	resource string,
	operation func() error,
) error {
	plan, err := s.canonicalPlanProvider().Prepare(ctx, scope, requesterID, req.TenantID, SensitiveOperationPlanSelector{Resource: resource})
	if err != nil {
		return ErrSensitiveOperationBinding
	}
	guard := NewSensitiveOperationGuard(s.sensitiveOperationPolicyRegistry())
	guard.security = s
	guard.planProvider = s.canonicalPlanProvider()
	return guard.Execute(ctx, plan, SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken,
		ApprovalID:  approvalID,
	}, operation)
}

func restoreReAuthToken(cache contractscache.Driver, key string, raw string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	return cache.Put(key, raw, ttl)
}

func joinRestoreError(operationErr error, restoreErr error) error {
	if restoreErr == nil {
		return operationErr
	}
	return errors.Join(operationErr, restoreErr)
}

func (s *EnterpriseSecurityControlService) memoryApproval(id string) (PermissionApprovalRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.approvals[strings.TrimSpace(id)]
	return req, ok
}

func (s *EnterpriseSecurityControlService) restoreRegisteredPermissionApproval(
	ctx context.Context,
	id string,
	memoryApproval PermissionApprovalRequest,
	memoryApprovalExists bool,
) error {
	if memoryApprovalExists {
		s.mu.Lock()
		s.approvals[strings.TrimSpace(id)] = memoryApproval
		s.mu.Unlock()
		return nil
	}
	if !hasEnterpriseSecurityApprovalTable() {
		return ErrApprovalRequired
	}
	now := time.Now()
	result, err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("enterprise_security_approval").
		Where("approval_id", strings.TrimSpace(id)).
		Where("status", "approved").
		WhereNotNull("used_at").
		Update(map[string]any{"used_at": nil, "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrApprovalRequired
	}
	return nil
}

func (s *EnterpriseSecurityControlService) ValidateSensitiveOperationWithApproval(
	ctx context.Context,
	req SensitiveOperationRequest,
	approvalID string,
	requesterID uint64,
	scope string,
	resource string,
) error {
	return s.validateSensitiveOperation(req, func() error {
		return s.ValidateRegisteredPermissionApproval(ctx, approvalID, requesterID, scope, resource)
	})
}

func (s *EnterpriseSecurityControlService) requireSensitiveOperation(req SensitiveOperationRequest, gate func() error) error {
	return s.withValidatedReAuthToken(req, gate, true)
}

func (s *EnterpriseSecurityControlService) validateSensitiveOperation(req SensitiveOperationRequest, gate func() error) error {
	return s.withValidatedReAuthToken(req, gate, false)
}

func (s *EnterpriseSecurityControlService) withValidatedReAuthToken(req SensitiveOperationRequest, gate func() error, consume bool) error {
	return s.withLockedReAuthToken(req, func(cache contractscache.Driver, key string, _ string, _ ReAuthTokenClaims) error {
		if gate != nil {
			if err := gate(); err != nil {
				return err
			}
		}
		if consume && !cache.Forget(key) {
			return ErrReAuthRequired
		}
		return nil
	})
}

func (s *EnterpriseSecurityControlService) withLockedReAuthToken(
	req SensitiveOperationRequest,
	operation func(contractscache.Driver, string, string, ReAuthTokenClaims) error,
) error {
	token := strings.TrimSpace(req.ReAuthToken)
	if token == "" {
		return ErrReAuthRequired
	}
	cache := enterpriseSecurityCache()
	key := enterpriseReAuthTokenKey(token)
	raw := cache.GetString(key)
	claims, err := parseReAuthClaims(raw)
	if err != nil {
		if raw != "" {
			cache.Forget(key)
		}
		return err
	}
	if !reAuthClaimsMatch(claims, req) {
		return ErrReAuthRequired
	}
	lockTTL := time.Until(claims.ExpiresAt)
	if lockTTL < enterpriseReAuthMinLockTTL {
		lockTTL = enterpriseReAuthMinLockTTL
	}
	lock := cache.Lock(enterpriseReAuthLockKey(token), lockTTL)
	if !lock.Get() {
		return ErrReAuthRequired
	}
	defer lock.Release()
	raw = cache.GetString(key)
	if raw == "" {
		return ErrReAuthRequired
	}
	claims, err = parseReAuthClaims(raw)
	if err != nil {
		cache.Forget(key)
		return err
	}
	if !reAuthClaimsMatch(claims, req) {
		return ErrReAuthRequired
	}
	return operation(cache, key, raw, claims)
}

func parseReAuthClaims(raw string) (ReAuthTokenClaims, error) {
	claims := ReAuthTokenClaims{}
	if raw == "" || json.Unmarshal([]byte(raw), &claims) != nil || !time.Now().Before(claims.ExpiresAt) {
		return ReAuthTokenClaims{}, ErrReAuthRequired
	}
	return claims, nil
}

func reAuthClaimsMatch(claims ReAuthTokenClaims, req SensitiveOperationRequest) bool {
	return claims.UserID == req.UserID && claims.TenantID == req.TenantID &&
		claims.Operation == strings.TrimSpace(req.Operation) && claims.Resource == strings.TrimSpace(req.Resource)
}

func enterpriseReAuthTokenKey(token string) string {
	return enterpriseReAuthTokenPrefix + sha256Hex([]byte(token))
}

func enterpriseReAuthLockKey(token string) string {
	return enterpriseReAuthTokenKey(token) + ":lock"
}

func (s *EnterpriseSecurityControlService) RequirePermissionApproval(req PermissionApprovalRequest) error {
	if strings.TrimSpace(req.Status) != "approved" || req.ApproverID == 0 {
		return ErrApprovalRequired
	}
	if !req.UsedAt.IsZero() {
		return ErrApprovalRequired
	}
	if req.RequesterID != 0 && req.RequesterID == req.ApproverID {
		return ErrApprovalSelfApproved
	}
	if !req.ExpiresAt.IsZero() && time.Now().After(req.ExpiresAt) {
		return ErrApprovalRequired
	}
	return nil
}

func (s *EnterpriseSecurityControlService) RegisterPermissionApproval(id string, req PermissionApprovalRequest) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrApprovalRequired
	}
	if err := s.RequirePermissionApproval(req); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals[id] = req
	return nil
}

func (s *EnterpriseSecurityControlService) RequireRegisteredPermissionApproval(ctx context.Context, id string, requesterID uint64, scope string, resource string) error {
	return s.registeredPermissionApproval(ctx, id, requesterID, scope, resource, true)
}

func (s *EnterpriseSecurityControlService) ValidateRegisteredPermissionApproval(ctx context.Context, id string, requesterID uint64, scope string, resource string) error {
	return s.registeredPermissionApproval(ctx, id, requesterID, scope, resource, false)
}

func (s *EnterpriseSecurityControlService) ValidateRegisteredPermissionApprovalBinding(
	ctx context.Context,
	id string,
	plan SensitiveOperationPlan,
) error {
	return s.registeredPermissionApprovalBinding(ctx, id, plan, false)
}

func (s *EnterpriseSecurityControlService) ConsumeRegisteredPermissionApprovalBinding(
	ctx context.Context,
	id string,
	plan SensitiveOperationPlan,
) error {
	return s.registeredPermissionApprovalBinding(ctx, id, plan, true)
}

func (s *EnterpriseSecurityControlService) registeredPermissionApproval(ctx context.Context, id string, requesterID uint64, scope string, resource string, consume bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrApprovalRequired
	}
	s.mu.Lock()
	req, ok := s.approvals[id]
	if ok {
		if err := validRegisteredPermissionApproval(req, requesterID, scope, resource); err != nil {
			s.mu.Unlock()
			return err
		}
		if consume {
			delete(s.approvals, id)
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if !ok {
		var err error
		req, ok, err = s.loadPermissionApproval(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrApprovalRequired
		}
	}
	if err := validRegisteredPermissionApproval(req, requesterID, scope, resource); err != nil {
		return err
	}
	if consume {
		return consumePermissionApproval(ctx, id, req)
	}
	return nil
}

func (s *EnterpriseSecurityControlService) registeredPermissionApprovalBinding(
	ctx context.Context,
	id string,
	plan SensitiveOperationPlan,
	consume bool,
) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrApprovalRequired
	}
	s.mu.Lock()
	req, ok := s.approvals[id]
	if ok {
		if err := validRegisteredPermissionApprovalBinding(req, plan); err != nil {
			s.mu.Unlock()
			return err
		}
		if consume {
			delete(s.approvals, id)
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	req, ok, err := s.loadPermissionApproval(ctx, id)
	if err != nil {
		return err
	}
	if !ok || validRegisteredPermissionApprovalBinding(req, plan) != nil {
		return ErrApprovalRequired
	}
	if !consume {
		return nil
	}
	return consumePermissionApprovalBinding(ctx, id, req, plan)
}

func (s *EnterpriseSecurityControlService) CreatePlatformApproval(ctx context.Context, req PlatformApprovalCreateRequest) (PermissionApprovalRecord, error) {
	req.PolicyKey = strings.TrimSpace(req.PolicyKey)
	req.Scope = strings.TrimSpace(req.Scope)
	req.Resource = strings.TrimSpace(req.Resource)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.RequesterID == 0 || req.Resource == "" || req.Reason == "" {
		return PermissionApprovalRecord{}, BusinessError{Message: "审批申请参数不完整"}
	}
	if req.PolicyKey == "" {
		req.PolicyKey = req.Scope
	}
	_, bindingAware := s.sensitiveOperationPolicyRegistry().Policy(req.PolicyKey)
	plan := SensitiveOperationPlan{}
	if bindingAware {
		if s.planProvider == nil && !enterpriseSecurityHasApprovalBindingColumns() {
			return PermissionApprovalRecord{}, ErrApprovalRequired
		}
		var err error
		plan, err = s.canonicalPlanProvider().Prepare(ctx, req.PolicyKey, req.RequesterID, req.TenantID, SensitiveOperationPlanSelector{Resource: req.Resource})
		if err != nil || strings.TrimSpace(plan.BindingDigest) == "" {
			return PermissionApprovalRecord{}, ErrSensitiveOperationBinding
		}
		req.PolicyKey = plan.PolicyKey
		req.Scope = plan.Scope
		req.Resource = plan.Resource
		req.Before = plan.Before
		req.After = plan.After
	}
	if req.Scope == "" {
		return PermissionApprovalRecord{}, BusinessError{Message: "审批申请参数不完整"}
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().Add(30 * time.Minute)
	}
	id, err := newApprovalID()
	if err != nil {
		return PermissionApprovalRecord{}, err
	}
	now := time.Now()
	record := PermissionApprovalRecord{
		ApprovalID:  id,
		RequesterID: req.RequesterID,
		TenantID:    req.TenantID,
		Scope:       req.Scope,
		Resource:    req.Resource,
		Status:      "pending",
		Reason:      req.Reason,
		ExpiresAt:   req.ExpiresAt,
	}
	values := map[string]any{
		"approval_id":     id,
		"requester_id":    req.RequesterID,
		"approver_id":     0,
		"tenant_id":       req.TenantID,
		"scope":           req.Scope,
		"resource":        req.Resource,
		"status":          "pending",
		"reason":          req.Reason,
		"before_snapshot": approvalSnapshot(req.Before),
		"after_snapshot":  approvalSnapshot(req.After),
		"expires_at":      req.ExpiresAt,
		"created_at":      now,
		"updated_at":      now,
	}
	if bindingAware {
		record.PolicyKey = req.PolicyKey
		record.BindingDigest = plan.BindingDigest
		values["policy_key"] = record.PolicyKey
		values["binding_digest"] = record.BindingDigest
	}
	err = OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("enterprise_security_approval").
		Create(values)
	return record, err
}

func (s *EnterpriseSecurityControlService) ApprovePlatformApproval(ctx context.Context, req PlatformApprovalApproveRequest) (PermissionApprovalRecord, error) {
	req.ApprovalID = strings.TrimSpace(req.ApprovalID)
	if req.ApprovalID == "" || req.ApproverID == 0 {
		return PermissionApprovalRecord{}, BusinessError{Message: "审批批准参数不完整"}
	}
	var row permissionApprovalRow
	orm := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection())
	if err := orm.Query().Table("enterprise_security_approval").Where("approval_id", req.ApprovalID).First(&row); err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return PermissionApprovalRecord{}, ErrApprovalRequired
		}
		return PermissionApprovalRecord{}, err
	}
	if row.ApprovalID == "" {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	if row.TenantID != req.TenantID {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	if row.RequesterID == req.ApproverID {
		return PermissionApprovalRecord{}, ErrApprovalSelfApproved
	}
	if strings.TrimSpace(row.Status) != "pending" || row.UsedAt != nil {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	if row.ExpiresAt != nil && enterpriseSecurityNow().After(*row.ExpiresAt) {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	expiresAt := req.ExpiresAt
	if expiresAt.IsZero() {
		if row.ExpiresAt != nil {
			expiresAt = *row.ExpiresAt
		} else {
			expiresAt = enterpriseSecurityNow().Add(30 * time.Minute)
		}
	}
	now := enterpriseSecurityNow()
	result, err := orm.Query().Table("enterprise_security_approval").
		Where("approval_id", req.ApprovalID).
		Where("tenant_id", req.TenantID).
		Where("status", "pending").
		WhereNull("used_at").
		Where("(expires_at IS NULL OR expires_at > ?)", now).
		Update(map[string]any{
			"approver_id": req.ApproverID,
			"status":      "approved",
			"expires_at":  expiresAt,
			"updated_at":  now,
		})
	if err != nil {
		return PermissionApprovalRecord{}, err
	}
	if result.RowsAffected != 1 {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	record := permissionApprovalRecordFromRow(row)
	record.ApproverID = req.ApproverID
	record.Status = "approved"
	record.ExpiresAt = expiresAt
	return record, nil
}

func (s *EnterpriseSecurityControlService) PlatformApproval(ctx context.Context, approvalID string) (PermissionApprovalRecord, error) {
	return s.Approval(ctx, approvalID, 0)
}

func (s *EnterpriseSecurityControlService) Approval(ctx context.Context, approvalID string, tenantID uint64) (PermissionApprovalRecord, error) {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	var row permissionApprovalRow
	err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("enterprise_security_approval").
		Where("approval_id", approvalID).
		Where("tenant_id", tenantID).
		First(&row)
	if err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return PermissionApprovalRecord{}, ErrApprovalRequired
		}
		return PermissionApprovalRecord{}, err
	}
	if row.ApprovalID == "" {
		return PermissionApprovalRecord{}, ErrApprovalRequired
	}
	return permissionApprovalRecordFromRow(row), nil
}

func validRegisteredPermissionApproval(req PermissionApprovalRequest, requesterID uint64, scope string, resource string) error {
	if requesterID != 0 && req.RequesterID != requesterID {
		return ErrApprovalRequired
	}
	if strings.TrimSpace(scope) != "" && strings.TrimSpace(req.Scope) != strings.TrimSpace(scope) {
		return ErrApprovalRequired
	}
	if strings.TrimSpace(resource) != "" && strings.TrimSpace(req.Resource) != strings.TrimSpace(resource) {
		return ErrApprovalRequired
	}
	return sharedEnterpriseSecurityControl.RequirePermissionApproval(req)
}

func validRegisteredPermissionApprovalBinding(req PermissionApprovalRequest, plan SensitiveOperationPlan) error {
	if err := validRegisteredPermissionApproval(req, plan.ActorID, plan.Scope, plan.Resource); err != nil {
		return err
	}
	if strings.TrimSpace(req.PolicyKey) != plan.PolicyKey || strings.TrimSpace(req.BindingDigest) != plan.BindingDigest {
		return ErrApprovalRequired
	}
	if req.TenantID != plan.TenantID {
		return ErrApprovalRequired
	}
	return nil
}

func (s *EnterpriseSecurityControlService) loadPermissionApproval(ctx context.Context, id string) (PermissionApprovalRequest, bool, error) {
	if !hasEnterpriseSecurityApprovalTable() {
		return PermissionApprovalRequest{}, false, nil
	}
	row := permissionApprovalRow{}
	err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("enterprise_security_approval").
		Where("approval_id", id).
		First(&row)
	if err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return PermissionApprovalRequest{}, false, nil
		}
		return PermissionApprovalRequest{}, false, err
	}
	if row.ApprovalID == "" {
		return PermissionApprovalRequest{}, false, nil
	}
	req := PermissionApprovalRequest{
		RequesterID:   row.RequesterID,
		ApproverID:    row.ApproverID,
		TenantID:      row.TenantID,
		PolicyKey:     row.PolicyKey,
		BindingDigest: row.BindingDigest,
		Scope:         row.Scope,
		Resource:      row.Resource,
		Status:        row.Status,
		Reason:        row.Reason,
		Before:        approvalSnapshotStrings(row.BeforeSnapshot),
		After:         approvalSnapshotStrings(row.AfterSnapshot),
	}
	if row.UsedAt != nil {
		req.UsedAt = *row.UsedAt
	}
	if row.ExpiresAt != nil {
		req.ExpiresAt = *row.ExpiresAt
	}
	return req, true, nil
}

func consumePermissionApproval(ctx context.Context, id string, req PermissionApprovalRequest) error {
	if !hasEnterpriseSecurityApprovalTable() {
		return ErrApprovalRequired
	}
	now := time.Now()
	query := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).
		Query().
		Table("enterprise_security_approval").
		Where("approval_id", id).
		Where("requester_id", req.RequesterID).
		Where("approver_id", req.ApproverID).
		Where("scope", strings.TrimSpace(req.Scope)).
		Where("resource", strings.TrimSpace(req.Resource)).
		Where("status", "approved").
		WhereNull("used_at")
	if !req.ExpiresAt.IsZero() {
		query = query.Where("expires_at > ?", now)
	}
	result, err := query.Update(map[string]any{
		"used_at":    now,
		"updated_at": now,
	})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrApprovalRequired
	}
	return nil
}

func consumePermissionApprovalBinding(ctx context.Context, id string, req PermissionApprovalRequest, plan SensitiveOperationPlan) error {
	if !hasEnterpriseSecurityApprovalTable() {
		return ErrApprovalRequired
	}
	query := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Query()
	return consumePermissionApprovalBindingWithQuery(query, id, req, plan)
}

func consumePermissionApprovalBindingWithQuery(query contractsorm.Query, id string, req PermissionApprovalRequest, plan SensitiveOperationPlan) error {
	if query == nil {
		return ErrApprovalRequired
	}
	now := enterpriseSecurityNow()
	query = query.Table("enterprise_security_approval").
		Where("approval_id", id).
		Where("requester_id", req.RequesterID).
		Where("approver_id", req.ApproverID).
		Where("tenant_id", plan.TenantID).
		Where("policy_key", plan.PolicyKey).
		Where("binding_digest", plan.BindingDigest).
		Where("scope", plan.Scope).
		Where("resource", plan.Resource).
		Where("status", "approved").
		WhereNull("used_at")
	if !req.ExpiresAt.IsZero() {
		query = query.Where("expires_at > ?", now)
	}
	result, err := query.Update(map[string]any{"used_at": now, "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrApprovalRequired
	}
	return nil
}

func hasEnterpriseSecurityApprovalTable() bool {
	schema := facades.Schema()
	if schema == nil {
		return false
	}
	return schema.Connection(PlatformConnection()).HasTable("enterprise_security_approval")
}

func hasEnterpriseSecurityApprovalBindingColumns() bool {
	schema := facades.Schema()
	if schema == nil || !schema.Connection(PlatformConnection()).HasTable("enterprise_security_approval") {
		return false
	}
	return schema.Connection(PlatformConnection()).HasColumns("enterprise_security_approval", []string{"tenant_id", "policy_key", "binding_digest"})
}

func (s *EnterpriseSecurityControlService) sensitiveOperationPolicyRegistry() *SensitiveOperationPolicyRegistry {
	if s != nil && s.policyRegistry != nil {
		return s.policyRegistry
	}
	return NewSensitiveOperationPolicyRegistry()
}

func (s *EnterpriseSecurityControlService) canonicalPlanProvider() SensitiveOperationPlanProvider {
	if s != nil && s.planProvider != nil {
		return s.planProvider
	}
	return NewSensitiveOperationPlanResolver(s.sensitiveOperationPolicyRegistry())
}

func newApprovalID() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return "approval-" + hex.EncodeToString(nonce), nil
}

func approvalSnapshot(items []string) any {
	if len(items) == 0 {
		return nil
	}
	raw, _ := json.Marshal(items)
	return string(raw)
}

func approvalSnapshotStrings(snapshot string) []string {
	items := make([]string, 0)
	if strings.TrimSpace(snapshot) == "" || json.Unmarshal([]byte(snapshot), &items) != nil {
		return nil
	}
	return canonicalSnapshot(items)
}

func permissionApprovalRecordFromRow(row permissionApprovalRow) PermissionApprovalRecord {
	record := PermissionApprovalRecord{
		ApprovalID:    row.ApprovalID,
		RequesterID:   row.RequesterID,
		ApproverID:    row.ApproverID,
		TenantID:      row.TenantID,
		PolicyKey:     row.PolicyKey,
		BindingDigest: row.BindingDigest,
		Scope:         row.Scope,
		Resource:      row.Resource,
		Status:        row.Status,
		Reason:        row.Reason,
	}
	if row.UsedAt != nil {
		record.UsedAt = *row.UsedAt
	}
	if row.ExpiresAt != nil {
		record.ExpiresAt = *row.ExpiresAt
	}
	return record
}

func (s *EnterpriseSecurityControlService) RequireAuditPruneProof(proof AuditPruneProof) error {
	if strings.TrimSpace(proof.ArchiveURI) == "" || strings.TrimSpace(proof.Digest) == "" || proof.VerifiedAt.IsZero() {
		return ErrWORMProofRequired
	}
	if proof.WindowFrom.IsZero() || proof.WindowTo.IsZero() || proof.WindowTo.Before(proof.WindowFrom) {
		return ErrWORMProofRequired
	}
	return nil
}

func (s *EnterpriseSecurityControlService) ValidateCSP(policy string) error {
	policy = strings.TrimSpace(policy)
	directives := cspDirectives(policy)
	scriptSource := directives["script-src"]
	if scriptSource == "" {
		scriptSource = directives["default-src"]
	}
	if strings.Contains(scriptSource, "'unsafe-inline'") {
		return ErrCSPUnsafeInline
	}
	if !hasCSPNonceOrHash(scriptSource) {
		return ErrCSPNonceHashRequired
	}
	return nil
}

func cspDirectives(policy string) map[string]string {
	items := map[string]string{}
	for _, part := range strings.Split(policy, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		items[strings.ToLower(fields[0])] = strings.Join(fields[1:], " ")
	}
	return items
}

func hasCSPNonceOrHash(source string) bool {
	for _, field := range strings.Fields(source) {
		field = strings.Trim(field, "'")
		if strings.HasPrefix(field, "nonce-") && len(strings.TrimPrefix(field, "nonce-")) > 0 {
			return true
		}
		if strings.HasPrefix(field, "sha256-") && len(strings.TrimPrefix(field, "sha256-")) > 0 {
			return true
		}
		if strings.HasPrefix(field, "sha384-") && len(strings.TrimPrefix(field, "sha384-")) > 0 {
			return true
		}
		if strings.HasPrefix(field, "sha512-") && len(strings.TrimPrefix(field, "sha512-")) > 0 {
			return true
		}
	}
	return false
}

// Source: sensitive_operation_export.go
type tenantExportSelector struct {
	TenantID     uint64
	Dataset      string
	Format       string
	FilterDigest string
}

func resolveTenantExportPlan(ctx context.Context, tenantID uint64, selector string) (string, []string, []string, error) {
	parsed, err := parseTenantExportSelector(selector)
	if err != nil || (tenantID != 0 && parsed.TenantID != tenantID) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	tenant, err := NewTenantService().WithContext(ctx).FindByID(parsed.TenantID)
	if err != nil {
		return "", nil, nil, err
	}
	policy, err := NewTenantGovernanceService().WithContext(ctx).Policy(tenant)
	if err != nil {
		return "", nil, nil, err
	}
	if !policy.DataExport.Enabled {
		return "", nil, nil, ErrTenantDataActionDenied
	}
	before, err := sensitiveSnapshot(struct {
		TenantID     uint64 `json:"tenant_id"`
		TenantCode   string `json:"tenant_code"`
		Dataset      string `json:"dataset"`
		Format       string `json:"format"`
		FilterDigest string `json:"filter_digest"`
		Policy       string `json:"policy_version"`
	}{tenant.ID, tenant.Code, parsed.Dataset, parsed.Format, parsed.FilterDigest, tenantGovernancePolicyVersion(policy)})
	if err != nil {
		return "", nil, nil, err
	}
	return strings.TrimSpace(selector), []string{before}, []string{"tenant-export:queued"}, nil
}

func parseTenantExportSelector(value string) (tenantExportSelector, error) {
	const prefix = "tenant-data:export:"
	value = strings.TrimSpace(value)
	parts := strings.Split(strings.TrimPrefix(value, prefix), ":")
	if !strings.HasPrefix(value, prefix) || len(parts) != 5 || parts[3] != "sha256" {
		return tenantExportSelector{}, ErrSensitiveOperationPolicy
	}
	tenantID, err := strconv.ParseUint(parts[0], 10, 64)
	parsed := tenantExportSelector{TenantID: tenantID, Dataset: parts[1], Format: parts[2], FilterDigest: "sha256:" + parts[4]}
	if err != nil || tenantID == 0 || !tenantExportDatasetAllowed(parsed.Dataset) || !tenantExportFormatAllowed(parsed.Format) || !isSHA256(parsed.FilterDigest) {
		return tenantExportSelector{}, ErrSensitiveOperationPolicy
	}
	return parsed, nil
}

func tenantExportResource(tenantID uint64, dataset, format, filterDigest string) string {
	return fmt.Sprintf("tenant-data:export:%d:%s:%s:%s", tenantID, dataset, format, filterDigest)
}

func tenantExportDatasetAllowed(dataset string) bool { return dataset == "users" }

func tenantExportFormatAllowed(format string) bool { return format == "jsonl" || format == "csv" }

// Source: sensitive_operation_guard.go
type SensitiveOperationEvidence struct {
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type SensitiveOperationGuard struct {
	registry     *SensitiveOperationPolicyRegistry
	security     *EnterpriseSecurityControlService
	planProvider SensitiveOperationPlanProvider
	audit        func(context.Context, SensitiveOperationPlan, string, string)
}

type SensitiveOperationPlanSelector struct {
	Resource string
}

type SensitiveOperationPlanProvider interface {
	Prepare(context.Context, string, uint64, uint64, SensitiveOperationPlanSelector) (SensitiveOperationPlan, error)
}

type sensitiveOperationPlanProviderFunc func(
	context.Context,
	string,
	uint64,
	uint64,
	SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error)

func (f sensitiveOperationPlanProviderFunc) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	return f(ctx, policyKey, actorID, tenantID, selector)
}

func NewSensitiveOperationGuard(registry *SensitiveOperationPolicyRegistry) *SensitiveOperationGuard {
	if registry == nil {
		registry = NewSensitiveOperationPolicyRegistry()
	}
	security := NewEnterpriseSecurityControlService()
	return &SensitiveOperationGuard{
		registry:     registry,
		security:     security,
		planProvider: security.canonicalPlanProvider(),
		audit:        recordSensitiveOperationAudit,
	}
}

func (g *SensitiveOperationGuard) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	input SensitiveOperationPrepareInput,
) (SensitiveOperationPlan, error) {
	return g.registry.Prepare(ctx, policyKey, actorID, tenantID, input)
}

func (g *SensitiveOperationGuard) PrepareCanonical(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	if g == nil || g.planProvider == nil {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	return g.planProvider.Prepare(ctx, policyKey, actorID, tenantID, selector)
}

func (g *SensitiveOperationGuard) Validate(ctx context.Context, plan SensitiveOperationPlan, evidence SensitiveOperationEvidence) error {
	policy, err := g.validPolicy(plan)
	if err != nil {
		return err
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	if policy.RequiresApproval {
		approvalGate := func() error {
			return g.security.ValidateRegisteredPermissionApprovalBinding(ctx, evidence.ApprovalID, plan)
		}
		if policy.RequiresReAuth {
			return g.security.validateSensitiveOperation(request, approvalGate)
		}
		return approvalGate()
	}
	if policy.RequiresReAuth {
		return g.security.validateSensitiveOperation(request, nil)
	}
	return nil
}

func (g *SensitiveOperationGuard) Execute(
	ctx context.Context,
	plan SensitiveOperationPlan,
	evidence SensitiveOperationEvidence,
	mutate func() error,
) error {
	policy, err := g.validPolicy(plan)
	if err != nil {
		return err
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	consume := func() error {
		if policy.RequiresApproval {
			return g.security.ConsumeRegisteredPermissionApprovalBinding(ctx, evidence.ApprovalID, plan)
		}
		return nil
	}
	if policy.RequiresReAuth {
		err = g.security.ExecuteSensitiveOperationNoRestore(request, consume, mutate)
	} else {
		err = consume()
		if err == nil {
			err = mutate()
		}
	}
	if err != nil {
		g.recordAudit(ctx, plan, evidence.ApprovalID, "operation_failed")
		return err
	}
	g.recordAudit(ctx, plan, evidence.ApprovalID, "success")
	return nil
}

type sensitiveOperationPlanResolver struct {
	registry *SensitiveOperationPolicyRegistry
}

type sensitiveModuleStateSnapshot struct {
	ModuleID      string `gorm:"column:module_id"`
	Version       string `gorm:"column:version"`
	TargetVersion string `gorm:"column:target_version"`
	Status        string `gorm:"column:status"`
	Enabled       bool   `gorm:"column:enabled"`
	LastAction    string `gorm:"column:last_action"`
}

type sensitiveLifecycleLockSnapshot struct {
	Key       string    `gorm:"column:key"`
	Owner     string    `gorm:"column:owner"`
	RunKey    string    `gorm:"column:run_key"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

type sensitiveMFASnapshot struct {
	UserID        uint64           `gorm:"column:user_id" json:"user_id"`
	Enabled       bool             `gorm:"column:enabled" json:"enabled"`
	ConfirmedAt   time.Time        `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
	RecoveryCodes models.JSONSlice `gorm:"column:recovery_codes;type:jsonb" json:"-"`
}

func NewSensitiveOperationPlanResolver(registry *SensitiveOperationPolicyRegistry) SensitiveOperationPlanProvider {
	if registry == nil {
		registry = NewSensitiveOperationPolicyRegistry()
	}
	return sensitiveOperationPlanResolver{registry: registry}
}

func (r sensitiveOperationPlanResolver) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	policy, ok := r.registry.Policy(policyKey)
	if !ok {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	resource, before, after, err := r.resolve(ctx, policy.PolicyKey, actorID, tenantID, selector.Resource)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	return r.registry.Prepare(ctx, policy.PolicyKey, actorID, tenantID, SensitiveOperationPrepareInput{
		Resource: resource,
		Before:   before,
		After:    after,
	})
}

func (r sensitiveOperationPlanResolver) resolve(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector string,
) (string, []string, []string, error) {
	switch policyKey {
	case "audit.prune.execute":
		return resolveAuditPruneSensitivePlan(ctx, selector)
	case "module.lifecycle.execute":
		return resolveLifecycleOperationPlan(ctx, selector)
	case "module.lifecycle.release-lock":
		return resolveLifecycleLockReleasePlan(ctx, selector)
	case "tenant.data.delete":
		return resolveTenantDeletionPlan(ctx, selector)
	case "tenant.data.export":
		return resolveTenantExportPlan(ctx, tenantID, selector)
	case "mfa.disable":
		return resolveMFADisablePlan(ctx, actorID, tenantID, selector)
	case "user.password.reset", "user.roles.sync", "role.permissions.sync":
		return resolveRBACSensitivePlan(ctx, policyKey, tenantID, selector)
	case "sso.provider.secret.change", "storage.secret.change":
		return resolveSecretSensitivePlan(ctx, policyKey, tenantID, selector)
	case "tenant.permissions.sync", "tenant.plan.change", "tenant.governance.change", "tenant.status.change":
		return resolveTenantSensitivePlan(ctx, policyKey, selector)
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func resolveAuditPruneSensitivePlan(ctx context.Context, selector string) (string, []string, []string, error) {
	planID, _, err := parseAuditPruneResourceSelector(selector)
	if err != nil {
		return "", nil, nil, err
	}
	plan, err := NewAuditPrunePlanService().WithContext(ctx).Load(planID)
	if err != nil {
		return "", nil, nil, err
	}
	resource := AuditPruneResource(plan)
	if resource != strings.TrimSpace(selector) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before, err := sensitiveSnapshot(struct {
		PlanID       string `json:"plan_id"`
		Scope        string `json:"scope"`
		TargetDigest string `json:"target_digest"`
		TargetCount  int64  `json:"target_count"`
	}{plan.PlanID, plan.Scope, plan.TargetDigest, plan.TargetCount})
	if err != nil {
		return "", nil, nil, err
	}
	return resource, []string{before}, []string{"audit-prune:executed"}, nil
}

func parseAuditPruneResourceSelector(selector string) (string, string, error) {
	const prefix = "audit-prune:"
	value := strings.TrimSpace(selector)
	parts := strings.SplitN(strings.TrimPrefix(value, prefix), ":", 2)
	if !strings.HasPrefix(value, prefix) || len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || !isSHA256(parts[1]) {
		return "", "", ErrSensitiveOperationPolicy
	}
	return parts[0], parts[1], nil
}

func resolveMFADisablePlan(ctx context.Context, actorID, tenantID uint64, selector string) (string, []string, []string, error) {
	want := fmt.Sprintf("mfa:user:%d:disable", actorID)
	if actorID == 0 || strings.TrimSpace(selector) != want {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	connection, table := PlatformConnection(), "platform_user_mfa"
	if tenantID != 0 {
		var tenant Tenant
		if err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Query().Table("tenant").Where("id", tenantID).First(&tenant); err != nil {
			return "", nil, nil, err
		}
		connection, table = TenantConnectionName(tenant), "user_mfa"
	}
	var row sensitiveMFASnapshot
	if err := OrmForConnectionWithContext(contextOrBackground(ctx), connection).Query().Table(table).Where("user_id", actorID).First(&row); err != nil {
		return "", nil, nil, err
	}
	recoveryCodes := jsonSliceStrings(row.RecoveryCodes)
	before, err := sensitiveSnapshot(struct {
		UserID        uint64    `json:"user_id"`
		Enabled       bool      `json:"enabled"`
		ConfirmedAt   time.Time `json:"confirmed_at,omitempty"`
		RecoveryCount int       `json:"recovery_count"`
	}{row.UserID, row.Enabled, row.ConfirmedAt, len(recoveryCodes)})
	if err != nil {
		return "", nil, nil, err
	}
	return want, []string{before}, []string{"mfa:disabled"}, nil
}

func resolveLifecycleOperationPlan(ctx context.Context, selector string) (string, []string, []string, error) {
	moduleID, action, ok := lifecycleOperationSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	orm := facades.Orm()
	if orm == nil {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	rows := make([]sensitiveModuleStateSnapshot, 0)
	query := orm.WithContext(contextOrBackground(ctx)).Query().Table("module_state").OrderBy("module_id")
	if moduleID != "all" {
		query = query.Where("module_id", moduleID)
	}
	if err := query.Get(&rows); err != nil {
		return "", nil, nil, err
	}
	if moduleID != "all" && len(rows) > 1 {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before, err := sensitiveSnapshots(rows)
	if err != nil {
		return "", nil, nil, err
	}
	if moduleID != "all" && len(rows) == 0 {
		before = []string{"module-state:" + moduleID + ":absent"}
	}
	return "module-lifecycle:" + moduleID + ":" + action, before, []string{"lifecycle-action:" + action}, nil
}

func resolveLifecycleLockReleasePlan(ctx context.Context, selector string) (string, []string, []string, error) {
	key, ok := lifecycleLockReleaseSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	orm := facades.Orm()
	if orm == nil {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	rows := make([]sensitiveLifecycleLockSnapshot, 0)
	query := orm.WithContext(contextOrBackground(ctx)).Query().Table("module_lifecycle_lock").
		Where("expires_at < ?", time.Now()).OrderBy("key").OrderBy("owner").OrderBy("run_key")
	if key != "all" {
		query = query.Where("key", key)
	}
	if err := query.Get(&rows); err != nil {
		return "", nil, nil, err
	}
	before, err := sensitiveSnapshots(rows)
	if err != nil {
		return "", nil, nil, err
	}
	return "module-lifecycle:stale-locks:" + key, before, append([]string(nil), before...), nil
}

func resolveTenantDeletionPlan(ctx context.Context, selector string) (string, []string, []string, error) {
	ids, mode, ok := tenantDeletionSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	tenants := make([]Tenant, 0, len(ids))
	if err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Query().
		Table("tenant").WhereIn("id", uint64Any(ids)).OrderBy("id").Get(&tenants); err != nil {
		return "", nil, nil, err
	}
	if len(tenants) != len(ids) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before := make([]string, 0, len(tenants))
	for _, tenant := range tenants {
		policy, err := NewTenantGovernanceService().WithContext(ctx).Policy(tenant)
		if err != nil {
			return "", nil, nil, err
		}
		snapshot, err := sensitiveSnapshot(struct {
			ID               uint64 `json:"id"`
			Code             string `json:"code"`
			Status           int8   `json:"status"`
			Plan             string `json:"plan"`
			Database         string `json:"database"`
			DeletionEnabled  bool   `json:"deletion_enabled"`
			ApprovalRequired bool   `json:"approval_required"`
		}{
			ID: tenant.ID, Code: tenant.Code, Status: tenant.Status, Plan: tenant.Plan, Database: tenant.DBDatabase,
			DeletionEnabled: policy.DataDeletion.Enabled, ApprovalRequired: policy.DataDeletion.RequiresApproval,
		})
		if err != nil {
			return "", nil, nil, err
		}
		before = append(before, snapshot)
	}
	resource := TenantDataActionResource("delete", ids, mode)
	return resource, before, []string{"tenant-deletion:" + mode}, nil
}

func lifecycleOperationSelector(value string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 || parts[0] != "module-lifecycle" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	action := strings.TrimSpace(parts[2])
	switch action {
	case "install", "upgrade", "rollback", "uninstall":
		return strings.TrimSpace(parts[1]), action, true
	default:
		return "", "", false
	}
}

func lifecycleLockReleaseSelector(value string) (string, bool) {
	const prefix = "module-lifecycle:stale-locks:"
	key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), prefix))
	if !strings.HasPrefix(strings.TrimSpace(value), prefix) || key == "" {
		return "", false
	}
	return key, true
}

func tenantDeletionSelector(value string) ([]uint64, string, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 4 || parts[0] != "tenant-data" || parts[1] != "delete" {
		return nil, "", false
	}
	mode := strings.TrimSpace(parts[3])
	if mode != "metadata" && mode != "database" {
		return nil, "", false
	}
	seen := map[uint64]struct{}{}
	ids := make([]uint64, 0)
	for _, rawID := range strings.Split(parts[2], ",") {
		id, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id == 0 {
			return nil, "", false
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, "", false
	}
	return ids, mode, true
}

func sensitiveSnapshots[T any](values []T) ([]string, error) {
	snapshots := make([]string, 0, len(values))
	for _, value := range values {
		snapshot, err := sensitiveSnapshot(value)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func sensitiveSnapshot(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (g *SensitiveOperationGuard) validPolicy(plan SensitiveOperationPlan) (SensitiveOperationPolicy, error) {
	policy, ok := g.registry.Policy(plan.PolicyKey)
	if !ok || plan.ActorID == 0 || policy.Scope != plan.Scope || plan.Resource == "" {
		return SensitiveOperationPolicy{}, ErrSensitiveOperationPolicy
	}
	digest, err := sensitiveOperationBindingDigest(plan.PolicyKey, plan.Scope, plan.Resource, plan.TenantID, plan.Before, plan.After)
	if err != nil || plan.BindingDigest == "" || plan.BindingDigest != digest {
		return SensitiveOperationPolicy{}, ErrSensitiveOperationPolicy
	}
	return policy, nil
}

func recordSensitiveOperationAudit(ctx context.Context, plan SensitiveOperationPlan, approvalID, outcome string) {
	RecordAuditEvent(ctx, AuditEvent{
		Action: "sensitive_operation", Outcome: outcome, Actor: fmt.Sprintf("user:%d", plan.ActorID),
		Fields: map[string]any{
			"policy_key": plan.PolicyKey, "approval_id": approvalID, "binding_digest": plan.BindingDigest,
		},
	})
}

func (g *SensitiveOperationGuard) recordAudit(ctx context.Context, plan SensitiveOperationPlan, approvalID, outcome string) {
	if g.audit != nil {
		g.audit(ctx, plan, approvalID, outcome)
	}
}

// Source: sensitive_operation_policy.go
var ErrSensitiveOperationPolicy = errors.New("sensitive operation policy is invalid")

type SensitiveOperationPolicy struct {
	PolicyKey        string
	Scope            string
	Permission       string
	TenantPermission string
	Action           string
	RequiresReAuth   bool
	RequiresApproval bool
	ResourceBuilder  func(SensitiveOperationPrepareInput) (string, error)
}

type SensitiveOperationPrepareInput struct {
	Resource string
	Before   []string
	After    []string
}

type SensitiveOperationPlan struct {
	PolicyKey     string   `json:"policy_key"`
	Scope         string   `json:"scope"`
	Resource      string   `json:"resource"`
	BindingDigest string   `json:"binding_digest"`
	ActorID       uint64   `json:"actor_id"`
	TenantID      uint64   `json:"tenant_id"`
	Before        []string `json:"before"`
	After         []string `json:"after"`
}

type SensitiveOperationPolicyRegistry struct {
	policies      map[string]SensitiveOperationPolicy
	bindingDigest func(string, string, string, uint64, []string, []string) (string, error)
}

type SensitiveOperationPolicyContract struct {
	PolicyKey        string `json:"policy_key"`
	Scope            string `json:"scope"`
	Permission       string `json:"permission"`
	TenantPermission string `json:"tenant_permission,omitempty"`
	Action           string `json:"action"`
	RequiresReAuth   bool   `json:"requires_reauth"`
	RequiresApproval bool   `json:"requires_approval"`
}

type SensitiveOperationRouteContract struct {
	RouteName   string `json:"route_name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	PolicyKey   string `json:"policy_key"`
	Domain      string `json:"domain"`
	Resource    string `json:"resource"`
	Permission  string `json:"required_permission"`
	FeatureTest string `json:"feature_test"`
}

func NewSensitiveOperationPolicyRegistry() *SensitiveOperationPolicyRegistry {
	policies := []SensitiveOperationPolicy{
		newSensitiveOperationPolicy("audit.prune.execute", "platform:security:control", "", "DELETE"),
		newSensitiveOperationPolicy("mfa.disable", "platform:security:mfa", "security:mfa", "POST"),
		{
			PolicyKey:        "module.lifecycle.execute",
			Scope:            "module.lifecycle.execute",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		{
			PolicyKey:        "module.admission.approve",
			Scope:            "module.admission.approve",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		{
			PolicyKey:        "module.replacement.emergency-remove",
			Scope:            "module.replacement.emergency-remove",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "DELETE",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		newSensitiveOperationPolicy("role.permissions.sync", "platform:role:setMenu", "permission:role:setMenu", "PUT"),
		newSensitiveOperationPolicy("sso.provider.secret.change", "", "security:ssoProvider:update", "PUT"),
		newSensitiveOperationPolicy("storage.secret.change", "platform:storageConfig:update", "", "PUT"),
		{
			PolicyKey:        "module.lifecycle.release-lock",
			Scope:            "module.lifecycle.release-lock",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		newSensitiveOperationPolicy("tenant.governance.change", "platform:tenant:governance", "", "PUT"),
		newSensitiveOperationPolicy("tenant.data.export", "platform:tenant:export", "", "POST"),
		newSensitiveOperationPolicy("tenant.permissions.sync", "platform:tenant:permissions", "", "PUT"),
		newSensitiveOperationPolicy("tenant.plan.change", "platform:tenant:updatePlan", "", "PUT"),
		newSensitiveOperationPolicy("tenant.status.change", "platform:tenant:suspend", "", "PUT"),
		newSensitiveOperationPolicy("user.password.reset", "platform:user:password", "permission:user:password", "PUT"),
		newSensitiveOperationPolicy("user.roles.sync", "platform:user:setRole", "permission:user:setRole", "PUT"),
		{
			PolicyKey:        "tenant.data.delete",
			Scope:            "tenant.data.delete",
			Permission:       "platform:tenant:destroy",
			Action:           "DELETE",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
	}
	registry := &SensitiveOperationPolicyRegistry{
		policies:      make(map[string]SensitiveOperationPolicy, len(policies)),
		bindingDigest: sensitiveOperationBindingDigest,
	}
	for _, policy := range policies {
		registry.policies[policy.PolicyKey] = policy
	}
	return registry
}

func (r *SensitiveOperationPolicyRegistry) Policy(policyKey string) (SensitiveOperationPolicy, bool) {
	if r == nil {
		return SensitiveOperationPolicy{}, false
	}
	policy, ok := r.policies[strings.TrimSpace(policyKey)]
	return policy, ok
}

func (r *SensitiveOperationPolicyRegistry) PermissionFor(policyKey string, tenantID uint64, resource string) (string, string, error) {
	policy, ok := r.Policy(policyKey)
	if !ok {
		return "", "", ErrSensitiveOperationPolicy
	}
	permission, action := policy.Permission, policy.Action
	if tenantID != 0 {
		permission = policy.TenantPermission
	}
	switch policy.PolicyKey {
	case "tenant.status.change":
		if strings.HasPrefix(resource, "tenant-change:status:") {
			var status int8
			if _, _, err := parseTenantSensitiveResource(resource, "status", &status); err != nil {
				return "", "", err
			}
			resource = ":status:" + tenantStatusName(status)
		}
		switch {
		case strings.HasSuffix(resource, ":status:suspended"):
			permission = "platform:tenant:suspend"
		case strings.HasSuffix(resource, ":status:active"):
			permission = "platform:tenant:resume"
		case strings.HasSuffix(resource, ":status:archived"):
			permission = "platform:tenant:archive"
		default:
			return "", "", ErrSensitiveOperationPolicy
		}
	case "sso.provider.secret.change":
		permission, action = mutationPermission(resource, "sso-provider", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete")
	case "storage.secret.change":
		permission, action = mutationPermission(resource, "storage-config", "platform:storageConfig:save", "platform:storageConfig:update", "platform:storageConfig:delete")
	}
	if strings.TrimSpace(permission) == "" || strings.TrimSpace(action) == "" {
		return "", "", ErrSensitiveOperationPolicy
	}
	return permission, action, nil
}

func mutationPermission(resource, prefix, createPermission, updatePermission, deletePermission string) (string, string) {
	switch {
	case resource == prefix+":create":
		return createPermission, "POST"
	case strings.HasPrefix(resource, prefix+":update:"):
		return updatePermission, "PUT"
	case strings.HasPrefix(resource, prefix+":delete:"):
		return deletePermission, "DELETE"
	default:
		return "", ""
	}
}

func (r *SensitiveOperationPolicyRegistry) Export() []SensitiveOperationPolicyContract {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.policies))
	for key := range r.policies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	contracts := make([]SensitiveOperationPolicyContract, 0, len(keys))
	for _, key := range keys {
		policy := r.policies[key]
		contracts = append(contracts, SensitiveOperationPolicyContract{
			PolicyKey: policy.PolicyKey, Scope: policy.Scope, Permission: policy.Permission,
			TenantPermission: policy.TenantPermission, Action: policy.Action,
			RequiresReAuth: policy.RequiresReAuth, RequiresApproval: policy.RequiresApproval,
		})
	}
	return contracts
}

func (r *SensitiveOperationPolicyRegistry) RouteContracts() []SensitiveOperationRouteContract {
	contracts := sensitiveOperationRouteContracts()
	sort.Slice(contracts, func(i, j int) bool {
		return contracts[i].RouteName < contracts[j].RouteName
	})
	return contracts
}

func sensitiveOperationRouteContracts() []SensitiveOperationRouteContract {
	return []SensitiveOperationRouteContract{
		newSensitiveRoute("platform.module-lifecycle.execute", "POST", "/admin/platform/module-lifecycle/execute", "module.lifecycle.execute", "platform", "module-lifecycle:alpha:upgrade", "platform:moduleLifecycle:execute", "TestPlatformAdminExecuteRequiresRegisteredApprovalAndBoundReAuth"),
		newSensitiveRoute("platform.module-lifecycle.release", "POST", "/admin/platform/module-lifecycle/locks/release-stale", "module.lifecycle.release-lock", "platform", "module-lifecycle:stale-locks:module-lifecycle:alpha", "platform:moduleLifecycle:execute", "TestStaleLockReleaseConsumesSecurityEvidenceOnce"),
		newSensitiveRoute("platform.role.set-menu", "PUT", "/admin/platform/role/{id}/permissions", "role.permissions.sync", "platform", "rbac:role:9:permissions:W10", "platform:role:setMenu", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.security.mfa.disable", "POST", "/admin/platform/security/mfa/disable", "mfa.disable", "platform", "mfa:user:9:disable", "platform:security:mfa", "TestPlatformMFADisableRequiresSensitiveEvidence"),
		newSensitiveRoute("platform.storage-config.create", "POST", "/admin/platform/storage-config", "storage.secret.change", "platform", "storage-config:create", "platform:storageConfig:save", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.storage-config.delete", "DELETE", "/admin/platform/storage-config", "storage.secret.change", "platform", "storage-config:delete:9", "platform:storageConfig:delete", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.storage-config.update", "PUT", "/admin/platform/storage-config/{id}", "storage.secret.change", "platform", "storage-config:update:9", "platform:storageConfig:update", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.archive", "PUT", "/admin/platform/tenant/{id}/archive", "tenant.status.change", "platform", "tenant:9:status:archived", "platform:tenant:archive", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.destroy", "DELETE", "/admin/platform/tenant", "tenant.data.delete", "platform", "tenant-data:delete:9:metadata", "platform:tenant:destroy", "TestPlatformTenantDestroyRequiresBoundApproval"),
		newSensitiveRoute("platform.tenant.export", "POST", "/admin/platform/tenant/{id}/exports", "tenant.data.export", "platform", "tenant-data:export:9:users:jsonl:sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "platform:tenant:export", "TestPlatformTenantExportRequiresBoundApproval"),
		newSensitiveRoute("platform.tenant.resume", "PUT", "/admin/platform/tenant/{id}/resume", "tenant.status.change", "platform", "tenant:9:status:active", "platform:tenant:resume", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.set-governance", "PUT", "/admin/platform/tenant/{id}/governance", "tenant.governance.change", "platform", "tenant-change:governance:e30", "platform:tenant:governance", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.set-permissions", "PUT", "/admin/platform/tenant/{id}/permissions", "tenant.permissions.sync", "platform", "tenant-change:permissions:e30", "platform:tenant:permissions", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.suspend", "PUT", "/admin/platform/tenant/{id}/suspend", "tenant.status.change", "platform", "tenant:9:status:suspended", "platform:tenant:suspend", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.update-plan", "PUT", "/admin/platform/tenant/{id}/plan", "tenant.plan.change", "platform", "tenant-change:plan:e30", "platform:tenant:updatePlan", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.user.password", "PUT", "/admin/platform/user/password", "user.password.reset", "platform", "rbac:user:9:password:reset", "platform:user:password", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.user.set-roles", "PUT", "/admin/platform/user/{id}/roles", "user.roles.sync", "platform", "rbac:user:9:roles:W10", "platform:user:setRole", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.role.set-permissions", "PUT", "/admin/role/{id}/permissions", "role.permissions.sync", "tenant", "rbac:role:9:permissions:W10", "permission:role:setMenu", "TestTenantRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.security.mfa.disable", "POST", "/admin/security/mfa/disable", "mfa.disable", "tenant", "mfa:user:9:disable", "security:mfa", "TestTenantMFADisableRequiresSensitiveEvidence"),
		newSensitiveRoute("tenant.sso-provider.create", "POST", "/admin/sso-provider", "sso.provider.secret.change", "tenant", "sso-provider:create", "security:ssoProvider:save", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.sso-provider.delete", "DELETE", "/admin/sso-provider", "sso.provider.secret.change", "tenant", "sso-provider:delete:9", "security:ssoProvider:delete", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.sso-provider.update", "PUT", "/admin/sso-provider/{id}", "sso.provider.secret.change", "tenant", "sso-provider:update:9", "security:ssoProvider:update", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.user.password", "PUT", "/admin/user/password", "user.password.reset", "tenant", "rbac:user:9:password:reset", "permission:user:password", "TestTenantRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.user.set-roles", "PUT", "/admin/user/{id}/roles", "user.roles.sync", "tenant", "rbac:user:9:roles:W10", "permission:user:setRole", "TestTenantRBACSensitiveMutationsRequireEvidence"),
	}
}

func newSensitiveRoute(routeName, method, path, policyKey, domain, resource, permission, featureTest string) SensitiveOperationRouteContract {
	return SensitiveOperationRouteContract{
		RouteName: routeName, Method: method, Path: path, PolicyKey: policyKey,
		Domain: domain, Resource: resource, Permission: permission, FeatureTest: featureTest,
	}
}

func (r *SensitiveOperationPolicyRegistry) Prepare(
	_ context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	input SensitiveOperationPrepareInput,
) (SensitiveOperationPlan, error) {
	policy, ok := r.Policy(policyKey)
	if !ok || actorID == 0 {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	resource, err := policy.ResourceBuilder(input)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	before := canonicalSnapshot(input.Before)
	after := canonicalSnapshot(input.After)
	digest, err := r.bindingDigestForPolicy()(policy.PolicyKey, policy.Scope, resource, tenantID, before, after)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	return SensitiveOperationPlan{
		PolicyKey: policy.PolicyKey, Scope: policy.Scope, Resource: resource, BindingDigest: digest,
		ActorID: actorID, TenantID: tenantID, Before: before, After: after,
	}, nil
}

func (r *SensitiveOperationPolicyRegistry) bindingDigestForPolicy() func(string, string, string, uint64, []string, []string) (string, error) {
	if r != nil && r.bindingDigest != nil {
		return r.bindingDigest
	}
	return sensitiveOperationBindingDigest
}

func sensitiveOperationInputResource(input SensitiveOperationPrepareInput) (string, error) {
	resource := strings.TrimSpace(input.Resource)
	if resource == "" {
		return "", ErrSensitiveOperationPolicy
	}
	return resource, nil
}

func canonicalSnapshot(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[strings.TrimSpace(value)] = struct{}{}
	}
	delete(unique, "")
	canonical := make([]string, 0, len(unique))
	for value := range unique {
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	return canonical
}

func sensitiveOperationBindingDigest(policyKey, scope, resource string, tenantID uint64, before, after []string) (string, error) {
	payload, err := json.Marshal(struct {
		PolicyKey string   `json:"policy_key"`
		Scope     string   `json:"scope"`
		Resource  string   `json:"resource"`
		TenantID  uint64   `json:"tenant_id"`
		Before    []string `json:"before"`
		After     []string `json:"after"`
	}{
		PolicyKey: strings.TrimSpace(policyKey), Scope: strings.TrimSpace(scope), Resource: strings.TrimSpace(resource),
		TenantID: tenantID,
		Before:   canonicalSnapshot(before), After: canonicalSnapshot(after),
	})
	if err != nil {
		return "", err
	}
	return sha256Hex(payload), nil
}

func newSensitiveOperationPolicy(policyKey, platformPermission, tenantPermission, action string) SensitiveOperationPolicy {
	return SensitiveOperationPolicy{
		PolicyKey: policyKey, Scope: policyKey, Permission: platformPermission, TenantPermission: tenantPermission,
		Action: action, RequiresReAuth: true, RequiresApproval: true, ResourceBuilder: sensitiveOperationInputResource,
	}
}

// Source: sensitive_operation_rbac.go
func resolveRBACSensitivePlan(ctx context.Context, policyKey string, tenantID uint64, selector string) (string, []string, []string, error) {
	switch policyKey {
	case "user.password.reset":
		resource, userID, err := parseRBACPasswordResetSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		if err := requireRBACUser(ctx, tenantID, userID); err != nil {
			return "", nil, nil, err
		}
		before, after, err := rbacPasswordSnapshots(userID, true)
		return resource, before, after, err
	case "user.roles.sync":
		resource, userID, roles, err := parseRBACUserRolesSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		before, err := rbacUserRolesSnapshot(ctx, tenantID, userID)
		if err != nil {
			return "", nil, nil, err
		}
		after, err := rbacSnapshot("user_roles", userID, roles)
		return resource, before, after, err
	case "role.permissions.sync":
		resource, roleID, permissions, err := parseRBACRolePermissionsSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		before, err := rbacRolePermissionsSnapshot(ctx, tenantID, roleID)
		if err != nil {
			return "", nil, nil, err
		}
		after, err := rbacSnapshot("role_permissions", roleID, permissions)
		return resource, before, after, err
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func rbacUserRolesSelector(userID uint64, roleCodes []string) (string, error) {
	return rbacSelector("user", userID, "roles", roleCodes)
}

func rbacRolePermissionsSelector(roleID uint64, permissions []string) (string, error) {
	return rbacSelector("role", roleID, "permissions", permissions)
}

func rbacPasswordResetSelector(userID uint64) string {
	return "rbac:user:" + strconv.FormatUint(userID, 10) + ":password:reset"
}

func parseRBACUserRolesSelector(selector string) (string, uint64, []string, error) {
	resource, id, desired, err := parseRBACSelector(selector, "user", "roles")
	return resource, id, desired, err
}

func parseRBACRolePermissionsSelector(selector string) (string, uint64, []string, error) {
	resource, id, desired, err := parseRBACSelector(selector, "role", "permissions")
	return resource, id, desired, err
}

func parseRBACPasswordResetSelector(selector string) (string, uint64, error) {
	parts := strings.Split(strings.TrimSpace(selector), ":")
	if len(parts) != 5 || parts[0] != "rbac" || parts[1] != "user" || parts[3] != "password" || parts[4] != "reset" {
		return "", 0, ErrSensitiveOperationPolicy
	}
	userID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || userID == 0 {
		return "", 0, ErrSensitiveOperationPolicy
	}
	return rbacPasswordResetSelector(userID), userID, nil
}

func rbacSelector(subject string, id uint64, operation string, desired []string) (string, error) {
	if id == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	canonical := canonicalRBACValues(desired)
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"rbac", subject, strconv.FormatUint(id, 10), operation,
		base64.RawURLEncoding.EncodeToString(payload),
	}, ":"), nil
}

func parseRBACSelector(selector, subject, operation string) (string, uint64, []string, error) {
	parts := strings.SplitN(strings.TrimSpace(selector), ":", 5)
	if len(parts) != 5 || parts[0] != "rbac" || parts[1] != subject || parts[3] != operation {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	id, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || id == 0 {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	desired := make([]string, 0)
	if json.Unmarshal(payload, &desired) != nil || !sameRBACValues(desired, canonicalRBACValues(desired)) {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	resource := strings.Join([]string{
		"rbac", subject, strconv.FormatUint(id, 10), operation, sha256Hex(payload),
	}, ":")
	return resource, id, desired, nil
}

func canonicalRBACValues(values []string) []string {
	return canonicalSnapshot(values)
}

func sameRBACValues(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func rbacPasswordSnapshots(userID uint64, configured bool) ([]string, []string, error) {
	state := "absent"
	if configured {
		state = "configured"
	}
	before, err := rbacSnapshot("user_password", userID, []string{state})
	if err != nil {
		return nil, nil, err
	}
	after, err := rbacSnapshot("user_password", userID, []string{"reset"})
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

func rbacUserRolesSnapshot(ctx context.Context, tenantID, userID uint64) ([]string, error) {
	roles := make([]string, 0)
	roleTable, relationTable := "role", "user_belongs_role"
	if tenantID == 0 {
		roleTable, relationTable = "platform_role", "platform_user_belongs_role"
	}
	err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(roleTable).
		Select(roleTable+".code").
		Join("JOIN "+relationTable+" ubr ON ubr.role_id = "+roleTable+".id").
		Where("ubr.user_id", userID).
		OrderBy(roleTable+".code").
		Pluck(roleTable+".code", &roles)
	if err != nil {
		return nil, err
	}
	return rbacSnapshot("user_roles", userID, roles)
}

func rbacRolePermissionsSnapshot(ctx context.Context, tenantID, roleID uint64) ([]string, error) {
	permissions := make([]string, 0)
	menuTable, relationTable := "menu", "role_belongs_menu"
	if tenantID == 0 {
		menuTable, relationTable = "platform_menu", "platform_role_belongs_menu"
	}
	err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(menuTable).
		Select(menuTable+".name").
		Join("JOIN "+relationTable+" rbm ON rbm.menu_id = "+menuTable+".id").
		Where("rbm.role_id", roleID).
		OrderBy(menuTable+".name").
		Pluck(menuTable+".name", &permissions)
	if err != nil {
		return nil, err
	}
	return rbacSnapshot("role_permissions", roleID, permissions)
}

func requireRBACUser(ctx context.Context, tenantID, userID uint64) error {
	count, err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(rbacUserTable(tenantID)).Where("id", userID).Count()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSensitiveOperationPolicy
	}
	return nil
}

func rbacConnection(ctx context.Context, tenantID uint64) string {
	if tenantID == 0 {
		return PlatformConnection()
	}
	return TenantConnectionFromContext(ctx)
}

func rbacUserTable(tenantID uint64) string {
	if tenantID == 0 {
		return "platform_user"
	}
	return "user"
}

func (s *PermissionAdminService) ResetPasswordSensitive(actorID, userID uint64, evidence SensitiveOperationEvidence) error {
	return executeRBACSensitive(s.ctx, "user.password.reset", actorID, s.tenant.ID, rbacPasswordResetSelector(userID), evidence, func() error {
		return s.ResetPassword(userID)
	})
}

func (s *PermissionAdminService) SyncUserRolesSensitive(actorID, userID uint64, roleCodes []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacUserRolesSelector(userID, roleCodes)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "user.roles.sync", actorID, s.tenant.ID, selector, evidence, func() error {
		return s.SyncUserRoles(userID, roleCodes)
	})
}

func (s *PermissionAdminService) SyncRolePermissionsSensitive(actorID, roleID uint64, permissions []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacRolePermissionsSelector(roleID, permissions)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "role.permissions.sync", actorID, s.tenant.ID, selector, evidence, func() error {
		return s.SyncRolePermissions(roleID, permissions)
	})
}

func (s *PlatformPermissionAdminService) ResetPasswordSensitive(actorID, userID uint64, evidence SensitiveOperationEvidence) error {
	return executeRBACSensitive(s.ctx, "user.password.reset", actorID, 0, rbacPasswordResetSelector(userID), evidence, func() error {
		return s.ResetPassword(userID)
	})
}

func (s *PlatformPermissionAdminService) SyncUserRolesSensitive(actorID, userID uint64, roleCodes []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacUserRolesSelector(userID, roleCodes)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "user.roles.sync", actorID, 0, selector, evidence, func() error {
		return s.SyncUserRoles(userID, roleCodes)
	})
}

func (s *PlatformPermissionAdminService) SyncRolePermissionsSensitive(actorID, roleID uint64, permissions []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacRolePermissionsSelector(roleID, permissions)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "role.permissions.sync", actorID, 0, selector, evidence, func() error {
		return s.SyncRolePermissions(roleID, permissions)
	})
}

func executeRBACSensitive(ctx context.Context, policyKey string, actorID, tenantID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, tenantID, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func rbacSnapshot(kind string, id uint64, values []string) ([]string, error) {
	raw, err := json.Marshal(struct {
		Kind   string   `json:"kind"`
		ID     uint64   `json:"id"`
		Values []string `json:"values"`
	}{Kind: kind, ID: id, Values: canonicalRBACValues(values)})
	if err != nil {
		return nil, err
	}
	return []string{string(raw)}, nil
}

// Source: sensitive_operation_secrets.go
func resolveSecretSensitivePlan(
	ctx context.Context,
	policyKey string,
	tenantID uint64,
	selector string,
) (string, []string, []string, error) {
	resourceType, action, err := secretSensitiveTarget(policyKey, selector)
	if err != nil {
		return "", nil, nil, err
	}
	before, after, err := secretSensitiveSnapshots(resourceType, action)
	if err != nil {
		return "", nil, nil, err
	}
	if action != "create" {
		ids := strings.Split(strings.TrimPrefix(selector, strings.ReplaceAll(resourceType, "_", "-")+":"+action+":"), ",")
		metadata, metadataErr := secretRotationMetadata(ctx, policyKey, tenantID, ids)
		if metadataErr != nil {
			return "", nil, nil, metadataErr
		}
		before = metadata
	}
	return selector, before, after, nil
}

func secretRotationMetadata(ctx context.Context, policyKey string, tenantID uint64, ids []string) ([]string, error) {
	connection, table := PlatformConnection(), "storage_config"
	columns := []string{"id", "secret_key_rotated_at"}
	if policyKey == "sso.provider.secret.change" {
		table = "sso_provider"
		columns = []string{"id", "jwt_secret_rotated_at", "client_secret_rotated_at", "updated_at"}
		if tenantID == 0 {
			return nil, ErrSensitiveOperationPolicy
		}
		connection = TenantConnectionFromContext(ctx)
	}
	rows := make([]map[string]any, 0)
	if err := OrmForConnectionWithContext(ctx, connection).Query().Table(table).Select(strings.Join(columns, ",")).WhereIn("id", stringAny(ids)).OrderBy("id").Get(&rows); err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, ErrSensitiveOperationPolicy
	}
	return sensitiveSnapshots(rows)
}

func secretSensitiveTarget(policyKey, selector string) (string, string, error) {
	policyKey = strings.TrimSpace(policyKey)
	parts := strings.Split(strings.TrimSpace(selector), ":")
	if len(parts) < 2 {
		return "", "", ErrSensitiveOperationPolicy
	}
	resourceType := parts[0]
	if (policyKey == "sso.provider.secret.change" && resourceType != "sso-provider") ||
		(policyKey == "storage.secret.change" && resourceType != "storage-config") {
		return "", "", ErrSensitiveOperationPolicy
	}
	action := parts[1]
	if action == "create" && len(parts) == 2 {
		return strings.ReplaceAll(resourceType, "-", "_"), action, nil
	}
	if (action != "update" && action != "delete") || len(parts) != 3 {
		return "", "", ErrSensitiveOperationPolicy
	}
	ids := strings.Split(parts[2], ",")
	if action == "update" && len(ids) != 1 {
		return "", "", ErrSensitiveOperationPolicy
	}
	for _, value := range ids {
		if id, err := strconv.ParseUint(value, 10, 64); err != nil || id == 0 {
			return "", "", ErrSensitiveOperationPolicy
		}
	}
	return strings.ReplaceAll(resourceType, "-", "_"), action, nil
}

func secretDeleteSelector(prefix string, ids []uint64) (string, error) {
	canonical := append([]uint64(nil), ids...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
	parts := make([]string, 0, len(canonical))
	var previous uint64
	for _, id := range canonical {
		if id == 0 {
			return "", ErrSensitiveOperationPolicy
		}
		if id != previous {
			parts = append(parts, strconv.FormatUint(id, 10))
			previous = id
		}
	}
	if len(parts) == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	return prefix + ":delete:" + strings.Join(parts, ","), nil
}

func executeSecretSensitive(ctx context.Context, policyKey string, actorID, tenantID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, tenantID, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func (s *SSOProviderService) CreateSensitive(input SSOProviderPayload, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) (SSOProvider, error) {
	if !ssoPayloadChangesProtectedConfiguration(input, SSOProvider{}) {
		return s.Create(input, operatorID)
	}
	var result SSOProvider
	err := executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, "sso-provider:create", evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Create(input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *SSOProviderService) UpdateSensitive(id uint64, input SSOProviderPayload, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) (SSOProvider, error) {
	var existing SSOProvider
	if err := s.orm().Query().Table("sso_provider").Where("id", id).First(&existing); err != nil {
		return SSOProvider{}, err
	}
	if !ssoPayloadChangesProtectedConfiguration(input, existing) {
		return s.Update(id, input, operatorID)
	}
	var result SSOProvider
	err := executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, "sso-provider:update:"+strconv.FormatUint(id, 10), evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Update(id, input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *SSOProviderService) DeleteSensitive(ids []uint64, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) error {
	selector, err := secretDeleteSelector("sso-provider", ids)
	if err != nil {
		return err
	}
	return executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, selector, evidence, func() error {
		return s.Delete(ids)
	})
}

func ssoPayloadChangesProtectedConfiguration(input SSOProviderPayload, existing SSOProvider) bool {
	next := input.Provider()
	return secretChanged(input.JWTSecret, existing.JWTSecret) ||
		secretChanged(input.ClientSecret, existing.ClientSecret) ||
		trimmedChanged(input.Issuer, existing.Issuer) ||
		trimmedChanged(input.DiscoveryURL, existing.DiscoveryURL) ||
		trimmedChanged(input.AuthorizationEndpoint, existing.AuthorizationEndpoint) ||
		trimmedChanged(input.TokenEndpoint, existing.TokenEndpoint) ||
		trimmedChanged(input.UserinfoEndpoint, existing.UserinfoEndpoint) ||
		trimmedChanged(input.JWKSURI, existing.JWKSURI) ||
		trimmedChanged(input.JWKSJSON, existing.JWKSJSON) ||
		trimmedChanged(input.Type, existing.Type) ||
		trimmedChanged(input.Audience, existing.Audience) ||
		trimmedChanged(input.ClientID, existing.ClientID) ||
		trimmedChanged(input.Scope, existing.Scope) ||
		trimmedChanged(input.RedirectURI, existing.RedirectURI) ||
		trimmedChanged(input.SAMLEntrypoint, existing.SAMLEntrypoint) ||
		trimmedChanged(input.SAMLEntityID, existing.SAMLEntityID) ||
		trimmedChanged(input.SAMLCertificate, existing.SAMLCertificate) ||
		next.Enabled != existing.Enabled ||
		next.EnablePKCE != existing.EnablePKCE ||
		next.EnableNonce != existing.EnableNonce ||
		next.AutoCreate != existing.AutoCreate ||
		!jsonMapsEqual(next.RoleMapping, existing.RoleMapping) ||
		!jsonMapsEqual(next.DataPermissionMapping, existing.DataPermissionMapping)
}

func jsonMapsEqual(left, right models.JSONMap) bool {
	leftJSON, leftErr := json.Marshal(nullIfEmpty(left))
	rightJSON, rightErr := json.Marshal(nullIfEmpty(right))
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func (s *StorageConfigService) CreateSensitive(input StorageConfigPayload, operatorID uint64, evidence SensitiveOperationEvidence) (StorageConfig, error) {
	if !storagePayloadChangesProtectedConfiguration(input, StorageConfig{}) {
		return s.Create(input, operatorID)
	}
	var result StorageConfig
	err := executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, "storage-config:create", evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Create(input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *StorageConfigService) UpdateSensitive(id uint64, input StorageConfigPayload, operatorID uint64, evidence SensitiveOperationEvidence) (StorageConfig, error) {
	existing, err := s.find(id)
	if err != nil {
		return StorageConfig{}, err
	}
	if !storagePayloadChangesProtectedConfiguration(input, existing) {
		return s.Update(id, input, operatorID)
	}
	var result StorageConfig
	err = executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, "storage-config:update:"+strconv.FormatUint(id, 10), evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Update(id, input, operatorID)
		return mutationErr
	})
	return result, err
}

func storagePayloadChangesProtectedConfiguration(input StorageConfigPayload, existing StorageConfig) bool {
	next := input.StorageConfig()
	return trimmedChanged(next.Provider, existing.Provider) ||
		trimmedChanged(next.Driver, existing.Driver) ||
		trimmedChanged(next.Bucket, existing.Bucket) ||
		trimmedChanged(next.Endpoint, existing.Endpoint) ||
		trimmedChanged(next.Region, existing.Region) ||
		trimmedChanged(next.AccessKey, existing.AccessKey) ||
		secretChanged(next.SecretKey, existing.SecretKey) ||
		trimmedChanged(next.BaseURL, existing.BaseURL) ||
		trimmedChanged(next.PathPrefix, existing.PathPrefix) ||
		next.IsDefault != existing.IsDefault ||
		next.Status != existing.Status ||
		!jsonMapsEqual(next.Options, existing.Options)
}

func secretChanged(input, existing string) bool {
	return strings.TrimSpace(input) != "" && input != existing
}

func trimmedChanged(input, existing string) bool {
	return strings.TrimSpace(input) != strings.TrimSpace(existing)
}

func (s *StorageConfigService) DeleteSensitive(ids []uint64, operatorID uint64, evidence SensitiveOperationEvidence) error {
	selector, err := secretDeleteSelector("storage-config", ids)
	if err != nil {
		return err
	}
	return executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, selector, evidence, func() error {
		return s.Delete(ids)
	})
}

func secretSensitiveSnapshots(resourceType, action string) ([]string, []string, error) {
	before := map[string]string{"resource_type": resourceType}
	after := map[string]string{"resource_type": resourceType}
	switch action {
	case "create":
		before["state"] = "absent"
		after["secret_presence"] = "changed"
		after["rotation_intent"] = "set"
	case "update":
		before["state"] = "existing"
		after["secret_presence"] = "changed"
		after["rotation_intent"] = "rotate"
	case "delete":
		before["state"] = "existing"
		after["secret_presence"] = "removed"
		after["rotation_intent"] = "remove"
	default:
		return nil, nil, ErrSensitiveOperationPolicy
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, nil, err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return nil, nil, err
	}
	return []string{string(beforeJSON)}, []string{string(afterJSON)}, nil
}

// Source: sensitive_operation_tenant.go
type tenantSensitiveSelector struct {
	TenantID uint64          `json:"tenant_id"`
	Desired  json.RawMessage `json:"desired"`
}

func tenantSensitiveResource(kind string, tenantID uint64, desired any) (string, error) {
	if tenantID == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	raw, err := json.Marshal(desired)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(tenantSensitiveSelector{TenantID: tenantID, Desired: raw})
	if err != nil {
		return "", err
	}
	return "tenant-change:" + kind + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func parseTenantSensitiveResource(resource, kind string, desired any) (uint64, string, error) {
	prefix := "tenant-change:" + kind + ":"
	if !strings.HasPrefix(resource, prefix) {
		return 0, "", ErrSensitiveOperationPolicy
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(resource, prefix))
	if err != nil {
		return 0, "", ErrSensitiveOperationPolicy
	}
	var selector tenantSensitiveSelector
	if json.Unmarshal(payload, &selector) != nil || selector.TenantID == 0 || json.Unmarshal(selector.Desired, desired) != nil {
		return 0, "", ErrSensitiveOperationPolicy
	}
	canonicalDesired, err := json.Marshal(desired)
	if err != nil {
		return 0, "", err
	}
	canonicalPayload, err := json.Marshal(tenantSensitiveSelector{TenantID: selector.TenantID, Desired: canonicalDesired})
	if err != nil {
		return 0, "", err
	}
	return selector.TenantID, fmt.Sprintf("tenant:%d:%s:%s", selector.TenantID, kind, sha256Hex(canonicalPayload)), nil
}

func resolveTenantSensitivePlan(ctx context.Context, policyKey, selector string) (string, []string, []string, error) {
	service := NewTenantService().WithContext(ctx)
	switch policyKey {
	case "tenant.permissions.sync":
		var desired TenantPermissionPayload
		id, resource, err := parseTenantSensitiveResource(selector, "permissions", &desired)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		return tenantSensitiveSnapshots(resource, TenantEffectivePermissionPayload(tenant), normalizePermissionPayload(desired))
	case "tenant.plan.change":
		var desired TenantPlanUpdatePayload
		id, resource, err := parseTenantSensitiveResource(selector, "plan", &desired)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		plan, err := NewTenantPlanService().WithContext(ctx).ActiveByCode(strings.TrimSpace(desired.Plan))
		if err != nil || plan.ID == 0 {
			return "", nil, nil, ErrSensitiveOperationPolicy
		}
		afterPermissions, _ := tenantPermissionPayloadFromFeatures(SnapshotFeaturesForPlan(plan.Features, desired.Features))
		return tenantSensitiveSnapshots(resource,
			map[string]any{"plan": tenant.Plan, "permissions": TenantEffectivePermissionPayload(tenant).Allowed},
			map[string]any{"plan": plan.Code, "permissions": normalizePermissionPayload(afterPermissions).Allowed},
		)
	case "tenant.governance.change":
		var patch TenantGovernancePatch
		id, resource, err := parseTenantSensitiveResource(selector, "governance", &patch)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		governance := NewTenantGovernanceService().WithContext(ctx)
		before, err := governance.Policy(tenant)
		if err != nil {
			return "", nil, nil, err
		}
		after := before
		applyTenantGovernancePatch(&after, patch)
		normalizeTenantGovernancePolicy(&after)
		return tenantSensitiveSnapshots(resource, before, after)
	case "tenant.status.change":
		var status int8
		id, _, err := parseTenantSensitiveResource(selector, "status", &status)
		if err != nil || !validTenantStatus(status) {
			return "", nil, nil, ErrSensitiveOperationPolicy
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		resource := "tenant:" + strconv.FormatUint(id, 10) + ":status:" + tenantStatusName(status)
		return tenantSensitiveSnapshots(resource, tenant.Status, status)
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func tenantSensitiveSnapshots(resource string, before, after any) (string, []string, []string, error) {
	beforeSnapshot, err := sensitiveSnapshot(before)
	if err != nil {
		return "", nil, nil, err
	}
	afterSnapshot, err := sensitiveSnapshot(after)
	if err != nil {
		return "", nil, nil, err
	}
	return resource, []string{beforeSnapshot}, []string{afterSnapshot}, nil
}

func tenantStatusName(status int8) string {
	switch status {
	case TenantStatusActive:
		return "active"
	case TenantStatusSuspended:
		return "suspended"
	case TenantStatusArchived:
		return "archived"
	default:
		return "invalid"
	}
}

func executeTenantSensitive(ctx context.Context, policyKey string, actorID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, 0, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func (s *TenantService) UpdatePermissionsSensitive(id uint64, payload TenantPermissionPayload, operator TenantPermissionOperator, evidence SensitiveOperationEvidence) (TenantPermissionPayload, error) {
	selector, err := tenantSensitiveResource("permissions", id, normalizePermissionPayload(payload))
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	var result TenantPermissionPayload
	err = executeTenantSensitive(s.ctx, "tenant.permissions.sync", operator.ID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.UpdatePermissions(id, payload, operator)
		return mutationErr
	})
	return result, err
}

func (s *TenantService) UpdatePlanSensitive(id uint64, input TenantPlanUpdatePayload, operator TenantPermissionOperator, evidence SensitiveOperationEvidence) (Tenant, error) {
	selector, err := tenantSensitiveResource("plan", id, input)
	if err != nil {
		return Tenant{}, err
	}
	var result Tenant
	err = executeTenantSensitive(s.ctx, "tenant.plan.change", operator.ID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.UpdatePlan(id, input, operator)
		return mutationErr
	})
	return result, err
}

func (s *TenantService) UpdateStatusSensitive(actorID, id uint64, status int8, evidence SensitiveOperationEvidence) error {
	selector, err := tenantSensitiveResource("status", id, status)
	if err != nil {
		return err
	}
	return executeTenantSensitive(s.ctx, "tenant.status.change", actorID, selector, evidence, func() error {
		return s.UpdateStatus(id, status)
	})
}

func (s *TenantGovernanceService) PatchPolicySensitive(actorID uint64, tenant Tenant, patch TenantGovernancePatch, evidence SensitiveOperationEvidence) (TenantGovernancePolicy, error) {
	if !s.hasGovernanceTable() {
		return TenantGovernancePolicy{}, ErrSensitiveOperationPolicy
	}
	selector, err := tenantSensitiveResource("governance", tenant.ID, patch)
	if err != nil {
		return TenantGovernancePolicy{}, err
	}
	var result TenantGovernancePolicy
	err = executeTenantSensitive(s.ctx, "tenant.governance.change", actorID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.PatchPolicy(tenant, patch)
		return mutationErr
	})
	return result, err
}

// Source: sensitive_operation_transaction.go
func (g *SensitiveOperationGuard) ExecutePlatformTransaction(
	ctx context.Context,
	plan SensitiveOperationPlan,
	evidence SensitiveOperationEvidence,
	mutate func(contractsorm.Query) error,
) error {
	policy, err := g.validPolicy(plan)
	if err != nil || mutate == nil {
		return ErrSensitiveOperationPolicy
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope,
		Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	operation := func() error {
		return OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Transaction(func(query contractsorm.Query) error {
			if policy.RequiresApproval {
				approval, ok, loadErr := g.security.loadPermissionApproval(ctx, evidence.ApprovalID)
				if loadErr != nil || !ok || validRegisteredPermissionApprovalBinding(approval, plan) != nil {
					return ErrApprovalRequired
				}
				if consumeErr := consumePermissionApprovalBindingWithQuery(query, evidence.ApprovalID, approval, plan); consumeErr != nil {
					return consumeErr
				}
			}
			return mutate(query)
		})
	}
	if policy.RequiresReAuth {
		err = g.security.ExecuteSensitiveOperationNoRestore(request, nil, operation)
	} else {
		err = operation()
	}
	if err != nil {
		g.recordAudit(ctx, plan, evidence.ApprovalID, "operation_failed")
		return err
	}
	g.recordAudit(ctx, plan, evidence.ApprovalID, "success")
	return nil
}

// Source: support.go
const RedactedValue = redaction.Value

type outboundHTTPPolicy struct {
	invalidURL     func() error
	unresolvedHost func() error
	invalidAddress func() error
	validateURL    func(*url.URL) error
	validateTarget func(*url.URL, []net.IP) error
}

func sha256Hex(payload []byte) string {
	return digest.SHA256Hex(payload)
}

func digestBytes(payload []byte) string {
	return digest.SHA256(payload)
}

func payloadIDs(values []any, key string) []uint64 {
	return idutil.PayloadIDs(values, key)
}

func compactPositiveIDs(values []uint64) []uint64 {
	return idutil.CompactPositiveIDs(values)
}

func stringAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func RedactSensitiveData(value any) any {
	return redaction.SensitiveData(value)
}

func securityStringSlice(key string) []string {
	return redaction.ConfigStringSlice(key)
}

func safeOutboundURL(raw string, policy outboundHTTPPolicy) (*url.URL, error) {
	return safehttp.URL(raw, safeOutboundPolicy(policy))
}

func safeOutboundHTTPClient(timeout time.Duration, policy outboundHTTPPolicy) http.Client {
	return safehttp.Client(timeout, safeOutboundPolicy(policy))
}

func isPrivateOutboundIP(ip net.IP) bool {
	return safehttp.IsPrivateIP(ip)
}

func safeOutboundPolicy(policy outboundHTTPPolicy) safehttp.Policy {
	return safehttp.Policy{
		InvalidURL:     policy.invalidURL,
		UnresolvedHost: policy.unresolvedHost,
		InvalidAddress: policy.invalidAddress,
		ValidateURL:    policy.validateURL,
		ValidateTarget: policy.validateTarget,
	}
}
