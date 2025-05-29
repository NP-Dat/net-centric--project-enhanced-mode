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
	PlayerAccount     *models.PlayerAccount
	Connection        net.Conn
	RequestTime       time.Time
	MatchedChan       chan struct{} // Closed when the player is matched and notified
	GameConcludedChan chan struct{} // Closed when game results processing is done for this player connection
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
		PlayerAccount:     player,
		Connection:        conn,
		RequestTime:       time.Now(),
		MatchedChan:       make(chan struct{}), // Initialize the notification channel
		GameConcludedChan: make(chan struct{}), // Initialize the game concluded channel
	}

	select {
	case matchmakingQueue <- queueEntry: // This is the first player entering the queue
		log.Printf("Player %s is waiting in queue. Connection will be held open.", player.Username)
		// Wait for this player to be matched and notified.
		<-queueEntry.MatchedChan
		log.Printf("Player %s has been matched and notified. Now waiting for game to conclude before closing TCP.", player.Username)
		<-queueEntry.GameConcludedChan // Wait for game results to be processed for this player
		log.Printf("Player %s game has concluded. Completing HandleMatchmakingRequest.", player.Username)
		return

	default: // This is the second player; queue was full (P1 was waiting)
		queueMutex.Lock()
		select {
		case waitingPlayer := <-matchmakingQueue: // Retrieve P1 (waitingPlayer)
			queueMutex.Unlock()
			log.Printf("Matching %s with %s", waitingPlayer.PlayerAccount.Username, player.Username)
			gameID := uuid.New().String()
			udpPort := GetNextUDPPort()

			resultsChan := make(chan network.GameResultInfo, 1)

			gameSession := GlobalSessionManager.CreateSession(gameID, waitingPlayer.PlayerAccount, player, udpPort, resultsChan)
			if gameSession == nil {
				log.Printf("Failed to create game session for %s and %s.", waitingPlayer.PlayerAccount.Username, player.Username)
				matchmakingQueue <- waitingPlayer // Put P1 back
				// For P2 (current player), their HandleMatchmakingRequest will simply return, and conn will be closed by server.go
				// We should also signal P2 that their game setup failed more explicitly if possible.
				close(queueEntry.GameConcludedChan) // Allow P2's handler to complete without error
				return
			}

			log.Printf("Match found: %s vs %s. GameID: %s, UDP Port: %d. Session created.", waitingPlayer.PlayerAccount.Username, player.Username, gameID, udpPort)
			go handleGameResults(resultsChan, waitingPlayer, queueEntry, gameID) // Pass queueEntry for P2

			notifyMatch(waitingPlayer.Connection, waitingPlayer.PlayerAccount, player, gameID, udpPort, true)
			notifyMatch(conn, player, waitingPlayer.PlayerAccount, gameID, udpPort, false)

			log.Printf("Closing MatchedChan for waiting player %s to allow their handler to proceed with game conclusion wait.", waitingPlayer.PlayerAccount.Username)
			close(waitingPlayer.MatchedChan)

			// P2's (current player, queueEntry) HandleMatchmakingRequest also waits for game conclusion.
			log.Printf("Player %s (P2) is now waiting for game to conclude before closing TCP.", queueEntry.PlayerAccount.Username)
			<-queueEntry.GameConcludedChan
			log.Printf("Player %s (P2) game has concluded. Completing HandleMatchmakingRequest.", queueEntry.PlayerAccount.Username)
			return

		default: // Should ideally not be reached
			queueMutex.Unlock()
			log.Printf("Error in matchmaking: queue was full but no waiting player found. %s is being added to queue.", player.Username)
			matchmakingQueue <- queueEntry
			<-queueEntry.MatchedChan
			log.Printf("Player %s (who was re-queued) has been matched. Waiting for game conclusion.", player.Username)
			<-queueEntry.GameConcludedChan
			log.Printf("Player %s (who was re-queued) game has concluded. Completing HandleMatchmakingRequest.", player.Username)
			return
		}
	}
}

// handleGameResults waits for results from a game session and sends them to players via TCP.
func handleGameResults(resultsChan <-chan network.GameResultInfo, p1Entry *PlayerQueueEntry, p2Entry *PlayerQueueEntry, gameID string) {
	log.Printf("[GameID: %s] Goroutine started to handle game results for %s and %s.", gameID, p1Entry.PlayerAccount.Username, p2Entry.PlayerAccount.Username)
	defer func() {
		log.Printf("[GameID: %s] Closing GameConcludedChan for %s.", gameID, p1Entry.PlayerAccount.Username)
		close(p1Entry.GameConcludedChan)
		log.Printf("[GameID: %s] Closing GameConcludedChan for %s.", gameID, p2Entry.PlayerAccount.Username)
		close(p2Entry.GameConcludedChan)
		log.Printf("[GameID: %s] Goroutine for handling game results finished for %s and %s.", gameID, p1Entry.PlayerAccount.Username, p2Entry.PlayerAccount.Username)
	}()

	select {
	case resultInfo, ok := <-resultsChan:
		if !ok {
			log.Printf("[GameID: %s] Results channel closed prematurely for %s and %s.", gameID, p1Entry.PlayerAccount.Username, p2Entry.PlayerAccount.Username)
			return
		}

		log.Printf("[GameID: %s] Received game results: P1(%s): %s, P2(%s): %s, Winner: %s, Reason: %s",
			gameID, resultInfo.Player1Username, resultInfo.Player1Result.Outcome,
			resultInfo.Player2Username, resultInfo.Player2Result.Outcome,
			resultInfo.OverallWinnerID, resultInfo.GameEndReason)

		// Send results to Player 1 (waitingPlayer)
		msgP1 := network.TCPMessage{
			Type:    network.MsgTypeGameOverResults,
			Payload: resultInfo.Player1Result,
		}
		if err := json.NewEncoder(p1Entry.Connection).Encode(msgP1); err != nil {
			log.Printf("[GameID: %s] Error sending GameOverResults to %s: %v", gameID, p1Entry.PlayerAccount.Username, err)
		} else {
			log.Printf("[GameID: %s] Sent GameOverResults to %s.", gameID, p1Entry.PlayerAccount.Username)
		}

		// Send results to Player 2 (currentPlayer in HandleMatchmakingRequest context)
		msgP2 := network.TCPMessage{
			Type:    network.MsgTypeGameOverResults,
			Payload: resultInfo.Player2Result,
		}
		if err := json.NewEncoder(p2Entry.Connection).Encode(msgP2); err != nil {
			log.Printf("[GameID: %s] Error sending GameOverResults to %s: %v", gameID, p2Entry.PlayerAccount.Username, err)
		} else {
			log.Printf("[GameID: %s] Sent GameOverResults to %s.", gameID, p2Entry.PlayerAccount.Username)
		}

	case <-time.After(10 * time.Minute): // Timeout if game session never sends results (e.g. crash)
		log.Printf("[GameID: %s] Timeout waiting for game results from session for %s and %s.", gameID, p1Entry.PlayerAccount.Username, p2Entry.PlayerAccount.Username)
	}
	// Note: The TCP connections (p1Entry.Connection, p2Entry.Connection) themselves are managed by their respective
	// handleConnection goroutines in server.go. This handleGameResults goroutine only sends one message
	// and then its defer closes the GameConcludedChans, which unblocks the HandleMatchmakingRequest calls.
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
