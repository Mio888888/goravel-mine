package auth

import (
	"strings"
	"time"

	"github.com/mojocn/base64Captcha"

	"goravel/app/facades"
)

const captchaTTL = 2 * time.Minute

type CaptchaService struct{}

type CaptchaResult struct {
	Key    string `json:"key"`
	Base64 string `json:"base64"`
}

func NewCaptchaService() *CaptchaService {
	return &CaptchaService{}
}

func (s *CaptchaService) Generate() (CaptchaResult, error) {
	driver := base64Captcha.NewDriverMath(44, 136, 2, base64Captcha.OptionShowSlimeLine, nil, nil, nil)
	captcha := base64Captcha.NewCaptcha(driver, base64Captcha.NewMemoryStore(1, captchaTTL))
	id, base64Image, answer, err := captcha.Generate()
	if err != nil {
		return CaptchaResult{}, err
	}
	_ = facades.Cache().Put(captchaCacheKey(id), answer, captchaTTL)
	return CaptchaResult{Key: id, Base64: base64Image}, nil
}

func (s *CaptchaService) Verify(key, answer string) bool {
	key = strings.TrimSpace(key)
	answer = strings.TrimSpace(answer)
	if key == "" {
		return false
	}
	expected := facades.Cache().GetString(captchaCacheKey(key))
	facades.Cache().Forget(captchaCacheKey(key))
	if expected == "" || answer == "" {
		return false
	}
	return strings.EqualFold(expected, answer)
}

func captchaCacheKey(key string) string {
	return "captcha:" + key
}
