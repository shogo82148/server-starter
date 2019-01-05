package main

import (
	"context"
	"io"
	"log"
	"net"

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
	var rerr error
	var buf [1024 * 1024]byte
	for rerr != io.EOF {
		var n int
		n, rerr = conn.Read(buf[:])
		if rerr == io.EOF && n == 0 {
			break
		}
		if rerr != nil && rerr != io.EOF {
			log.Printf("read error: %s", rerr)
		}
		if _, err := conn.Write(buf[:n]); err != nil {
			log.Printf("write error: %s", err)
			return
		}
	}
}
