package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"

	"docparser/internal/extract"
)

type extractRequest struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

type extractResponse struct {
	Success bool   `json:"success"`
	Text    string `json:"text"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req extractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, extractResponse{Success: false, Text: "invalid json: " + err.Error()})
		return
	}

	if strings.TrimSpace(req.Filename) == "" {
		writeJSON(w, http.StatusBadRequest, extractResponse{Success: false, Text: "filename is required"})
		return
	}
	if strings.TrimSpace(req.ContentBase64) == "" {
		writeJSON(w, http.StatusBadRequest, extractResponse{Success: false, Text: "content_base64 is required"})
		return
	}

	data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, extractResponse{Success: false, Text: "invalid base64: " + err.Error()})
		return
	}

	text, err := extract.ExtractText(req.Filename, data)
	if err != nil {
		writeJSON(w, http.StatusOK, extractResponse{Success: false, Text: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, extractResponse{Success: true, Text: text})
}

func main() {
	flagPort := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/extract", handleExtract)

	port := strings.TrimSpace(*flagPort)
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
