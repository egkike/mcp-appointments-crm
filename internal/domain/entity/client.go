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
// Accepts an optional leading '+' followed by 4–15 digits (E.164 subset): the
// 15-digit maximum IS enforced here — the doc and the implementation agree.
func (c *Client) HasValidPhone() bool {
	phone := c.Phone
	start := 0
	if len(phone) > 0 && phone[0] == '+' {
		start = 1
	}
	digitCount := len(phone) - start
	if digitCount < 4 || digitCount > 15 {
		return false
	}
	for i := start; i < len(phone); i++ {
		if phone[i] < '0' || phone[i] > '9' {
			return false
		}
	}
	return true
}
