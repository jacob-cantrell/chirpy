package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jacob-cantrell/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	count := cfg.fileserverHits.Load()
	fmt.Fprintf(w, "<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", count)
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	// Reset server hit counter
	_ = cfg.fileserverHits.Swap(0)

	// Delete all users from user DB
	err := cfg.queries.DeleteAllUsers(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not reset user database table")
	}

	// Write response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if cfg.platform == "dev" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	// Response struct
	type errorResponse struct {
		Error string `json:"error"`
	}

	// Generate JSON for error response
	respondWithJSON(w, code, errorResponse{
		Error: msg,
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(dat)
}

func cleanString(msg string) string {
	// Split string
	words := strings.Split(msg, " ")

	// Loop through words
	for i := range words {
		if strings.ToLower(words[i]) == "kerfuffle" || strings.ToLower(words[i]) == "sharbert" || strings.ToLower(words[i]) == "fornax" {
			words[i] = "****"
		}
	}

	// Rejoin words into string and return it
	return strings.Join(words, " ")
}

func main() {
	godotenv.Load()
	cfg := apiConfig{}

	// Load .env variables
	cfg.platform = os.Getenv("PLATFORM")
	dbURL := os.Getenv("DB_URL")

	// Open database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	// Load SQLC generated queries into apiConfig
	cfg.queries = database.New(db)

	// Create ServeMux and HandleFunc for endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Request body
		type parameters struct {
			Email string `json:"email"`
		}

		// Decode JSON Request body
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			// Handle decode error
			respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
			return
		}

		// Execute SQL query to create user
		dbUser, err := cfg.queries.CreateUser(r.Context(), params.Email)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
			return
		}

		// Map to json
		u := User{
			ID:        dbUser.ID,
			CreatedAt: dbUser.CreatedAt,
			UpdatedAt: dbUser.UpdatedAt,
			Email:     dbUser.Email,
		}

		// Respond with JSON
		respondWithJSON(w, http.StatusCreated, u)
	})
	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		// Request body struct
		type parameters struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
		}

		// Decode json
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			// Handle decode error
			respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
			return
		}

		// Check length of request, handle error
		if len(params.Body) > 140 {
			respondWithError(w, http.StatusBadRequest, "Chirp is too long")
			return
		}

		// Valid chirp, create params & execute query
		chirpParams := database.CreateChirpParams{
			Body:   params.Body,
			UserID: params.UserID,
		}
		dbChirp, err := cfg.queries.CreateChirp(r.Context(), chirpParams)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp record")
			return
		}

		// Map to json
		c := Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      cleanString(dbChirp.Body),
			UserID:    dbChirp.UserID,
		}

		// Respond with JSON & created status code
		respondWithJSON(w, http.StatusCreated, c)
	})
	mux.HandleFunc("GET /admin/metrics", cfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)
	mux.Handle("/app/", http.StripPrefix("/app", cfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	server.ListenAndServe()
}
