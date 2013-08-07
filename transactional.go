package main

import (
	"log"
	"net/http"
	"runtime"
)

type Transactional struct {
	Handler
}

func (h *Transactional) Serve(t *Task) {
	tx, err := db.Begin()
	if err != nil {
		log.Println("BEGIN failed:", err)
		t.Rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	t.Tx = tx

	defer func() {
		if err := recover(); err != nil {
			if err := tx.Rollback(); err != nil {
				log.Println("ROLLBACK failed:", err)
			}
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			log.Printf("Panic: %v\n%s\n", err, buf)
			t.Rw.WriteHeader(http.StatusInternalServerError)
		} else {
			if err := tx.Commit(); err != nil {
				log.Println("COMMIT failed:", err)
			}
		}
		t.Tx = nil
	}()

	_, err = tx.Exec("SET TRANSACTION ISOLATION LEVEL SERIALIZABLE")
	if err != nil {
		log.Println("SET TRANSACTION failed:", err)
		t.Rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	t.Rw.Header().Set("Cache-Control", "no-cache, no-store,  must-revalidate")
	t.Rw.Header().Set("Pragma", "no-cache")
	h.Handler.Serve(t)
}
