package main

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
	"sync"
)

type sqlDatastore struct {
	db       *sql.DB
	ids      map[string]int64
	connSema chan int
	sync.Mutex
	insertCond  *sync.Cond
	insertQueue []sqlDsRecord
}

type sqlDsRecord struct {
	name  string
	ts    int64
	value float64
}

func NewSqlDatastore(db *sql.DB, maxConn int) Datastore {
	ds := &sqlDatastore{
		db:         db,
		ids:        make(map[string]int64),
		connSema:   make(chan int, maxConn),
		insertCond: sync.NewCond(&sync.Mutex{}),
	}
	go ds.doInserts()
	return ds
}

func (ds *sqlDatastore) Open() error {
	rows, err := ds.db.Query(`SELECT "id", "name" FROM "metrics"`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		id, name := int64(0), ""
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		ds.ids[name] = id
	}

	return nil
}

func (ds *sqlDatastore) Close() error {
	panic("sqlDatastore cannot be stopped")
}

func (ds *sqlDatastore) Insert(name string, r Record) error {
	ds.insertCond.L.Lock()
	defer ds.insertCond.L.Unlock()

	ds.insertQueue = append(ds.insertQueue, sqlDsRecord{name, r.Ts, r.Value})
	ds.insertCond.Signal()
	return nil
}

func (ds *sqlDatastore) doInserts() {
	for {
		ds.insertCond.L.Lock()
		if len(ds.insertQueue) == 0 {
			ds.insertCond.Wait()
		}
		data := ds.insertQueue
		ds.insertQueue = make([]sqlDsRecord, 0, 2*len(data))
		ds.insertCond.L.Unlock()

		values, params := make([]interface{}, 0, 3*len(data)), []string{}
		for _, r := range data {
			id, err := ds.getMetricId(r.name, true)
			if err != nil {
				log.Println("doInserts:", err)
				continue
			}

			params = append(params,
				"($"+strconv.Itoa(len(values)+1)+
					", $"+strconv.Itoa(len(values)+2)+
					", $"+strconv.Itoa(len(values)+3)+")")
			values = append(values, id, r.ts, r.value)
		}

		_, err := ds.db.Query(`
			INSERT INTO "stats" ("metric_id", "timestamp", "value")
			VALUES `+strings.Join(params, ", "),
			values...)
		if err != nil {
			log.Println("doInserts:", err)
		}
	}
}

func (ds *sqlDatastore) Query(name string, from, until int64) ([]Record, error) {
	ds.connSema <- 1
	defer func() { <-ds.connSema }()

	id, err := ds.getMetricId(name, false)
	if err != nil {
		return nil, err
	}

	if id == -1 {
		return []Record{}, nil
	}

	rows, err := ds.db.Query(`
		SELECT "timestamp", "value"
		FROM "stats"
		WHERE "metric_id" = $1 AND "timestamp" BETWEEN $2 AND $3
		ORDER BY "timestamp" ASC`,
		id, from, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	capacity := (until - from) / 60
	if capacity < 0 {
		capacity = 0
	}

	result := make([]Record, 0, capacity)
	for rows.Next() {
		ts, value := int64(0), float64(0)
		if err := rows.Scan(&ts, &value); err != nil {
			return nil, err
		}
		result = append(result, Record{Ts: ts, Value: value})
	}

	return result, nil
}

func (ds *sqlDatastore) LatestBefore(name string, ts int64) (Record, error) {
	ds.connSema <- 1
	defer func() { <-ds.connSema }()

	var rec Record

	id, err := ds.getMetricId(name, false)
	if err != nil {
		return rec, err
	}

	if id == -1 {
		return Record{}, nil
	}

	row := ds.db.QueryRow(`
		SELECT "timestamp", "value"
		FROM "stats"
		WHERE "metric_id" = $1 AND "timestamp" <= $2
		ORDER BY "timestamp" DESC
		LIMIT 1`,
		id, ts)

	err = row.Scan(&rec.Ts, &rec.Value)

	if err == sql.ErrNoRows {
		err = ErrNoData
	}

	return rec, err

}

func (ds *sqlDatastore) getMetricId(name string, create bool) (int64, error) {
	ds.connSema <- 1
	defer func() { <-ds.connSema }()

	ds.Lock()
	defer ds.Unlock()

	id, ok := ds.ids[name]
	if !ok && create {
		_, err := ds.db.Exec(`INSERT INTO "metrics" ("name") VALUES ($1)`,
			name)
		if err != nil {
			return 0, err
		}
		row := ds.db.QueryRow(`SELECT "id" FROM "metrics" WHERE "name" = $1`,
			name)
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
		ds.ids[name] = id
	} else if !ok {
		id = -1
	}

	return id, nil
}
