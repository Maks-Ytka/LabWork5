package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Maks-Ytka/LabWork5/datastore"
)

type putRequest struct {
	Value string `json:"value"`
}

type getResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var db *datastore.Db

func main() {
	_ = os.MkdirAll("out", os.ModePerm)

	var err error
	db, err = datastore.Open("out")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/db/", handleDB)

	log.Println("Database server is running on :8079")
	if err := http.ListenAndServe(":8079", nil); err != nil {
		log.Fatalf("failed to start HTTP server: %v", err)
	}
}

func handleDB(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/db/")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleGet(w, key)
	case http.MethodPost:
		handlePost(w, r, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGet(w http.ResponseWriter, key string) {
	value, err := db.Get(key)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	resp := getResponse{
		Key:   key,
		Value: value,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handlePost(w http.ResponseWriter, r *http.Request, key string) {
	var req putRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := db.Put(key, req.Value); err != nil {
		http.Error(w, "failed to write", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
