package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	r := router.New()

	fmt.Printf("OpenSynapse control plane listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
