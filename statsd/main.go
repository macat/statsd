package main

import (
	"log"
	"os"
	"os/signal"
	"flag"
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

	srv := &Server{Ds: ds}
	srv.Start()

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

	C := make(chan os.Signal)
	signal.Notify(C, os.Interrupt)
	<-C

	log.Println("Received SIGTERM, stopping...")

	if ui != nil {
		ui.Stop()
		log.Println("UDP injector stopped")
	}

	if ti != nil {
		ti.Stop()
		log.Println("TCP injector stopped")
	}

	srv.Stop()
	log.Println("Server stopped")

	if api != nil {
		api.Stop()
		log.Println("Query API stopped")
	}
}
