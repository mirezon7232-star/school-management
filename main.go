package main

import (
	"fmt"
	"log"
	"net/http"

	"school-management/handlers"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to School Management System!")
}

func main() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/api/students", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.GetStudents(w, r)
		case http.MethodPost:
			handlers.CreateStudent(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/api/student", handlers.GetStudentByID)

	fmt.Println("Server running at http://localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
