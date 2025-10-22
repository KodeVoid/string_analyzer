package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	// "fmt" // <-- Replaced with slog
	"log/slog" // <-- ADDED: Proper structured logging
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Models
type StringCreateRequest struct {
	Value string `json:"value"`
}

type Properties struct {
	Length                int            `json:"length"`
	IsPalindrome          bool           `json:"is_palindrome"`
	UniqueCharacters      int            `json:"unique_characters"`
	WordCount             int            `json:"word_count"`
	SHA256Hash            string         `json:"sha256_hash"`
	CharacterFrequencyMap map[string]int `json:"character_frequency_map"`
}

type StringResource struct {
	ID         string     `json:"id"`
	Value      string     `json:"value"`
	Properties Properties `json:"properties"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ErrorResponse struct {
	Status  int    `json:"status"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

type ListResponse struct {
	Data           []StringResource `json:"data"`
	Count          int              `json:"count"`
	FiltersApplied map[string]any   `json:"filters_applied"`
}

type NaturalLanguageResponse struct {
	Data             []StringResource `json:"data"`
	Count            int              `json:"count"`
	InterpretedQuery InterpretedQuery `json:"interpreted_query"`
}

type InterpretedQuery struct {
	Original      string         `json:"original"`
	ParsedFilters map[string]any `json:"parsed_filters"`
}

// Storage interface - implement with your choice of DB
type StringStore interface {
	Create(sr *StringResource) error
	Get(value string) (*StringResource, error)
	Delete(value string) error
	List(filters map[string]any, limit, offset int) ([]StringResource, int, error)
	Exists(value string) bool
}

type Handler struct {
	store StringStore
}

func NewHandler(store StringStore) *Handler {
	return &Handler{store: store}
}

// Helper functions
func ComputeProperties(value string) Properties {
	hash := sha256.Sum256([]byte(value))
	hashStr := hex.EncodeToString(hash[:])

	freqMap := make(map[string]int)
	for _, r := range value {
		freqMap[string(r)]++
	}

	isPalin := isPalindrome(value)
	wordCount := countWords(value)

	return Properties{
		Length:                len(value),
		IsPalindrome:          isPalin,
		UniqueCharacters:      len(freqMap),
		WordCount:             wordCount,
		SHA256Hash:            hashStr,
		CharacterFrequencyMap: freqMap,
	}
}

func isPalindrome(s string) bool {
	// Remove non-alphanumeric and convert to lowercase
	cleaned := strings.Builder{}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cleaned.WriteRune(r)
		}
	}
	str := cleaned.String()

	for i := 0; i < len(str)/2; i++ {
		if str[i] != str[len(str)-1-i] {
			return false
		}
	}
	return true
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		// Use structured logging instead of fmt.Println
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, errorType, message string) {
	// --- ADDED LOGGING ---
	// Log client-side (4xx) errors as Info, server-side (5xx) as Error
	if status >= 500 {
		slog.Error("sending server error response",
			"status", status,
			"error_type", errorType,
			"message", message,
		)
	} else {
		slog.Info("sending client error response",
			"status", status,
			"error_type", errorType,
			"message", message,
		)
	}
	// --- END ADDED ---

	writeJSON(w, status, ErrorResponse{
		Status:  status,
		Error:   errorType,
		Message: message,
	})
}

// POST /strings
func (h *Handler) CreateString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only POST is allowed")
		return
	}

	var req StringCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "Invalid JSON: "+err.Error())
		return
	}

	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "Missing required field: value")
		return
	}

	// Check if already exists
	if h.store.Exists(req.Value) {
		writeError(w, http.StatusConflict, "Conflict", "String already exists")
		return
	}

	props := ComputeProperties(req.Value)
	resource := &StringResource{
		ID:         props.SHA256Hash,
		Value:      req.Value,
		Properties: props,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.store.Create(resource); err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	// --- ADDED LOGGING ---
	slog.Info("string created", "id", resource.ID, "value", resource.Value)
	// --- END ADDED ---

	writeJSON(w, http.StatusCreated, resource)
}

// GET /strings/{string_value}
func (h *Handler) GetString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only GET is allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/strings/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusBadRequest, "Bad Request", "Missing string value in path")
		return
	}

	stringValue, err := url.PathUnescape(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "Invalid URL encoding")
		return
	}

	// --- ADDED LOGGING ---
	slog.Info("getting string", "value", stringValue)
	// --- END ADDED ---

	resource, err := h.store.Get(stringValue)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "String not found")
		return
	}

	writeJSON(w, http.StatusOK, resource)
}

// DELETE /strings/{string_value}
func (h *Handler) DeleteString(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only DELETE is allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/strings/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusBadRequest, "Bad Request", "Missing string value in path")
		return
	}

	stringValue, err := url.PathUnescape(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "Invalid URL encoding")
		return
	}

	err = h.store.Delete(stringValue)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "String not found")
		return
	}

	// --- ADDED LOGGING ---
	slog.Info("string deleted", "value", stringValue)
	// --- END ADDED ---

	w.WriteHeader(http.StatusNoContent)
}

