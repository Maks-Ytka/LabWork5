package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Maks-Ytka/LabWork5/httptools"
	"github.com/Maks-Ytka/LabWork5/signal"
)

var (
	port     = flag.Int("port", 8080, "server port")
	teamName = "maksytka"
)

const dbServiceURL = "http://db:8079"

type dbRecord struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type postBody struct {
	Value string `json:"value"`
}

func main() {
	flag.Parse()

	time.Sleep(3 * time.Second)

	today := time.Now().Format("2006-01-02")
	body, _ := json.Marshal(postBody{Value: today})
	resp, err := http.Post(dbServiceURL+"/db/"+teamName, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatalf("failed to init db with date: %v", err)
	}
	resp.Body.Close()

	h := http.NewServeMux()
	h.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	h.HandleFunc("/api/v1/some-data", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key param", http.StatusBadRequest)
			return
		}

		resp, err := http.Get(dbServiceURL + "/db/" + key)
		if err != nil {
			http.Error(w, "db request failed", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "db read failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	server := httptools.CreateServer(*port, h)
	server.Start()
	signal.WaitForTerminationSignal()
}
