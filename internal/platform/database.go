package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type DBConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

func Open(
	ctx context.Context,
	cfg DBConfig,
	logger *slog.Logger,
) (*gorm.DB, func() error, error) {
	if ctx == nil {
		return nil, nil, errors.New("database context is nil")
	}
	if cfg.DSN == "" {
		return nil, nil, errors.New("database DSN is required")
	}
	if cfg.MaxOpenConns <= 0 || cfg.MaxIdleConns < 0 ||
		cfg.MaxIdleConns > cfg.MaxOpenConns {
		return nil, nil, errors.New("database connection pool limits are invalid")
	}
	if cfg.ConnMaxLifetime <= 0 || cfg.ConnMaxIdleTime <= 0 || cfg.PingTimeout <= 0 {
		return nil, nil, errors.New("database durations must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger:         newGORMLogger(logger),
		TranslateError: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("opening gorm database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("getting sql database: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("pinging database: %w", err)
	}

	return db, sqlDB.Close, nil
}

type gormSlogLogger struct {
	logger        *slog.Logger
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGORMLogger(logger *slog.Logger) gormlogger.Interface {
	return &gormSlogLogger{
		logger:        logger,
		level:         gormlogger.Warn,
		slowThreshold: 200 * time.Millisecond,
	}
}

func (l *gormSlogLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	copy := *l
	copy.level = level
	return &copy
}

func (l *gormSlogLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= gormlogger.Info {
		l.logger.InfoContext(ctx, formatGORMMessage(msg, data...))
	}
}

func (l *gormSlogLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= gormlogger.Warn {
		l.logger.WarnContext(ctx, formatGORMMessage(msg, data...))
	}
}

func (l *gormSlogLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= gormlogger.Error {
		l.logger.ErrorContext(ctx, formatGORMMessage(msg, data...))
	}
}

func formatGORMMessage(msg string, data ...interface{}) string {
	if len(data) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, data...)
}

func (l *gormSlogLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (string, int64),
	err error,
) {
	if l.level == gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	_, rows := fc()
	attrs := []any{
		slog.Duration("elapsed", elapsed),
		slog.Int64("rows", rows),
	}

	switch {
	case err != nil && l.level >= gormlogger.Error:
		attrs = append(attrs, slog.Any("error", err))
		l.logger.ErrorContext(ctx, "gorm query failed", attrs...)
	case elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		l.logger.WarnContext(ctx, "slow gorm query", attrs...)
	case l.level >= gormlogger.Info:
		l.logger.DebugContext(ctx, "gorm query completed", attrs...)
	}
}