// GET /strings/list
func (h *Handler) ListStrings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only GET is allowed")
		return
	}

	query := r.URL.Query()
	filters := make(map[string]any)

	// Parse is_palindrome
	if val := query.Get("is_palindrome"); val != "" {
		isPalin, err := strconv.ParseBool(val)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid is_palindrome value")
			return
		}
		filters["is_palindrome"] = isPalin
	}

	// Parse min_length
	if val := query.Get("min_length"); val != "" {
		minLen, err := strconv.Atoi(val)
		if err != nil || minLen < 0 {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid min_length value")
			return
		}
		filters["min_length"] = minLen
	}

	// Parse max_length
	if val := query.Get("max_length"); val != "" {
		maxLen, err := strconv.Atoi(val)
		if err != nil || maxLen < 0 {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid max_length value")
			return
		}
		filters["max_length"] = maxLen
	}

	// Parse word_count
	if val := query.Get("word_count"); val != "" {
		wordCount, err := strconv.Atoi(val)
		if err != nil || wordCount < 0 {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid word_count value")
			return
		}
		filters["word_count"] = wordCount
	}

	// Parse contains_character
	if val := query.Get("contains_character"); val != "" {
		if len([]rune(val)) != 1 {
			writeError(w, http.StatusBadRequest, "Bad Request", "contains_character must be exactly one character")
			return
		}
		filters["contains_character"] = val
	}

	// Parse limit (default 25, max 100)
	limit := 25
	if val := query.Get("limit"); val != "" {
		l, err := strconv.Atoi(val)
		if err != nil || l < 1 || l > 100 {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid limit value (1-100)")
			return
		}
		limit = l
	}

	// Parse offset (default 0)
	offset := 0
	if val := query.Get("offset"); val != "" {
		o, err := strconv.Atoi(val)
		if err != nil || o < 0 {
			writeError(w, http.StatusBadRequest, "Bad Request", "Invalid offset value")
			return
		}
		offset = o
	}

	// --- ADDED LOGGING ---
	// Log the filters, but only if there are any
	if len(filters) > 0 {
		slog.Info("listing strings with filters", "filters", filters, "limit", limit, "offset", offset)
	} else {
		slog.Info("listing all strings", "limit", limit, "offset", offset)
	}
	// --- END ADDED ---

	data, count, err := h.store.List(filters, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	response := ListResponse{
		Data:           data,
		Count:          count,
		FiltersApplied: filters,
	}

	writeJSON(w, http.StatusOK, response)
}

// GET /strings/filter-by-natural-language
func (h *Handler) FilterByNaturalLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed", "Only GET is allowed")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "Missing required query parameter: query")
		return
	}

	// Parse natural language query into filters
	filters, err := parseNaturalLanguageQuery(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "Unable to parse query: "+err.Error())
		return
	}

	// --- ADDED LOGGING ---
	slog.Info("parsed natural language query", "original_query", query, "parsed_filters", filters)
	// --- END ADDED ---

	// Use default pagination
	limit := 25
	offset := 0

	data, count, err := h.store.List(filters, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	response := NaturalLanguageResponse{
		Data:  data,
		Count: count,
		InterpretedQuery: InterpretedQuery{
			Original:      query,
			ParsedFilters: filters,
		},
	}

	writeJSON(w, http.StatusOK, response)
}

// Simple natural language parser
func parseNaturalLanguageQuery(query string) (map[string]any, error) {
	filters := make(map[string]any)
	lower := strings.ToLower(query)
	words := strings.Fields(lower) // Get words for easier parsing

	// Check for palindrome
	// Use "palindrom" to catch "palindrome" and "palindromic"
	if strings.Contains(lower, "palindrom") {
		filters["is_palindrome"] = true
	}

	// Check for word count patterns
	for i, word := range words {
		if (word == "word" || word == "words") && i > 0 {
			if count, err := strconv.Atoi(words[i-1]); err == nil {
				// Handles: "5 words"
				filters["word_count"] = count
				break
			} else if words[i-1] == "single" {
				// Handles: "single word"
				filters["word_count"] = 1
				break
			}
		}
	}

	// Check for length patterns: "longer than 10", "length > 20", "shorter than 5"
	for i, word := range words {
		if word == "than" && i > 0 && i < len(words)-1 {
			if length, err := strconv.Atoi(words[i+1]); err == nil {
				if words[i-1] == "longer" {
					filters["min_length"] = length + 1
				} else if words[i-1] == "shorter" {
					filters["max_length"] = length - 1
				}
			}
		}
	}

	// Check for "contains X" or "containing X" pattern
	containsIndex := strings.Index(lower, "containing")
	searchWord := "containing"

	if containsIndex == -1 {
		containsIndex = strings.Index(lower, "contains")
		searchWord = "contains"
	}

	if containsIndex != -1 {
		// Get the part of the string *after* the search word
		// e.g., from "strings containing the letter z", we get " the letter z"
		remainingStr := lower[containsIndex+len(searchWord):]

		// Heuristic 1: Find "letter X"
		// e.g., "... containing the letter z"
		letterIndex := strings.Index(remainingStr, "letter ")
		if letterIndex != -1 {
			// "letter " is 7 chars. Get char after it.
			charAfter := strings.TrimSpace(remainingStr[letterIndex+len("letter "):])
			if len(charAfter) > 0 {
				filters["contains_character"] = string([]rune(charAfter)[0])
			}
		} else {
			// Heuristic 2: Find the last word of the *whole query* and check if 1 char
			// e.g., "... contains z"
			lastWord := words[len(words)-1]
			if len([]rune(lastWord)) == 1 {
				filters["contains_character"] = lastWord
			}
		}
		// You could add more heuristics here, like for "contains 'a'"
	}

	return filters, nil
}
