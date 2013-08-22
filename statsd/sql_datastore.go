package main

import (
	"database/sql"
	"sync"
)

type sqlDatastore struct {
	db  *sql.DB
	ids map[string]int64
	sync.Mutex
}

func NewSqlDatastore(db *sql.DB) Datastore {
	return &sqlDatastore{
		db:  db,
		ids: make(map[string]int64),
	}
}

func (ds *sqlDatastore) Init() error {
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

func (ds *sqlDatastore) Insert(name string, r Record) error {
	id, err := ds.getMetricId(name, true)
	if err != nil {
		return err
	}

	_, err = ds.db.Exec(`
		INSERT INTO "stats" ("metric_id", "timestamp", "value")
		VALUES ($1, $2, $3)`,
		id, r.Ts, r.Value)
	if err != nil {
		return err
	}

	return nil
}

func (ds *sqlDatastore) Query(name string, from, until int64) ([]Record, error) {
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
