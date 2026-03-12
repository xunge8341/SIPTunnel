package service

import "time"

type RetryPolicy struct {
	MaxAttempts int
	BaseBackoff time.Duration
}

func (p RetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	return p.BaseBackoff * time.Duration(1<<(attempt-2))
}
