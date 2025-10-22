package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	// Import the package we are testing
	"github.com/kodevoid/string_analyzer/internals/handlers"
)

// --- Test Setup ---
// We copy the InMemoryStore from main.go so the test package can create its own instance.
// This is a standard practice for test doubles.

type InMemoryStore struct {
	mu    sync.RWMutex
	store map[string]*handlers.StringResource
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		store: make(map[string]*handlers.StringResource),
	}
}

func (s *InMemoryStore) Create(sr *handlers.StringResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.store[sr.Value]; exists {
		return fmt.Errorf("string already exists")
	}
	s.store[sr.Value] = sr
	return nil
}

func (s *InMemoryStore) Get(value string) (*handlers.StringResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, exists := s.store[value]
	if !exists {
		return nil, fmt.Errorf("string not found")
	}
	return res, nil
}

func (s *InMemoryStore) Delete(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.store[value]; !exists {
		return fmt.Errorf("string not found")
	}
	delete(s.store, value)
	return nil
}

func (s *InMemoryStore) Exists(value string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.store[value]
	return exists
}

func (s *InMemoryStore) List(filters map[string]any, limit, offset int) ([]handlers.StringResource, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var allResults []handlers.StringResource
	for _, res := range s.store {
		if s.matchesFilters(res, filters) {
			allResults = append(allResults, *res)
		}
	}
	totalCount := len(allResults)
	start := offset
	end := offset + limit
	if start > totalCount {
		return []handlers.StringResource{}, totalCount, nil
	}
	if end > totalCount {
		end = totalCount
	}
	paginatedResults := allResults[start:end]
	return paginatedResults, totalCount, nil
}

func (s *InMemoryStore) matchesFilters(res *handlers.StringResource, filters map[string]any) bool {
	for key, val := range filters {
		switch key {
		case "is_palindrome":
			if res.Properties.IsPalindrome != val.(bool) {
				return false
			}
		case "min_length":
			if res.Properties.Length < val.(int) {
				return false
			}
		case "max_length":
			if res.Properties.Length > val.(int) {
				return false
			}
		case "word_count":
			if res.Properties.WordCount != val.(int) {
				return false
			}
		case "contains_character":
			char := val.(string)
			if _, exists := res.Properties.CharacterFrequencyMap[char]; !exists {
				return false
			}
		}
	}
	return true
}

// setupTestServer creates a new server and store for each test to ensure isolation.
func setupTestServer() (*httptest.Server, *InMemoryStore) {
	store := NewInMemoryStore()
	// Use the SetupRoutes function from the handlers package
	router := handlers.SetupRoutes(store)
	server := httptest.NewServer(router)
	return server, store
}

// seedStore is a helper to populate the store for list/filter tests
func seedStore(store *InMemoryStore, values ...string) {
	for _, val := range values {
		props := handlers.ComputeProperties(val)
		_ = store.Create(&handlers.StringResource{
			ID:         props.SHA256Hash,
			Value:      val,
			Properties: props,
			CreatedAt:  time.Now(),
		})
	}
}

// --- Test Cases ---

func TestCreateString(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	t.Run("201 Created - new string", func(t *testing.T) {
		value := "A man, a plan, a canal: Panama"
		body, _ := json.Marshal(map[string]string{"value": value})
		reqBody := bytes.NewBuffer(body)

		resp, err := server.Client().Post(server.URL+"/strings", "application/json", reqBody)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
		}

		var resource handlers.StringResource
		if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resource.Value != value {
			t.Errorf("Expected value '%s', got '%s'", value, resource.Value)
		}
		if !resource.Properties.IsPalindrome {
			t.Error("Expected IsPalindrome to be true")
		}
		if resource.Properties.WordCount != 7 {
			t.Errorf("Expected WordCount 7, got %d", resource.Properties.WordCount)
		}
		if resource.Properties.Length != 30 {
			t.Errorf("Expected Length 30, got %d", resource.Properties.Length)
		}
		if resource.ID != resource.Properties.SHA256Hash {
			t.Error("Expected ID to match SHA256Hash")
		}
	})

	t.Run("409 Conflict - duplicate string", func(t *testing.T) {
		// Use a new server/store for this test case
		server, store := setupTestServer()
		defer server.Close()

		value := "hello"
		seedStore(store, value) // Pre-seed the store

		body, _ := json.Marshal(map[string]string{"value": value})
		reqBody := bytes.NewBuffer(body)

		resp, err := server.Client().Post(server.URL+"/strings", "application/json", reqBody)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected status %d, got %d", http.StatusConflict, resp.StatusCode)
		}

		var errResp handlers.ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("Failed to decode error response: %v", err)
		}
		if errResp.Error != "Conflict" {
			t.Errorf("Expected error 'Conflict', got '%s'", errResp.Error)
		}
	})

	t.Run("400 Bad Request - missing value", func(t *testing.T) {
		server, _ := setupTestServer()
		defer server.Close()

		body, _ := json.Marshal(map[string]string{"value": ""}) // Empty value
		reqBody := bytes.NewBuffer(body)

		resp, _ := server.Client().Post(server.URL+"/strings", "application/json", reqBody)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})

	t.Run("400 Bad Request - invalid json", func(t *testing.T) {
		server, _ := setupTestServer()
		defer server.Close()

		reqBody := bytes.NewBufferString(`{"value": "test"`) // Malformed JSON

		resp, _ := server.Client().Post(server.URL+"/strings", "application/json", reqBody)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})
}

