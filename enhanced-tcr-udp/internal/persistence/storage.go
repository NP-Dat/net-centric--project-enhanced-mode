package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"

	"enhanced-tcr-udp/internal/models"

	"golang.org/x/crypto/bcrypt"
)

const (
	playerDataDir = "data/players_enhanced/"
	gameConfigDir = "config_enhanced/"
)

// LoadPlayerAccount loads a player's account data from a JSON file.
func LoadPlayerAccount(username string) (*models.PlayerAccount, error) {
	filePath := filepath.Join(playerDataDir, username+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var acc models.PlayerAccount
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

// SavePlayerAccount saves a player's account data to a JSON file.
// It also handles hashing the password if it's not already hashed.
func SavePlayerAccount(acc *models.PlayerAccount) error {
	// Ensure player data directory exists
	if err := os.MkdirAll(playerDataDir, 0755); err != nil {
		return err
	}

	// Hash password if not already hashed (e.g. new account)
	// This is a basic check; a more robust system would indicate if a password is new or being changed.
	if len(acc.HashedPassword) < 40 { // Bcrypt hashes are typically longer
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(acc.HashedPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		acc.HashedPassword = string(hashedBytes)
	}

	filePath := filepath.Join(playerDataDir, acc.Username+".json")
	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadTroopConfig loads troop specifications from troops.json.
func LoadTroopConfig() (map[string]models.TroopSpec, error) {
	filePath := filepath.Join(gameConfigDir, "troops.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var troops map[string]models.TroopSpec
	if err := json.Unmarshal(data, &troops); err != nil {
		return nil, err
	}
	return troops, nil
}

// LoadTowerConfig loads tower specifications from towers.json.
func LoadTowerConfig() (map[string]models.TowerSpec, error) {
	filePath := filepath.Join(gameConfigDir, "towers.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var towers map[string]models.TowerSpec
	if err := json.Unmarshal(data, &towers); err != nil {
		return nil, err
	}
	return towers, nil
}
