package auth

import (
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPasswordCheck(t *testing.T) {
	pasword := "1234"
	hashedPw, err := HashPassword(pasword)
	if err != nil {
		log.Fatal(err)
	}
	err = CheckPasswordHash(pasword, hashedPw)
	if err != nil {
		log.Fatal(err)
	}
}

func TestJWT(t *testing.T) {
	id, err := uuid.NewRandom()
	if err != nil {
		log.Fatal(err)
	}
	jwtSecret, err := MakeJWT(id, "12093487hello", 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	idReturned, err := ValidateJWT(jwtSecret, "12093487hello")
	if err != nil {
		log.Fatal(err)
	}
	if id != idReturned {
		log.Fatal("id's didn't match")
	}
}

func TestGetBearerToken(t *testing.T) {
	header := http.Header{}
	header.Add("Authorization", "Bearer TOKEN_STRING")
	testReturnValue, err := GetBearerToken(header)
	if err != nil {
		log.Fatal(err)
	}
	if testReturnValue != "TOKEN_STRING" {
		log.Fatalf("TOKEN_STRING and %v don't match", testReturnValue)
	}
}
