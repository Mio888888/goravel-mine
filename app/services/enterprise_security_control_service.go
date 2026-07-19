package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	frameworkcache "github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/facades"
	"goravel/app/models"
)

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
