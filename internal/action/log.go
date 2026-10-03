package action

import (
	"sync/atomic"

	"go.uber.org/zap"
)

//nolint:gochecknoglobals // actions run in one process, with one logger
var (
	actionLogger atomic.Pointer[zap.SugaredLogger]
	nopLogger    = zap.NewNop().Sugar()
)

// SetLogger logs what an action works around without failing: a step of an
// animation that did not land, or a window or display it could not read.
// Until it is called, as in a CLI action run without the daemon, nothing is
// logged, since the CLI prints what it returns.
func SetLogger(logger *zap.SugaredLogger) {
	actionLogger.Store(logger)
}

func logger() *zap.SugaredLogger {
	if logger := actionLogger.Load(); logger != nil {
		return logger
	}

	return nopLogger
}
