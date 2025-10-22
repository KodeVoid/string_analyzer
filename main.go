package main

import (
	"database/sql"
	"encoding/json" // <-- 1. FIXED: Added missing import
	"fmt"
	"log/slog" // <-- 2. CLEANUP: Using structured logging
	"net/http"
	"os" // <-- 3. CLEANUP: Added for slog and PORT
	"strings"
	"sync"
	"time"

	"github.com/kodevoid/string_analyzer/internals/handlers"
	_ "modernc.org/sqlite" // SQLite driver in pure Go
)

// SQLiteStore implements the handlers.StringStore interface with a SQLite backend
type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex // for simple serialization of writes
}

// NewSQLiteStore opens/creates the DB file and prepares the table
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}

	// Create table if not exists
	createTable := `
	CREATE TABLE IF NOT EXISTS strings (
		id TEXT PRIMARY KEY,
		value TEXT UNIQUE,
		length INTEGER,
		is_palindrome INTEGER,
		unique_characters INTEGER,
		word_count INTEGER,
		sha256_hash TEXT,
		char_freq_map TEXT,
		created_at TEXT
	);
	`
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return store, nil
}

// Create inserts a new string resource
func (s *SQLiteStore) Create(sr *handlers.StringResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize CharacterFrequencyMap as JSON
	charMapJSON, err := json.Marshal(sr.Properties.CharacterFrequencyMap)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
	INSERT INTO strings (id, value, length, is_palindrome, unique_characters, word_count, sha256_hash, char_freq_map, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sr.ID, sr.Value, sr.Properties.Length,
		boolToInt(sr.Properties.IsPalindrome),
		sr.Properties.UniqueCharacters,
		sr.Properties.WordCount,
		sr.Properties.SHA256Hash,
		string(charMapJSON),
		sr.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// Get retrieves a string resource by value
func (s *SQLiteStore) Get(value string) (*handlers.StringResource, error) {
	row := s.db.QueryRow(`SELECT id, value, length, is_palindrome, unique_characters, word_count, sha256_hash, char_freq_map, created_at
		FROM strings WHERE value = ?`, value)

	var sr handlers.StringResource
	var charMapStr string
	var isPalInt int
	var createdAtStr string

	err := row.Scan(&sr.ID, &sr.Value, &sr.Properties.Length, &isPalInt,
		&sr.Properties.UniqueCharacters, &sr.Properties.WordCount,
		&sr.Properties.SHA256Hash, &charMapStr, &createdAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("string not found")
		}
		return nil, err
	}

	// Decode JSON map
	if err := json.Unmarshal([]byte(charMapStr), &sr.Properties.CharacterFrequencyMap); err != nil {
		return nil, err
	}

	sr.Properties.IsPalindrome = intToBool(isPalInt)

	// Parse time
	sr.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, err
	}

	return &sr, nil
}

// Delete removes a string resource
func (s *SQLiteStore) Delete(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM strings WHERE value = ?`, value)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("string not found")
	}
	return nil
}

// Exists checks if a string exists
func (s *SQLiteStore) Exists(value string) bool {
	row := s.db.QueryRow(`SELECT 1 FROM strings WHERE value = ?`, value)
	var dummy int
	err := row.Scan(&dummy)
	return err == nil
}

// List retrieves filtered, paginated resources
func (s *SQLiteStore) List(filters map[string]any, limit, offset int) ([]handlers.StringResource, int, error) {
	// --- 4. FIXED: Logic for List function ---
	baseQuery := `SELECT id, value, length, is_palindrome, unique_characters, word_count, sha256_hash, char_freq_map, created_at FROM strings`
	countBaseQuery := `SELECT COUNT(*) FROM strings`

	whereClauses := []string{}
	args := []any{}

	if v, ok := filters["is_palindrome"]; ok {
		whereClauses = append(whereClauses, "is_palindrome = ?")
		args = append(args, boolToInt(v.(bool)))
	}
	if v, ok := filters["min_length"]; ok {
		whereClauses = append(whereClauses, "length >= ?")
		args = append(args, v.(int))
	}
	if v, ok := filters["max_length"]; ok {
		whereClauses = append(whereClauses, "length <= ?")
		args = append(args, v.(int))
	}
	if v, ok := filters["word_count"]; ok {
		whereClauses = append(whereClauses, "word_count = ?")
		args = append(args, v.(int))
	}
	if v, ok := filters["contains_character"]; ok {
		// Use json_extract to check if the key (character) exists in the JSON map.
		whereClauses = append(whereClauses, "json_extract(char_freq_map, '$.' || ?) IS NOT NULL")
		args = append(args, v.(string))
	}

	// Build the final queries
	query := baseQuery
	countQuery := countBaseQuery

	if len(whereClauses) > 0 {
		whereStr := " WHERE " + strings.Join(whereClauses, " AND ")
		query += whereStr
		countQuery += whereStr
	}

	// Add pagination to the main query *after* filters
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	// Run the main query
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []handlers.StringResource
	for rows.Next() {
		var sr handlers.StringResource
		var charMapStr string
		var isPalInt int
		var createdAtStr string

		if err := rows.Scan(&sr.ID, &sr.Value, &sr.Properties.Length, &isPalInt,
			&sr.Properties.UniqueCharacters, &sr.Properties.WordCount,
			&sr.Properties.SHA256Hash, &charMapStr, &createdAtStr); err != nil {
			return nil, 0, err
		}

		if err := json.Unmarshal([]byte(charMapStr), &sr.Properties.CharacterFrequencyMap); err != nil {
			return nil, 0, err
		}

		sr.Properties.IsPalindrome = intToBool(isPalInt)
		sr.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, 0, err
		}

		results = append(results, sr)
	}

	// Run the count query
	var total int
	row := s.db.QueryRow(countQuery, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// --- Helper Functions ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

// --- Main Application ---

func main() {
	// --- 5. CLEANUP: Setup structured JSON logging ---
	// This single line configures the global logger used in handlers.go
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// 1. Create SQLite-backed store
	store, err := NewSQLiteStore("strings.db")
	if err != nil {
		slog.Error("Failed to open SQLite store", "error", err)
		os.Exit(1)
	}
	defer store.db.Close() // Close DB when program exits
	slog.Info("database connection established", "path", "strings.db")

	// 2. Setup HTTP routes
	// This uses the SetupRoutes from your handlers package
	router := handlers.SetupRoutes(store)

	// --- 6. CLEANUP: Use PORT from environment for deployment ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000" // Default to 8080
	}

	// 3. Start HTTP server
	slog.Info("Starting String Analyzer server", "port", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}
