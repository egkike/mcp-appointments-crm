package model

// Client represents a customer of the business.
// Phone is unique because it serves as the chat ID for WhatsApp/Telegram.
type Client struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Phone       string  `json:"phone"`
	Email       *string `json:"email"`
	Preferences *string `json:"preferences"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
