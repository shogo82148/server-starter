package main

import (
	"log"
	"os"

	starter "github.com/shogo82148/server-starter"
)

func main() {
	s, err := starter.ParseArgs(os.Args)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	s.Run()
}
