package mdlib

import (
	"log"
	"os"
)

// Logger is a simple interface to abstract the logger implementation.  Go core `log` is used by default.
type Logger interface {
	Printf(format string, v ...any)
	Noticef(format string, v ...any)
	Errorf(format string, v ...any)
}

// NewDefaultLogger returns the default logger
func NewDefaultLogger() Logger {
	return defaultLogger{log.New(os.Stderr, "", log.LstdFlags)}
}

type defaultLogger struct {
	*log.Logger
}

var _ Logger = (*defaultLogger)(nil)

func (l defaultLogger) Noticef(format string, v ...any) {
	l.Printf("NOTICE: "+format, v...)
}

func (l defaultLogger) Errorf(format string, v ...any) {
	l.Printf("ERROR: "+format, v...)
}
