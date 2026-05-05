package redis

import (
	"GopherAI/config"
	"fmt"
)

func GenerateCaptcha(email string) string {
	return fmt.Sprintf(config.DefaultRedisKeyConfig.CaptchaPrefix, email)
}
