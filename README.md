
# String Analyzer Service 🚀

A RESTful API service built in Go that analyzes strings, computes their properties, and stores them for querying.

This service implements the 5 required endpoints for the "Backend Wizards — Stage 1 Task".

---

## Features

- **String Analysis**: Computes length, word count, palindrome status, unique characters, SHA-256 hash, and a character frequency map.
- **Full CRUD**: Create, retrieve, and delete stored strings.
- **Advanced Filtering**: List strings by their properties (length, word count, etc.).
- **Natural Language Query**: Filter strings using simple English queries (e.g., "all single word palindromes").

---

## API Endpoints

### 1. Create / Analyze a String

Analyzes and stores a new string. Returns a `409 Conflict` if the string already exists.

- **Endpoint**: `POST /strings`
- **Request Body**:
  ```json
  {
    "value": "A man, a plan, a canal: Panama"
  }
````

  - **Success Response (201 Created)**:
    ```json
    {
      "id": "f290d81084200882e505a7690f14652c7176a3915c8f131de7e753cec8f89831",
      "value": "A man, a plan, a canal: Panama",
      "properties": {
        "length": 30,
        "is_palindrome": true,
        "unique_characters": 11,
        "word_count": 7,
        "sha256_hash": "f290d81084200882e505a7690f14652c7176a3915c8f131de7e753cec8f89831",
        "character_frequency_map": {
          " ": 6,
          ",": 2,
          ":": 1,
          "A": 1,
          "P": 1,
          "a": 5,
          "c": 1,
          "l": 2,
          "m": 2,
          "n": 3,
          "p": 1
        }
      },
      "created_at": "2025-10-22T14:30:00Z"
    }
    ```

### 2\. Get a Specific String

Retrieves the analysis for a specific, URL-encoded string.

  - **Endpoint**: `GET /strings/{string_value}`
  - **Example**: `GET /strings/hello%20world`
  - **Success Response (200 OK)**: Returns the `StringResource` object (same format as above).
  - **Error Response**: `404 Not Found` if the string doesn't exist.

### 3\. Get All Strings with Filtering

Returns a paginated list of all stored strings, with support for query filters.

  - **Endpoint**: `GET /strings/list`
  - **Query Parameters**:
      - `is_palindrome` (bool): `true` or `false`
      - `min_length` (int): Minimum string length
      - `max_length` (int): Maximum string length
      - `word_count` (int): Exact word count
      - `contains_character` (string): A single character that must be in the string
  - **Success Response (200 OK)**:
    ```json
    {
      "data": [
        {
          "id": "...",
          "value": "racecar",
          /* ... properties */
        }
      ],
      "count": 1,
      "filters_applied": {
        "is_palindrome": true,
        "word_count": 1
      }
    }
    ```

### 4\. Natural Language Filtering

Returns a list of strings matching a simple English query.

  - **Endpoint**: `GET /strings/filter-by-natural-language`
  - **Query Parameter**:
      - `query` (string): The natural language query (e.g., `all single word palindromic strings`).
  - **Success Response (200 OK)**:
    ```json
    {
      "data": [
        {
          "id": "...",
          "value": "madam",
          /* ... properties */
        }
      ],
      "count": 1,
      "interpreted_query": {
        "original": "all single word palindromic strings",
        "parsed_filters": {
          "is_palindrome": true,
          "word_count": 1
        }
      }
    }
    ```

### 5\. Delete a String

Deletes a specific, URL-encoded string from the store.

  - **Endpoint**: `DELETE /strings/{string_value}`
  - **Example**: `DELETE /strings/hello%20world`
  - **Success Response**: `204 No Content`
  - **Error Response**: `404 Not Found` if the string doesn't exist.

-----

## Setup and Installation

### Dependencies

This project uses Go modules. All dependencies are listed in the `go.mod` file and can be installed locally.

1.  **Clone the repository:**

    ```sh
    git clone [https://github.com/your-username/string_analyzer.git](https://github.com/your-username/string_analyzer.git)
    cd string_analyzer
    ```

2.  **Install dependencies:**

    ```sh
    go mod tidy
    ```

-----

## How to Run

### Run Locally

To run the server on your local machine:

```sh
go run .
```

The server will start and listen on `http://localhost:8080`.

### Run Tests

To run the integration tests and verify all endpoints are working correctly:

```sh
go test ./internals/handlers/
```

```
```
