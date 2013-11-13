package main

import (
	"database/sql"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	addr    = ":9000"
	appRoot = "/"
)

var (
	server http.Server
	db     *sql.DB
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbDriver := os.Getenv("DB_DRIVER")
	dbSetup := []string{"user=" + os.Getenv("DB_USER"),
		"password=" + os.Getenv("DB_PASSWORD"),
		"dbname=" + os.Getenv("DB_NAME"),
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

	server.Addr = addr
	server.Handler = http.HandlerFunc(topHttpHandler)

	if err := server.ListenAndServe(); err != nil {
		log.Fatalln(err)
	}
}

func topHttpHandler(rw http.ResponseWriter, rq *http.Request) {
	rw2 := &ResponseWriter{rw, 200}
	topHandler.Serve(&Task{Rw: rw2, Rq: rq})
}
