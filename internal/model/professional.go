package model

// Professional represents a staff member who provides services.
type Professional struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	RoleSpecialty *string `json:"role_specialty"`
	Status        string  `json:"status"`
	Email         *string `json:"email"`
	Phone         *string `json:"phone"`
	Specialties   *string `json:"specialties"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}
