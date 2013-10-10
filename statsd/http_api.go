package main

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.net/websocket"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type HttpApi struct {
	Addr     string
	Server   *Server
	mu       sync.Mutex
	running  bool
	listener *net.TCPListener
	httpSrv  http.Server
	wg       sync.WaitGroup
}

func (ha *HttpApi) Start() error {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	if ha.running {
		return Error("API already running")
	}

	addr, err := net.ResolveTCPAddr("tcp", ha.Addr)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	ha.running = true
	ha.listener = listener
	ha.httpSrv.Handler = http.HandlerFunc(ha.serveHTTP)
	go func() {
		err := ha.httpSrv.Serve(listener)
		if err != nil {
			log.Println("http.Server.Serve:", err)
		}
	}()
	return nil
}

func (ha *HttpApi) Stop() error {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	if !ha.running {
		return Error("API not running")
	}

	ha.running = false
	ha.listener.Close()
	ha.wg.Wait()
	return nil
}

func (ha *HttpApi) serveHTTP(rw http.ResponseWriter, rq *http.Request) {
	ha.wg.Add(1)
	defer ha.wg.Done()

	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic:", err)
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte("Internal Server Error"))
		}
	}()

	rw.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	rw.Header().Set("Pragma", "no-cache")
	rw.Header().Set("Access-Control-Allow-Origin", "*")

	typ := rq.URL.Query().Get("type")
	watch := strings.ToLower(rq.Header.Get("Upgrade")) == "websocket"

	switch {
	case typ == "live" && watch:
		ha.serveLiveWatch(rw, rq)
	case typ == "live" && !watch:
		ha.serveLiveLog(rw, rq)
	case typ == "archive" && watch:
		ha.serveArchiveWatch(rw, rq)
	case typ == "archive" && !watch:
		ha.serveArchiveLog(rw, rq)
	case typ == "list":
		ha.serveList(rw, rq)
	default:
		ha.sendError(Error("Invalid type"), rw)
	}
}

func (ha *HttpApi) serveLiveWatch(rw http.ResponseWriter, rq *http.Request) {
	m, chs := ha.metricAndChannels(rq)
	watcher, err := ha.Server.LiveWatch(m, chs)
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	ha.serveWs(watcher, 1, rw, rq)
}

func (ha *HttpApi) serveLiveLog(rw http.ResponseWriter, rq *http.Request) {
	m, chs := ha.metricAndChannels(rq)
	data, ts, err := ha.Server.LiveLog(m, chs)
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	ha.serveData(ts, data, 1, rw)
}

func (ha *HttpApi) serveArchiveWatch(rw http.ResponseWriter, rq *http.Request) {
	m, chs := ha.metricAndChannels(rq)
	og, err := ha.params(rq, "offset", "granularity")
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	watcher, err := ha.Server.Watch(m, chs, og[0], og[1])
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	ha.serveWs(watcher, og[1], rw, rq)
}

func (ha *HttpApi) serveArchiveLog(rw http.ResponseWriter, rq *http.Request) {
	m, chs := ha.metricAndChannels(rq)
	flg, err := ha.params(rq, "from", "length", "granularity")
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	data, err := ha.Server.Log(m, chs, flg[0], flg[1], flg[2])
	if err != nil {
		ha.sendError(err, rw)
	}
	ha.serveData(flg[0], data, flg[2], rw)
}

func (ha *HttpApi) serveList(rw http.ResponseWriter, rq *http.Request) {
	names, err := ha.Server.Ds.ListNames(rq.URL.Query().Get("pattern"))
	if err != nil {
		ha.sendError(err, rw)
		return
	}
	for _, name := range names {
		rw.Write([]byte(name))
		rw.Write([]byte("\n"))
	}
}

func (ha *HttpApi) sendError(err error, rw http.ResponseWriter) {
	if _, ok := err.(Error); ok {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte(err.Error()))
	} else {
		log.Println(err)
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("Internal Server Error"))
	}
}

func (ha *HttpApi) metricAndChannels(rq *http.Request) (string, []string) {
	q := rq.URL.Query()
	return q.Get("metric"), strings.Split(q.Get("channels"), ",")
}

func (ha *HttpApi) params(rq *http.Request, vars ...string) ([]int64, error) {
	q := rq.URL.Query()
	r := make([]int64, len(vars))
	for i, n := range vars {
		v, err := strconv.ParseInt(q.Get(n), 10, 64)
		if err != nil {
			return nil, Error("Not an integer: " + n)
		}
		r[i] = v
	}
	return r, nil
}

func (ha *HttpApi) serveWs(w *Watcher, n int64, rw http.ResponseWriter, rq *http.Request) {
	websocket.Handler(func(conn *websocket.Conn) {
		buf := new(bytes.Buffer)
		for values := range w.C {
			if err := ha.writeRecord(w.Ts, values, buf); err != nil {
				w.Close()
				break
			}
			if _, err := buf.WriteTo(conn); err != nil {
				w.Close()
				break
			}
			buf.Reset()
			w.Ts += n
		}
	}).ServeHTTP(rw, rq)
}

type byteStringWriter interface {
	WriteString(string) (int, error)
	WriteByte(byte) error
}

func (ha *HttpApi) serveData(ts int64, data [][]float64, n int64, rw http.ResponseWriter) {
	buf := bufio.NewWriter(rw)
	for _, values := range data {
		ha.writeRecord(ts, values, buf)
		buf.WriteByte('\n')
		ts += n
	}
	buf.Flush()
}

func (ha *HttpApi) writeRecord(ts int64, values []float64, w byteStringWriter) error {
	w.WriteString(strconv.FormatInt(ts, 10))
	for _, val := range values {
		if err := w.WriteByte(','); err != nil {
			return err
		}
		_, err := w.WriteString(strconv.FormatFloat(val, 'e', -1, 64))
		if err != nil {
			return err
		}
	}
	return nil
}
