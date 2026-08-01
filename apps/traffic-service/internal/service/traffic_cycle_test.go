package service

import (
	"testing"
	"time"
)

func TestCycleResetAnchor(t *testing.T) {
	loc := time.Local
	mk := func(y int, m time.Month, d int, h int, min ...int) time.Time {
		mi := 0
		if len(min) > 0 {
			mi = min[0]
		}
		return time.Date(y, m, d, h, mi, 0, 0, loc)
	}

	cases := []struct {
		name      string
		startedAt time.Time
		now       time.Time
		want      time.Time
	}{
		{
			name:      "purchase day is first anchor",
			startedAt: mk(2026, 8, 15, 10),
			now:       mk(2026, 8, 20, 8),
			want:      mk(2026, 8, 15, 10),
		},
		{
			name:      "before next 30-day anchor uses current cycle start",
			startedAt: mk(2026, 8, 15, 10),
			now:       mk(2026, 9, 10, 8),
			want:      mk(2026, 8, 15, 10),
		},
		{
			name:      "30 days later is the next anchor",
			startedAt: mk(2026, 8, 15, 10),
			now:       mk(2026, 9, 14, 11),
			want:      mk(2026, 9, 14, 10),
		},
		{
			name:      "multi-cycle anchor after two 30-day periods",
			startedAt: mk(2026, 1, 31, 8),
			now:       mk(2026, 4, 30, 12),
			want:      mk(2026, 4, 1, 8),
		},
		{
			name:      "before first anchor returns zero",
			startedAt: mk(2026, 8, 20, 8),
			now:       mk(2026, 8, 10, 12),
			want:      time.Time{},
		},
		{
			name:      "exactly at next anchor boundary",
			startedAt: mk(2026, 8, 15, 10),
			now:       mk(2026, 9, 14, 10),
			want:      mk(2026, 9, 14, 10),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cycleResetAnchor(c.startedAt, c.now)
			if !got.Equal(c.want) {
				t.Errorf("cycleResetAnchor(%s, %s) = %s, want %s",
					c.startedAt.Format("2006-01-02 15:04"),
					c.now.Format("2006-01-02 15:04"),
					got.Format("2006-01-02 15:04"),
					c.want.Format("2006-01-02 15:04"))
			}
		})
	}
}
