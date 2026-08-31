// Testing when a logger terminates unexpectedly

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		f, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintln(f, os.Getpid())
		f.Close() //nolint:errcheck // The process ID is only a test marker.
	}
	go logging()

	time.Sleep(5 * time.Second)
}

func logging() {
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		log.Fatal(err)
	}
}
