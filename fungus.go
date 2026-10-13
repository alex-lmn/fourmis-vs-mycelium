// Propagation des champignons.
//
// Regles :
//   - Deux especes : champignon normal (marron) et champignon venimeux (violet).
//     Les fourmis peuvent marcher dessus et recolter la foret en dessous, mais
//     elles s'exposent au virus.
//   - Un champignon adulte lache des spores au hasard autour de lui. Une spore
//     pousse plus facilement sur la foret que sur le sol nu. Elle est ecrasee
//     si une fourmi passe juste a cote. Trois fois plus de spores sous la pluie.
//   - Un champignon venimeux donne des petits venimeux ; un normal peut muter.
//   - Regle du jeu de la vie (tous les LifeEvery ticks) : une case vide avec
//     exactement 3 champignons voisins voit naitre un champignon ; un
//     champignon entoure de SuffocateAt voisins ou plus meurt etouffe.
//   - Un champignon vieillit et meurt en lachant ses dernieres spores.
//   - Aura : un champignon peut contaminer les fourmis qui passent tout pres
//     (rayon 2 et risque plus fort pour un venimeux). Voir virus.go.
//   - Un champignon detruit par les fourmis laisse souvent du mycelium dans le
//     sol : il repousse apres RegrowTime ticks (temps de respawn), sauf si une
//     fourmi est dessus a ce moment-la.
package main

import "math"

const (
	NoFungus = iota
	Mushroom
	Venomous
)

const (
	MushroomHP      = 8.0
	VenomousHP      = 14.0
	MatureAge       = 80    // age a partir duquel il lache des spores
	SporeRate       = 0.003 // chance de lacher une spore par tick
	SporeRange      = 2.5   // distance moyenne d'une spore
	SporeForest     = 0.45  // chance de germer sur la foret
	SporeSoil       = 0.1   // chance de germer sur le sol nu
	LifeEvery       = 12    // frequence de la regle du jeu de la vie
	LifeBirthChance = 0.25  // chance de naissance avec 3 voisins
	SuffocateAt     = 5     // un champignon avec autant de voisins (ou plus) meurt etouffe
	MyceliumChance  = 0.75  // chance de laisser du mycelium en mourant
	RegrowTime      = 600   // temps minimum de repousse
	VenomMutation   = 0.05  // un normal donne un venimeux
	VenomHeredity   = 0.5   // un venimeux donne un venimeux
)

type Fungus struct {
	Kind   int
	HP     float64
	Age    int
	MaxAge int
}

func (w *World) newFungus(kind int) Fungus {
	hp := MushroomHP
	if kind == Venomous {
		hp = VenomousHP
	}
	return Fungus{Kind: kind, HP: hp, MaxAge: 1500 + w.rng.Intn(2000)}
}

// destroyFungus : champignon detruit par une fourmi, il laisse peut-etre du mycelium.
func (w *World) destroyFungus(x, y int) {
	w.Fungus[y][x] = Fungus{}
	if w.rng.Float64() < MyceliumChance {
		w.Mycelium[y][x] = w.Tick + RegrowTime + w.rng.Intn(RegrowTime)
	}
}

func (w *World) stepFungus() {
	rate := SporeRate
	if w.Raining {
		rate *= 3
	}

	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			w.regrow(x, y)

			f := &w.Fungus[y][x]
			if f.Kind == NoFungus {
				continue
			}
			f.Age++
			if f.Age > f.MaxAge {
				kind := f.Kind
				w.Fungus[y][x] = Fungus{}
				w.Stats.FungusDeaths++
				for i := 0; i < 3; i++ {
					w.spore(x, y, kind) // ses dernieres spores
				}
				continue
			}
			if f.Age > MatureAge && w.rng.Float64() < rate {
				w.spore(x, y, f.Kind)
			}
			// Les champignons marquent le danger autour d'eux.
			if f.Kind == Venomous {
				w.PherDanger[y*w.W+x] = math.Min(PherMax, w.PherDanger[y*w.W+x]+0.3)
			} else {
				w.PherDanger[y*w.W+x] = math.Min(PherMax, w.PherDanger[y*w.W+x]+0.05)
			}
		}
	}

	w.fungusAura()
	if w.Tick%LifeEvery == 0 {
		w.fungusLife()
	}
}

