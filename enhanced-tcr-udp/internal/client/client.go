package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"

	"github.com/nsf/termbox-go"
)

const (
	ServerAddressTCP = "localhost:8080" // Assuming server runs on this TCP port
)

// Client holds the state for a game client
type Client struct {
	PlayerAccount *models.PlayerAccount
	TCPConn       net.Conn
	UDPConn       *net.UDPConn // For UDP communication
	ServerUDPAddr *net.UDPAddr // To store the resolved server UDP address
	ui            *TermboxUI   // Reference to the termbox UI
}

// NewClient creates a new client instance
func NewClient(ui *TermboxUI) *Client {
	c := &Client{ui: ui}
	if ui != nil {
		ui.SetClient(c) // Pass client reference to UI
	}
	return c
}

// AuthenticateWithUI prompts the user for credentials via TermboxUI and attempts to log in.
func (c *Client) AuthenticateWithUI() (*models.PlayerAccount, error) {
	if c.ui == nil {
		// Fallback or error if UI is not initialized
		log.Println("Termbox UI not available, attempting console authentication.")
		return c.authenticateWithConsole() // Call existing console method as fallback
	}

	c.ui.ClearScreen()
	c.ui.DisplayStaticText(1, 1, "Login Required", termbox.ColorWhite, termbox.ColorBlack)
	username := c.ui.GetTextInput("Username: ", 1, 3, termbox.ColorWhite, termbox.ColorBlack)
	if username == "" { // Assuming empty means ESC was pressed or input cancelled
		return nil, fmt.Errorf("login cancelled by user")
	}
	password := c.ui.GetTextInput("Password: ", 1, 4, termbox.ColorWhite, termbox.ColorBlack)
	if password == "" {
		return nil, fmt.Errorf("login cancelled by user")
	}

	return c.performLogin(username, password)
}

