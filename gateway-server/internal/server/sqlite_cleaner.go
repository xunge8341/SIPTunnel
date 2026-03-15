package server

import (
	"context"
	"time"

	"siptunnel/internal/persistence"
)

type sqliteCleaner struct {
	store       *persistence.SQLiteStore
	interval    time.Duration
	stop        chan struct{}
	done        chan struct{}
	LastRunAt   string
	LastResult  string
	LastRemoved int
}

func newSQLiteCleaner(store *persistence.SQLiteStore, interval time.Duration) *sqliteCleaner {
	return &sqliteCleaner{store: store, interval: interval, stop: make(chan struct{}), done: make(chan struct{})}
}

func (c *sqliteCleaner) Start() {
	go func() {
		defer close(c.done)
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.store.Cleanup(context.Background()); err != nil {
					c.LastResult = err.Error()
				} else {
					c.LastResult = "执行成功"
				}
				c.LastRunAt = time.Now().UTC().Format(time.RFC3339)
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *sqliteCleaner) Close() error {
	close(c.stop)
	<-c.done
	return nil
}
