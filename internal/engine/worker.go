package engine

import (
	"time"
)

type TTLWorker struct {
	engine *Engine
	stop   chan struct{}
	done   chan struct{}
}

func NewTTLWorker(engine *Engine) *TTLWorker {
	return &TTLWorker{
		engine: engine,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (w *TTLWorker) Start() {
	go func() {
		defer close(w.done)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				current := now.UnixNano()
				currentSecond := current / int64(time.Second)

				w.engine.expire(currentSecond, current)

			case <-w.stop:
				return
			}
		}
	}()
}

func (w *TTLWorker) Stop() {
	close(w.stop)
	<-w.done
}
