package server

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"

	// "enhanced-tcr-udp/internal/game" // For GameSession creation later
	"github.com/google/uuid" // For generating unique Game IDs
)

// PlayerQueueEntry stores information about a player waiting in the matchmaking queue.
type PlayerQueueEntry struct {
	PlayerAccount *models.PlayerAccount
	Connection    net.Conn
	RequestTime   time.Time
}

var (
	matchmakingQueue = make(chan *PlayerQueueEntry, 2) // Buffered channel for two players
	queueMutex       = &sync.Mutex{}
	// nextUDPPort can be managed by SessionManager or a global counter for simplicity in Sprint 1
	currentUDPPort = 8081 // Starting UDP port, to be incremented
	portMutex      = &sync.Mutex{}
	// Global instance of GameSessionManager
	GlobalSessionManager = NewGameSessionManager()
)

// GetNextUDPPort provides a simple way to get unique UDP ports for game sessions.
func GetNextUDPPort() int {
	portMutex.Lock()
	defer portMutex.Unlock()
	port := currentUDPPort
	currentUDPPort++
	return port
}

// HandleMatchmakingRequest handles a client's request to find a match.
// For Sprint 1, this will be a very simple implementation:
// - The first player waits.
// - The second player joins, a match is made, and both are notified.
// This function will block until a match is made or a timeout (not implemented in this version).
func HandleMatchmakingRequest(conn net.Conn, player *models.PlayerAccount) {
	log.Printf("Player %s entered matchmaking.", player.Username)

	queueEntry := &PlayerQueueEntry{
		PlayerAccount: player,
		Connection:    conn,
		RequestTime:   time.Now(),
	}

	// Try to add to queue or find a match
	// This uses a channel as a waiting queue. A more robust system would use a dedicated queue structure.
	select {
	case matchmakingQueue <- queueEntry:
		// Player added to queue, waiting for another player
		log.Printf("Player %s is waiting in queue.", player.Username)
		// Inform client they are searching (optional for simple version)
		// For now, the client will just block waiting for MatchFoundResponse
		return // The goroutine handling this connection will wait until a match is processed
	default:
		// Queue is full (i.e., one player is already waiting), try to match
		queueMutex.Lock()
		select {
		case waitingPlayer := <-matchmakingQueue:
			queueMutex.Unlock() // Unlock early if we got a player
			log.Printf("Matching %s with %s", waitingPlayer.PlayerAccount.Username, player.Username)
			// Create a game session ID
			gameID := uuid.New().String()
			udpPort := GetNextUDPPort()

			// Create the game session using the global manager
			gameSession := GlobalSessionManager.CreateSession(gameID, waitingPlayer.PlayerAccount, player, udpPort)
			if gameSession == nil {
				log.Printf("Failed to create game session for %s and %s. One or both players returned to queue.", waitingPlayer.PlayerAccount.Username, player.Username)
				// Handle error: put both players back in queue or notify them of failure.
				// Simple approach: try to put waitingPlayer back. Current player will also re-queue on next attempt.
				matchmakingQueue <- waitingPlayer // Potential blocking if queue is full again, needs robust handling
				return
			}

			// Notify both players
			notifyMatch(waitingPlayer.Connection, waitingPlayer.PlayerAccount, player, gameID, udpPort, true)
			notifyMatch(conn, player, waitingPlayer.PlayerAccount, gameID, udpPort, false)

			log.Printf("Match found: %s vs %s. GameID: %s, UDP Port: %d. Session created.", waitingPlayer.PlayerAccount.Username, player.Username, gameID, udpPort)
			// gameSession.Start() // Game will be started by the session manager or game loop later
		default:
			queueMutex.Unlock() // Unlock if no player (should not happen if queue was full)
			// This case should ideally not be reached if logic is correct with a 2-buffer channel
			log.Printf("Error in matchmaking: queue was full but no waiting player found. %s is returning to queue.", player.Username)
			// Re-add current player to queue if match fails unexpectedly
			matchmakingQueue <- queueEntry
			return
		}
		return
	}
}

func notifyMatch(conn net.Conn, player *models.PlayerAccount, opponent *models.PlayerAccount, gameID string, udpPort int, isPlayerOne bool) {
	matchResponse := network.MatchFoundResponse{
		GameID:      gameID,
		Opponent:    *opponent, // Sending the full opponent PlayerAccount model
		UDPPort:     udpPort,
		IsPlayerOne: isPlayerOne,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(matchResponse); err != nil {
		log.Printf("Error sending MatchFoundResponse to %s: %v", player.Username, err)
		// TODO: Handle this error, e.g., try to notify the other player about the issue,
		// or put the other player back in the queue.
	}
}

// This function would be called by the main server loop when a new connection is established
// and authenticated. The server then needs to route requests based on type.
// For now, this is a placeholder for how matchmaking might be initiated.
/*
func ProcessClientRequests(conn net.Conn, player *models.PlayerAccount) {
    decoder := json.NewDecoder(conn)
    for {
        var msg network.TCPMessage // Assuming a generic message envelope
        if err := decoder.Decode(&msg); err != nil {
            log.Printf("Error decoding message from %s: %v", player.Username, err)
            return // Close connection or handle error
        }

        switch msg.Type {
        case network.MsgTypeMatchmakingRequest:
            // Assuming payload is network.MatchmakingRequest, but for simplicity, we use player from auth
            HandleMatchmakingRequest(conn, player)
            // After matchmaking, this connection might be kept for game results or closed if UDP takes over fully.
            return // Exit this loop after matchmaking request is handled for now
        // Add other message type handlers here
        default:
            log.Printf("Received unknown message type: %s from %s", msg.Type, player.Username)
        }
    }
}
*/
