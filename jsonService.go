package main

import (
	"admin/access"
	"admin/uuid"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// TODO: permissions

// Model

type JsonService struct {
	Tx     *sql.Tx
	Id     string
	Url    string
	Config interface{}
}

func JsonServices(tx *sql.Tx) ([]*JsonService, error) {
	rows, err := tx.Query(`
		SELECT id, url, config
		FROM json_services`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	svcs := make([]*JsonService, 0)
	for rows.Next() {
		var (
			id, url string
			cfgSl   []byte
			cfg     interface{}
		)
		err := rows.Scan(&id, &url, &cfgSl)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(cfgSl, &cfg)
		if err != nil {
			return nil, err
		}
		svcs = append(svcs, &JsonService{
			Tx:     tx,
			Id:     id,
			Url:    url,
			Config: cfg,
		})
	}

	return svcs, nil
}

func (s *JsonService) Create() error {
	id, err := uuid.New4()
	if err != nil {
		return err
	}
	s.Id = id

	cfg, err := json.Marshal(s.Config)
	if err != nil {
		return err
	}

	_, err = s.Tx.Exec(`
		INSERT INTO json_services (id, url, config)
		VALUES ($1, $2, $3)`,
		s.Id, s.Url, cfg)

	return err
}

func (s *JsonService) Load() error {
	row := s.Tx.QueryRow(`
		SELECT id, url, config
		FROM json_services
		WHERE id = $1`, s.Id)

	var (
		id, url string
		cfgSl   []byte
		cfg     interface{}
	)
	err := row.Scan(&id, &url, &cfgSl)
	if err != nil {
		return err
	}

	err = json.Unmarshal(cfgSl, &cfg)
	if err != nil {
		return err
	}

	s.Id, s.Url, s.Config = id, url, cfg
	return nil
}

func (s *JsonService) Update() error {
	cfg, err := json.Marshal(s.Config)
	if err != nil {
		return err
	}

	_, err = s.Tx.Exec(`
		UPDATE json_services
		SET url = $1, config = $2
		WHERE id = $3`,
		s.Url, cfg, s.Id)

	return err
}

func (s *JsonService) Delete() error {
	_, err := s.Tx.Exec(`
		DELETE FROM json_services
		WHERE id = $1`,
		s.Id)

	return err
}

// Routing

var JsonServiceRouter = &Transactional{PrefixRouter{
	"/": MethodRouter{
		"GET":  HandlerFunc(getJsonServices),
		"POST": HandlerFunc(postJsonService),
	},
	"*uuid": PrefixRouter{
		"/": MethodRouter{
			"GET":    HandlerFunc(getJsonService),
			"PUT":    HandlerFunc(putJsonService),
			"DELETE": HandlerFunc(deleteJsonService),
		},
		"/data": PrefixRouter{
			"/proxy": HandlerFunc(proxyJsonServiceData),
		},
	},
}}

// Controllers

func getJsonServices(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "GET", "services_json", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	svcs, err := JsonServices(t.Tx)
	if err != nil {
		panic(err)
	}

	response := make([]interface{}, 0)
	for _, svc := range svcs {
		response = append(response, map[string]interface{}{
			"id":     svc.Id,
			"url":    svc.Url,
			"config": svc.Config,
		})
	}
	t.SendJson(response)
}

func postJsonService(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "POST", "services_json", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.SendError("Invalid JSON")
		return
	}

	url, ok := data["url"].(string)
	if !ok {
		t.SendError("Invalid JSON")
		return
	}

	svc := &JsonService{
		Tx:     t.Tx,
		Url:    url,
		Config: data["config"],
	}
	if err := svc.Create(); err != nil {
		panic(err)
	}

	t.SendJson(map[string]string{"id": svc.Id})
}

func getJsonService(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "GET", "services_json", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	svc := &JsonService{Tx: t.Tx, Id: t.UUID}
	if err := svc.Load(); err == sql.ErrNoRows {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}
	t.SendJson(map[string]interface{}{
		"id":     svc.Id,
		"url":    svc.Url,
		"config": svc.Config,
	})
}

func putJsonService(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "PUT", "services_json", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	svc := &JsonService{Tx: t.Tx, Id: t.UUID}
	if err := svc.Load(); err == sql.ErrNoRows {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}

	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.SendError("Invalid JSON")
		return
	}

	url, ok := data["url"].(string)
	if !ok {
		t.SendError("Invalid JSON")
		return
	}

	svc.Url, svc.Config = url, data["config"]
	if err := svc.Update(); err != nil {
		panic(err)
	}
}

func deleteJsonService(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "DELETE", "services_json", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	svc := &JsonService{Tx: t.Tx, Id: t.UUID}
	if err := svc.Load(); err == sql.ErrNoRows {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}

	if err := svc.Delete(); err != nil {
		panic(err)
	}
}

func proxyJsonServiceData(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, t.Rq.Method, "services_json_data", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	svc := &JsonService{Tx: t.Tx, Id: t.UUID}
	if err := svc.Load(); err == sql.ErrNoRows {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}

	URL, err := url.Parse(svc.Url)
	if err != nil {
		panic(err)
	}

	values := URL.Query()
	for k, v := range t.Rq.URL.Query() {
		values.Del(k)
		for _, vv := range v {
			values.Add(k, vv)
		}
	}
	URL.RawQuery = values.Encode()

	req := &http.Request{
		Method:        t.Rq.Method,
		URL:           URL,
		Body:          t.Rq.Body,
		Close:         true,
		ContentLength: t.Rq.ContentLength,
		Header:        make(http.Header),
	}
	req.Header.Add("Authorization", t.Rq.Header.Get("Authorization"))
	req.Header.Add("Content-Type", t.Rq.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	t.Rw.WriteHeader(resp.StatusCode)
	t.Rw.Header().Add("Content-Type", resp.Header.Get("Content-Type"))
	t.Rw.Header().Add("Location", resp.Header.Get("Location"))

	io.Copy(t.Rw, resp.Body)
}
