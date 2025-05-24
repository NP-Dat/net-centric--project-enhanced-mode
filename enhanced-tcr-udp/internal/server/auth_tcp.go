package server

import (
	"errors"
	"log"
	"os"
	"sync"

	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/persistence"

	"golang.org/x/crypto/bcrypt"
)

// AuthManager handles TCP authentication for users.
type AuthManager struct {
	activeUsers map[string]string // Maps username to clientID (e.g., remote address)
	mu          sync.RWMutex
}

// NewAuthManager creates a new authentication manager.
func NewAuthManager() *AuthManager {
	return &AuthManager{
		activeUsers: make(map[string]string),
	}
}

// Login authenticates a user or creates a new account if one doesn't exist.
// If successful, it marks the user as active with the given clientID.
func (am *AuthManager) Login(username, password, clientID string) (*models.PlayerAccount, error) {
	if username == "" || password == "" {
		return nil, errors.New("username and password cannot be empty")
	}

	acc, err := persistence.LoadPlayerAccount(username)
	if err != nil {
		if os.IsNotExist(err) {
			// Account does not exist, create a new one
			log.Printf("No account found for user '%s'. Creating a new account.", username)
			newAcc := &models.PlayerAccount{
				Username:       username,
				HashedPassword: password, // SavePlayerAccount will hash this
				EXP:            0,
				Level:          1,
			}
			if saveErr := persistence.SavePlayerAccount(newAcc); saveErr != nil {
				log.Printf("Error saving new player account for %s: %v", username, saveErr)
				return nil, errors.New("error creating user account")
			}
			log.Printf("New account created successfully for user: %s", username)
			acc = newAcc // Use the newly created account for subsequent login logic
		} else {
			// Other error loading account
			log.Printf("Error loading player account for %s: %v", username, err)
			return nil, errors.New("error accessing player account")
		}
	} else {
		// Account exists, verify password
		if err := bcrypt.CompareHashAndPassword([]byte(acc.HashedPassword), []byte(password)); err != nil {
			log.Printf("Invalid password for user: %s", username)
			return nil, errors.New("invalid username or password")
		}
	}

	// Check and register active user
	am.mu.Lock()
	defer am.mu.Unlock()

	if existingClientID, isLoggedIn := am.activeUsers[username]; isLoggedIn {
		if existingClientID != clientID {
			log.Printf("User %s already logged in from another client (%s)", username, existingClientID)
			return nil, errors.New("user already logged in from another client")
		}
		// Already logged in from the same client, proceed
		log.Printf("User %s re-confirmed login from client %s", username, clientID)
	} else {
		am.activeUsers[username] = clientID
		log.Printf("User %s logged in successfully with client ID %s", username, clientID)
	}

	return acc, nil
}

// Logout removes a user from the active users list.
func (am *AuthManager) Logout(username string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, isLoggedIn := am.activeUsers[username]; isLoggedIn {
		delete(am.activeUsers, username)
		log.Printf("User %s logged out.", username)
	} else {
		log.Printf("Attempted to logout user %s who was not logged in.", username)
	}
}

// IsUserLoggedIn checks if a user is currently logged in.
func (am *AuthManager) IsUserLoggedIn(username string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	_, ok := am.activeUsers[username]
	return ok
}
