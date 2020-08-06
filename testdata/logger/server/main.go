package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Fprintln(os.Stderr, "server: started")

	// it is dummy server. do nothing.

	// wait for receiving SIGTERM
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	sig := <-ch
	fmt.Fprintln(os.Stderr, "server: received "+sig.String())
}
