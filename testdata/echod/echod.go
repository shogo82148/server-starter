package main

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shogo82148/server-starter/listener"
)

var pid []byte

func main() {
	go watchSignal()

	pid = []byte(strconv.Itoa(os.Getpid()))
	ll, err := listener.Ports()
	if err != nil {
		log.Fatal(err)
	}
	l, err := ll.ListenAll(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := l[0].Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	var rerr error
	var rbuf [1024 * 1024]byte
	var wbuf [1024 * 1024]byte
	for rerr != io.EOF {
		var n int
		n, rerr = conn.Read(rbuf[:])
		if rerr != nil && rerr != io.EOF {
			log.Printf("read error: %s", rerr)
			return
		}

		copy(wbuf[:len(pid)], pid)
		wbuf[len(pid)] = ':'
		copy(wbuf[len(pid)+1:], rbuf[:n])
		if _, err := conn.Write(wbuf[:len(pid)+n+1]); err != nil {
			log.Printf("write error: %s", err)
			return
		}
	}
}

func watchSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGUSR1)
	for range c {
		go func() {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}()
	}
}
