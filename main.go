package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Thijs-Desjardijn/chirpy/internal/database"

	_ "github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type chirp struct {
	Body   string `json:"body"`
	UserId string `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

type user struct {
	Email string `json:"email"`
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`
<html>

<body>
	<h1>Welcome, Chirpy Admin</h1>
	<p>Chirpy has been visited %d times!</p>
</body>

</html>
	`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	var chirpReq chirp
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&chirpReq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"Something went wrong"}`))
		return
	}
	if len(chirpReq.Body) > 140 {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"Chirp is too long"}`))
		return
	}
	badWords := []string{"kerfuffle", "sharbert", "fornax"}
	splitChirp := strings.Split(chirpReq.Body, " ")
	for i, w := range splitChirp {
		for _, bW := range badWords {
			if strings.ToLower(w) == bW {
				splitChirp[i] = "****"
			}
		}
	}
	chirpReq.Body = strings.Join(splitChirp, " ")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"cleaned_body":"%s"}`, chirpReq.Body)))
}

func (cfg *apiConfig) handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.dbQueries.RemoveUsers(context.Background())
	cfg.fileserverHits.Swap(0)
	w.WriteHeader(200)
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	var userR user
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&userR)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf(`{"failed":"decode jsondata: %v"}`, err)))
		return
	}
	user, err := cfg.dbQueries.CreateUser(r.Context(), userR.Email)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf(`{"failed":"creating user in database failed: %v"}`, err)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write([]byte(fmt.Sprintf(`{
  "id": "%v",
  "created_at": "%v",
  "updated_at": "%v",
  "email": "%s"
}`, user.ID, user.CreatedAt, user.UpdatedAt, user.Email)))
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)
	apiCfg := apiConfig{dbQueries: dbQueries}
	serveMux := http.NewServeMux()
	server := http.Server{
		Addr:                         ":8080",
		Handler:                      serveMux,
		DisableGeneralOptionsHandler: false,
	}
	go func() {
		server.ListenAndServe()
	}()
	serveMux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	serveMux.HandleFunc("GET /api/healthz", apiCfg.handlerReadiness)
	serveMux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	serveMux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	serveMux.HandleFunc("POST /api/chirps", apiCfg.handlerChirp)
	serveMux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	select {}
}
