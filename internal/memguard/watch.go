// Package memguard bounds a process's memory: it reads the process's peak
// resident size and reports when it passes a limit, so a runaway stops small
// and loud instead of taking the machine (0.10.13's re-embed passed 100 GB).
package memguard

import (
	"sync"
	"time"
)

// Watch samples rss every interval and calls over once, with the size read,
// the first time it is above limit. The returned stop ends sampling and waits
// for the sampler to exit; call it when the watched work is done. Once stop
// is called, over is not: work that finished is not reported. over must exit
// the process or return promptly, or stop waits for it. A limit of 0 or less,
// or a nil rss, turns the watch off.
func Watch(limit int64, every time.Duration, rss func() int64, over func(used int64)) (stop func()) {
	if limit <= 0 || rss == nil {
		return func() {}
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if used := rss(); used > limit {
					select {
					case <-done: // the work finished while memory was read
					default:
						over(used)
					}
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}
