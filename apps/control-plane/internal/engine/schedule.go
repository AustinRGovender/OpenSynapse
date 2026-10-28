package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Stage is one step of a VU schedule: ramp linearly to Target over Duration.
type Stage struct {
	Duration time.Duration
	Target   int
}

// Schedule is the VU profile the engine drives through k6's REST API when a
// scenario runs under the externally-controlled executor. It reproduces the
// semantics of k6's ramping-vus executor (linear ramps between stages).
type Schedule struct {
	StartVUs int
	Stages   []Stage
}

// ConstantSchedule holds vus for the whole duration.
func ConstantSchedule(vus int, d time.Duration) Schedule {
	return Schedule{StartVUs: vus, Stages: []Stage{{Duration: d, Target: vus}}}
}

// Duration is the total length of the schedule.
func (s Schedule) Duration() time.Duration {
	var d time.Duration
	for _, st := range s.Stages {
		d += st.Duration
	}
	return d
}

// PeakVUs is the highest VU count the schedule reaches.
func (s Schedule) PeakVUs() int {
	peak := s.StartVUs
	for _, st := range s.Stages {
		if st.Target > peak {
			peak = st.Target
		}
	}
	return peak
}

// TargetAt returns the VU count the schedule calls for at elapsed time t.
func (s Schedule) TargetAt(t time.Duration) int {
	from := s.StartVUs
	var start time.Duration
	for _, st := range s.Stages {
		end := start + st.Duration
		if t < end {
			if st.Duration <= 0 {
				return st.Target
			}
			frac := float64(t-start) / float64(st.Duration)
			return int(math.Round(float64(from) + frac*float64(st.Target-from)))
		}
		from = st.Target
		start = end
	}
	return from
}

// ParseDuration parses k6 duration strings ("30s", "2m", "1h30m", "1m30s",
// bare seconds "45"). k6 accepts the same forms as Go's time.ParseDuration
// plus a unitless number of seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// MaxDurationFor returns the hard upper bound the k6 process is started with
// for a planned duration: the plan plus a 50% buffer (at least five minutes),
// capped at an extra 24 hours. Live "extend duration" requests cannot go past it.
func MaxDurationFor(planned time.Duration) time.Duration {
	buffer := planned / 2
	if buffer < 5*time.Minute {
		buffer = 5 * time.Minute
	}
	if buffer > 24*time.Hour {
		buffer = 24 * time.Hour
	}
	return planned + buffer
}
