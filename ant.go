// Comportement des fourmis.
//
// Regles :
//   - Deux castes : les ouvrieres recoltent la foret, les soldates chassent
//     les champignons.
//   - Une ouvriere recolte la foret a cote d'elle (jusqu'a WorkerCapacity
//     unites) et la ramene au nid en laissant une piste de pheromone
//     "nourriture". En explorant, elle laisse une piste "maison".
//   - Chaque fourmi consomme de l'energie. Au nid, elle depose sa recolte,
//     mange dans la reserve et se fait soigner du virus.
//   - Une ouvriere mourante de faim (energie < DesperateLevel) mange un
//     champignon voisin en dernier recours : elle recupere de l'energie mais
//     attrape le virus (voir virus.go).
//   - Une soldate qui touche un champignon l'attaque, et risque d'attraper
//     le virus a chaque coup. Sa carapace divise ce risque par 2.
//   - Les soldates chassent les zombies en priorite et les eliminent
//     proprement (ni nuage toxique, ni champignon).
//   - Croque-mortes : une fourmi morte pres d'un nid est evacuee, son cadavre
//     ne libere ni nuage toxique, ni champignon, ni spores.
//   - Une fourmi infectee ou attaquee donne l'alarme : les fourmis proches
//     sont alertees (les ouvrieres fuient le danger, les soldates y vont).
//   - Trop de fourmis au meme endroit : elles s'ecartent au hasard.
//   - La reine pond un oeuf quand la reserve le permet ; il eclot apres
//     HatchTime ticks (temps de respawn). La colonie fait plus de soldates
//     quand les champignons sont nombreux.
//   - Une colonie riche fonde de nouveaux nids loin des autres, la ou ses
//     ouvrieres trouvent de la foret. Toutes les fourmis rentrent au nid le
//     plus proche ; la reserve de nourriture est commune a tous les nids.
package main

import "math"

// Directions : N, NE, E, SE, S, SO, O, NO (dans l'ordre pour pouvoir tourner).
var dx8 = [8]int{0, 1, 1, 1, 0, -1, -1, -1}
var dy8 = [8]int{-1, -1, 0, 1, 1, 1, 0, -1}

const (
	MaxEnergy          = 100.0
	FoodEnergy         = 25.0 // energie apportee par 1 unite de nourriture
	WorkerHP           = 10.0
	SoldierHP          = 25.0
	WorkerCost         = 0.02 // energie depensee par tick
	SoldierCost        = 0.03
	WorkerCapacity     = 2 // unites de foret transportees par voyage
	WorkerScan         = 8 // rayon de vision
	SoldierScan        = 9
	AlarmRadius        = 6
	CrowdRadius        = 2
	CrowdLimit         = 6
	DesperateLevel     = 10.0 // en dessous : mange un champignon en dernier recours
	SoldierArmor       = 0.5  // les soldates ont 2 fois moins de risques d'etre contaminees
	ReturnLevel        = 22.0 // en dessous : rentre au nid manger
	SoldierReturnLevel = 40.0

	EggCost      = 12.0 // nourriture pour pondre un oeuf
	StockReserve = 40.0 // reserve minimum gardee pour nourrir les fourmis
	// + ReservePerAnt unite de reserve par fourmi vivante avant de pondre
	ReservePerAnt  = 0.3
	EatOnSpotLevel = 40.0 // en dessous : une fourmi mange sur place ce qu'elle recolte
	HatchTime      = 300  // temps d'eclosion d'un oeuf (respawn)
	LayEvery       = 4    // chaque reine (une par nid) pond au plus un oeuf tous les LayEvery ticks
	MemoryEvery    = 100  // la colonie repere des forets lointaines tous les MemoryEvery ticks
	MemorySize     = 256  // nombre de cases de foret retenues
	MaxAnts        = 100000
)

type Ant struct {
	ID      int
	X, Y    int
	Dir     int
	Soldier bool
	HP      float64
	MaxHP   float64
	Energy  float64
	Virus   float64 // charge virale : 0 = saine, 1 = maximum
	Venom   bool    // infectee par la souche venimeuse
	Immune  int     // ticks d'immunite restants apres guerison
	Resting bool    // malade : reste au nid jusqu'a guerison complete
	// Exploration : foret lointaine visee quand elle ne voit rien autour d'elle.
	Exploring    bool
	GoalX, GoalY int
	idx          int32 // indice dans w.Ants (grille spatiale, V4)
	cell         int32 // cellule de la grille spatiale (V4)
	Carry        int   // nourriture transportee
	Age          int
	MaxAge       int
	Alarm        int // > 0 : fourmi en alerte
	Dead         bool
	Cause        string
}

