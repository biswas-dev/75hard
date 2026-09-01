package config

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// MustInitLogger builds the process logger: JSON in production, human-readable
// console output everywhere else.
func MustInitLogger(env, level string) *zap.Logger {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if lv, err := zapcore.ParseLevel(level); err == nil {
		cfg.Level = zap.NewAtomicLevelAt(lv)
	}

	logger, err := cfg.Build()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	return logger
}
