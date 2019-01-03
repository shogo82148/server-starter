package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
	go watchSignal()

	gen, err := strconv.Atoi(os.Getenv("SERVER_STARTER_GENERATION"))
	if err != nil {
		log.Fatal(err)
	}
	if gen == 1 || (gen >= 3 && gen < 5) {
		// emulate startup failure
		os.Exit(1)
	}

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
	conn.Write([]byte(os.Getenv("SERVER_STARTER_GENERATION")))
	conn.Close()
}

func watchSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	os.Exit(0)
}
