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
	MatchedChan   chan struct{} // Closed when the player is matched and notified
}

var (
	matchmakingQueue = make(chan *PlayerQueueEntry, 1) // Changed buffer size from 2 to 1
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
func HandleMatchmakingRequest(conn net.Conn, player *models.PlayerAccount) {
	log.Printf("Player %s entered matchmaking.", player.Username)

	queueEntry := &PlayerQueueEntry{
		PlayerAccount: player,
		Connection:    conn,
		RequestTime:   time.Now(),
		MatchedChan:   make(chan struct{}), // Initialize the notification channel
	}

	select {
	case matchmakingQueue <- queueEntry: // This is the first player entering the queue
		log.Printf("Player %s is waiting in queue. Connection will be held open.", player.Username)
		// Wait for this player to be matched and notified.
		// This blocks until MatchedChan is closed when this player is matched.
		<-queueEntry.MatchedChan
		log.Printf("Player %s has been matched and notified. Completing HandleMatchmakingRequest.", player.Username)
		return // Now safe to return; connection will be closed by handleConnection's defer

	default: // This is the second player; queue was full (P1 was waiting)
		queueMutex.Lock()
		select {
		case waitingPlayer := <-matchmakingQueue: // Retrieve P1 (waitingPlayer)
			queueMutex.Unlock() // Unlock early as we have the waiting player
			log.Printf("Matching %s with %s", waitingPlayer.PlayerAccount.Username, player.Username)
			gameID := uuid.New().String()
			udpPort := GetNextUDPPort()

			gameSession := GlobalSessionManager.CreateSession(gameID, waitingPlayer.PlayerAccount, player, udpPort)
			if gameSession == nil {
				log.Printf("Failed to create game session for %s and %s. %s (waiting player) put back in queue. %s (current player) request ends.",
					waitingPlayer.PlayerAccount.Username, player.Username, waitingPlayer.PlayerAccount.Username, player.Username)
				// Put waitingPlayer back into the queue. Their MatchedChan is still open, so they continue to wait.
				matchmakingQueue <- waitingPlayer
				// The current player (player / conn) request ends here; their connection will close.
				// They would need to retry matchmaking.
				return
			}

			// Notify both players
			notifyMatch(waitingPlayer.Connection, waitingPlayer.PlayerAccount, player, gameID, udpPort, true) // Notify P1
			notifyMatch(conn, player, waitingPlayer.PlayerAccount, gameID, udpPort, false)                    // Notify P2

			log.Printf("Match found: %s vs %s. GameID: %s, UDP Port: %d. Session created.", waitingPlayer.PlayerAccount.Username, player.Username, gameID, udpPort)

			// After P1 (waitingPlayer) has been successfully notified, close their MatchedChan
			// to unblock their HandleMatchmakingRequest goroutine.
			log.Printf("Closing MatchedChan for waiting player %s to allow their handler to complete.", waitingPlayer.PlayerAccount.Username)
			close(waitingPlayer.MatchedChan)

			// P2's (current player) HandleMatchmakingRequest completes here as well.
			log.Printf("HandleMatchmakingRequest for current player %s (P2) is completing.", player.Username)
			return

		default: // Should ideally not be reached if queue was full means a player was there.
			queueMutex.Unlock()
			log.Printf("Error in matchmaking: queue was full but no waiting player found. %s is being added to queue and will wait.", player.Username)
			// This means the current player (queueEntry) will now become the waiting player.
			matchmakingQueue <- queueEntry // Add current player to the queue
			<-queueEntry.MatchedChan       // Wait for this player to be matched
			log.Printf("Player %s (who was re-queued after an error) has been matched. Completing HandleMatchmakingRequest.", player.Username)
			return
		}
	}
}

func notifyMatch(conn net.Conn, player *models.PlayerAccount, opponent *models.PlayerAccount, gameID string, udpPort int, isPlayerOne bool) {
	matchResponse := network.MatchFoundResponse{
		GameID:             gameID,
		Opponent:           *opponent,
		UDPPort:            udpPort,
		IsPlayerOne:        isPlayerOne,
		PlayerSessionToken: player.Username,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(matchResponse); err != nil {
		log.Printf("Error sending MatchFoundResponse to %s: %v. This might leave the other player hanging or cause issues.", player.Username, err)
		// If sending fails, the MatchedChan for a waiting player might not be closed if this is the critical notification.
		// Or, if it's for P2, P1 might have been unblocked but P2 didn't get message.
		// More robust error handling needed here for production (e.g., attempt to remove session, notify other player of failure).
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