func (w *World) spawnAnt(soldier bool, nest *Nest) {
	a := &Ant{
		ID: w.nextID, Soldier: soldier, Dir: w.rng.Intn(8),
		Energy: MaxEnergy, MaxAge: 6000 + w.rng.Intn(5000),
		MaxHP: WorkerHP,
	}
	if soldier {
		a.MaxHP = SoldierHP
	}
	a.HP = a.MaxHP
	angle := w.rng.Float64() * 2 * math.Pi
	r := w.rng.Float64() * NestRadius
	a.X = min(w.W-1, max(0, nest.X+int(math.Cos(angle)*r)))
	a.Y = min(w.H-1, max(0, nest.Y+int(math.Sin(angle)*r)))
	w.nextID++
	w.Ants = append(w.Ants, a)
	w.Stats.Born++
}

func (w *World) stepAnts() {
	w.rebuildGrid() // V4 : des fourmis sont nees depuis le dernier tick
	w.propagateAlarm()
	w.spreadVirus()
	w.groom()

	for _, a := range w.Ants {
		if !a.Dead { // une zombie peut avoir ete eliminee plus tot dans le tick
			w.updateAnt(a)
		}
	}

	// V3 : les survivantes sont copiees dans un second tableau reutilise, puis
	// les deux tableaux sont echanges. On ne compacte pas w.Ants sur place :
	// killAnt parcourt w.Ants (nuage toxique) pendant cette boucle.
	survivors := w.antsSpare[:0]
	for _, a := range w.Ants {
		if a.Dead {
			w.killAnt(a)
		} else {
			survivors = append(survivors, a)
		}
	}
	clear(w.Ants) // l'ancien tableau ne retient plus les fourmis mortes
	w.antsSpare = w.Ants
	w.Ants = survivors
	w.rebuildGrid() // V4 : les mortes sortent de la grille (spores, repousse)
}

// propagateAlarm : chaque fourmi en alerte previent les fourmis proches.
// V4 : on ne lit que les fourmis des cellules voisines (chaque voisine est
// traitee independamment, l'ordre ne compte pas).
func (w *World) propagateAlarm() {
	for _, a := range w.Ants {
		if a.Alarm <= 5 {
			continue
		}
		for _, j := range w.near(a.X, a.Y, AlarmRadius) {
			b := w.Ants[j]
			if b == a || b.Alarm >= a.Alarm-5 {
				continue
			}
			dx, dy := a.X-b.X, a.Y-b.Y
			if dx*dx+dy*dy <= AlarmRadius*AlarmRadius {
				b.Alarm = a.Alarm - 5
			}
		}
	}
}

func (w *World) updateAnt(a *Ant) {
	a.Age++
	if a.Soldier {
		a.Energy -= SoldierCost
	} else {
		a.Energy -= WorkerCost
	}
	w.progressVirus(a)
	if a.Alarm > 0 {
		a.Alarm--
	}

	switch {
	case a.HP <= 0:
		a.Dead, a.Cause = true, "virus"
	case a.Energy <= 0:
		a.Dead, a.Cause = true, "starve"
	case a.Age >= a.MaxAge:
		a.Dead, a.Cause = true, "old"
	}
	if a.Dead {
		return
	}

	if w.inNest(a.X, a.Y) {
		w.restAtNest(a)
	}
	w.depositPheromones(a)

	if a.Virus > SickLevel {
		a.Resting = true
	}
	if a.Virus == 0 {
		a.Resting = false // guerie (au nid ou par toilettage) : elle reprend le travail
	}
	switch {
	case a.Zombie():
		w.zombieAct(a)
	case a.Resting:
		w.sickAct(a)
	case a.Soldier:
		w.soldierAct(a)
	default:
		w.workerAct(a)
	}
}

