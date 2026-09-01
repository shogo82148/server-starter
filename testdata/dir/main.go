package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
	go watchSignal()

	l, err := listener.ListenAll()
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
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	conn.Write([]byte(dir))
	conn.Close()
}

func watchSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGUSR1)
	for range c {
		os.Exit(0)
	}
}
