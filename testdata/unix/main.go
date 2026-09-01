package main

import (
	"io"
	"log"
	"net"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
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