// restAtNest : depot de la recolte, repas et soins.
func (w *World) restAtNest(a *Ant) {
	if a.Carry > 0 {
		w.Stock += float64(a.Carry)
		a.Carry = 0
	}
	need := (MaxEnergy - a.Energy) / FoodEnergy
	take := math.Min(need, math.Min(w.Stock, 0.25))
	if take > 0 {
		a.Energy += take * FoodEnergy
		w.Stock -= take
	}
	w.cure(a)
	a.HP = math.Min(a.MaxHP, a.HP+0.05)
}

func (w *World) depositPheromones(a *Ant) {
	x, y := a.X, a.Y
	if !a.Soldier {
		if a.Carry > 0 {
			w.PherFood[y*w.W+x] = math.Min(PherMax, w.PherFood[y*w.W+x]+1.0)
		} else {
			w.PherHome[y*w.W+x] = math.Min(PherMax, w.PherHome[y*w.W+x]+0.4)
		}
	}
	if a.Virus > 0.2 || a.Alarm > 10 {
		w.PherDanger[y*w.W+x] = math.Min(PherMax, w.PherDanger[y*w.W+x]+0.6)
	}
}

func (w *World) workerAct(a *Ant) {
	// Affamee, elle ne rentre manger que si la reserve n'est pas vide ;
	// sinon elle part chercher de la foret et la mange sur place.
	hungryHome := a.Energy < ReturnLevel && w.Stock >= 1
	returning := a.Carry >= WorkerCapacity || hungryHome ||
		(a.Carry > 0 && !w.hasFoodNear(a.X, a.Y))

	if !returning {
		// Mourante de faim : elle mange un champignon... et attrape le virus.
		if a.Energy < DesperateLevel {
			if x, y, ok := w.adjacentFungus(a.X, a.Y); ok {
				w.eatMushroom(a, x, y)
				return
			}
		}
		if x, y, ok := w.adjacentFood(a.X, a.Y); ok {
			w.harvest(a, x, y)
			return
		}
	}

	tx, ty, hasTarget := 0, 0, false
	if !returning {
		tx, ty, hasTarget = w.scanFood(a.X, a.Y, WorkerScan)
		if hasTarget {
			a.Exploring = false
		} else {
			// Rien en vue : elle part vers une foret dont la colonie se souvient.
			tx, ty, hasTarget = w.exploreTarget(a)
		}
	}
	fear := 1.0
	if a.Alarm > 0 {
		fear = 2.5
	}

	w.move(a, func(nx, ny int) float64 {
		s := 0.0
		if returning {
			s += 1.6*(w.distNest(a.X, a.Y)-w.distNest(nx, ny)) + 0.2*w.PherHome[ny*w.W+nx]
		} else {
			s += 0.8*w.PherFood[ny*w.W+nx] - 0.05*w.PherHome[ny*w.W+nx]
			if hasTarget {
				s += 1.2 * (dist(a.X, a.Y, tx, ty) - dist(nx, ny, tx, ty))
			}
		}
		s -= fear * w.PherDanger[ny*w.W+nx]
		return s
	})
}

func (w *World) soldierAct(a *Ant) {
	hungry := a.Energy < SoldierReturnLevel
	returning := hungry && w.Stock >= 1

	// Affamee et reserve vide : elle mange la foret comme une ouvriere.
	if hungry && !returning {
		if x, y, ok := w.adjacentFood(a.X, a.Y); ok {
			w.harvest(a, x, y)
			return
		}
		if tx, ty, ok := w.scanFood(a.X, a.Y, SoldierScan); ok {
			w.move(a, func(nx, ny int) float64 {
				return 1.5 * (dist(a.X, a.Y, tx, ty) - dist(nx, ny, tx, ty))
			})
			return
		}
	}

	if !returning {
		// Priorite aux zombies : elles propagent le virus dans la colonie.
		if z, ok := w.nearestZombie(a, 1); ok {
			w.killZombie(a, z)
			return
		}
		if x, y, ok := w.adjacentFungus(a.X, a.Y); ok {
			w.attackMushroom(a, x, y)
			return
		}
	}

	tx, ty, hasTarget := 0, 0, false
	if !returning {
		if z, ok := w.nearestZombie(a, SoldierScan); ok {
			tx, ty, hasTarget = z.X, z.Y, true
		} else {
			tx, ty, hasTarget = w.scanFungus(a.X, a.Y, SoldierScan)
		}
	}
	leash := 0.4 * float64(max(w.W, w.H))
	far := w.distNest(a.X, a.Y) > leash

	w.move(a, func(nx, ny int) float64 {
		s := 0.6 * w.PherDanger[ny*w.W+nx]
		if returning || (far && !hasTarget) {
			s += 1.6 * (w.distNest(a.X, a.Y) - w.distNest(nx, ny))
		}
		if hasTarget {
			s += 1.5 * (dist(a.X, a.Y, tx, ty) - dist(nx, ny, tx, ty))
		}
		return s
	})
}

