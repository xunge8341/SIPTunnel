package server

import (
	"context"
	"time"

	"siptunnel/internal/persistence"
)

type sqliteCleaner struct {
	store    *persistence.SQLiteStore
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
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
				_ = c.store.Cleanup(context.Background())
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
