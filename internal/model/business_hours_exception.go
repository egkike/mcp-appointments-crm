package model

// BusinessHoursException represents a date-specific override of the regular
// weekly business hours (holidays, special events, vacations).
type BusinessHoursException struct {
	ID            int     `json:"id"`
	ExceptionDate string  `json:"exception_date"`
	IsClosed      bool    `json:"is_closed"`
	OpenTime      *string `json:"open_time"`
	CloseTime     *string `json:"close_time"`
	Reason        *string `json:"reason"`
	CreatedAt     string  `json:"created_at"`
}
