package server

import (
	"encoding/json"
	"net"

	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"
	"enhanced-tcr-udp/internal/persistence"

	"log"

	"golang.org/x/crypto/bcrypt"
)

// HandleLogin handles a login request from a client over TCP.
func HandleLogin(conn net.Conn) (*models.PlayerAccount, error) {
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var loginReq network.LoginRequest
	if err := decoder.Decode(&loginReq); err != nil {
		log.Printf("Error decoding login request: %v", err)
		// Attempt to send a generic error response if possible
		_ = encoder.Encode(network.LoginResponse{Success: false, Message: "Invalid login request format"})
		return nil, err
	}

	log.Printf("Login attempt from username: %s", loginReq.Username)

	playerAcc, err := persistence.LoadPlayerAccount(loginReq.Username)
	if err != nil {
		log.Printf("Failed to load player account for %s: %v", loginReq.Username, err)
		resp := network.LoginResponse{Success: false, Message: "User not found or error loading data."}
		if err := encoder.Encode(resp); err != nil {
			log.Printf("Error sending login response: %v", err)
		}
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(playerAcc.HashedPassword), []byte(loginReq.Password))
	if err != nil {
		// Password does not match
		log.Printf("Invalid password for user %s", loginReq.Username)
		resp := network.LoginResponse{Success: false, Message: "Invalid username or password."}
		if err := encoder.Encode(resp); err != nil {
			log.Printf("Error sending login response: %v", err)
		}
		return nil, err // Or a more specific error indicating bad credentials
	}

	log.Printf("User %s logged in successfully", loginReq.Username)
	resp := network.LoginResponse{
		Success: true,
		Message: "Login successful!",
		Player:  playerAcc,
	}
	if err := encoder.Encode(resp); err != nil {
		log.Printf("Error sending successful login response: %v", err)
		return nil, err // If we can't send success, the client won't know
	}

	return playerAcc, nil
}
