package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
)

func main() {
	var dataDir, apiAddr, udpAddr, tcpAddr string
	var nosync bool

	flag.StringVar(&dataDir, "data", "", "     Data directory")
	flag.StringVar(&apiAddr, "api", ":5999", " HTTP query API address")
	flag.StringVar(&udpAddr, "udp", ":6000", " UDP input address")
	flag.StringVar(&tcpAddr, "tcp", ":6000", " TCP input address")
	flag.BoolVar(&nosync, "nosync", false, "Don't call sync() after every disk write")
	flag.Parse()

	if len(dataDir) == 0 {
		os.Stderr.Write([]byte("No data directory specified\n"))
		return
	}

	log.Println("StatsD starting...")

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)

	ds := &FsDatastore{Dir: dataDir, NoSync: nosync}
	if err := ds.Open(); err != nil {
		log.Println("FsDatastore.Open:", err)
		return
	}
	defer func() {
		ds.Close()
		log.Println("Datastore closed")
	}()
	log.Println("Datastore opened")

	lld := new(LiveLogData)
	lldfn := dataDir + string(os.PathSeparator) + "live_log"
	if err := lld.ReadFrom(lldfn); err != nil {
		log.Println("Failed to load the live log:", err)
		lld = nil
	} else {
		log.Println("Live log loaded")
	}

	wcsfn := dataDir + string(os.PathSeparator) + "wildcards"
	wcs, err := loadWildcards(wcsfn)
	if err == nil {
		log.Println("Wildcards loaded")
	} else {
		log.Println("Failed to load wildcards:", err)
	}

	srv := &Server{Ds: ds, AutoWc: true}
	log.Println("Server started")
	srv.Start(lld, wcs)
	lld = nil

	var api *HttpApi
	if len(apiAddr) > 0 {
		api = &HttpApi{Addr: apiAddr, Server: srv}
		if err := api.Start(); err != nil {
			log.Println("HttpApi.Start:", err)
		}
		log.Println("Query API listening on TCP address", api.Addr)
	}

	var ui *UDPInjector
	if len(udpAddr) > 0 {
		ui = &UDPInjector{Addr: udpAddr, Server: srv}
		if err := ui.Start(); err != nil {
			log.Println("UDPInjector.Start:", err)
			return
		}
		log.Println("Listening on UDP address", ui.Addr)
	}

	var ti *TCPInjector
	if len(tcpAddr) > 0 {
		ti = &TCPInjector{Addr: tcpAddr, Server: srv}
		if err := ti.Start(); err != nil {
			log.Println("TCPInjector.Start:", err)
			return
		}
		log.Println("Listening on TCP address", ti.Addr)
	}

	<-sigint
	log.Println("Received SIGTERM, stopping...")

	lld, wcs, _ = srv.Stop()
	log.Println("Server stopped")

	if ui != nil {
		ui.Stop()
		log.Println("UDP injector stopped")
	}

	if ti != nil {
		ti.Stop()
		log.Println("TCP injector stopped")
	}

	if err := lld.WriteTo(lldfn); err == nil {
		log.Println("Live log saved")
	} else {
		log.Println("Failed to save the live log:", err)
		if err := os.Remove(lldfn); err != nil {
			log.Println(err)
		}
	}

	if err := saveWildcards(wcsfn, wcs); err == nil {
		log.Println("Wildcards saved")
	} else {
		log.Println("Failed to save wildcards:", err)
		if err := os.Remove(wcsfn); err != nil {
			log.Println(err)
		}
	}

	if api != nil {
		api.Stop()
		log.Println("Query API stopped")
	}
}

func saveWildcards(fn string, wcs []string) error {
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, name := range wcs {
		if _, err := f.Write([]byte(name)); err != nil {
			return err
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			return err
		}
	}

	return nil
}

func loadWildcards(fn string) ([]string, error) {
	buff, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	r := make([]string, 0)
	for _, line := range bytes.Split(buff, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		r = append(r, string(line))
	}

	return r, nil
}
