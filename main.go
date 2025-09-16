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
	"time"

	"github.com/Thijs-Desjardijn/chirpy/internal/database"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type chirp struct {
	Id     uuid.UUID `json:"id"`
	Body   string    `json:"body"`
	UserId uuid.UUID `json:"user_id"`
}

type ChirpRes struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

type user struct {
	Email string `json:"email"`
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.dbQueries.RemoveUsers(context.Background())
	cfg.dbQueries.RemoveChirps(context.Background())
	cfg.fileserverHits.Swap(0)
	w.WriteHeader(200)
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

func errHandler(w http.ResponseWriter, message string, errorCode int) {
	w.WriteHeader(errorCode)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, message)))
}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	var chirpReq chirp
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&chirpReq)
	if err != nil {
		errHandler(w, fmt.Sprintf("decoding failed %v", err), 400)
		return
	}
	if len(chirpReq.Body) > 140 {
		errHandler(w, "Chirp is too long", 404)
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
	args := database.CreateChirpParams{
		Body:   chirpReq.Body,
		UserID: chirpReq.UserId,
	}
	chirp, err := cfg.dbQueries.CreateChirp(context.Background(), args)
	if err != nil {
		errHandler(w, fmt.Sprintf("failed to save chirp in database: %v", err), 500)
		return
	}
	w.WriteHeader(201)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{
  "id": "%v",
  "created_at": "%v",
  "updated_at": "%v",
  "body": "%s",
  "user_id": "%v"
}`, chirp.ID, chirp.CreatedAt, chirp.UpdatedAt, chirp.Body, chirp.UserID)))
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	var chirpBody chirp
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&chirpBody)
	if err != nil {
		errHandler(w, fmt.Sprintf("decoding failed %v", err), 404)
		return
	}
	chirp, err := cfg.dbQueries.GetChirp(context.Background(), chirpBody.Id)
	if err != nil {
		errHandler(w, fmt.Sprintf("getting chirp failed %v", err), 404)
		return
	}
	formattedChirp := ChirpRes{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	data, err := json.Marshal(formattedChirp)
	if err != nil {
		errHandler(w, fmt.Sprintf("marshal failed %v", err), 404)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	allChirps, err := cfg.dbQueries.GetAllChirps(r.Context())
	if err != nil {
		errHandler(w, fmt.Sprintf("getting all chirps failed %v", err), 500)
		return
	}
	var formattedAllchirps []ChirpRes
	for _, chirp := range allChirps {
		formattedChirp := ChirpRes{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
		formattedAllchirps = append(formattedAllchirps, formattedChirp)
	}
	jsonAllChirps, err := json.Marshal(formattedAllchirps)
	if err != nil {
		errHandler(w, fmt.Sprintf("marshal failed %v", err), 400)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(jsonAllChirps)
}

func (cfg *apiConfig) handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	var userR user
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&userR)
	if err != nil {
		errHandler(w, fmt.Sprintf("decoding failed %v", err), 400)
		return
	}
	user, err := cfg.dbQueries.CreateUser(r.Context(), userR.Email)
	if err != nil {
		errHandler(w, fmt.Sprintf("creating user in database failed: %v", err), 500)
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
	serveMux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)
	select {}
}
