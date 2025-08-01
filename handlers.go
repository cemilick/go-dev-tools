package main

import (
    "crypto/rand"
    "fmt"
    "html/template"
    "math/big"
    "net/http"
    "time"
)

func IndexHandler(w http.ResponseWriter, r *http.Request) {
    tmpl := template.Must(template.ParseFiles("templates/index.html"))
    tmpl.Execute(w, nil)
}

func GenerateTokenHandler(w http.ResponseWriter, r *http.Request) {
	length := 44
	charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()_+-/"
	token := make([]byte, length)
	for i := 0; i < length; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		token[i] = charset[n.Int64()]
	}
	fmt.Fprint(w, string(token))
}

func GenerateSecretHandler(w http.ResponseWriter, r *http.Request) {
	length := 15
	charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()_+{}[]:;,.<>?/|~`"
	secret := make([]byte, length)
	for i := 0; i < length; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		secret[i] = charset[n.Int64()]
	}
	fmt.Fprint(w, string(secret))
}

func GenerateTimestampHandler(w http.ResponseWriter, r *http.Request) {
    ts := time.Now().Unix()
    fmt.Fprint(w, ts)
}

func GeneratePasswordHandler(w http.ResponseWriter, r *http.Request) {
    length := 16
    charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+"
    password := ""
    for i := 0; i < length; i++ {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
        password += string(charset[n.Int64()])
    }
    fmt.Fprint(w, password)
}
