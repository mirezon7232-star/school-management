package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"school-management/auth"
	"school-management/handlers"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var pageTemplates *template.Template

func main() {
	if err := auth.Init(); err != nil {
		log.Fatal("Could not set up accounts: ", err)
	}

	var err error

	pageTemplates, err = template.ParseFiles("templates/index.html")
	if err != nil {
		log.Fatal("Could not load templates/index.html: ", err)
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/logout", logoutHandler)
	http.HandleFunc("/api/me", meHandler)

	http.HandleFunc("/api/students", auth.RequireLogin(studentsHandler))
	http.HandleFunc("/api/student", auth.RequireLogin(studentHandler))

	http.HandleFunc("/api/teachers", auth.RequireLogin(teachersHandler))
	http.HandleFunc("/api/teacher", auth.RequireAdmin(teacherHandler))
	http.HandleFunc("/api/reset-password", auth.RequireAdmin(resetPasswordHandler))

	log.Println("School Management System running at http://localhost:8080")

	server := &http.Server{
		Addr:              ":8080",
		Handler:           securityHeaders(http.DefaultServeMux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pageTemplates.Execute(w, nil); err != nil {
		log.Println("Template error:", err)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input LoginRequest

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, user, err := auth.Login(input.Username, input.Password, clientIP(r))

	switch {
	case err == nil:

	case errors.Is(err, auth.ErrLocked):
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	case errors.Is(err, auth.ErrInvalidLogin):
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	default:
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // Local development only; use true with HTTPS.
		MaxAge:   int(auth.SessionTTL / time.Second),
	})

	writeJSON(w, http.StatusOK, user)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie(auth.CookieName); err == nil {
		auth.Logout(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusNoContent)
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func studentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetStudents(w, r)

	case http.MethodPost:
		auth.RequireAdmin(handlers.CreateStudent)(w, r)

	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func studentHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetStudentByID(w, r)

	case http.MethodPut:
		auth.RequireAdmin(handlers.UpdateStudent)(w, r)

	case http.MethodDelete:
		auth.RequireAdmin(handlers.DeleteStudent)(w, r)

	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func teachersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetTeachers(w, r)

	case http.MethodPost:
		auth.RequireAdmin(handlers.CreateTeacher)(w, r)

	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func teacherHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	handlers.DeleteTeacher(w, r)
}

func resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	handlers.ResetPassword(w, r)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Println("JSON encoding error:", err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}

		next.ServeHTTP(w, r)
	})
}
