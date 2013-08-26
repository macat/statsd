package main

import (
	"log"
	"net"
	"sync"
)

const MsgMaxSize = 1024

type UDPInjector struct {
	Addr     string
	Server   Server
	mu       sync.Mutex
	conn     *net.UDPConn
	running  bool
	ch1, ch2 chan int
}

func (ui *UDPInjector) Start() error {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if ui.running {
		return Error("Injector already running")
	}

	addr, err := net.ResolveUDPAddr("udp", ui.Addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	ui.conn, ui.running = conn, true
	ui.ch1, ui.ch2 = make(chan int, 1), make(chan int)

	go ui.run(ui.Server)
	return nil
}

func (ui *UDPInjector) Stop() error {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if !ui.running {
		return Error("Injector already stopped")
	}

	ui.ch1 <- 1
	ui.conn.Close()
	<-ui.ch2
	return nil
}

func (ui *UDPInjector) run(srv Server) {
loop:
	for {
		select {
		case <-ui.ch1:
			break loop
		default:
			buff := make([]byte, MsgMaxSize)
			n, err := ui.conn.Read(buff)
			if err != nil {
				log.Println("UDPConn.Read:", err)
				continue
			}
			go srv.InjectBytes(buff[0:n])
		}
	}
	ui.running = false
	ui.ch2 <- 1
}
