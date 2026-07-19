package services

import (
	"errors"

	"goravel/app/models"
)

func (s *PassportService) appendPasswordUpdate(userID uint64, input ProfileUpdate, values map[string]any) error {
	if input.NewPassword == "" {
		return nil
	}
	if input.NewPassword != input.NewPasswordConfirmation {
		return ErrInvalidCredentials
	}
	if err := ValidatePasswordPolicy(input.NewPassword); err != nil {
		return err
	}
	if err := NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).ValidateReuse(userID, input.NewPassword); err != nil {
		return err
	}

	var user models.User
	if err := s.orm().Query().Where("id", userID).First(&user); err != nil {
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

func ProfileValidationMessage(err error) string {
	if errors.Is(err, ErrBusinessRule) {
		return err.Error()
	}
	return "旧密码错误或新密码不一致"
}
