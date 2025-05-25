package main

import (
	"fmt"
	"log"

	// "os"

	"enhanced-tcr-udp/internal/client"
	"enhanced-tcr-udp/internal/models"  // For PlayerAccount type hint
	"enhanced-tcr-udp/internal/network" // For MatchFoundResponse type hint

	"github.com/nsf/termbox-go"
)

func main() {
	log.Println("Starting Enhanced TCR Client with Termbox UI...")

	ui := client.NewTermboxUI()
	err := ui.Init()
	if err != nil {
		log.Fatalf("Failed to initialize termbox: %v", err)
		// Fallback to console if termbox fails? For now, just exit.
		return
	}
	defer ui.Close()

	ui.ClearScreen()
	ui.DisplayStaticText(1, 1, "Welcome to Enhanced TCR Client!", termbox.ColorCyan, termbox.ColorBlack)

	gameClient := client.NewClient(ui) // Pass UI to client
	// defer gameClient.CloseConnections() // Ensure connections are closed on exit -- We will call this manually now

	var player *models.PlayerAccount
	player, err = gameClient.AuthenticateWithUI() // Modified to use UI
	if err != nil {
		ui.DisplayStaticText(1, 7, fmt.Sprintf("Authentication failed: %v", err), termbox.ColorRed, termbox.ColorBlack)
		ui.DisplayStaticText(1, 9, "Press ESC to exit.", termbox.ColorWhite, termbox.ColorBlack)
		ui.RunSimpleEvacuateLoop() // Wait for user to exit
		return
	}

	ui.ClearScreen()
	ui.DisplayStaticText(1, 1, fmt.Sprintf("Welcome, %s (Level %d, EXP %d)!", player.Username, player.Level, player.EXP), termbox.ColorGreen, termbox.ColorBlack)
	ui.DisplayStaticText(1, 3, "Login successful. Requesting matchmaking...", termbox.ColorWhite, termbox.ColorBlack)

	var matchInfo *network.MatchFoundResponse              // Use the type from network package
	matchInfo, err = gameClient.RequestMatchmakingWithUI() // Modified to use UI for status updates
	if err != nil {
		ui.DisplayStaticText(1, 5, fmt.Sprintf("Matchmaking failed: %v", err), termbox.ColorRed, termbox.ColorBlack)
		ui.DisplayStaticText(1, 7, "Press ESC to exit.", termbox.ColorWhite, termbox.ColorBlack)
		ui.RunSimpleEvacuateLoop()
		return
	}

	ui.ClearScreen()
	ui.DisplayStaticText(1, 1, "Match Found!", termbox.ColorGreen, termbox.ColorBlack)
	ui.DisplayStaticText(1, 3, fmt.Sprintf("Game ID: %s", matchInfo.GameID), termbox.ColorWhite, termbox.ColorBlack)
	ui.DisplayStaticText(1, 4, fmt.Sprintf("Opponent: %s (Level %d)", matchInfo.Opponent.Username, matchInfo.Opponent.Level), termbox.ColorWhite, termbox.ColorBlack)
	ui.DisplayStaticText(1, 5, fmt.Sprintf("UDP Port for Game: %d", matchInfo.UDPPort), termbox.ColorWhite, termbox.ColorBlack)
	ui.DisplayStaticText(1, 6, fmt.Sprintf("You are PlayerOne: %t", matchInfo.IsPlayerOne), termbox.ColorWhite, termbox.ColorBlack)

	ui.DisplayStaticText(1, 8, "Attempting to send a UDP ping to global echo server (localhost:8081)...", termbox.ColorYellow, termbox.ColorBlack)
	termbox.Flush() // Ensure message is displayed before potential blocking call

	// Use a placeholder gameID and token for this global ping, or use actual if available
	// For global echo, gameID/token might not be strictly checked by the echo server.
	pingGameID := "global_ping_test"
	if matchInfo != nil && matchInfo.GameID != "" {
		pingGameID = matchInfo.GameID // Use actual game ID if we have one
	}
	pingPlayerToken := "test_client"
	if player != nil {
		pingPlayerToken = player.Username
	}

	udpResponse, udpErr := gameClient.SendBasicUDPMessage(pingGameID, pingPlayerToken, 8081, "Hello UDP Echo Server!")
	if udpErr != nil {
		ui.DisplayStaticText(1, 9, fmt.Sprintf("UDP Ping failed: %v", udpErr), termbox.ColorRed, termbox.ColorBlack)
	} else {
		ui.DisplayStaticText(1, 9, fmt.Sprintf("UDP Ping successful! Response: %s", udpResponse), termbox.ColorGreen, termbox.ColorBlack)
	}

	ui.DisplayStaticText(1, 11, "Client is ready for game-specific UDP gameplay. Press ESC to exit this screen.", termbox.ColorYellow, termbox.ColorBlack)
	quitRequested := ui.RunSimpleEvacuateLoop()

	log.Println("Termbox loop exited.")

	if quitRequested {
		log.Println("Quit was requested. Sending PlayerQuitUDP message...")
		if err := gameClient.SendPlayerQuitMessage(); err != nil {
			log.Printf("Error sending player quit message from main: %v", err)
		}
		// Optionally, add a small delay here if issues persist, e.g., time.Sleep(100 * time.Millisecond)
		// This gives the UDP packet a moment to be processed by the OS network stack before connections are closed.
	}

	// Connections are closed by defer gameClient.CloseConnections() when main exits.

	log.Println("Exiting client application.")

	// Explicitly close connections after everything, including sending quit message.
	log.Println("Closing client connections manually...")
	gameClient.CloseConnections()
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
