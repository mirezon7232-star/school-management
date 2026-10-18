
package main

import (
	"fmt"
	"log"
	"net/http"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to School Management System!")
}

func main() {
	http.HandleFunc("/", homeHandler)

	fmt.Println("Server running at http://localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
