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
	mux := http.NewServeMux()
	mux.HandleFunc("/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		value, ok := os.LookupEnv(key)
		if ok {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(value))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found!"))
		}
	})
	server := &http.Server{
		Handler: mux,
	}

	l, err := listener.ListenAll(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		<-c
		if err := server.Shutdown(context.Background()); err != nil {
			log.Fatal(err)
		}
	}()
	if err := server.Serve(l[0]); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
