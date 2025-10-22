package handlers

import (
	"net/http"
)

// HandleStringValue acts as a sub-router for the /strings/{string_value} path.
// It is registered on the prefix "/strings/" and dispatches to the correct
// handler (GetString or DeleteString) based on the HTTP method.
// This approach is compatible with the provided handlers that parse the path manually.
func (h *Handler) HandleStringValue(w http.ResponseWriter, r *http.Request) {
	// Prevent this prefix handler from matching /strings/list or /strings/filter-by-natural-language
	// This is a safeguard, as ServeMux should prioritize more specific routes first.
	if r.URL.Path == "/strings/list" || r.URL.Path == "/strings/filter-by-natural-language" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.GetString(w, r)
	case http.MethodDelete:
		h.DeleteString(w, r)
	default:
		// Send a 405 Method Not Allowed response.
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only GET and DELETE are allowed for this path")
	}
}

// SetupRoutes initializes a new http.ServeMux, registers all API endpoints
// from the OpenAPI spec, and returns the mux as an http.Handler.
// It takes a StringStore implementation as an argument to inject the dependency.
func SetupRoutes(store StringStore) http.Handler {
	// Create the main handler which contains the storage dependency
	h := NewHandler(store)

	// Create a new ServeMux (HTTP router)
	mux := http.NewServeMux()

	// Register routes based on OpenAPI spec.
	// The http.ServeMux matches longer, more specific paths first.

	// POST /strings
	// Handles creating a new string resource.
	// The handler itself enforces the POST method.
	mux.HandleFunc("/strings", h.CreateString)

	// GET /strings/list
	// Handles listing/filtering strings.
	// The handler itself enforces the GET method.
	mux.HandleFunc("/strings/list", h.ListStrings)

	// GET /strings/filter-by-natural-language
	// Handles natural language queries.
	// The handler itself enforces the GET method.
	mux.HandleFunc("/strings/filter-by-natural-language", h.FilterByNaturalLanguage)

	// GET /strings/{string_value}
	// DELETE /strings/{string_value}
	//
	// This path uses a prefix match on "/strings/".
	// The HandleStringValue helper function multiplexes based on the method (GET/DELETE).
	// The individual handlers (GetString, DeleteString) are responsible for
	// parsing the {string_value} from r.URL.Path, which this routing setup supports.
	mux.HandleFunc("/strings/", h.HandleStringValue)

	return mux
}
