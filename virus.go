// Virus : le poison des champignons est contagieux entre fourmis.
//
// Regles :
//   - Une fourmi est contaminee en mangeant un champignon, en l'attaquant
//     ou en passant pres d'un champignon. Deux souches : normale et
//     venimeuse (deux fois plus rapide et deux fois plus contagieuse).
//   - La charge virale (0 a 1) grandit a chaque tick et fait perdre des
//     points de vie, de plus en plus vite.
//   - Contagion : une fourmi infectee contamine les fourmis collees a elle
//     (on compare chaque infectee a toutes les fourmis).
//   - Toilettage social : une fourmi peu infectee est nettoyee par les soeurs
//     saines qui la touchent ; elle peut guerir avant de tomber malade.
//   - Malade (charge > SickLevel), elle arrete de travailler, rentre au nid
//     et y reste jusqu'a guerison complete. Quarantaine : tant qu'elle se
//     repose au nid, elle ne contamine personne.
//   - Guerie, elle est immunisee pendant ImmuneTime ticks, mais seulement
//     contre la contagion entre fourmis : les champignons et les cadavres
//     restent toujours toxiques.
//   - Au-dela de ZombieLevel, elle devient zombie (comme avec le cordyceps) :
//     elle ne peut plus guerir, chasse les fourmis saines autour d'elle (sans
//     proie, elle erre au hasard), n'approche pas des nids et contamine deux
//     fois plus.
//   - Une fourmi infectee qui meurt loin des nids libere un nuage toxique :
//     toutes les fourmis dans un rayon de DeathRadius sont contaminees, meme
//     immunisees. Puis un champignon pousse a l'endroit de sa mort et une
//     salve de spores s'envole. Pres d'un nid, le cadavre est evacue par les
//     croque-mortes (voir killAnt).
package main

import (
	"math"
	"slices"
)

const (
	InfectLoad      = 0.05   // charge d'une contamination par contact
	EatLoad         = 0.25   // charge en mangeant un champignon
	VenomEatLoad    = 0.4    // charge en mangeant un venimeux
	VirusGrowth     = 0.0025 // croissance de la charge par tick
	VenomGrowth     = 0.005  // souche venimeuse
	VirusDamage     = 0.06   // PV perdus par tick a charge maximale
	Contagion       = 0.01   // chance de contaminer une voisine par tick
	VenomContagion  = 0.02
	AttackInfect    = 0.1   // chance d'etre contaminee en frappant un champignon
	VenomAttackRisk = 0.25  // idem pour un venimeux
	AuraInfect      = 0.002 // chance par tick pres d'un champignon
	VenomAuraInfect = 0.01  // chance par tick pres d'un venimeux (rayon 2)
	SickLevel       = 0.2   // au-dessus : rentre se soigner au nid
	GroomCure       = 0.002 // charge retiree par tick par chaque soeur saine qui la toilette
	GroomMax        = 3     // nombre max de soeurs qui toilettent en meme temps
	ZombieLevel     = 0.5   // au-dessus : zombie
	ZombieSight     = 12    // rayon dans lequel une zombie repere ses proies
	NestCure        = 0.006 // charge soignee par tick au nid
	ImmuneTime      = 1500  // duree de l'immunite apres guerison
	CorpseSpores    = 4     // spores liberees par un cadavre infecte
	DeathRadius     = 2     // rayon du nuage toxique d'un cadavre
	DeathLoad       = 0.2   // charge recue par les voisines d'un cadavre
)

func (a *Ant) Zombie() bool { return a.Virus >= ZombieLevel }

// infect contamine une fourmi. L'immunite protege seulement de la contagion
// entre fourmis : avec force (champignon, cadavre), elle est contaminee
// quand meme et perd son immunite.
func (w *World) infect(a *Ant, load float64, venom, force bool) {
	if a.Immune > 0 {
		if !force {
			return
		}
		a.Immune = 0
	}
	if a.Virus == 0 {
		w.Stats.Infections++
	}
	a.Virus = math.Min(1, a.Virus+load)
	if venom {
		a.Venom = true
	}
	if a.Alarm < 20 {
		a.Alarm = 20
	}
}

// progressVirus : la maladie avance et abime la fourmi.
func (w *World) progressVirus(a *Ant) {
	if a.Immune > 0 {
		a.Immune--
	}
	if a.Virus == 0 {
		return
	}
	growth := VirusGrowth
	if a.Venom {
		growth = VenomGrowth
	}
	a.Virus = math.Min(1, a.Virus+growth)
	a.HP -= a.Virus * VirusDamage
}

// cure : soins au nid. Une zombie ne peut plus guerir.
func (w *World) cure(a *Ant) {
	if a.Virus == 0 || a.Zombie() {
		return
	}
	a.Virus -= NestCure
	if a.Virus <= 0 {
		a.Virus, a.Venom, a.Resting = 0, false, false
		a.Immune = ImmuneTime
		w.Stats.Cured++
	}
}

