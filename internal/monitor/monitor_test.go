package monitor

import (
	"testing"
	"time"
)

func TestAlignToInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		interval time.Duration
		expected time.Time
	}{
		{
			name:     "5 minutes interval - aligned input",
			input:    time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
			interval: 5 * time.Minute,
			expected: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "5 minutes interval - unaligned minutes",
			input:    time.Date(2026, 6, 15, 12, 3, 0, 0, time.UTC),
			interval: 5 * time.Minute,
			expected: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "15 minutes interval - unaligned minutes and seconds",
			input:    time.Date(2026, 6, 15, 12, 37, 45, 123456789, time.UTC),
			interval: 15 * time.Minute,
			expected: time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC),
		},
		{
			name:     "1 hour interval - unaligned minutes",
			input:    time.Date(2026, 6, 15, 12, 45, 0, 0, time.UTC),
			interval: 1 * time.Hour,
			expected: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "6 hours interval - unaligned hours and minutes",
			input:    time.Date(2026, 6, 15, 15, 30, 0, 0, time.UTC),
			interval: 6 * time.Hour,
			expected: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "1 day interval - unaligned hours and minutes",
			input:    time.Date(2026, 6, 15, 17, 45, 30, 0, time.UTC),
			interval: 24 * time.Hour,
			expected: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "3 days interval - unaligned days",
			input: time.Date(
				2026,
				6,
				15,
				12,
				0,
				0,
				0,
				time.UTC,
			), // 2026-06-15 is 20619 days since epoch
			interval: 3 * 24 * time.Hour, // 20619 is a multiple of 3 (20619 / 3 = 6873)
			expected: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "3 days interval - unaligned days offset",
			input: time.Date(
				2026,
				6,
				16,
				12,
				0,
				0,
				0,
				time.UTC,
			), // 2026-06-16 is 20620 days since epoch
			interval: 3 * 24 * time.Hour,
			expected: time.Date(
				2026,
				6,
				15,
				0,
				0,
				0,
				0,
				time.UTC,
			), // Expect to align to the 20619th day
		},
		{
			name:     "Zero interval - no alignment",
			input:    time.Date(2026, 6, 15, 12, 34, 56, 0, time.UTC),
			interval: 0,
			expected: time.Date(2026, 6, 15, 12, 34, 56, 0, time.UTC),
		},
		{
			name:     "Negative interval - no alignment",
			input:    time.Date(2026, 6, 15, 12, 34, 56, 0, time.UTC),
			interval: -10 * time.Minute,
			expected: time.Date(2026, 6, 15, 12, 34, 56, 0, time.UTC),
		},
		{
			name:     "Local timezone alignment",
			input:    time.Date(2026, 6, 15, 12, 34, 56, 0, time.FixedZone("UTC+7", 7*60*60)),
			interval: 30 * time.Minute,
			expected: time.Date(2026, 6, 15, 12, 30, 0, 0, time.FixedZone("UTC+7", 7*60*60)),
		},
		{
			name:     "Arbitrary interval PT1H30M (90 mins)",
			input:    time.Date(2026, 6, 15, 12, 45, 0, 0, time.UTC),
			interval: 1*time.Hour + 30*time.Minute,
			expected: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Arbitrary interval P3DT2H10M (74h 10m)",
			input:    time.Date(1970, 1, 7, 4, 25, 0, 0, time.UTC),
			interval: 3*24*time.Hour + 2*time.Hour + 10*time.Minute,
			expected: time.Date(1970, 1, 7, 4, 20, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := alignToInterval(tt.input, tt.interval)
			if !result.Equal(tt.expected) {
				t.Errorf(
					"alignToInterval(%v, %v) = %v; want %v",
					tt.input,
					tt.interval,
					result,
					tt.expected,
				)
			}
		})
	}
}

func TestMonitorBehaviorAndNextIteration(t *testing.T) {
	tests := []struct {
		name            string
		bootTime        time.Time
		interval        time.Duration
		expectedNextDue time.Time
	}{
		{
			name:            "Boot at 00:05, 15m interval -> next due at 00:15",
			bootTime:        time.Date(2026, 6, 15, 0, 5, 0, 0, time.UTC),
			interval:        15 * time.Minute,
			expectedNextDue: time.Date(2026, 6, 15, 0, 15, 0, 0, time.UTC),
		},
		{
			name:            "Boot at 00:05, 30m override -> next due at 00:30",
			bootTime:        time.Date(2026, 6, 15, 0, 5, 0, 0, time.UTC),
			interval:        30 * time.Minute,
			expectedNextDue: time.Date(2026, 6, 15, 0, 30, 0, 0, time.UTC),
		},
		{
			name:            "Boot at 12:45, PT1H30M (90m) interval -> next due at 13:30",
			bootTime:        time.Date(2026, 6, 15, 12, 45, 0, 0, time.UTC),
			interval:        1*time.Hour + 30*time.Minute,
			expectedNextDue: time.Date(2026, 6, 15, 13, 30, 0, 0, time.UTC),
		},
		{
			name:            "Boot at 1970-01-07 04:25, P3DT2H10M (74h 10m) interval -> next due at 1970-01-10 06:30",
			bootTime:        time.Date(1970, 1, 7, 4, 25, 0, 0, time.UTC),
			interval:        3*24*time.Hour + 2*time.Hour + 10*time.Minute,
			expectedNextDue: time.Date(1970, 1, 10, 6, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate immediate run on boot: updates lastCheck
			lastCheck := alignToInterval(tt.bootTime, tt.interval)

			// 1. At bootTime (e.g. 00:05), check if due.
			// It should NOT be due because time since lastCheck (e.g. 5 mins) < interval (e.g. 15 mins)
			isDueAtBoot := tt.bootTime.Sub(lastCheck) >= tt.interval
			if isDueAtBoot {
				t.Errorf(
					"expected NOT to be due at boot time %v (lastCheck %v, interval %v)",
					tt.bootTime,
					lastCheck,
					tt.interval,
				)
			}

			// 2. Check 1 second before the expected next due time.
			// It should NOT be due.
			oneSecBefore := tt.expectedNextDue.Add(-1 * time.Second)
			isDueBefore := oneSecBefore.Sub(lastCheck) >= tt.interval
			if isDueBefore {
				t.Errorf(
					"expected NOT to be due 1s before next iteration %v (lastCheck %v)",
					oneSecBefore,
					lastCheck,
				)
			}

			// 3. Check exactly at the expected next due time.
			// It should BE due.
			isDueAtExpected := tt.expectedNextDue.Sub(lastCheck) >= tt.interval
			if !isDueAtExpected {
				t.Errorf(
					"expected to BE due at next iteration %v (lastCheck %v)",
					tt.expectedNextDue,
					lastCheck,
				)
			}
		})
	}
}
