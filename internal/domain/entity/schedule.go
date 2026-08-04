package entity

// Schedule represents a professional's working hours for a specific day of the week.
// StartTime and EndTime are local daily times in "HH:MM" format (not datetimes).
type Schedule struct {
	ID             int
	ProfessionalID string
	DayOfWeek      int
	StartTime      string
	EndTime        string
}

// IncludesTime reports whether the given HH:MM time falls within the schedule's
// [StartTime, EndTime) range. The start time is inclusive; the end time is exclusive.
// Compares strings lexicographically; callers must pass a valid "HH:MM" string.
func (s *Schedule) IncludesTime(hhmm string) bool {
	return hhmm >= s.StartTime && hhmm < s.EndTime
}
