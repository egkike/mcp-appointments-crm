package entity

// Client represents a customer of the business.
// Phone serves as the chat ID for WhatsApp/Telegram and must be unique.
type Client struct {
	ID          string
	Name        string
	Phone       string
	Email       *string
	Preferences *string
	Active      bool
	CreatedAt   string
	UpdatedAt   string
}

// IsActive reports whether the client has an active account.
func (c *Client) IsActive() bool {
	return c.Active
}

// HasValidPhone reports whether the phone number is in a valid format.
// Accepts an optional leading '+' followed by 4–15 digits (E.164 subset).
func (c *Client) HasValidPhone() bool {
	phone := c.Phone
	if len(phone) < 4 {
		return false
	}
	start := 0
	if phone[0] == '+' {
		if len(phone) < 5 {
			return false
		}
		start = 1
	}
	for i := start; i < len(phone); i++ {
		if phone[i] < '0' || phone[i] > '9' {
			return false
		}
	}
	return true
}
