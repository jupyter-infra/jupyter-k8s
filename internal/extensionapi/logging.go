package extensionapi

import (
	"context"

	"github.com/go-logr/logr"
)

type loggerKey struct{}

// AddLoggerToContext returns a new Context with the given logger
func AddLoggerToContext(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// GetLoggerFromContext returns the Logger stored in context
// Returns a no-op logger if none is found
func GetLoggerFromContext(ctx context.Context) logr.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(logr.Logger); ok {
		return logger
	}
	return logr.Discard() // Return no-op logger as fallback
}
