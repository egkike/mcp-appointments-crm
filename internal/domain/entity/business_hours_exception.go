package entity

// BusinessHoursException represents a date-specific override of the regular
// weekly business hours (holidays, special events, vacations).
// Date, OpenTime, and CloseTime are kept as strings (date-only and time-of-day).
type BusinessHoursException struct {
	ID            int
	ExceptionDate string
	IsClosed      bool
	OpenTime      *string
	CloseTime     *string
	Reason        *string
	CreatedAt     string
}

// IsClosedDay reports whether the business is fully closed on this exception date.
func (e *BusinessHoursException) IsClosedDay() bool {
	return e.IsClosed
}

// EffectiveHours returns the open and close times for this exception date.
// Returns ok=false if the business is closed or if either time is nil.
func (e *BusinessHoursException) EffectiveHours() (open, close string, ok bool) {
	if e.IsClosed || e.OpenTime == nil || e.CloseTime == nil {
		return "", "", false
	}
	return *e.OpenTime, *e.CloseTime, true
}
