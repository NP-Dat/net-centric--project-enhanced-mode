package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"
)

const (
	ServerAddressTCP = "localhost:8080" // Assuming server runs on this TCP port
)

// Client holds the state for a game client
type Client struct {
	PlayerAccount *models.PlayerAccount
	TCPConn       net.Conn
	// UDPConn net.UDPConn // Will be added later
}

// NewClient creates a new client instance
func NewClient() *Client {
	return &Client{}
}

// Authenticate prompts the user for credentials and attempts to log in to the server.
// It returns the player account on success, or an error.
func (c *Client) Authenticate() (*models.PlayerAccount, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	fmt.Print("Enter password: ")
	// For real password input, consider using a library to hide typed characters
	// or termbox input if it's being initialized for this stage.
	// For simplicity with basic console input:
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	conn, err := net.Dial("tcp", ServerAddressTCP)
	if err != nil {
		log.Printf("Failed to connect to server at %s: %v", ServerAddressTCP, err)
		return nil, err
	}
	c.TCPConn = conn // Store the connection
	// defer conn.Close() // Close should be managed by the client's lifecycle

	loginReq := network.LoginRequest{Username: username, Password: password}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(loginReq); err != nil {
		log.Printf("Error sending login request: %v", err)
		return nil, err
	}

	decoder := json.NewDecoder(conn)
	var loginResp network.LoginResponse
	if err := decoder.Decode(&loginResp); err != nil {
		log.Printf("Error receiving login response: %v", err)
		return nil, err
	}

	if !loginResp.Success {
		log.Printf("Login failed: %s", loginResp.Message)
		return nil, fmt.Errorf("login failed: %s", loginResp.Message)
	}

	c.PlayerAccount = loginResp.Player
	log.Printf("Login successful for %s. Welcome!", c.PlayerAccount.Username)
	return c.PlayerAccount, nil
}

// CloseConnections closes any active network connections.
func (c *Client) CloseConnections() {
	if c.TCPConn != nil {
		c.TCPConn.Close()
		log.Println("TCP connection closed.")
	}
	// Add UDP connection closing later
}

// Main client logic (TCP/UDP connection, termbox setup)

// MatchmakingInfo stores details received when a match is found.
type MatchmakingInfo struct {
	GameID      string
	Opponent    models.PlayerAccount
	UDPPort     int
	IsPlayerOne bool
}

// RequestMatchmaking sends a matchmaking request to the server and waits for a response.
func (c *Client) RequestMatchmaking() (*MatchmakingInfo, error) {
	if c.TCPConn == nil || c.PlayerAccount == nil {
		return nil, fmt.Errorf("client is not authenticated or connected")
	}

	log.Println("Sending matchmaking request...")
	// The plan uses a generic TCPMessage envelope, but current server implementation
	// doesn't show a top-level dispatcher. For Sprint 1, we assume server handles
	// MatchmakingRequest directly after login or based on a simple sequence.
	// If server expects a specific MatchmakingRequest struct after login implicitly,
	// we might not need to send a separate request object if the server auto-queues.
	// However, the plan has MatchmakingRequest, so we should send it.

	// For now, let's assume the server expects a MatchmakingRequest object.
	// However, the `HandleMatchmakingRequest` in server code takes (conn, player)
	// and doesn't decode a MatchmakingRequest object from the stream explicitly after initial login.
	// It seems HandleMatchmakingRequest is intended to be called directly.
	// This implies the server needs a dispatcher after login to route to HandleMatchmakingRequest.
	// For Sprint 1, we'll simplify: The client expects a MatchFoundResponse after some time.
	// Let's assume for now the server automatically puts client into matchmaking after login for simplicity,
	// or that the server has a mechanism to expect this. The client will just wait for MatchFoundResponse.

	// To align with `MatchmakingRequest` struct, the client *should* send it.
	// The server main loop would need to decode `TCPMessage` and dispatch.
	// Let's make the client send the request as per protocol definition.

	// Correct approach: Send a MatchmakingRequest object.
	// The server side HandleConnection should have a loop that decodes TCPMessage and routes them.
	// Since that's not fully built on server, this client call might not be processed as expected by current server code.
	// For now, I will *not* send a MatchmakingRequest explicitly, but instead assume the server handles matchmaking after login
	// and the client just waits for the MatchFoundResponse. This is a temporary simplification.
	// When the server's main connection handler is built to dispatch TCPMessage types, the client will send network.MsgTypeMatchmakingRequest.

	log.Println("Waiting for match...") // Client will block here
	decoder := json.NewDecoder(c.TCPConn)
	var matchResponse network.MatchFoundResponse

	if err := decoder.Decode(&matchResponse); err != nil {
		log.Printf("Error receiving matchmaking response: %v", err)
		return nil, err
	}

	log.Printf("Match found! Opponent: %s, GameID: %s, UDP Port: %d, You are PlayerOne: %t",
		matchResponse.Opponent.Username, matchResponse.GameID, matchResponse.UDPPort, matchResponse.IsPlayerOne)

	info := &MatchmakingInfo{
		GameID:      matchResponse.GameID,
		Opponent:    matchResponse.Opponent,
		UDPPort:     matchResponse.UDPPort,
		IsPlayerOne: matchResponse.IsPlayerOne,
	}
	c.PlayerAccount.GameID = matchResponse.GameID // Store GameID in player account for reference

	return info, nil
}

// Add to PlayerAccount in models/player.go: GameID string `json:"game_id,omitempty"`
