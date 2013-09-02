package main

import (
	"log"
	"net"
	"sync"
)

const MsgMaxSize = 512

type UDPInjector struct {
	Addr    string
	Server  *Server
	mu      sync.Mutex
	conn    *net.UDPConn
	running bool
	wg      sync.WaitGroup
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

	go ui.run()
	return nil
}

func (ui *UDPInjector) Stop() error {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if !ui.running {
		return Error("Injector not running")
	}

	ui.running = false
	ui.conn.Close()
	ui.wg.Wait()
	return nil
}

func (ui *UDPInjector) run() {
	for {
		if !ui.running {
			return
		}
		buff := make([]byte, MsgMaxSize)
		n, err := ui.conn.Read(buff)
		if err != nil {
			log.Println("UDPConn.Read:", err)
			continue
		}
		go func() {
			ui.wg.Add(1)
			ui.Server.InjectBytes(buff[0:n])
			ui.wg.Done()
		}()
	}
}
