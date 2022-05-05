package id

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/yomo/pkg/logger"
)

// New generate id
func New() string {
	id, err := gonanoid.New()
	if err != nil {
		logger.Errorf("generated id err=%v", err)
		return ""
	}
	return id
}