// authenticateWithConsole is the original console-based authentication method.
func (c *Client) authenticateWithConsole() (*models.PlayerAccount, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	fmt.Print("Enter password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	return c.performLogin(username, password)
}

// performLogin contains the common logic for sending login request and handling response.
func (c *Client) performLogin(username, password string) (*models.PlayerAccount, error) {
	conn, err := net.Dial("tcp", ServerAddressTCP)
	if err != nil {
		log.Printf("Failed to connect to server at %s: %v", ServerAddressTCP, err)
		return nil, err
	}
	c.TCPConn = conn

	loginReq := network.LoginRequest{Username: username, Password: password}
	// Use TCPMessage envelope if server expects it, for now direct object.
	encoder := json.NewEncoder(c.TCPConn)
	if err := encoder.Encode(loginReq); err != nil {
		log.Printf("Error sending login request: %v", err)
		c.CloseConnections() // Close connection on error
		return nil, err
	}

	decoder := json.NewDecoder(c.TCPConn)
	var loginResp network.LoginResponse
	if err := decoder.Decode(&loginResp); err != nil {
		log.Printf("Error receiving login response: %v", err)
		c.CloseConnections()
		return nil, err
	}

	if !loginResp.Success {
		log.Printf("Login failed: %s", loginResp.Message)
		// Don't close connection here, server already sent response, client main loop may want to show message.
		// c.CloseConnections() // No, let main handle this based on error.
		return nil, fmt.Errorf("server: %s", loginResp.Message)
	}

	c.PlayerAccount = loginResp.Player
	log.Printf("Login successful for %s.", c.PlayerAccount.Username)
	return c.PlayerAccount, nil
}

// CloseConnections closes any active network connections.
func (c *Client) CloseConnections() {
	if c.TCPConn != nil {
		c.TCPConn.Close()
		c.TCPConn = nil
		log.Println("TCP connection closed.")
	}
	if c.UDPConn != nil {
		c.UDPConn.Close()
		c.UDPConn = nil
		log.Println("UDP connection closed.")
	}
}

// Main client logic (TCP/UDP connection, termbox setup)

// MatchmakingInfo stores details received when a match is found.
type MatchmakingInfo struct {
	GameID      string
	Opponent    models.PlayerAccount
	UDPPort     int
	IsPlayerOne bool
}

// RequestMatchmakingWithUI sends a matchmaking request and updates UI.
func (c *Client) RequestMatchmakingWithUI() (*network.MatchFoundResponse, error) {
	if c.TCPConn == nil || c.PlayerAccount == nil {
		return nil, fmt.Errorf("client is not authenticated or connected")
	}

	if c.ui != nil {
		c.ui.DisplayStaticText(1, 5, "Sending matchmaking request...", termbox.ColorYellow, termbox.ColorBlack)
	} else {
		log.Println("Sending matchmaking request...")
	}

	// TODO (Sprint 2+): Implement explicit PDU-driven matchmaking.
	// The client should send a network.TCPMessage with Type network.MsgTypeMatchmakingRequest
	// and Payload network.MatchmakingRequest{PlayerID: c.PlayerAccount.Username}.
	// The server's handleConnection would then need to decode this TCPMessage and dispatch
	// to HandleMatchmakingRequest, instead of calling it implicitly after login.
	// Example PDU construction:
	// matchmakingPDU := network.TCPMessage{
	// 	Type:    network.MsgTypeMatchmakingRequest,
	// 	Payload: network.MatchmakingRequest{PlayerID: c.PlayerAccount.Username},
	// }
	// encoder := json.NewEncoder(c.TCPConn)
	// if err := encoder.Encode(matchmakingPDU); err != nil {
	// 	log.Printf("Error sending matchmaking PDU: %v", err)
	// 	return nil, err
	// }
	// log.Println("Matchmaking PDU sent, awaiting MatchFoundResponse.")

	// Current (Sprint 1) server directly sends MatchFoundResponse after auth completes and matchmaking happens implicitly.
	// Server `internal/server/server.go`'s `handleConnection` calls `HandleMatchmakingRequest` directly.
	// So client just waits for `MatchFoundResponse`.

	if c.ui != nil {
		c.ui.DisplayStaticText(1, 6, "Waiting for match...", termbox.ColorYellow, termbox.ColorBlack)
	} else {
		log.Println("Waiting for match...")
	}

	decoder := json.NewDecoder(c.TCPConn)
	var matchResponse network.MatchFoundResponse

	if err := decoder.Decode(&matchResponse); err != nil {
		if c.ui != nil {
			c.ui.DisplayStaticText(1, 7, fmt.Sprintf("Error receiving match: %v", err), termbox.ColorRed, termbox.ColorBlack)
		}
		log.Printf("Error receiving matchmaking response: %v", err)
		return nil, err
	}

	if c.ui != nil {
		// Message already displayed by main.go after this returns
	}
	log.Printf("Match found! Opponent: %s, GameID: %s, UDP Port: %d",
		matchResponse.Opponent.Username, matchResponse.GameID, matchResponse.UDPPort)

	c.PlayerAccount.GameID = matchResponse.GameID

	// Establish UDP connection
	// TODO: Get server IP from config or a more robust mechanism
	serverIP := "127.0.0.1" // Assuming localhost for now
	err := c.EstablishUDPConnection(serverIP, matchResponse.UDPPort)
	if err != nil {
		log.Printf("Failed to establish UDP connection: %v", err)
		// Decide if this is a fatal error for matchmaking
		return &matchResponse, fmt.Errorf("failed to establish UDP connection: %w", err)
	}
	log.Printf("UDP connection established to %s:%d", serverIP, matchResponse.UDPPort)

	// Start listening for UDP messages in a new goroutine
	go c.ListenForUDPMessages()

	return &matchResponse, nil
}

// EstablishUDPConnection resolves the server's UDP address and prepares the UDPConn.
// It doesn't "connect" in the TCP sense but sets up the remote address.
func (c *Client) EstablishUDPConnection(serverIP string, udpPort int) error {
	if c.UDPConn != nil {
		// Close existing UDP connection if any, before creating a new one.
		// This might be needed if the client could go through matchmaking multiple times.
		c.UDPConn.Close()
		c.UDPConn = nil
	}

	serverAddr := fmt.Sprintf("%s:%d", serverIP, udpPort)
	raddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Printf("Failed to resolve UDP server address %s: %v", serverAddr, err)
		return err
	}
	c.ServerUDPAddr = raddr // Store the resolved remote address

	// For a client, DialUDP can be used to set a default destination,
	// allowing use of Read and Write. Or ListenUDP can be used to receive from any source
	// and then SendTo to send to specific server.
	// Using DialUDP here for simplicity if we assume most comms are with this server.
	// If client needs to receive from other peers or multiple servers on same port, ListenUDP is better.
	conn, err := net.DialUDP("udp", nil, raddr) // nil for local address, OS will pick
	if err != nil {
		log.Printf("Failed to dial UDP for server %s: %v", serverAddr, err)
		return err
	}
	c.UDPConn = conn
	log.Printf("UDP 'connection' established (DialUDP) to %s", serverAddr)
	return nil
}

