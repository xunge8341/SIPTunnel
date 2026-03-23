package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/persistence"
)

type sqliteCleaner struct {
	store    *persistence.SQLiteStore
	schedule string
	stop     chan struct{}
	reload   chan struct{}
	done     chan struct{}

	mu          sync.RWMutex
	LastRunAt   string
	LastResult  string
	LastRemoved int
}

func newSQLiteCleaner(store *persistence.SQLiteStore, schedule string) *sqliteCleaner {
	if strings.TrimSpace(schedule) == "" {
		schedule = defaultCleanupSchedule
	}
	return &sqliteCleaner{store: store, schedule: schedule, stop: make(chan struct{}), reload: make(chan struct{}, 1), done: make(chan struct{})}
}

func (c *sqliteCleaner) Start() {
	go func() {
		defer close(c.done)
		for {
			delay, err := nextCleanupDelay(c.currentSchedule(), time.Now().UTC())
			if err != nil {
				c.setLastResult(err.Error())
				delay = 30 * time.Minute
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				removed, runErr := c.store.CleanupWithStats(context.Background())
				c.mu.Lock()
				c.LastRunAt = formatTimestamp(time.Now().UTC())
				c.LastRemoved = removed
				if runErr != nil {
					c.LastResult = runErr.Error()
				} else {
					c.LastResult = "执行成功"
				}
				c.mu.Unlock()
			case <-c.reload:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			case <-c.stop:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
	}()
}

func (c *sqliteCleaner) UpdateSchedule(schedule string) error {
	if err := validateCleanupSchedule(schedule); err != nil {
		return err
	}
	c.mu.Lock()
	c.schedule = strings.TrimSpace(schedule)
	c.mu.Unlock()
	select {
	case c.reload <- struct{}{}:
	default:
	}
	return nil
}

func (c *sqliteCleaner) Snapshot() (string, string, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastRunAt, c.LastResult, c.LastRemoved
}

func (c *sqliteCleaner) currentSchedule() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.schedule
}

func (c *sqliteCleaner) setLastResult(result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastResult = result
}

func (c *sqliteCleaner) Close() error {
	close(c.stop)
	<-c.done
	return nil
}
