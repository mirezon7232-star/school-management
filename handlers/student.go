package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"school-management/models"
)

var (
	students   = make([]models.Student, 0)
	nextID     = 1
	studentsMu sync.RWMutex
)

func GetStudents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	studentsMu.RLock()
	defer studentsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(students)
}

func CreateStudent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var student models.Student

	err := json.NewDecoder(r.Body).Decode(&student)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if student.Name == "" || student.Roll <= 0 || student.Class == "" {
		http.Error(
			w,
			"Name, positive roll number, and class are required",
			http.StatusBadRequest,
		)
		return
	}

	studentsMu.Lock()
	student.ID = nextID
	nextID++
	students = append(students, student)
	studentsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(student)
}

func GetStudentByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idText := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 {
		http.Error(w, "Valid student ID is required", http.StatusBadRequest)
		return
	}

	studentsMu.RLock()
	defer studentsMu.RUnlock()

	for _, student := range students {
		if student.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(student)
			return
		}
	}

	http.Error(w, "Student not found", http.StatusNotFound)
}