func TestGetString(t *testing.T) {
	server, store := setupTestServer()
	defer server.Close()

	// Seed data
	value := "hello world"
	seedStore(store, value)
	encodedValue := url.PathEscape(value)

	t.Run("200 OK - found", func(t *testing.T) {
		resp, err := server.Client().Get(server.URL + "/strings/" + encodedValue)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		var resource handlers.StringResource
		if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resource.Value != value {
			t.Errorf("Expected value '%s', got '%s'", value, resource.Value)
		}
		if resource.Properties.WordCount != 2 {
			t.Errorf("Expected WordCount 2, got %d", resource.Properties.WordCount)
		}
	})

	t.Run("404 Not Found", func(t *testing.T) {
		resp, err := server.Client().Get(server.URL + "/strings/notfound")
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
		}
	})
}

func TestDeleteString(t *testing.T) {
	server, store := setupTestServer()
	defer server.Close()

	value := "to be deleted"
	seedStore(store, value)
	encodedValue := url.PathEscape(value)

	t.Run("204 No Content - deleted", func(t *testing.T) {
		// Verify it exists first
		if !store.Exists(value) {
			t.Fatal("Test setup failed: string was not seeded")
		}

		req, _ := http.NewRequest(http.MethodDelete, server.URL+"/strings/"+encodedValue, nil)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}

		// Verify it's gone from the store
		if store.Exists(value) {
			t.Error("Expected string to be deleted from store, but it still exists")
		}
	})

	t.Run("404 Not Found", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, server.URL+"/strings/notfound", nil)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
		}
	})
}

func TestListStrings(t *testing.T) {
	server, store := setupTestServer()
	defer server.Close()

	// Seed data
	seedStore(store,
		"racecar",                         // P:true, W:1, L:7
		"hello world",                     // P:false, W:2, L:11, C:'o'
		"test",                            // P:false, W:1, L:4
		"A man, a plan, a canal: Panama",  // P:true, W:7, L:30, C:'a'
		"madam",                           // P:true, W:1, L:5, C:'a'
	)

	// Helper to decode list response
	decodeList := func(resp *http.Response) (handlers.ListResponse, error) {
		var listResp handlers.ListResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return listResp, fmt.Errorf("failed to decode list response: %v", err)
		}
		return listResp, nil
	}

	t.Run("GET /strings/list - no filter", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		list, err := decodeList(resp)
		if err != nil {
			t.Fatal(err)
		}
		if list.Count != 5 {
			t.Errorf("Expected count 5, got %d", list.Count)
		}
	})

	t.Run("GET /strings/list - filter is_palindrome=true", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?is_palindrome=true")
		list, _ := decodeList(resp)
		if list.Count != 3 {
			t.Errorf("Expected count 3, got %d", list.Count)
		}
		if val, ok := list.FiltersApplied["is_palindrome"].(bool); !ok || !val {
			t.Error("Expected filters_applied.is_palindrome to be true")
		}
	})

	t.Run("GET /strings/list - filter word_count=1", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?word_count=1")
		list, _ := decodeList(resp)
		if list.Count != 3 {
			t.Errorf("Expected count 3, got %d", list.Count)
		}
	})

	t.Run("GET /strings/list - filter min_length=10", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?min_length=10")
		list, _ := decodeList(resp)
		if list.Count != 2 {
			t.Errorf("Expected count 2, got %d", list.Count)
		}
	})

	t.Run("GET /strings/list - filter max_length=10", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?max_length=10")
		list, _ := decodeList(resp)
		if list.Count != 3 {
			t.Errorf("Expected count 3, got %d", list.Count)
		}
	})

	t.Run("GET /strings/list - filter contains_character=o", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?contains_character=o")
		list, _ := decodeList(resp)
		if list.Count != 1 {
			t.Errorf("Expected count 1, got %d", list.Count)
		}
		if list.Data[0].Value != "hello world" {
			t.Errorf("Expected data[0].Value to be 'hello world', got '%s'", list.Data[0].Value)
		}
	})

	t.Run("GET /strings/list - combined filters", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?is_palindrome=true&word_count=1")
		list, _ := decodeList(resp)
		if list.Count != 2 {
			t.Errorf("Expected count 2 (racecar, madam), got %d", list.Count)
		}
	})

	t.Run("GET /strings/list - 400 Bad Request", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/list?min_length=-1")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})
}

