package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

const defaultDaemonAddr = "127.0.0.1:43123"

type apiServer struct {
	service *CaptureService
}

type textRequest struct {
	ClientID string `json:"client_id"`
	Text     string `json:"text"`
}

func newAPIServer(service *CaptureService) *apiServer {
	return &apiServer{service: service}
}

func (s *apiServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/capture", s.handleCapture)
	mux.HandleFunc("/command", s.handleCommand)
	mux.HandleFunc("/review", s.handleReview)
	return mux
}

func (s *apiServer) serve(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.handler(),
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- server.Shutdown(shutdownCtx)
	}()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return <-errCh
}

func (s *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"status":  "ok",
		"address": defaultDaemonAddr,
	})
}

func (s *apiServer) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req textRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}
	resp, err := s.service.Process(r.Context(), CaptureRequest{
		ClientID: req.ClientID,
		Kind:     RequestKindCapture,
		Text:     req.Text,
	}, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *apiServer) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req textRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}
	resp, err := s.service.Process(r.Context(), CaptureRequest{
		ClientID: req.ClientID,
		Kind:     RequestKindCommand,
		Text:     req.Text,
	}, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *apiServer) handleReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	day := r.URL.Query().Get("day")
	clientID := r.URL.Query().Get("client_id")
	resp, err := s.service.Process(r.Context(), CaptureRequest{
		ClientID:  clientID,
		Kind:      RequestKindReview,
		ReviewDay: day,
	}, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, CaptureResponse{
		OK:    false,
		Error: err.Error(),
		Reply: err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