// spreadVirus : chaque fourmi infectee peut contaminer celles qui la touchent.
func (w *World) spreadVirus() {
	for _, a := range w.Ants {
		// Quarantaine : une malade qui se repose au nid est isolee.
		if a.Virus == 0 || (a.Resting && w.inNest(a.X, a.Y)) {
			continue
		}
		rate := Contagion
		if a.Venom {
			rate = VenomContagion
		}
		if a.Zombie() {
			rate *= 2
		}
		// V4 : on garde les fourmis collees (cellules voisines), puis on trie
		// ce petit groupe par indice pour tirer les nombres aleatoires dans le
		// meme ordre que la boucle naive.
		touching := w.touchBuf[:0]
		for _, j := range w.near(a.X, a.Y, 1) {
			if b := w.Ants[j]; abs(a.X-b.X) <= 1 && abs(a.Y-b.Y) <= 1 {
				touching = append(touching, j)
			}
		}
		slices.Sort(touching)
		w.touchBuf = touching
		for _, j := range touching {
			b := w.Ants[j]
			if b == a || b.Virus > 0 || b.Immune > 0 {
				continue
			}
			if w.rng.Float64() < rate {
				w.infect(b, InfectLoad, a.Venom, false)
			}
		}
	}
}

// groom : toilettage social. Une fourmi peu infectee (pas encore malade) est
// nettoyee par les soeurs saines qui la touchent (on parcourt toutes les
// fourmis).
func (w *World) groom() {
	for _, a := range w.Ants {
		if a.Virus == 0 || a.Virus > SickLevel {
			continue
		}
		helpers := 0
		for _, j := range w.near(a.X, a.Y, 1) { // V4 : on compte, l'ordre ne compte pas
			b := w.Ants[j]
			if b != a && b.Virus == 0 && abs(a.X-b.X) <= 1 && abs(a.Y-b.Y) <= 1 {
				helpers++
				if helpers == GroomMax {
					break
				}
			}
		}
		if helpers == 0 {
			continue
		}
		a.Virus -= GroomCure * float64(helpers)
		if a.Virus <= 0 {
			a.Virus, a.Venom, a.Resting = 0, false, false
			a.Immune = ImmuneTime
			w.Stats.Groomed++
		}
	}
}

// sickAct : une fourmi malade rentre au nid et y reste jusqu'a guerison.
func (w *World) sickAct(a *Ant) {
	if w.inNest(a.X, a.Y) {
		return
	}
	w.move(a, func(nx, ny int) float64 {
		return 2 * (w.distNest(a.X, a.Y) - w.distNest(nx, ny))
	})
}

// zombieAct : une zombie chasse la fourmi saine la plus proche pour la
// contaminer. Sans proie en vue, elle erre au hasard. Elle n'approche pas
// des nids (les gardes la repoussent).
func (w *World) zombieAct(a *Ant) {
	prey, found := w.nearestHealthy(a, ZombieSight)
	nearNest := w.nearNest(a.X, a.Y)
	w.move(a, func(nx, ny int) float64 {
		s := w.rng.Float64() * 1.5
		if found {
			s += 1.5 * (dist(a.X, a.Y, prey.X, prey.Y) - dist(nx, ny, prey.X, prey.Y))
		}
		if nearNest {
			s += 2 * (w.distNest(nx, ny) - w.distNest(a.X, a.Y))
		}
		return s
	})
}

// nearestHealthy : fourmi saine et non immunisee la plus proche
// (on parcourt toutes les fourmis).
func (w *World) nearestHealthy(a *Ant, r int) (*Ant, bool) {
	var best *Ant
	bestDist := math.Inf(1)
	// V4 : cellules voisines ; a distance egale, plus petit indice (= boucle naive).
	for _, j := range w.near(a.X, a.Y, r) {
		b := w.Ants[j]
		if b == a || b.Virus > 0 || b.Immune > 0 || abs(b.X-a.X) > r || abs(b.Y-a.Y) > r {
			continue
		}
		if d := dist(a.X, a.Y, b.X, b.Y); d < bestDist || (d == bestDist && b.idx < best.idx) {
			best, bestDist = b, d
		}
	}
	return best, best != nil
}

// virusCorpse : le cadavre d'une fourmi infectee contamine ses voisines,
// puis un champignon pousse a sa place et des spores s'envolent.
func (w *World) virusCorpse(a *Ant) {
	// Nuage toxique : on parcourt toutes les fourmis. L'immunite ne protege pas.
	for _, j := range w.near(a.X, a.Y, DeathRadius) { // V4 : chaque voisine independante
		b := w.Ants[j]
		if b == a || b.Dead || abs(b.X-a.X) > DeathRadius || abs(b.Y-a.Y) > DeathRadius {
			continue
		}
		w.infect(b, DeathLoad, a.Venom, true)
		w.Stats.CorpseInfections++
	}

	kind := Mushroom
	if a.Venom {
		kind = Venomous
	}
	if w.Fungus[a.Y][a.X].Kind == NoFungus && !w.inNest(a.X, a.Y) {
		w.Fungus[a.Y][a.X] = w.newFungus(kind)
		w.Stats.CorpseBirths++
	}
	for i := 0; i < CorpseSpores; i++ {
		w.spore(a.X, a.Y, kind)
	}
}