// move : la fourmi note les 8 cases autour d'elle et va sur la meilleure.
// Elle prefere aller tout droit, avec un peu de hasard. Elle peut marcher sur
// les champignons (mais leur aura peut la contaminer).
func (w *World) move(a *Ant, score func(nx, ny int) float64) {
	crowded := w.isCrowded(a)
	best, bestScore := -1, math.Inf(-1)
	for d := 0; d < 8; d++ {
		nx, ny := a.X+dx8[d], a.Y+dy8[d]
		if !w.inside(nx, ny) {
			continue
		}
		turn := abs(d - a.Dir)
		if turn > 4 {
			turn = 8 - turn
		}
		s := score(nx, ny) - 0.3*float64(turn) + w.rng.Float64()*0.8
		if crowded {
			s += w.rng.Float64() * 2
		}
		if s > bestScore {
			best, bestScore = d, s
		}
	}
	if best < 0 {
		a.Dir = w.rng.Intn(8)
		return
	}
	a.X += dx8[best]
	a.Y += dy8[best]
	a.Dir = best
	w.moveInGrid(a) // V4 : la grille suit la fourmi
}

// isCrowded : plus de CrowdLimit fourmis a CrowdRadius cases ou moins ?
// V1 : arret des que la limite est depassee. V4 : seulement les cellules
// voisines (on compte, l'ordre ne compte pas).
func (w *World) isCrowded(a *Ant) bool {
	n := 0
	for _, j := range w.near(a.X, a.Y, CrowdRadius) {
		b := w.Ants[j]
		if b != a && abs(b.X-a.X) <= CrowdRadius && abs(b.Y-a.Y) <= CrowdRadius {
			n++
			if n > CrowdLimit {
				return true
			}
		}
	}
	return false
}

// adjacentFood : une case de foret sur la fourmi ou a cote d'elle. Elle
// prefere une case saine ; sinon elle prend une case sous un champignon.
func (w *World) adjacentFood(x, y int) (int, int, bool) {
	for _, clean := range []bool{true, false} {
		for d := -1; d < 8; d++ {
			nx, ny := x, y // d = -1 : la case de la fourmi
			if d >= 0 {
				nx, ny = x+dx8[d], y+dy8[d]
			}
			if !w.inside(nx, ny) || w.Food[ny][nx] == 0 {
				continue
			}
			if (w.Fungus[ny][nx].Kind == NoFungus) == clean {
				return nx, ny, true
			}
		}
	}
	return 0, 0, false
}

func (w *World) hasFoodNear(x, y int) bool {
	_, _, ok := w.adjacentFood(x, y)
	return ok
}

func (w *World) adjacentFungus(x, y int) (int, int, bool) {
	for d := 0; d < 8; d++ {
		nx, ny := x+dx8[d], y+dy8[d]
		if w.inside(nx, ny) && w.Fungus[ny][nx].Kind != NoFungus {
			return nx, ny, true
		}
	}
	return 0, 0, false
}

// scanFood : case de foret la plus proche dans le rayon de vision. Une case
// sous un champignon compte comme 4 cases plus loin (elle est risquee).
func (w *World) scanFood(x, y, r int) (int, int, bool) {
	bx, by, best := 0, 0, math.Inf(1)
	for ny := y - r; ny <= y+r; ny++ {
		for nx := x - r; nx <= x+r; nx++ {
			if !w.inside(nx, ny) || w.Food[ny][nx] == 0 {
				continue
			}
			d := dist(x, y, nx, ny)
			if w.Fungus[ny][nx].Kind != NoFungus {
				d += 4
			}
			if d < best {
				bx, by, best = nx, ny, d
			}
		}
	}
	return bx, by, !math.IsInf(best, 1)
}

