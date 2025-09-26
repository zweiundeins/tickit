package logger

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// Setup configures and returns a zerolog logger
// In development mode, it provides colored console output
// In production mode, it provides structured JSON output
func Setup(isDevelopment bool) zerolog.Logger {
	// Set global log level to Info - keeps important messages while reducing noise
	// Use Debug level only when specifically needed for troubleshooting
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Uncomment the following lines only when debugging is needed:
	// if isDevelopment {
	// 	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	// }

	var output io.Writer = os.Stdout

	if isDevelopment {
		// Pretty console output for development
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	// Create logger with timestamp
	logger := zerolog.New(output).With().
		Timestamp().
		Caller().
		Logger()

	// Set as global logger
	zlog.Logger = logger

	return logger
}

// GetLogger returns the global logger instance
func GetLogger() zerolog.Logger {
	return zlog.Logger
}

// NewStdLibLogger creates a stdlib log.Logger that writes to zerolog
func NewStdLibLogger() *log.Logger {
	return log.New(&stdlibAdapter{logger: zlog.Logger}, "", 0)
}

// stdlibAdapter adapts zerolog to stdlib log interface
type stdlibAdapter struct {
	logger zerolog.Logger
}

func (s *stdlibAdapter) Write(p []byte) (n int, err error) {
	// Remove trailing newline if present
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	s.logger.Error().Msg(msg)
	return len(p), nil
}

// Helper functions for common logging patterns

// Info logs an info level message with fields
func Info() *zerolog.Event {
	return zlog.Info()
}

// Error logs an error level message with fields
func Error() *zerolog.Event {
	return zlog.Error()
}

// Debug logs a debug level message with fields
func Debug() *zerolog.Event {
	return zlog.Debug()
}

// Warn logs a warn level message with fields
func Warn() *zerolog.Event {
	return zlog.Warn()
}

// With creates a child logger with additional fields
func With() zerolog.Context {
	return zlog.With()
}
