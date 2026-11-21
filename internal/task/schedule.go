// Package task owns the built-in, bounded task scheduler and its execution log.
package task

import (
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
)

type schedule struct {
	interval time.Duration
	location *time.Location
	hour     int
	minute   int
}

func newSchedule(spec config.TaskSchedule) (schedule, error) {
	if err := spec.Validate(); err != nil {
		return schedule{}, err
	}
	if spec.Kind == "interval" {
		return schedule{interval: time.Duration(spec.EverySeconds) * time.Second}, nil
	}
	location, err := time.LoadLocation(spec.Timezone)
	if err != nil {
		return schedule{}, err
	}
	at, err := time.Parse("15:04", spec.At)
	if err != nil {
		return schedule{}, err
	}
	return schedule{location: location, hour: at.Hour(), minute: at.Minute()}, nil
}

// next is strictly after the supplied time. Intervals are anchored at the Unix
// epoch, so restarting Core cannot move their phase. Daily schedules pick the
// first matching minute; a missing local time is skipped. The durable local-date
// slot in the dispatcher prevents a second execution during a repeated DST hour.
func (s schedule) next(after time.Time) time.Time {
	if s.interval > 0 {
		seconds := int64(s.interval / time.Second)
		return time.Unix((after.Unix()/seconds+1)*seconds, 0).UTC()
	}
	// Only calculated at startup and when a task becomes due, not every tick.
	for t, end := after.Truncate(time.Minute).Add(time.Minute), after.Add(72*time.Hour); t.Before(end); t = t.Add(time.Minute) {
		local := t.In(s.location)
		if local.Hour() == s.hour && local.Minute() == s.minute {
			return t.UTC()
		}
	}
	// Fail closed if unusual timezone data contains no matching minute.
	return time.Time{}
}

func (s schedule) slot(at time.Time) string {
	if s.interval > 0 {
		return "interval:" + at.UTC().Format(time.RFC3339)
	}
	return "daily:" + at.In(s.location).Format("2006-01-02")
}