// scanFungus : champignon le plus proche dans le rayon de vision.
func (w *World) scanFungus(x, y, r int) (int, int, bool) {
	bx, by, best := 0, 0, math.Inf(1)
	for ny := y - r; ny <= y+r; ny++ {
		for nx := x - r; nx <= x+r; nx++ {
			if !w.inside(nx, ny) || w.Fungus[ny][nx].Kind == NoFungus {
				continue
			}
			if d := dist(x, y, nx, ny); d < best {
				bx, by, best = nx, ny, d
			}
		}
	}
	return bx, by, !math.IsInf(best, 1)
}

func (w *World) harvest(a *Ant, x, y int) {
	w.Food[y][x]--
	w.Stats.ForestEaten++
	if a.Energy < EatOnSpotLevel {
		// Affamee : elle mange sur place au lieu de rapporter.
		a.Energy = math.Min(MaxEnergy, a.Energy+FoodEnergy)
	} else {
		a.Carry++
		a.Energy = math.Min(MaxEnergy, a.Energy+2) // elle grignote un peu
	}

	// Recolter sous un champignon expose au virus, comme l'attaquer.
	if f := w.Fungus[y][x]; f.Kind != NoFungus {
		venom := f.Kind == Venomous
		risk := AttackInfect
		if venom {
			risk = VenomAttackRisk
		}
		if w.rng.Float64() < risk {
			w.infect(a, InfectLoad, venom, true)
		}
	}
}

func (w *World) eatMushroom(a *Ant, x, y int) {
	f := &w.Fungus[y][x]
	a.Energy = math.Min(MaxEnergy, a.Energy+40)
	if f.Kind == Venomous {
		w.infect(a, VenomEatLoad, true, true)
	} else {
		w.infect(a, EatLoad, false, true)
	}
	a.Alarm = 30
	w.Stats.MushEaten++
	f.HP -= 3
	if f.HP <= 0 {
		w.destroyFungus(x, y)
	}
}

func (w *World) attackMushroom(a *Ant, x, y int) {
	f := &w.Fungus[y][x]
	f.HP--
	venom := f.Kind == Venomous
	risk := AttackInfect
	if venom {
		risk = VenomAttackRisk
	}
	if w.rng.Float64() < risk*SoldierArmor {
		w.infect(a, InfectLoad, venom, true)
	}
	if f.HP <= 0 {
		w.destroyFungus(x, y)
		w.Stats.MushKilled++
	}
}

// nearestZombie : zombie la plus proche dans un rayon r.
// V4 : cellules voisines seulement. A distance egale, on garde le plus petit
// indice : c'est le meme choix que la boucle naive (premiere trouvee), sans
// avoir a trier les voisines.
func (w *World) nearestZombie(a *Ant, r int) (*Ant, bool) {
	var best *Ant
	bestDist := math.Inf(1)
	for _, j := range w.near(a.X, a.Y, r) {
		b := w.Ants[j]
		if b == a || b.Dead || !b.Zombie() || abs(b.X-a.X) > r || abs(b.Y-a.Y) > r {
			continue
		}
		if d := dist(a.X, a.Y, b.X, b.Y); d < bestDist || (d == bestDist && b.idx < best.idx) {
			best, bestDist = b, d
		}
	}
	return best, best != nil
}

// killZombie : la soldate elimine proprement la zombie (pas de nuage toxique
// ni de champignon). Elle risque un peu d'etre contaminee.
func (w *World) killZombie(a, z *Ant) {
	z.Dead, z.Cause = true, "killed"
	if w.rng.Float64() < AttackInfect*SoldierArmor {
		w.infect(a, InfectLoad, z.Venom, false)
	}
}

func (w *World) killAnt(a *Ant) {
	switch a.Cause {
	case "killed":
		w.Stats.ZombiesKilled++
		return // eliminee par une soldate : rien ne pousse
	case "virus":
		w.Stats.DeadVirus++
	case "starve":
		w.Stats.DeadStarve++
	case "old":
		w.Stats.DeadOld++
	}
	w.PherDanger[a.Y*w.W+a.X] = PherMax

	if a.Virus > 0 {
		if w.nearNest(a.X, a.Y) {
			// Croque-mortes : les cadavres pres du nid sont evacues.
			w.Stats.Evacuated++
		} else {
			w.virusCorpse(a)
		}
	}
}

