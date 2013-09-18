package main

import (
	"database/sql"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

func dbTest(rq *http.Request, test func(*Task)) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbDriver := os.Getenv("DB_DRIVER")
	dbSetup := []string{
		"user=" + os.Getenv("DB_USER"),
		"dbname=" + os.Getenv("DB_NAME_TEST"),
		"sslmode=" + os.Getenv("DB_SSLMODE")}
	dsName := strings.Join(dbSetup, " ")

	if d, err := sql.Open(dbDriver, dsName); err != nil {
		log.Fatalln(err)
	} else {
		db = d
	}

	if err := db.Ping(); err != nil {
		log.Fatalln(err)
	}

	rw := httptest.NewRecorder()
	test(&Task{Rq: rq, Rw: &ResponseWriter{rw, 0}})
}
