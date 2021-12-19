package wrapper

import "log"

var logger Logger = &defaultLogger{}

func SetLogger(l Logger) {
	logger = l
}

type Logger interface {
	Info(args ...interface{})
	Warning(args ...interface{})
	Error(args ...interface{})
}

type defaultLogger struct{}

func (l *defaultLogger) Info(args ...interface{}) {
	log.Print(args...)
}

func (l *defaultLogger) Warning(args ...interface{}) {
	log.Print(args...)
}

func (l *defaultLogger) Error(args ...interface{}) {
	log.Print(args...)
}
