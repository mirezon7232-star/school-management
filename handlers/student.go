package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"school-management/auth"
	"school-management/models"
)


var (
	students   = make([]models.Student, 0)
	nextID     = 1
	studentsMu sync.RWMutex 
)


func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}


func readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func getIDFromURL(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func validateStudent(s *models.Student) string {
	s.Name = strings.TrimSpace(s.Name)
	s.Class = strings.TrimSpace(s.Class)
	s.Guardian = strings.TrimSpace(s.Guardian)

	if s.Name == "" {
		return "Name is required"
	}
	if s.Roll <= 0 {
		return "Roll must be a positive number"
	}
	if s.Class == "" {
		return "Class is required"
	}
	if len(s.Name) > 100 || len(s.Class) > 50 || len(s.Guardian) > 100 {
		return "One of the fields is too long"
	}
	return ""
}

func rollExists(roll int, class string, excludeID int) bool {
	for _, s := range students {
		if s.ID != excludeID && s.Roll == roll && s.Class == class {
			return true
		}
	}
	return false
}


func viewFor(s models.Student, sess auth.Session) models.StudentView {
	full := models.StudentView{
		ID: s.ID, Name: s.Name, Roll: s.Roll,
		Class: s.Class, Guardian: s.Guardian, Username: s.Username,
	}

	if sess.Role == auth.RoleAdmin {
		return full
	}
	if sess.StudentID == s.ID {
		full.Own = true
		return full
	}
	return models.StudentView{ID: s.ID, Name: s.Name, Roll: s.Roll}
}

func GetStudents(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Please log in", http.StatusUnauthorized)
		return
	}

	studentsMu.RLock()
	defer studentsMu.RUnlock()

	out := make([]models.StudentView, 0, len(students))
	for _, s := range students {
		out = append(out, viewFor(s, sess))
	}
	writeJSON(w, http.StatusOK, out)
}


func CreateStudent(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Please log in", http.StatusUnauthorized)
		return
	}

	var student models.Student
	if !readJSON(w, r, &student) {
		return
	}

	if msg := validateStudent(&student); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	studentsMu.Lock()
	defer studentsMu.Unlock()

	if rollExists(student.Roll, student.Class, 0) {
		http.Error(w, "This roll already exists in this class", http.StatusConflict)
		return
	}

	username, password, err := auth.AddStudentAccount(nextID, student.Name)
	if err != nil {
		http.Error(w, "Could not create the student login", http.StatusInternalServerError)
		return
	}

	student.ID = nextID
	student.Username = username
	nextID++
	students = append(students, student)

	// Password শুধু এই একবারই দেখানো হয়
	writeJSON(w, http.StatusCreated, models.CreatedStudent{
		StudentView: viewFor(student, sess),
		Password:    password,
	})
}

func GetStudentByID(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Please log in", http.StatusUnauthorized)
		return
	}

	id, ok := getIDFromURL(r)
	if !ok {
		http.Error(w, "Valid student ID is required", http.StatusBadRequest)
		return
	}

	studentsMu.RLock()
	defer studentsMu.RUnlock()

	for _, student := range students {
		if student.ID == id {
			writeJSON(w, http.StatusOK, viewFor(student, sess))
			return
		}
	}

	http.Error(w, "Student not found", http.StatusNotFound)
}

func UpdateStudent(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.CurrentSession(r)
	if !ok {
		http.Error(w, "Please log in", http.StatusUnauthorized)
		return
	}

	id, ok := getIDFromURL(r)
	if !ok {
		http.Error(w, "Valid student ID is required", http.StatusBadRequest)
		return
	}

	var updated models.Student
	if !readJSON(w, r, &updated) {
		return
	}

	if msg := validateStudent(&updated); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	studentsMu.Lock()
	defer studentsMu.Unlock()

	for i, student := range students {
		if student.ID == id {
			if rollExists(updated.Roll, updated.Class, id) {
				http.Error(w, "This roll already exists in this class", http.StatusConflict)
				return
			}

			updated.ID = id
			updated.Username = student.Username // login নাম browser থেকে বদলানো যাবে না
			students[i] = updated
			auth.RenameUser(updated.Username, updated.Name)

			writeJSON(w, http.StatusOK, viewFor(updated, sess))
			return
		}
	}

	http.Error(w, "Student not found", http.StatusNotFound)
}

// DELETE /api/student?id=1 : Student Delete করা (admin only), login ও মুছে যায়
func DeleteStudent(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromURL(r)
	if !ok {
		http.Error(w, "Valid student ID is required", http.StatusBadRequest)
		return
	}

	studentsMu.Lock()
	defer studentsMu.Unlock()

	for i, student := range students {
		if student.ID == id {
			students = append(students[:i], students[i+1:]...)
			auth.RemoveUser(student.Username)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "Student not found", http.StatusNotFound)
}
