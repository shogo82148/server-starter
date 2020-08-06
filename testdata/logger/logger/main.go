// very simple logger program

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: logger filename")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "logger: started")

	// outputs logs into the file
	f, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	go logging(f)

	// wait for receiving SIGTERM
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	sig := <-ch
	fmt.Fprintln(os.Stderr, "logger: received "+sig.String())
}

func logging(f *os.File) {
	defer f.Close()
	if _, err := io.Copy(f, os.Stdin); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stderr, "logger: received EOF")
}
