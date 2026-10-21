package handlers

import (
	"errors"
	"net/http"
	"strings"

	"school-management/auth"
)

type teacherView struct {
	Name     string `json:"name"`
	Subject  string `json:"subject,omitempty"`
	Username string `json:"username,omitempty"` 
}

func GetTeachers(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Please log in", http.StatusUnauthorized)
		return
	}

	list := auth.Teachers()
	out := make([]teacherView, 0, len(list))

	for _, t := range list {
		v := teacherView{Name: t.Name, Subject: t.Subject}
		if sess.Role == auth.RoleAdmin {
			v.Username = t.Username
		}
		out = append(out, v)
	}

	writeJSON(w, http.StatusOK, out)
}
func CreateTeacher(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Subject  string `json:"subject"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)

	if in.Name == "" {
		http.Error(w, "Teacher name is required", http.StatusBadRequest)
		return
	}
	if len(in.Name) > 100 || len(in.Subject) > 100 {
		http.Error(w, "One of the fields is too long", http.StatusBadRequest)
		return
	}

	err := auth.AddTeacher(in.Name, in.Subject, in.Username, in.Password)

	var inputErr auth.InputError
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, teacherView{
			Name:     in.Name,
			Subject:  in.Subject,
			Username: strings.ToLower(strings.TrimSpace(in.Username)),
		})
	case errors.Is(err, auth.ErrExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.As(err, &inputErr):
		http.Error(w, inputErr.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "Could not add the teacher", http.StatusInternalServerError)
	}
}

func DeleteTeacher(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.CurrentSession(r)
	username := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("username")))

	if username == sess.Username {
		http.Error(w, "You cannot remove your own account", http.StatusBadRequest)
		return
	}

	switch err := auth.RemoveTeacher(username); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, auth.ErrProtected):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, auth.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "Could not remove the teacher", http.StatusInternalServerError)
	}
}

func ResetPassword(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("username")))

	password, err := auth.ResetPassword(username)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"username": username, "password": password})
	case errors.Is(err, auth.ErrProtected):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, auth.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "Could not reset the password", http.StatusInternalServerError)
	}
}
