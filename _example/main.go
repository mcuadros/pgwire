package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/mcuadros/pgwire"
)

func main() {

	s := pgwire.MakeServer(
		&pgwire.Config{
			Insecure: true,
		},
		pgwire.NewExecutor(),
	)

	// Listen for incoming connections.
	l, err := net.Listen("tcp", ":26257")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer l.Close()

	for {
		// Listen for an incoming connection.
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		// Handle  in a new goroutine.
		go s.ServeConn(context.TODO(), conn)
	}
}
