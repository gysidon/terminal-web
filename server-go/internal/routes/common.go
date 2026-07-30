package routes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// strParam 取路由参数。
func strParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
