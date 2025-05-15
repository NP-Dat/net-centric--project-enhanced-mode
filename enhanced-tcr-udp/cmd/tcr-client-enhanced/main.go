package main

import (
	"fmt"
	"log"

	"enhanced-tcr-udp/internal/client"
	// "github.com/nsf/termbox-go" // Keep for later UI integration
)

func main() {
	// Termbox initialization will be done later when UI is more developed.
	/*
		err := termbox.Init()
		if err != nil {
			panic(err)
		}
		defer termbox.Close()
	*/

	log.Println("Starting Enhanced TCR Client...")

	gameClient := client.NewClient()
	defer gameClient.CloseConnections() // Ensure connections are closed on exit

	player, err := gameClient.Authenticate()
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
		return
	}

	fmt.Printf("Welcome, %s (Level %d, EXP %d)!\n", player.Username, player.Level, player.EXP)
	fmt.Println("Login successful. Requesting matchmaking...")

	matchInfo, err := gameClient.RequestMatchmaking()
	if err != nil {
		log.Fatalf("Matchmaking failed: %v", err)
		return
	}

	fmt.Printf("Match found! Game ID: %s, Opponent: %s, UDP Port: %d\n",
		matchInfo.GameID, matchInfo.Opponent.Username, matchInfo.UDPPort)
	fmt.Println("Client is ready for UDP gameplay (not implemented in Sprint 1).")

	// Placeholder for further client logic (matchmaking, game loop, termbox UI)
	// For now, we can just wait for a key press to exit to keep it simple.
	fmt.Println("Press Enter to exit.")
	fmt.Scanln()
}

/*
func sendUDPPing() {
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:8081")
	if err != nil {
		fmt.Println("Error resolving UDP address:", err.Error())
		os.Exit(1)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error dialing UDP:", err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("Sending UDP message 'hello udp server' to localhost:8081")
	_, err = conn.Write([]byte("hello udp server"))
	if err != nil {
		fmt.Println("Error sending UDP message:", err.Error())
		return
	}

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf) // We don't need the remote address here as DialUDP connects us
	if err != nil {
		fmt.Println("Error reading UDP response:", err.Error())
		return
	}

	udpResponse := string(buf[:n])
	fmt.Printf("Received UDP response: %s\n", udpResponse)
}
*/
