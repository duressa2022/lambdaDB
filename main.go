package main

import (
	"log"
	"time"

	"github.com/duressa2022/lambdaDB/internal/network"
)

func main() {
	cfg := &network.Config{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
		MaxConns:     10000,
		Logger:       network.Logger{},
	}

	server, err := network.NewServer(cfg)
	if err != nil {
		log.Fatalf("Server initialization failed: %v", err)
	}

	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server exited with error: %v", err)
		}
	}()

	<-server.DoneChan
	log.Println("Server shutdown complete")
}
