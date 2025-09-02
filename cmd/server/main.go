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

type batchItem struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"content_base64"`
}

type batchRequest struct {
	Files []batchItem `json:"files"`
}

type batchResponseItem struct {
	Filename string `json:"filename"`
	Success  bool   `json:"success"`
	Text     string `json:"text"`
}

type batchResponse struct {
	Results []batchResponseItem `json:"results"`
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

func handleExtractBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if len(req.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "files is required and must be non-empty"})
		return
	}

	results := make([]batchResponseItem, 0, len(req.Files))
	for _, f := range req.Files {
		item := batchResponseItem{Filename: strings.TrimSpace(f.Filename)}
		if item.Filename == "" {
			item.Success = false
			item.Text = "filename is required"
			results = append(results, item)
			continue
		}
		if strings.TrimSpace(f.ContentBase64) == "" {
			item.Success = false
			item.Text = "content_base64 is required"
			results = append(results, item)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(f.ContentBase64)
		if err != nil {
			item.Success = false
			item.Text = "invalid base64: " + err.Error()
			results = append(results, item)
			continue
		}
		text, err := extract.ExtractText(item.Filename, data)
		if err != nil {
			item.Success = false
			item.Text = err.Error()
		} else {
			item.Success = true
			item.Text = text
		}
		results = append(results, item)
	}

	writeJSON(w, http.StatusOK, batchResponse{Results: results})
}

func main() {
	flagPort := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/extract", handleExtract)
	mux.HandleFunc("/extract/batch", handleExtractBatch)

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
