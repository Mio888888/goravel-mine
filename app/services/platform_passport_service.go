package services

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

const platformTokenSubject = "platform"

type PlatformPassportService struct {
	ctx context.Context
}

func NewPlatformPassportService() *PlatformPassportService {
	return &PlatformPassportService{}
}

func (s *PlatformPassportService) WithContext(ctx context.Context) *PlatformPassportService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PlatformPassportService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (s *PlatformPassportService) Orm() contractsorm.Orm {
	return s.orm()
}

func (s *PlatformPassportService) Login(username, password string, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	if err := NewPlatformBootstrapService().EnsureLocalDefaults(); err != nil {
		return LoginResult{}, err
	}
	if err := CheckLoginLockout("platform", username); err != nil {
		return LoginResult{}, err
	}
	if err := CheckLoginRisk("platform", username, signal.IP, signal.UserAgent); err != nil {
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().
		Table("platform_user").
		Where("username", username).
		Where("user_type", "900").
		First(&user); err != nil {
		_ = RecordLoginFailure("platform", username)
		_ = RecordLoginRiskFailure("platform", username, signal.IP, signal.UserAgent)
		return LoginResult{}, ErrInvalidCredentials
	}

	if !passwordHashMatches(user.Password, password) {
		_ = RecordLoginFailure("platform", username)
		_ = RecordLoginRiskFailure("platform", username, signal.IP, signal.UserAgent)
		return LoginResult{}, ErrInvalidCredentials
	}
	passwordHistory := PlatformPasswordHistoryService().WithContext(s.ctx)
	if err := passwordHistory.SeedIfMissing(user); err != nil {
		return LoginResult{}, err
	}

	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	if mfa := NewPlatformMFAService().WithContext(s.ctx); mfa.Enabled(user.ID) {
		token, err := mfa.IssueChallenge("platform", user.ID)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{MFARequired: true, MFAToken: token}, nil
	}
	if result, challenged, err := s.passwordChangeChallengeIfExpired(passwordHistory, user.ID); err != nil || challenged {
		return result, err
	}
	_ = RecordLoginSuccess("platform", username)
	_ = RecordLoginRiskSuccess("platform", username, signal.IP, signal.UserAgent)

	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) CompleteMFALogin(mfaToken, code string, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	mfa := NewPlatformMFAService().WithContext(s.ctx)
	userID, err := mfa.ChallengeUserID("platform", mfaToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	commitMFA, err := mfa.PrepareVerify(user.ID, code)
	if err != nil {
		_ = RecordLoginFailure("platform", user.Username)
		_ = RecordLoginRiskFailure("platform", user.Username, signal.IP, signal.UserAgent)
		return LoginResult{}, err
	}
	consumedUserID, err := mfa.ConsumeChallenge("platform", mfaToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrMFATokenInvalid
	}
	if err := commitMFA(); err != nil {
		return LoginResult{}, err
	}
	_ = RecordLoginSuccess("platform", user.Username)
	_ = RecordLoginRiskSuccess("platform", user.Username, signal.IP, signal.UserAgent)
	if result, challenged, err := s.passwordChangeChallengeIfExpired(PlatformPasswordHistoryService().WithContext(s.ctx), user.ID); err != nil || challenged {
		return result, err
	}
	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) CompletePasswordChange(passwordChangeToken string, input ProfileUpdate, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	userID, err := PasswordChangeChallengeUserID("platform", passwordChangeToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	if input.NewPassword == "" {
		return LoginResult{}, BusinessError{Message: "新密码不能为空"}
	}
	values := map[string]any{"updated_at": time.Now()}
	if err := s.appendPasswordUpdate(user.ID, input, values); err != nil {
		return LoginResult{}, err
	}
	consumedUserID, err := ConsumePasswordChangeChallenge("platform", passwordChangeToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrPasswordChangeTokenInvalid
	}
	if err := s.updatePasswordWithValues(user.ID, values); err != nil {
		return LoginResult{}, err
	}
	_ = RecordLoginSuccess("platform", user.Username)
	_ = RecordLoginRiskSuccess("platform", user.Username, signal.IP, signal.UserAgent)
	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) UserFromAuthorization(authorization string) (models.User, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return models.User{}, ErrUnauthorized
	}

	userID, err := s.UserIDFromAuthorization(authorization, "access")
	if err != nil {
		return models.User{}, err
	}

	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return models.User{}, ErrUnauthorized
	}

	if user.Status == 2 {
		return models.User{}, ErrUserDisabled
	}

	return user, nil
}

func (s *PlatformPassportService) Refresh(authorization string) (LoginResult, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return LoginResult{}, ErrUnauthorized
	}

	userID, err := s.UserIDFromAuthorization(authorization, "refresh")
	if err != nil {
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}

	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(user.ID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(user.ID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}

	if err := blacklistToken(bearerToken(authorization), tokenBlacklistTTL(refreshTTL)); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpireAt:     accessTTL,
	}, nil
}

func (s *PlatformPassportService) Logout(authorization string) error {
	token := bearerToken(authorization)
	if token == "" {
		return ErrUnauthorized
	}

	if _, err := s.UserIDFromAuthorization(authorization, "access"); err != nil {
		return err
	}

	return blacklistToken(token, tokenBlacklistTTL(AccessTokenTTLSeconds()))
}

