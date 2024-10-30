package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	server := NewServer()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("Shutting down server...")
		server.listener.Close()
		os.Exit(0)
	}()

	server.Start()
}
