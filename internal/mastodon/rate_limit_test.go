package mastodon

import (
	"testing"
	"time"
)

func TestDeletionLimiterSleepsBeforeThirtyFirstDelete(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration

	limiter := newDeletionLimiter(
		30,
		30*time.Minute,
		func() time.Time { return now },
		func(d time.Duration) {
			slept = append(slept, d)
			now = now.Add(d)
		},
	)

	for i := 0; i < 30; i++ {
		limiter.Wait()
		now = now.Add(10 * time.Second)
	}

	limiter.Wait()

	if len(slept) != 1 {
		t.Fatalf("len(slept) = %d, want 1", len(slept))
	}
	if slept[0] != 25*time.Minute {
		t.Fatalf("slept[0] = %s, want %s", slept[0], 25*time.Minute)
	}
}
