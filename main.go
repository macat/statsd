package main

import (
	"database/sql"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"runtime"
)

const (
	addr     = ":9000"
	dbDriver = "postgres"
	dsName   = "sslmode=disable"
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
	tx, err := db.Begin()
	if err != nil {
		log.Println("BEGIN failed:", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec("SET TRANSACTION ISOLATION LEVEL SERIALIZABLE")
	if err != nil {
		log.Println("SET TRANSACTION failed:", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer func() {
		if err := recover(); err != nil {
			if err := tx.Rollback(); err != nil {
				log.Println("ROLLBACK failed:", err)
			}
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			log.Printf("Panic: %v\n%s\n", err, buf)
			rw.WriteHeader(http.StatusInternalServerError)
		} else {
			if err = tx.Commit(); err != nil {
				log.Println("COMMIT failed:", err)
			}
		}
	}()

	rw.Header().Set("Cache-Control", "no-cache, no-store,  must-revalidate")
	rw.Header().Set("Pragma", "no-cache")
	topHandler.Serve(&Task{rw, rq, tx})
}
