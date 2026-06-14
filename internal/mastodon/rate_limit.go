package mastodon

import "time"

type deletionLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time
	sleep  func(time.Duration)
	recent []time.Time
}

func newDeletionLimiter(limit int, window time.Duration, now func() time.Time, sleep func(time.Duration)) *deletionLimiter {
	return &deletionLimiter{
		limit:  limit,
		window: window,
		now:    now,
		sleep:  sleep,
		recent: make([]time.Time, 0, limit),
	}
}

func (l *deletionLimiter) Wait() {
	current := l.now()
	l.prune(current)

	if len(l.recent) >= l.limit {
		waitUntil := l.recent[0].Add(l.window)
		if delay := waitUntil.Sub(current); delay > 0 {
			l.sleep(delay)
		}
		current = l.now()
		l.prune(current)
	}

	l.recent = append(l.recent, current)
}

func (l *deletionLimiter) prune(current time.Time) {
	cut := 0
	for cut < len(l.recent) && !l.recent[cut].Add(l.window).After(current) {
		cut++
	}
	if cut > 0 {
		l.recent = append([]time.Time(nil), l.recent[cut:]...)
	}
}