// regrow : le mycelium refait pousser un champignon quand son temps est venu.
func (w *World) regrow(x, y int) {
	t := w.Mycelium[y][x]
	if t == 0 || w.Tick < t {
		return
	}
	if w.Fungus[y][x].Kind != NoFungus || w.antAt(x, y) {
		return // occupe : il reessaiera au prochain tick
	}
	kind := Mushroom
	if w.rng.Float64() < 0.2 {
		kind = Venomous
	}
	w.Fungus[y][x] = w.newFungus(kind)
	w.Mycelium[y][x] = 0
	w.Stats.Regrowths++
}

// spore : une spore tombe pres de (x, y) et germe peut-etre.
func (w *World) spore(x, y, parent int) {
	nx := x + int(math.Round(w.rng.NormFloat64()*SporeRange))
	ny := y + int(math.Round(w.rng.NormFloat64()*SporeRange))
	if !w.inside(nx, ny) || w.Fungus[ny][nx].Kind != NoFungus || w.nearNest(nx, ny) {
		return
	}
	// Une fourmi a cote ecrase la spore. V4 : cellules voisines seulement.
	for _, j := range w.near(nx, ny, 1) {
		a := w.Ants[j]
		if abs(a.X-nx) <= 1 && abs(a.Y-ny) <= 1 {
			w.Stats.SporesCrushed++
			return
		}
	}
	chance := SporeSoil
	if w.Food[ny][nx] > 0 {
		chance = SporeForest // la foret est un terrain ideal
	}
	if w.rng.Float64() >= chance {
		return
	}
	kind := Mushroom
	if parent == Venomous && w.rng.Float64() < VenomHeredity ||
		parent == Mushroom && w.rng.Float64() < VenomMutation {
		kind = Venomous
	}
	w.Fungus[ny][nx] = w.newFungus(kind)
	w.Stats.SporeBirths++
}

// fungusAura : chaque champignon peut contaminer les fourmis proches
// (chaque champignon est compare a toutes les fourmis).
func (w *World) fungusAura() {
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			f := w.Fungus[y][x]
			if f.Kind == NoFungus {
				continue
			}
			venom := f.Kind == Venomous
			r, chance := 1, AuraInfect
			if venom {
				r, chance = 2, VenomAuraInfect
			}
			for _, a := range w.Ants {
				if abs(a.X-x) > r || abs(a.Y-y) > r {
					continue
				}
				if venom && a.Alarm < 15 {
					a.Alarm = 15
				}
				risk := chance
				if a.Soldier {
					risk *= SoldierArmor
				}
				if a.Virus == 0 && w.rng.Float64() < risk {
					w.infect(a, InfectLoad, venom, true)
				}
			}
		}
	}
}

// fungusLife : regle du jeu de la vie sur la couche des champignons.
func (w *World) fungusLife() {
	// V3 : tableaux plats reutilises d'un appel a l'autre, remis a zero.
	count, venom := w.lifeCount, w.lifeVenom
	clear(count)
	clear(venom)
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			for d := 0; d < 8; d++ {
				nx, ny := x+dx8[d], y+dy8[d]
				if !w.inside(nx, ny) {
					continue
				}
				switch w.Fungus[ny][nx].Kind {
				case Mushroom:
					count[y*w.W+x]++
				case Venomous:
					count[y*w.W+x]++
					venom[y*w.W+x]++
				}
			}
		}
	}

	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			n := count[y*w.W+x]
			if w.Fungus[y][x].Kind != NoFungus {
				if n >= SuffocateAt {
					w.Fungus[y][x] = Fungus{} // etouffe
					w.Stats.FungusDeaths++
				}
			} else if n == 3 && !w.nearNest(x, y) && w.rng.Float64() < LifeBirthChance {
				kind := Mushroom
				if venom[y*w.W+x] >= 2 {
					kind = Venomous
				}
				w.Fungus[y][x] = w.newFungus(kind)
				w.Stats.LifeBirths++
			}
		}
	}
}

// antAt : une fourmi est-elle sur cette case ? V4 : sa cellule seulement.
func (w *World) antAt(x, y int) bool {
	for _, j := range w.near(x, y, 0) {
		a := w.Ants[j]
		if a.X == x && a.Y == y {
			return true
		}
	}
	return false
}
