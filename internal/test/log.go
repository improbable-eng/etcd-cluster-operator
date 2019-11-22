// Copied from "github.com/go-logr/logr/testing"
// With added implementation of log.Info, log.V, log.WithName and log.WithValues

package test

import (
	"testing"

	"github.com/go-logr/logr"
)

// TestLogger is a logr.Logger that prints through a testing.T object.
type TestLogger struct {
	T      *testing.T
	v      int
	name   string
	values []interface{}
}

var _ logr.Logger = TestLogger{}

func (log TestLogger) Info(msg string, args ...interface{}) {
	log.T.Logf("log.V(%d).WithName(%q).Info: %s -- %v", log.v, log.name, msg, append(log.values, args...))
}

func (_ TestLogger) Enabled() bool {
	return false
}

func (log TestLogger) Error(err error, msg string, args ...interface{}) {
	log.T.Logf("log.V(%d).WithName(%q).Error: %s: %v -- %v", log.v, log.name, msg, err, append(log.values, args...))
}

func (log TestLogger) V(v int) logr.InfoLogger {
	log.v = v
	return log
}

func (log TestLogger) WithName(name string) logr.Logger {
	log.name = name
	return log
}

func (log TestLogger) WithValues(values ...interface{}) logr.Logger {
	log.values = values
	return log
}
