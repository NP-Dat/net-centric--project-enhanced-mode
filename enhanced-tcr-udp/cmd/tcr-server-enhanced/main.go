package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	fmt.Println("Starting TCP server...")
	listener, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Println("Server listening on localhost:8080 for TCP")

	go startUDPServer() // Start UDP server in a goroutine

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting TCP: ", err.Error())
			continue
		}
		go handleTCPConnection(conn) // Changed to handleTCPConnection
	}
}

func handleTCPConnection(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error reading:", err.Error())
		return
	}

	clientAddr := conn.RemoteAddr().String()
	receivedMsg := string(buf[:n])
	fmt.Printf("Received from %s: %s\n", clientAddr, receivedMsg)

	if receivedMsg == "ping" {
		_, err = conn.Write([]byte("pong"))
		if err != nil {
			fmt.Println("Error writing:", err.Error())
			return
		}
		fmt.Printf("Sent 'pong' to %s\n", clientAddr)
	} else {
		_, err = conn.Write([]byte("Expected 'ping'"))
		if err != nil {
			fmt.Println("Error writing:", err.Error())
			return
		}
		fmt.Printf("Sent 'Expected \"ping\"' to %s\n", clientAddr)
	}
}

func startUDPServer() {
	udpAddr, err := net.ResolveUDPAddr("udp", "localhost:8081")
	if err != nil {
		fmt.Println("Error resolving UDP address:", err.Error())
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Error listening on UDP:", err.Error())
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("UDP server listening on localhost:8081")

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Error reading from UDP:", err.Error())
			continue
		}
		receivedMsg := string(buf[:n])
		fmt.Printf("UDP: Received from %s: %s\n", remoteAddr, receivedMsg)

		// Echo back the message
		_, err = conn.WriteToUDP([]byte("UDP Server echoes: "+receivedMsg), remoteAddr)
		if err != nil {
			fmt.Println("Error writing to UDP:", err.Error())
		}
	}
}
