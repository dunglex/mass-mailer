package main

import (
	"log"

	"xdung24/mass-mailer/internal/mailer"
)

func main() {
	if err := mailer.Run(); err != nil {
		log.Fatal(err)
	}
}
