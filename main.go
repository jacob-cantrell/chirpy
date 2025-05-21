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
	"github.com/jacob-cantrell/chirpy/internal/auth"
	"github.com/jacob-cantrell/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
	secret         string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type Token struct {
	Token string `json:"token"`
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
	cfg.secret = os.Getenv("SECRET")

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
	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		//  Execute sql query
		dbChirps, err := cfg.queries.GetAllChirps(r.Context())
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps records")
		}

		// Map to json; intiailize array
		var chirps []Chirp
		for _, c := range dbChirps {
			chirps = append(chirps, Chirp{
				ID:        c.ID,
				CreatedAt: c.CreatedAt,
				UpdatedAt: c.UpdatedAt,
				Body:      c.Body,
				UserID:    c.UserID,
			})
		}

		// Response
		respondWithJSON(w, http.StatusOK, chirps)
	})
	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		// Get ID passed in
		chirpID, err := uuid.Parse(r.PathValue("chirpID"))
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Could not parse path value")
		}

		// Execute sql query
		dbChirp, err := cfg.queries.GetChirpById(r.Context(), chirpID)
		if err != nil {
			respondWithError(w, http.StatusNotFound, "Could not retrieve chirp with given ID")
		}

		// Map to json
		chirp := Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}

		// Response
		respondWithJSON(w, http.StatusOK, chirp)
	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Request body
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
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

		// Get hashed password
		hPw, err := auth.HashPassword(params.Password)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		}

		// Execute SQL query to create user
		userParams := database.CreateUserParams{
			Email:          params.Email,
			HashedPassword: hPw,
		}
		dbUser, err := cfg.queries.CreateUser(r.Context(), userParams)
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
	mux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Request body
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		// Decode JSON request body
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			// Handle decode error
			respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
			return
		}

		// Get bearer token (required)
		tokString, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "No valid Bearer token in Authorization header")
			return
		}

		// Validate JWT
		userId, err := auth.ValidateJWT(tokString, cfg.secret)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "JWT validation failed")
			return
		}

		// Get hashed password
		hPw, err := auth.HashPassword(params.Password)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		}

		// Execute SQL query to update user
		updatedUserParams := database.UpdateUserInformationParams{
			Email:          params.Email,
			HashedPassword: hPw,
			ID:             userId,
		}

		dbUser, err := cfg.queries.UpdateUserInformation(r.Context(), updatedUserParams)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error updating user information")
			return
		}

		// Map to JSON & respond
		u := User{
			ID:        dbUser.ID,
			CreatedAt: dbUser.CreatedAt,
			UpdatedAt: dbUser.UpdatedAt,
			Email:     dbUser.Email,
		}

		respondWithJSON(w, http.StatusOK, u)
	})
	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		// Request body struct
		type parameters struct {
			Body string `json:"body"`
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

		// Check bearer token
		tokString, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "No valid Bearer token in Authorization header")
			return
		}

		// Validate JWT
		tok, err := auth.ValidateJWT(tokString, cfg.secret)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "JWT validation failed")
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
			UserID: tok,
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
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		// Request body
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
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

		// Exec GetUserByEmail query
		dbUser, err := cfg.queries.GetUserByEmail(r.Context(), params.Email)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		// Compare passwords
		if err := auth.CheckPasswordHash(dbUser.HashedPassword, params.Password); err != nil {
			respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		// Create access token
		tok, err := auth.MakeJWT(dbUser.ID, cfg.secret, time.Hour)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating JWT token")
			return
		}

		// Create refresh token
		refreshTokString, err := auth.MakeRefreshToken()
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating refresh token")
			return
		}

		// Add refresh token to database
		refreshParams := database.CreateRefreshTokenParams{
			Token:     refreshTokString,
			UserID:    dbUser.ID,
			ExpiresAt: time.Now().Add(time.Hour * 24 * 60),
		}

		_, err = cfg.queries.CreateRefreshToken(r.Context(), refreshParams)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error adding refresh token to database")
			return
		}

		// Map to json
		u := User{
			ID:           dbUser.ID,
			CreatedAt:    dbUser.CreatedAt,
			UpdatedAt:    dbUser.UpdatedAt,
			Email:        dbUser.Email,
			Token:        tok,
			RefreshToken: refreshTokString,
		}

		// Response
		respondWithJSON(w, http.StatusOK, u)
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		// Check bearer token
		tokString, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "No valid Bearer token in Authorization header")
			return
		}

		// Look up token in database
		refreshTok, err := cfg.queries.GetRefreshTokenByTokenString(r.Context(), tokString)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Refresh token not found in database")
			return
		}

		// Check if expired
		if time.Now().After(refreshTok.ExpiresAt) {
			respondWithError(w, http.StatusUnauthorized, "Refresh token is expired")
			return
		}

		// Check if revoked
		if refreshTok.RevokedAt.Valid {
			respondWithError(w, http.StatusUnauthorized, "Refresh token is revoked")
			return
		}

		// Create access token
		accessToken, err := auth.MakeJWT(refreshTok.UserID, cfg.secret, time.Hour)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating JWT token")
			return
		}

		// Respond w JSON
		respondWithJSON(w, http.StatusOK, Token{Token: accessToken})

	})
	mux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, r *http.Request) {
		// Check bearer token
		tokString, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "No valid Bearer token in Authorization header")
			return
		}

		// Look up token in database
		_, err = cfg.queries.GetRefreshTokenByTokenString(r.Context(), tokString)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Refresh token not found in database")
			return
		}

		// Revoke token
		err = cfg.queries.RevokeRefreshTokenAccess(r.Context(), tokString)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error revoking refresh token")
			return
		}

		respondWithJSON(w, http.StatusNoContent, nil)
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
