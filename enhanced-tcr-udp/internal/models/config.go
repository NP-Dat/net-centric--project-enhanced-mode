package models

// TowerSpec defines the base specifications for a type of tower.
type TowerSpec struct {
	ID         string  `json:"id"`          // e.g., "king_tower", "guard_tower_1"
	Name       string  `json:"name"`        // e.g., "King Tower", "Guard Tower"
	BaseHP     int     `json:"base_hp"`     // Base Hit Points
	BaseATK    int     `json:"base_atk"`    // Base Attack
	BaseDEF    int     `json:"base_def"`    // Base Defense
	CritChance float64 `json:"crit_chance"` // Critical Hit Chance (0.0 to 1.0)
	EXPYield   int     `json:"exp_yield"`   // EXP awarded when this tower is destroyed
}

// TroopSpec defines the base specifications for a type of troop.
type TroopSpec struct {
	ID       string `json:"id"`        // e.g., "pawn", "queen"
	Name     string `json:"name"`      // e.g., "Pawn", "Queen"
	ManaCost int    `json:"mana_cost"` // MANA required to deploy
	BaseHP   int    `json:"base_hp"`   // Base Hit Points (if it were to fight, though troops only attack towers)
	BaseATK  int    `json:"base_atk"`  // Base Attack
	BaseDEF  int    `json:"base_def"`  // Base Defense (if it were to be attacked, though towers only attack troops)
	// Note: Troops have 0% base CRIT according to plan.
}

// GameConfig holds all configurable game parameters, typically loaded from JSON files.
type GameConfig struct {
	Towers map[string]TowerSpec `json:"towers"` // Keyed by Tower ID
	Troops map[string]TroopSpec `json:"troops"` // Keyed by Troop ID
	// Other global game settings can be added here
	// e.g., MaxMana, ManaRegenRate, GameDurationSeconds
}
