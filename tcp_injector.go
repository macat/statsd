package main

import (
	"log"
	"net"
	"sync"
)

const TcpMsgMaxSize = 128

type TCPInjector struct {
	Addr     string
	Server   *Server
	mu, cmu  sync.Mutex
	listener *net.TCPListener
	conns    []*net.TCPConn
	running  bool
	wg       sync.WaitGroup
}

func (ti *TCPInjector) Start() error {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	if ti.running {
		return Error("Injector already running")
	}

	addr, err := net.ResolveTCPAddr("tcp", ti.Addr)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	ti.listener, ti.running = listener, true

	go ti.run()
	return nil
}

func (ti *TCPInjector) Stop() error {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	if !ti.running {
		return Error("Injector not running")
	}

	ti.running = false
	ti.listener.Close()
	ti.wg.Wait()
	return nil
}

func (ti *TCPInjector) run() {
	for {
		conn, err := ti.listener.AcceptTCP()
		if err != nil {
			log.Println("TCPListener.Accept:", err)
			break
		}
		ti.cmu.Lock()
		ti.conns = append(ti.conns, conn)
		i := len(ti.conns) - 1
		ti.cmu.Unlock()
		ti.wg.Add(1)
		go ti.serve(conn, i)
	}
	ti.cmu.Lock()
	for _, conn := range ti.conns {
		conn.Close()
	}
	ti.cmu.Unlock()
}

func (ti *TCPInjector) serve(conn *net.TCPConn, i int) {
	buff, bsize, drop := make([]byte, TcpMsgMaxSize), 0, false
	for {
		n, err := conn.Read(buff[bsize:])
		if n > 0 {
			bsize += n
			for i := 0; i < bsize; i++ {
				if buff[i] == '\n' {
					if !drop {
						ti.Server.InjectBytes(buff[0:i])
					}
					bsize = copy(buff[0:], buff[i+1:bsize])
					i, drop = 0, false
				}
			}
			if bsize == len(buff) {
				drop, bsize = true, 0
			}
		}
		if err != nil {
			log.Println("TCPConn.Read:", err)
			break
		}
	}
	ti.cmu.Lock()
	ti.conns[i] = ti.conns[len(ti.conns)-1]
	ti.conns = ti.conns[0 : len(ti.conns)-1]
	ti.cmu.Unlock()
	ti.wg.Done()
}