func (s *PlatformPassportService) UserIDFromAuthorization(authorization, tokenType string) (uint64, error) {
	tokenText := bearerToken(authorization)
	if tokenText == "" {
		return 0, ErrUnauthorized
	}

	claims := jwt.MapClaims{}
	secret, err := JWTSecret()
	if err != nil {
		return 0, err
	}

	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}

		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return 0, ErrUnauthorized
	}

	if claims["sub"] != platformTokenSubject || claims["type"] != tokenType {
		return 0, ErrUnauthorized
	}

	userID, err := claimUint64(claims["uid"])
	if err != nil || userID == 0 {
		return 0, ErrUnauthorized
	}

	return userID, nil
}

func (s *PlatformPassportService) FormatUserInfo(user models.User) (UserInfo, error) {
	roles, err := s.UserRoles(user.ID)
	if err != nil {
		return UserInfo{}, err
	}

	backendSetting := user.BackendSetting
	if len(backendSetting) == 0 {
		backendSetting = nil
	}

	return UserInfo{
		ID:             user.ID,
		Username:       user.Username,
		Nickname:       user.Nickname,
		Avatar:         user.Avatar,
		Signed:         user.Signed,
		Dashboard:      user.Dashboard,
		BackendSetting: backendSetting,
		Phone:          user.Phone,
		Email:          user.Email,
		Departments:    []DepartmentInfo{},
		Positions:      []PositionInfo{},
		Roles:          roles,
	}, nil
}

func (s *PlatformPassportService) UserRoles(userID uint64) ([]RoleInfo, error) {
	roles := make([]RoleInfo, 0)
	err := s.orm().Query().
		Table("platform_role").
		Select("platform_role.id", "platform_role.code", "platform_role.name").
		Join("JOIN platform_user_belongs_role ubr ON ubr.role_id = platform_role.id").
		Where("ubr.user_id", userID).
		Where("platform_role.status", 1).
		OrderBy("platform_role.sort").
		OrderBy("platform_role.id").
		Scan(&roles)

	return roles, err
}

func (s *PlatformPassportService) IsSuperAdmin(user models.User) (bool, error) {
	if user.ID == 1 {
		return true, nil
	}

	roles, err := s.UserRoles(user.ID)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role.Code == "PlatformSuperAdmin" {
			return true, nil
		}
	}

	return false, nil
}

func (s *PlatformPassportService) UpdateProfile(userID uint64, input ProfileUpdate) error {
	values, err := profileUpdateValues(input)
	if err != nil {
		return err
	}
	if err := s.appendPasswordUpdate(userID, input, values); err != nil {
		return err
	}

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").
			Where("id", userID).
			Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
		}
		return nil
	})

	return err
}

func (s *PlatformPassportService) appendPasswordUpdate(userID uint64, input ProfileUpdate, values map[string]any) error {
	if input.NewPassword == "" {
		return nil
	}
	if input.NewPassword != input.NewPasswordConfirmation {
		return ErrInvalidCredentials
	}
	if err := ValidatePasswordPolicy(input.NewPassword); err != nil {
		return err
	}
	if err := PlatformPasswordHistoryService().WithContext(s.ctx).ValidateReuse(userID, input.NewPassword); err != nil {
		return err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return ErrUnauthorized
	}
	if !passwordHashMatches(user.Password, input.OldPassword) {
		return ErrInvalidCredentials
	}
	password, err := makePasswordHash(input.NewPassword)
	if err != nil {
		return err
	}
	values["password"] = password
	return nil
}

func (s *PlatformPassportService) updatePassword(userID uint64, input ProfileUpdate) error {
	values := map[string]any{"updated_at": time.Now()}
	if err := s.appendPasswordUpdate(userID, input, values); err != nil {
		return err
	}
	return s.updatePasswordWithValues(userID, values)
}

func (s *PlatformPassportService) updatePasswordWithValues(userID uint64, values map[string]any) error {
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").Where("id", userID).Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
		}
		return nil
	})
}

func (s *PlatformPassportService) buildToken(userID uint64, tokenType string, ttlSeconds int) (string, error) {
	secret, err := JWTSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  platformTokenSubject,
		"uid":  userID,
		"type": tokenType,
		"jti":  rand.Text(),
		"iat":  now.Unix(),
	}
	if ttlSeconds > 0 {
		claims["exp"] = now.Add(time.Duration(ttlSeconds) * time.Second).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func (s *PlatformPassportService) issueLoginTokens(userID uint64) (LoginResult, error) {
	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(userID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(userID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpireAt: accessTTL}, nil
}

func (s *PlatformPassportService) passwordChangeChallengeIfExpired(passwordHistory *PasswordHistoryService, userID uint64) (LoginResult, bool, error) {
	if err := passwordHistory.CheckMaxAge(userID); err != nil {
		if errors.Is(err, ErrBusinessRule) {
			token, challengeErr := IssuePasswordChangeChallenge("platform", userID)
			if challengeErr != nil {
				return LoginResult{}, false, challengeErr
			}
			return LoginResult{PasswordChangeRequired: true, PasswordChangeToken: token}, true, nil
		}
		return LoginResult{}, false, err
	}
	return LoginResult{}, false, nil
}
