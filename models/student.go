package models

type Student struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Roll     int    `json:"roll"`
	Class    string `json:"class"`
	Guardian string `json:"guardian"`
	Username string `json:"username"` // login username
}


type StudentView struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Roll     int    `json:"roll"`
	Class    string `json:"class,omitempty"`
	Guardian string `json:"guardian,omitempty"`
	Username string `json:"username,omitempty"`
	Own      bool   `json:"own,omitempty"`
}

type CreatedStudent struct {
	StudentView
	Password string `json:"password"`
}
