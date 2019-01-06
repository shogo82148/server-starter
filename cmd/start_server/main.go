package main

import (
	"log"
	"os"

	starter "github.com/shogo82148/server-starter"
)

func main() {
	log.SetFlags(0) // same log format as https://metacpan.org/pod/Server::Starter
	s, err := starter.ParseArgs(os.Args)
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
