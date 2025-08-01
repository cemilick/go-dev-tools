package main

import (
    "log"
    "net/http"
)

func main() {
    // Static file untuk CSS (opsional)
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    // Halaman utama
    http.HandleFunc("/", IndexHandler)

    // API endpoints
    http.HandleFunc("/generate-token", GenerateTokenHandler)
    http.HandleFunc("/generate-secret", GenerateSecretHandler)
    http.HandleFunc("/generate-timestamp", GenerateTimestampHandler)
    http.HandleFunc("/generate-password", GeneratePasswordHandler)

    log.Println("Server running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
