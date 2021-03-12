// Testing when a logger terminates unexpectedly

package main

import (
	"io"
	"log"
	"os"
	"time"
)

func main() {
	go logging()

	time.Sleep(5 * time.Second)
}

func logging() {
	if _, err := io.Copy(os.Discard, os.Stdin); err != nil {
		log.Fatal(err)
	}
}
