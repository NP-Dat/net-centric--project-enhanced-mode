package game

import (
	"enhanced-tcr-udp/internal/models"
	"math/rand"
	"time"
)

// CalculateDamage calculates damage based on attacker and defender stats.
// Returns the damage dealt.
func CalculateDamage(attackerATK, defenderDEF int, isTowerAttack bool, towerCritChance float64) int {
	dmg := attackerATK - defenderDEF
	if isTowerAttack && rand.Float64() < towerCritChance { // Check for CRIT
		// Critical Hit Damage: DMG = (Attacker_ATK * 1.2) - Defender_DEF
		// Ensure ATK is treated as float for multiplication, then convert result to int.
		dmg = int(float64(attackerATK)*1.2) - defenderDEF
		// Optionally, log or signal that a CRIT occurred
	}

	if dmg < 0 {
		dmg = 0
	}
	return dmg
}

// ApplyDamage reduces defender's HP by the calculated damage.
// It modifies the CurrentHP of the tower or troop directly.
func ApplyDamageToTower(tower *models.TowerInstance, damage int) {
	tower.CurrentHP -= damage
	if tower.CurrentHP < 0 {
		tower.CurrentHP = 0
	}
}

// ApplyDamageToTroop reduces defender's HP by the calculated damage.
// It modifies the CurrentHP of the tower or troop directly.
func ApplyDamageToTroop(troop *models.ActiveTroop, damage int) {
	troop.CurrentHP -= damage
	if troop.CurrentHP < 0 {
		troop.CurrentHP = 0
	}
}

// HealTower increases tower's HP by the heal amount, up to its MaxHP.
func HealTower(tower *models.TowerInstance, healAmount int) {
	tower.CurrentHP += healAmount
	if tower.CurrentHP > tower.MaxHP {
		tower.CurrentHP = tower.MaxHP
	}
}

func init() {
	rand.Seed(time.Now().UnixNano()) // Initialize random seed for CRIT chance
}

// Damage, CRIT calculation, etc.
