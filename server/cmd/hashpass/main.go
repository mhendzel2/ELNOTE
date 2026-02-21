package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/mjhen/elnote/server/internal/auth"
)

func main() {
	password := flag.String("password", "", "password to hash with Argon2id")
	flag.Parse()

	if *password == "" {
		log.Fatal("password is required")
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	fmt.Println(hash)
}
