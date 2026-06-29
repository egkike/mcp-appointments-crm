package model

// Schedule represents a professional's working hours for a specific day of the week.
// StartTime and EndTime are local daily times in "HH:MM" format (not datetimes).
type Schedule struct {
	ID             int    `json:"id"`
	ProfessionalID string `json:"professional_id"`
	DayOfWeek      int    `json:"day_of_week"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
}
