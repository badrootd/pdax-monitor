// Package rollbar sets up rollbar
package rollbar

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/go-kit/kit/log"
	"github.com/rollbar/rollbar-go"
)

// SetUp bootstraps Rollbar client.
func SetUp(l log.Logger, token string, env string) {
	rollbar.SetLogger(newLogger(l))
	rollbar.SetEnabled(true)
	rollbar.SetToken(token)
	rollbar.SetEnvironment(env)
}

// TearDown gracefully shuts down Rollbar client so that it will send all the buffered data to the server.
// It is a blocking operation.
//
// TearDown watches for panics in the call stack.
// If panic occurs it is sent to Rollbar and propagated further.
//
// TearDown must be called with `defer` (due to how panics in Go are handled).
func TearDown() {
	if rec := recover(); rec != nil {
		err := fmt.Sprintf("panic: %v \n stack trace: %s", rec, debug.Stack())
		rollbar.Critical(err)
		rollbar.Wait()
		panic(err)
	}

	rollbar.Wait()
}

// Error sends error to Rollbar server.
func Error(ctx context.Context, err error) {
	stacktrace := fmt.Sprintf("error stacktrace: %v", err)
	rollbar.Error(ctx, err, map[string]interface{}{
		"detail": stacktrace,
	})
}
