package server

import (
	"encoding/json"
	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"
	"io"
	"log"
	"net"
	"os"
)

const (
	DefaultListenAddress = "localhost:8080"
)

// Server represents the main game server.
type Server struct {
	listenAddress  string
	listener       net.Listener
	authManager    *AuthManager
	sessionManager *GameSessionManager
	// Add other global server components here, e.g., config loader
}

// NewServer creates and initializes a new game server.
func NewServer(listenAddr string) *Server {
	if listenAddr == "" {
		listenAddr = DefaultListenAddress
	}
	return &Server{
		listenAddress:  listenAddr,
		authManager:    NewAuthManager(),     // From auth_tcp.go
		sessionManager: GlobalSessionManager, // From matchmaking_tcp.go (or init here)
	}
}

// Start begins the server's operations, listening for incoming connections.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.listenAddress)
	if err != nil {
		log.Printf("Error listening on %s: %v", s.listenAddress, err)
		return err
	}
	s.listener = listener
	log.Printf("Server listening for TCP connections on %s", s.listenAddress)

	// For now, keep the simple global UDP echo server from main.go if needed for testing,
	// or integrate a general purpose UDP port here if the design changes.
	// Game-specific UDP will be handled by GameSession instances on their own ports.

	// Accept connections in a loop
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("Error accepting TCP connection: %v", err)
			// Depending on the error, we might want to break or continue
			if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
				log.Println("Permanent error accepting connections. Shutting down listener.")
				return err // Stop if listener is closed or has a permanent error
			}
			continue // Continue for temporary errors
		}
		log.Printf("Accepted new TCP connection from %s", conn.RemoteAddr().String())
		go s.handleConnection(conn)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	log.Println("Stopping server...")
	if s.listener != nil {
		s.listener.Close()
	}
	// Add cleanup for other resources if necessary (e.g., active sessions)
}

// handleConnection manages an individual client connection.
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		log.Printf("Closing connection with %s", conn.RemoteAddr().String())
		conn.Close()
	}()

	clientAddr := conn.RemoteAddr().String()
	log.Printf("Handling connection for %s", clientAddr)

	// 1. Authentication Phase
	var playerAccount *models.PlayerAccount
	var err error

	// Expect LoginRequest
	// In a more robust system, we'd have a loop reading TCPMessage envelopes
	// For Sprint 1, assume first message after connect is LoginRequest
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn) // For sending responses

	var loginReq network.LoginRequest
	if err = decoder.Decode(&loginReq); err != nil {
		if err == io.EOF {
			log.Printf("Client %s disconnected before login.", clientAddr)
			return
		}
		log.Printf("Error decoding login request from %s: %v", clientAddr, err)
		// Optionally send an error response if possible
		return
	}

	playerAccount, err = s.authManager.Login(loginReq.Username, loginReq.Password, clientAddr)
	if err != nil {
		log.Printf("Authentication failed for user '%s' from %s: %v", loginReq.Username, clientAddr, err)
		response := network.LoginResponse{Success: false, Message: err.Error()}
		if encErr := encoder.Encode(response); encErr != nil {
			log.Printf("Error sending login failure response to %s: %v", clientAddr, encErr)
		}
		return // Authentication failed, close connection.
	}

	log.Printf("User '%s' authenticated successfully from %s.", playerAccount.Username, clientAddr)
	response := network.LoginResponse{Success: true, Message: "Login successful", Player: playerAccount}
	if err := encoder.Encode(response); err != nil {
		log.Printf("Error sending login success response to %s: %v", clientAddr, err)
		s.authManager.Logout(playerAccount.Username) // Rollback active user status
		return
	}

	// 2. Post-Authentication: Matchmaking or other actions
	// For Sprint 1, directly proceed to matchmaking.
	// A more advanced server would wait for a MatchmakingRequest PDU.
	// The current HandleMatchmakingRequest is designed to be called directly.
	log.Printf("User '%s' proceeding to matchmaking.", playerAccount.Username)
	HandleMatchmakingRequest(conn, playerAccount) // This function will block until match or timeout

	// After HandleMatchmakingRequest returns, the TCP connection's role for this client might be over,
	// or it might be kept for game end results. The current Matchmaking logic sends MatchFoundResponse
	// and then the connection might be idle until the game ends or if other TCP messages are planned.
	// For now, handleConnection will exit, and conn will be closed by defer.
	// If the connection needs to be kept alive for game results (as per plan),
	// HandleMatchmakingRequest should not be the end of this goroutine's lifecycle for this conn.
	// This implies that player connections perhaps need to be managed by SessionManager after match.

	log.Printf("Client %s has completed its initial TCP interaction (auth + matchmaking).", clientAddr)
}

// Optional: Run a simple UDP echo server on a known port for basic UDP testing.
// This is separate from game-specific UDP ports.
func StartGlobalUDPEchoServer(address string) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Printf("Error resolving global UDP address %s: %v", address, err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Printf("Error listening on global UDP %s: %v", address, err)
		os.Exit(1)
	}
	defer conn.Close()
	log.Printf("Global UDP echo server listening on %s", address)

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from global UDP: %v", err)
			continue
		}
		receivedMsg := string(buf[:n])
		log.Printf("Global UDP: Received from %s: %s", remoteAddr, receivedMsg)

		_, err = conn.WriteToUDP([]byte("UDP Echo: "+receivedMsg), remoteAddr)
		if err != nil {
			log.Printf("Error writing to global UDP: %v", err)
		}
	}
}
