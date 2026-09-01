package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/shogo82148/server-starter/listener"
)

func main() {
	l, err := listener.ListenAll()
	if err != nil {
		log.Fatal(err)
	}

	addrFile := os.Args[1]
	addr := l[0].Addr().String()
	if err := os.WriteFile(addrFile, []byte(addr), 0644); err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		}),
	}
	go func() {
		if err := server.Serve(l[0]); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	if err := server.Shutdown(context.Background()); err != nil {
		log.Fatal(err)
	}
}
