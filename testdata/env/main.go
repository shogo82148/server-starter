package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
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
	env := os.Getenv("FOO")
	conn.Write([]byte(env))
}
