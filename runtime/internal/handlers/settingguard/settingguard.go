package settingguard

import (
	"sync/atomic"
)

type Guard struct {
	writesLocked atomic.Bool
}

func (g *Guard) LockWrites() {
	g.writesLocked.Store(true)
}

func (g *Guard) UnlockWrites() {
	g.writesLocked.Store(false)
}

func (g *Guard) WritesLocked() bool {
	return g.writesLocked.Load()
}
