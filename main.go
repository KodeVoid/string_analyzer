package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/kodevoid/string_analyzer/internals/handlers"
)

// InMemoryStore is a simple in-memory implementation of the StringStore interface.
// It is safe for concurrent use.
type InMemoryStore struct {
	mu    sync.RWMutex
	store map[string]*handlers.StringResource
}

// NewInMemoryStore creates a new, empty in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		store: make(map[string]*handlers.StringResource),
	}
}

// Create adds a new StringResource to the store.
// In this implementation, we trust the handler to have already checked for existence.
func (s *InMemoryStore) Create(sr *handlers.StringResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use the original value as the key, as that's what Get/Delete use.
	s.store[sr.Value] = sr
	return nil
}

// Get retrieves a StringResource by its exact string value.
func (s *InMemoryStore) Get(value string) (*handlers.StringResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res, exists := s.store[value]
	if !exists {
		return nil, fmt.Errorf("string not found")
	}
	return res, nil
}

// Delete removes a StringResource by its exact string value.
func (s *InMemoryStore) Delete(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.store[value]; !exists {
		return fmt.Errorf("string not found")
	}
	delete(s.store, value)
	return nil
}

// Exists checks if a string already exists in the store.
func (s *InMemoryStore) Exists(value string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.store[value]
	return exists
}

// List retrieves a paginated and filtered list of StringResources.
func (s *InMemoryStore) List(filters map[string]any, limit, offset int) ([]handlers.StringResource, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allResults []handlers.StringResource

	// Iterate and apply filters
	for _, res := range s.store {
		if s.matchesFilters(res, filters) {
			allResults = append(allResults, *res)
		}
	}

	totalCount := len(allResults)

	// Apply pagination
	start := offset
	end := offset + limit

	if start > totalCount {
		// Offset is past the end of the results
		return []handlers.StringResource{}, totalCount, nil
	}

	if end > totalCount {
		// Don't slice past the end
		end = totalCount
	}

	paginatedResults := allResults[start:end]

	return paginatedResults, totalCount, nil
}

// matchesFilters is a helper to check if a resource matches all provided filters.
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
			// Check if the character frequency map has the character
			char := val.(string)
			if _, exists := res.Properties.CharacterFrequencyMap[char]; !exists {
				return false
			}
		}
	}
	return true
}

func main() {
	// 1. Create an instance of our StringStore implementation
	store := NewInMemoryStore()

	// 2. Pass the store to the SetupRoutes function to get the router
	//    (assuming SetupRoutes is in the 'handlers' package)
	router := handlers.SetupRoutes(store)

	// 3. Start the HTTP server
	port := ":8000"
	log.Printf("Starting String Analyzer server on %s...", port)

	// http.ListenAndServe starts the server
	err := http.ListenAndServe(port, router)
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