func TestFilterByNaturalLanguage(t *testing.T) {
	server, store := setupTestServer()
	defer server.Close()

	// Seed data
	seedStore(store,
		"racecar",                         // P:true, W:1, L:7
		"hello world",                     // P:false, W:2, L:11, C:'o'
		"test",                            // P:false, W:1, L:4
		"A man, a plan, a canal: Panama",  // P:true, W:7, L:30, C:'a'
		"madam",                           // P:true, W:1, L:5, C:'a'
		"strings containing z",            // P:false, W:3, L:20, C:'z'
	)

	// Helper to decode NL response
	decodeNL := func(resp *http.Response) (handlers.NaturalLanguageResponse, error) {
		var nlResp handlers.NaturalLanguageResponse
		if err := json.NewDecoder(resp.Body).Decode(&nlResp); err != nil {
			return nlResp, fmt.Errorf("failed to decode natural language response: %v", err)
		}
		return nlResp, nil
	}

	t.Run("single word palindrome", func(t *testing.T) {
		query := url.QueryEscape("all single word palindromic strings")
		resp, _ := server.Client().Get(server.URL + "/strings/filter-by-natural-language?query=" + query)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		nl, err := decodeNL(resp)
		if err != nil {
			t.Fatal(err)
		}

		if nl.Count != 2 {
			t.Errorf("Expected count 2 (racecar, madam), got %d", nl.Count)
		}
		// JSON unmarshals numbers to float64 into an interface{}
		if val, ok := nl.InterpretedQuery.ParsedFilters["word_count"].(float64); !ok || val != 1 {
			t.Errorf("Expected parsed filter word_count=1, got %v", nl.InterpretedQuery.ParsedFilters["word_count"])
		}
		if val, ok := nl.InterpretedQuery.ParsedFilters["is_palindrome"].(bool); !ok || !val {
			t.Errorf("Expected parsed filter is_palindrome=true, got %v", nl.InterpretedQuery.ParsedFilters["is_palindrome"])
		}
	})

	t.Run("longer than 10", func(t *testing.T) {
		query := url.QueryEscape("strings longer than 10 characters")
		resp, _ := server.Client().Get(server.URL + "/strings/filter-by-natural-language?query=" + query)
		nl, _ := decodeNL(resp)

		if nl.Count != 3 { // "hello world", "A man...", "strings containing z"
			t.Errorf("Expected count 3, got %d", nl.Count)
		}
		if val, ok := nl.InterpretedQuery.ParsedFilters["min_length"].(float64); !ok || val != 11 {
			t.Errorf("Expected parsed filter min_length=11, got %v", nl.InterpretedQuery.ParsedFilters["min_length"])
		}
	})

	t.Run("contains letter z", func(t *testing.T) {
		query := url.QueryEscape("strings containing the letter z")
		resp, _ := server.Client().Get(server.URL + "/strings/filter-by-natural-language?query=" + query)
		nl, _ := decodeNL(resp)

		if nl.Count != 1 {
			t.Errorf("Expected count 1, got %d", nl.Count)
		}
		if nl.Data[0].Value != "strings containing z" {
			t.Errorf("Expected data[0].Value to be 'strings containing z', got '%s'", nl.Data[0].Value)
		}
		if val, ok := nl.InterpretedQuery.ParsedFilters["contains_character"].(string); !ok || val != "z" {
			t.Errorf("Expected parsed filter contains_character='z', got %v", nl.InterpretedQuery.ParsedFilters["contains_character"])
		}
	})

	t.Run("400 Bad Request - no query", func(t *testing.T) {
		resp, _ := server.Client().Get(server.URL + "/strings/filter-by-natural-language?query=")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})
}
