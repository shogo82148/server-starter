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

	gen, ok := listener.Generation()
	if !ok {
		log.Fatal("SERVER_STARTER_GENERATION is not set")
	}

	if gen == 1 || (gen >= 3 && gen < 5) {
		// emulate startup failure
		os.Exit(1)
	}

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
	conn.Write([]byte(os.Getenv(listener.GenerationEnvName)))
	conn.Close()
}

func watchSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	os.Exit(0)
}
