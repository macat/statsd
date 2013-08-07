package main

import (
	"database/sql"
	_ "github.com/lib/pq"
	"log"
	"net/http"
)

const (
	addr     = ":9000"
	dbDriver = "postgres"
	dsName   = "sslmode=disable"
	appRoot  = "/"
)

var (
	server http.Server
	db     *sql.DB
)

func main() {
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
	topHandler.Serve(&Task{Rw: rw, Rq: rq})
}
