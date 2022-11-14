package rollbar

import (
	"fmt"

	"github.com/go-kit/kit/log"
)

// logger is an adaptor for go-kit logger, we need it to plug it in Rollbar client.
type logger struct {
	l log.Logger
}

func newLogger(l log.Logger) *logger {
	return &logger{
		l: l,
	}
}

func (rl *logger) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	rl.l.Log("level", "error", "msg", msg)
}