// SendPlayerQuitMessage informs the server that the client is quitting the game.
func (c *Client) SendPlayerQuitMessage() error {
	if c.UDPConn == nil || c.PlayerAccount == nil || c.PlayerAccount.GameID == "" {
		log.Println("Cannot send quit message: UDP not connected, not authenticated, or no game ID.")
		return fmt.Errorf("client not in a state to send quit message")
	}

	quitMsg := network.UDPMessage{
		// Seq: Sequence numbers might be useful here if reliable quit is critical
		Timestamp:   time.Now(),
		SessionID:   c.PlayerAccount.GameID,
		PlayerToken: c.PlayerAccount.Username, // Or a specific session token if used
		Type:        network.UDPMsgTypePlayerQuit,
		Payload:     network.PlayerQuitUDP{}, // Empty payload for now
	}

	jsonData, err := json.Marshal(quitMsg)
	if err != nil {
		log.Printf("Error marshalling PlayerQuitUDP message: %v", err)
		return err
	}

	log.Printf("Sending PlayerQuitUDP message for session %s", c.PlayerAccount.GameID)
	_, err = c.UDPConn.Write(jsonData)
	if err != nil {
		log.Printf("Error sending PlayerQuitUDP message: %v", err)
		return err
	}
	return nil
}

// SendBasicUDPMessage sends a simple string message over UDP to the game server's assigned UDP port.
// This function seems to be for a basic ping and creates its own temporary connection.
// For game state, we'll likely use the persistent c.UDPConn.
func (c *Client) SendBasicUDPMessage(gameID string, playerToken string, udpPort int, message string) (string, error) {
	if c.PlayerAccount == nil {
		return "", fmt.Errorf("player not authenticated")
	}

	serverAddr := fmt.Sprintf("localhost:%d", udpPort)
	remoteAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return "", fmt.Errorf("failed to resolve UDP server address %s: %v", serverAddr, err)
	}

	// Establish UDP connection if not already done or if port changed (simple case: always dial)
	// A more robust client might maintain c.UDPConn across calls if appropriate.
	conn, err := net.DialUDP("udp", nil, remoteAddr) // nil for local address, OS will pick
	if err != nil {
		return "", fmt.Errorf("failed to dial UDP %s: %v", serverAddr, err)
	}
	defer conn.Close() // Close this specific connection after use
	// c.UDPConn = conn   // DO NOT OVERWRITE THE MAIN GAME UDP CONNECTION

	log.Printf("Sending UDP message to %s: %s", serverAddr, message)
	udpPDU := network.UDPMessage{
		// Seq: We are not tracking sequence numbers in this basic send yet
		Timestamp:   time.Now(),
		SessionID:   gameID,
		PlayerToken: playerToken,  // Could be c.PlayerAccount.Username or a session-specific token
		Type:        "basic_ping", // Example type
		Payload:     message,
	}
	jsonData, jsonErr := json.Marshal(udpPDU)
	if jsonErr != nil {
		return "", fmt.Errorf("failed to marshal UDP PDU: %v", jsonErr)
	}

	_, err = conn.Write(jsonData)
	if err != nil {
		return "", fmt.Errorf("failed to send UDP message: %v", err)
	}

	// Wait for a response (simple echo)
	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) // Timeout for response
	n, _, err := conn.ReadFromUDP(buffer)                 // Read from the connected UDP socket
	if err != nil {
		return "", fmt.Errorf("failed to read UDP response: %v", err)
	}

	responsePayload := string(buffer[:n])
	log.Printf("Received UDP response: %s", responsePayload)
	return responsePayload, nil
}

// Authenticate is the old method, preserved for now if needed or for non-UI contexts.
func (c *Client) Authenticate() (*models.PlayerAccount, error) {
	return c.authenticateWithConsole()
}

// RequestMatchmaking is the old method, preserved for now if needed or for non-UI contexts.
func (c *Client) RequestMatchmaking() (*network.MatchFoundResponse, error) {
	// This is a simplified version. The new RequestMatchmakingWithUI is preferred.
	if c.TCPConn == nil || c.PlayerAccount == nil {
		return nil, fmt.Errorf("client is not authenticated or connected")
	}
	log.Println("Waiting for match (console mode)...")
	decoder := json.NewDecoder(c.TCPConn)
	var matchResponse network.MatchFoundResponse
	if err := decoder.Decode(&matchResponse); err != nil {
		log.Printf("Error receiving matchmaking response (console): %v", err)
		return nil, err
	}
	log.Printf("Match found (console)! Opponent: %s, GameID: %s, UDP Port: %d",
		matchResponse.Opponent.Username, matchResponse.GameID, matchResponse.UDPPort)
	c.PlayerAccount.GameID = matchResponse.GameID
	return &matchResponse, nil
}

// Add to PlayerAccount in models/player.go: GameID string `json:"game_id,omitempty"`
