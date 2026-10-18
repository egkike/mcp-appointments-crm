package entity

import "time"

// DayKey is the day encoding used by BusinessProfile.BusinessHours and its
// lookups (GetOpenClose): 1=Monday, 2=Tuesday, ..., 6=Saturday, 7=Sunday.
//
// It is a nominal type on purpose. The project uses two different weekday
// encodings, and mixing them silently reads the wrong day's schedule:
//
//   - DayKey (this type): the business_hours JSON key, "1".."7", Monday=1.
//   - ScheduleWeekday: time.Weekday and schedules.day_of_week, 0..6, Sunday=0.
//
// The only supported way to build a DayKey from a time.Weekday is
// ProfileDayKey; production call sites must never pass a raw weekday integer.
type DayKey int

// ScheduleWeekday is the day encoding used by Go's time.Weekday and by
// schedules.day_of_week: 0=Sunday, 1=Monday, ..., 6=Saturday.
//
// It is a nominal type for the same reason as DayKey (see there): it keeps the
// two weekday conventions from being passed to the wrong lookup. The only
// supported way to build a ScheduleWeekday from a time.Weekday is ScheduleDay;
// production call sites must never use a raw int(time.Weekday()) again.
type ScheduleWeekday int

// ProfileDayKey translates a time.Weekday into the business_hours day key
// (1=Monday .. 7=Sunday). It is one of the two translation points between the
// project's day encodings, and the single source of truth for the asymmetric
// Sunday=7 mapping.
func ProfileDayKey(w time.Weekday) DayKey {
	if w == time.Sunday {
		return 7
	}
	return DayKey(w)
}

// ScheduleDay translates a time.Weekday into the schedules.day_of_week encoding
// (0=Sunday .. 6=Saturday). It is the only translation point for that encoding;
// service call sites use it instead of a raw int(w.Weekday()) so the two
// conventions cannot be mixed up.
func ScheduleDay(w time.Weekday) ScheduleWeekday {
	return ScheduleWeekday(w)
}
