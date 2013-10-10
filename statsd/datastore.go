package main

type Record struct {
	Ts    int64
	Value float64
}

type Datastore interface {
	Open() error
	Close() error
	Insert(name string, r Record) error
	Query(name string, form, until int64) ([]Record, error)
	LatestBefore(name string, ts int64) (Record, error)
	ListNames(pattern string) ([]string, error)
}

const ErrNoData = Error("No data")
