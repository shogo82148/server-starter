package main

import (
	"bytes"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
	go watchSignal()

	conn, err := listener.ListenPacketAll()
	if err != nil {
		log.Fatal(err)
	}
	defer conn[0].Close()

	for {
		var buf [1024 * 1024]byte
		n, addr, err := conn[0].ReadFrom(buf[:])
		if err != nil {
			log.Fatal(err)
		}
		if _, err := conn[0].WriteTo(bytes.ToUpper(buf[:n]), addr); err != nil {
			log.Fatal(err)
		}
	}
}

func watchSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGUSR1)
	<-c
	os.Exit(0)
}
