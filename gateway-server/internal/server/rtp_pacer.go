package server

import (
	"context"
	"time"
)

type rtpSendPacer struct {
	next       time.Time
	bitrateBps int64
	minSpacing time.Duration
}

func newRTPSendPacer(profile rtpSendProfile) *rtpSendPacer {
	bitrate := profile.bitrateBps
	if bitrate <= 0 {
		bitrate = rtpTargetBitrateBps
	}
	spacing := profile.minSpacing
	if spacing <= 0 {
		spacing = standardRTPMinSpacing
	}
	return &rtpSendPacer{bitrateBps: bitrate, minSpacing: spacing}
}

func (p *rtpSendPacer) Wait(ctx context.Context) error {
	if p == nil || p.next.IsZero() {
		return ctx.Err()
	}
	now := time.Now()
	if !now.Before(p.next) {
		return ctx.Err()
	}
	timer := time.NewTimer(p.next.Sub(now))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *rtpSendPacer) Advance(packetBytes int) {
	if p == nil || packetBytes <= 0 {
		return
	}
	bits := int64(packetBytes * 8)
	delay := time.Duration(bits * int64(time.Second) / p.bitrateBps)
	if delay < p.minSpacing {
		delay = p.minSpacing
	}
	now := time.Now()
	if p.next.IsZero() || p.next.Before(now) {
		p.next = now
	}
	p.next = p.next.Add(delay)
}
