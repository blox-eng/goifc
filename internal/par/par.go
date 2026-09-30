// Package par runs goroutines whose panics reach the caller.
//
// goifc parses untrusted files, and callers guard it with recover. A panic in a
// goroutine of its own cannot be recovered by anyone and ends the process, so
// the library's parallel stages run their work through a Group, which carries
// the first panic back to the goroutine that waits.
package par

import "sync"

// Group is a sync.WaitGroup whose Wait re-panics with the first panic of any
// goroutine it started. The zero value is ready to use.
type Group struct {
	wg    sync.WaitGroup
	once  sync.Once
	first any
}

// Go runs fn in a new goroutine.
func (g *Group) Go(fn func()) {
	g.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				g.once.Do(func() { g.first = r })
			}
		}()
		fn()
	})
}

// Wait waits for every goroutine started by Go, then panics with the first
// panic among them, if any.
func (g *Group) Wait() {
	g.wg.Wait()
	if g.first != nil {
		panic(g.first)
	}
}