// stepNest : eclosion des oeufs, ponte, puis fondation de nouveaux nids.
func (w *World) stepNest() {
	var waiting []Egg
	for _, egg := range w.Eggs {
		if egg.Hatch <= w.Tick && len(w.Ants) < MaxAnts {
			w.spawnAnt(w.wantSoldier(), egg.Nest)
		} else {
			waiting = append(waiting, egg)
		}
	}
	w.Eggs = waiting

	// Chaque nid a sa reine : plus la colonie a de nids, plus elle pond. Les
	// reines gardent une reserve pour nourrir les fourmis deja nees.
	if w.Tick%LayEvery == 0 {
		reserve := StockReserve + ReservePerAnt*float64(len(w.Ants))
		for _, nest := range w.Nests {
			if w.Stock < EggCost+reserve || len(w.Ants)+len(w.Eggs) >= MaxAnts {
				break
			}
			w.Stock -= EggCost
			w.Eggs = append(w.Eggs, Egg{Hatch: w.Tick + HatchTime, Nest: nest})
			w.Stats.EggsLaid++
		}
	}

	if w.Tick%MemoryEvery == 0 {
		w.rememberForest()
	}

	if w.Tick%FoundEvery == 0 {
		w.foundNest()
	}
}

// rememberForest : la colonie note des cases de foret prises au hasard sur
// toute la carte (parcours complet de la grille). Les ouvrieres qui ne voient
// plus de foret partent vers l'une d'elles.
func (w *World) rememberForest() {
	all := w.forestScratch[:0] // V3 : tampon reutilise
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			if w.Food[y][x] > 0 {
				all = append(all, [2]int{x, y})
			}
		}
	}
	w.forestScratch = all
	w.ForestMemory = w.ForestMemory[:0]
	for i := 0; i < MemorySize && len(all) > 0; i++ {
		w.ForestMemory = append(w.ForestMemory, all[w.rng.Intn(len(all))])
	}
}

// exploreTarget : destination d'une ouvriere qui ne voit pas de foret. Elle
// garde la meme tant qu'elle n'y est pas arrivee et qu'il y reste de la foret.
func (w *World) exploreTarget(a *Ant) (int, int, bool) {
	if a.Exploring && w.Food[a.GoalY][a.GoalX] > 0 && dist(a.X, a.Y, a.GoalX, a.GoalY) > 2 {
		return a.GoalX, a.GoalY, true
	}
	a.Exploring = false
	if len(w.ForestMemory) == 0 {
		return 0, 0, false
	}
	g := w.ForestMemory[w.rng.Intn(len(w.ForestMemory))]
	a.GoalX, a.GoalY, a.Exploring = g[0], g[1], true
	return a.GoalX, a.GoalY, true
}

// foundNest : une colonie riche et nombreuse fonde un nouveau nid la ou une
// ouvriere chargee se trouve loin de tous les nids et a l'abri des champignons.
func (w *World) foundNest() {
	if len(w.Nests) >= MaxNests || w.Stock < FoundCost+StockReserve || len(w.Ants) < FoundMinAnts {
		return
	}
	var candidates []*Ant
	for _, a := range w.Ants {
		if a.Soldier || a.Carry == 0 || w.distNest(a.X, a.Y) < NestSpacing {
			continue
		}
		if _, _, found := w.scanFungus(a.X, a.Y, SafeRadius); found {
			continue
		}
		candidates = append(candidates, a)
	}
	if len(candidates) == 0 {
		return
	}
	a := candidates[w.rng.Intn(len(candidates))]
	w.Nests = append(w.Nests, &Nest{X: a.X, Y: a.Y, Founded: w.Tick})
	w.Stock -= FoundCost
	w.Stats.NestsFounded++
}

// wantSoldier : plus il y a de champignons, plus la colonie fait de soldates
// (de 12% a 40%).
func (w *World) wantSoldier() bool {
	soldiers := 0
	for _, a := range w.Ants {
		if a.Soldier {
			soldiers++
		}
	}
	pressure := float64(w.Mushrooms+2*w.VenomousN) / float64(w.W*w.H) * 50
	target := 0.12 + 0.28*math.Min(1, pressure)
	return float64(soldiers) < target*float64(len(w.Ants)+1)
}
