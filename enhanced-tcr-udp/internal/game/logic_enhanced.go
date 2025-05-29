package game

import (
	"enhanced-tcr-udp/internal/models"
	"fmt"
	"sort"
	"strings"
)

// FindLowestHPTower finds the opponent's tower with the lowest absolute HP,
// respecting the Guard Tower 1 targeting rule.
func FindLowestHPTower(attackingPlayerID string, game *models.GameSession) *models.TowerInstance {
	var opponentPlayer *models.PlayerInGame
	if game.Player1.Account.Username == attackingPlayerID {
		opponentPlayer = game.Player2
	} else {
		opponentPlayer = game.Player1
	}

	if opponentPlayer == nil || len(opponentPlayer.Towers) == 0 {
		return nil // No opponent or opponent has no towers
	}

	opponentTowers := opponentPlayer.Towers // These are already pointers

	// Check Guard Tower 1 status
	gt1Destroyed := true
	var gt1 *models.TowerInstance
	for _, t := range opponentTowers {
		// Assuming SpecID contains "guard_tower_1" for GT1. This needs to be robust.
		// And GameSpecificID might be like "playerX_guard_tower_1"
		if strings.Contains(strings.ToLower(t.SpecID), "guard_tower_1") { // Or use GameSpecificID if more reliable
			gt1 = t
			if t.CurrentHP > 0 {
				gt1Destroyed = false
			}
			break
		}
	}

	var validTargets []*models.TowerInstance
	if !gt1Destroyed && gt1 != nil {
		// GT1 must be targeted if not destroyed and it exists
		if gt1.CurrentHP > 0 {
			validTargets = append(validTargets, gt1)
		}
	} else {
		// GT1 is destroyed or doesn't exist (or wasn't found by name), all other towers are valid targets
		for _, t := range opponentTowers {
			isGT1 := strings.Contains(strings.ToLower(t.SpecID), "guard_tower_1")
			if gt1Destroyed || !isGT1 { // If GT1 was destroyed, or this tower is not GT1
				if t.CurrentHP > 0 { // Can only target towers with HP > 0
					validTargets = append(validTargets, t)
				}
			}
		}
	}

	if len(validTargets) == 0 {
		// This can happen if GT1 is the only target and it's now destroyed in the same tick it was targeted.
		// Or if all opponent towers are destroyed.
		return nil
	}

	// Sort valid targets by current HP (ascending)
	sort.Slice(validTargets, func(i, j int) bool {
		return validTargets[i].CurrentHP < validTargets[j].CurrentHP
	})

	return validTargets[0] // Return the one with the lowest HP
}

// FindTroopToAttack selects a troop for a tower to attack.
// Logic: last attacker or oldest attacker.
// This simplified version attacks the "oldest" deployed troop (by DeployedAt timestamp).
func FindTroopToAttack(towerOwnerID string, game *models.GameSession) *models.ActiveTroop {
	var opponentPlayer *models.PlayerInGame
	if game.Player1.Account.Username == towerOwnerID {
		opponentPlayer = game.Player2
	} else {
		opponentPlayer = game.Player1
	}

	if opponentPlayer == nil || len(opponentPlayer.DeployedTroops) == 0 {
		return nil
	}

	var opponentActiveTroops []*models.ActiveTroop
	// Accessing DeployedTroops needs to be concurrency-safe.
	// Assuming the caller (e.g., game_session.go) handles locking for the game session state.
	for _, troop := range opponentPlayer.DeployedTroops {
		if troop.CurrentHP > 0 {
			opponentActiveTroops = append(opponentActiveTroops, troop)
		}
	}

	if len(opponentActiveTroops) == 0 {
		return nil
	}

	// Sort by DeployedAt time to find the "oldest"
	sort.Slice(opponentActiveTroops, func(i, j int) bool {
		return opponentActiveTroops[i].DeployedAt.Before(opponentActiveTroops[j].DeployedAt)
	})

	return opponentActiveTroops[0]
}

// ApplyQueenHeal finds the friendly tower with the lowest absolute HP and heals it.
func ApplyQueenHeal(deployingPlayerID string, game *models.GameSession, healAmount int) (string, *models.TowerInstance, int, error) {
	var actingPlayer *models.PlayerInGame
	if game.Player1.Account.Username == deployingPlayerID {
		actingPlayer = game.Player1
	} else {
		actingPlayer = game.Player2
	}

	if actingPlayer == nil {
		return "Deploying player not found in game session.", nil, 0, fmt.Errorf("player %s not found", deployingPlayerID)
	}

	var friendlyTowers []*models.TowerInstance
	for _, tower := range actingPlayer.Towers {
		if tower.CurrentHP > 0 && tower.CurrentHP < tower.MaxHP { // Eligible if not destroyed and not at max HP
			friendlyTowers = append(friendlyTowers, tower)
		}
	}

	if len(friendlyTowers) == 0 {
		return "No friendly towers eligible for healing.", nil, 0, nil // Not an error, but no action taken
	}

	// Sort by current HP (ascending) to find the one with lowest absolute HP
	sort.Slice(friendlyTowers, func(i, j int) bool {
		return friendlyTowers[i].CurrentHP < friendlyTowers[j].CurrentHP
	})

	targetTower := friendlyTowers[0]
	originalHP := targetTower.CurrentHP
	HealTower(targetTower, healAmount) // HealTower is from combat.go
	healedAmount := targetTower.CurrentHP - originalHP

	msg := fmt.Sprintf("Queen healed %s's %s from %d to %d HP (+%d).",
		targetTower.OwnerID, targetTower.SpecID, originalHP, targetTower.CurrentHP, healedAmount)
	return msg, targetTower, healedAmount, nil
}

// Core Enhanced TCR game rules, state
