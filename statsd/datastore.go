package main

type Record struct {
	Ts    int64
	Value float64
}

type Datastore interface {
	Init() error
	Insert(name string, r Record) error
	Query(name string, form, until int64) ([]Record, error)
	LatestBefore(name string, ts int64) (Record, error)
}
