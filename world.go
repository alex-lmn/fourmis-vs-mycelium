// Fourmis vs Mycelium : une colonie de fourmis doit manger toute la foret
// pendant qu'un champignon venimeux se propage et les empoisonne.
//
// Version volontairement NAIVE : aucune optimisation (parcours complets de
// la grille a chaque tick, comparaisons de toutes les fourmis entre elles,
// grilles reallouees a chaque tick...). Le but est d'optimiser ensuite.
//
// Le monde est decoupe en fichiers :
//   - world.go     : la carte, la generation, la meteo, les statistiques
//   - ant.go       : le comportement des fourmis
//   - fungus.go    : la propagation des champignons
//   - virus.go     : le virus des champignons, contagieux entre fourmis
//   - pheromone.go : les pheromones (evaporation + diffusion)
package main

import (
	"math"
	"math/rand"
)

// Parametres generaux de la simulation.
const (
	MaxFood      = 5     // nourriture max d'une case de foret
	NestRadius   = 4     // rayon de la fourmiliere
	SafeRadius   = 10    // aucun champignon ne pousse aussi pres d'un nid
	StartStock   = 300.0 // nourriture dans le nid au depart
	MaxNests     = 8     // nombre max de nids
	NestSpacing  = 55.0  // distance min entre deux nids
	FoundCost    = 150.0 // nourriture pour fonder un nouveau nid
	FoundEvery   = 300   // la colonie essaie de fonder un nid tous les FoundEvery ticks
	FoundMinAnts = 800   // fourmis necessaires pour fonder un nid
	RainChance   = 1.0 / 3000
	// Les fourmis gagnent quand il reste VictoryForest % de foret ou moins
	// (les dernieres cases sont souvent sous des champignons, impossibles a
	// recolter).
	VictoryForest = 2.0
	HistoryMax    = 600 // nombre max de points dans l'historique
	MinSize       = 60
	MaxSize       = 2000
	MaxStartAnts  = 50000
)

type Nest struct {
	X       int `json:"x"`
	Y       int `json:"y"`
	Founded int `json:"founded"` // tick de fondation
}

type Egg struct {
	Hatch int   // tick d'eclosion
	Nest  *Nest // nid ou il a ete pondu
}

// Stats : compteurs cumules depuis le debut de la partie.
type Stats struct {
	Born         int `json:"born"`
	EggsLaid     int `json:"eggsLaid"`
	NestsFounded int `json:"nestsFounded"`
	DeadVirus    int `json:"deadVirus"`
	Infections   int `json:"infections"`
	Cured        int `json:"cured"`
	// contaminations par le nuage toxique d'un cadavre
	CorpseInfections int `json:"corpseInfections"`
	ZombiesKilled    int `json:"zombiesKilled"` // eliminees par les soldates
	Groomed          int `json:"groomed"`       // gueries par le toilettage social
	Evacuated        int `json:"evacuated"`     // cadavres evacues par les croque-mortes
	DeadStarve       int `json:"deadStarve"`
	DeadOld          int `json:"deadOld"`
	ForestEaten      int `json:"forestEaten"`
	MushEaten        int `json:"mushEaten"`
	MushKilled       int `json:"mushKilled"`
	SporeBirths      int `json:"sporeBirths"`
	CorpseBirths     int `json:"corpseBirths"`
	LifeBirths       int `json:"lifeBirths"`
	Regrowths        int `json:"regrowths"`
	FungusDeaths     int `json:"fungusDeaths"`
	SporesCrushed    int `json:"sporesCrushed"`
}

// HistPoint : un point des courbes affichees dans le navigateur.
type HistPoint struct {
	T         int     `json:"t"`
	Workers   int     `json:"workers"`
	Soldiers  int     `json:"soldiers"`
	Mushrooms int     `json:"mushrooms"`
	Venomous  int     `json:"venomous"`
	Infected  int     `json:"infected"`
	ForestPct float64 `json:"forestPct"`
	Stock     float64 `json:"stock"`
}

type World struct {
	W, H int
	rng  *rand.Rand
	Tick int

	Food     [][]int // nourriture de chaque case de foret (0 = sol nu)
	Fungus   [][]Fungus
	Mycelium [][]int // tick de repousse d'un champignon detruit (0 = rien)
	// Grilles de pheromones a plat (V3) : case (x, y) a l'indice y*W+x.
	PherFood   []float64
	PherHome   []float64
	PherDanger []float64
	pherSpare  []float64 // tampon de rechange pour la diffusion (double tampon)

	// Tampons reutilises d'un tick a l'autre (V3, zero allocation).
	lifeCount     []int    // fungusLife : nombre de champignons voisins
	lifeVenom     []int    // fungusLife : nombre de venimeux voisins
	antsSpare     []*Ant   // stepAnts : liste des survivantes
	forestScratch [][2]int // rememberForest : cases de foret

	// Grille spatiale (V4, voir spatial.go).
	cells        [][]int32 // indices des fourmis de chaque cellule
	gridW, gridH int
	nearBuf      []int32  // resultat de near, reutilise
	touchBuf     []int32  // spreadVirus : fourmis collees, reutilise
	auraBuf      []uint64 // fungusAura (V6) : couples (case, fourmi), reutilise

	Nests []*Nest // le premier nid est au centre, les autres sont fondes en cours de partie
	Stock float64 // nourriture stockee, partagee par tous les nids
	Eggs  []Egg
	// Cases de foret dont la colonie se souvient (voir rememberForest).
	ForestMemory [][2]int
	Ants         []*Ant
	nextID       int

	Raining  bool
	rainLeft int

	Stats         Stats
	InitialForest int

	// Recalcules a chaque tick par un parcours complet (countAll).
	ForestLeft int
	Workers    int
	Soldiers   int
	Carrying   int
	Mushrooms  int
	VenomousN  int
	MyceliumN  int
	Infected   int // fourmis porteuses du virus
	Zombies    int
	ImmuneN    int

	History   []HistPoint
	histEvery int

	Result string // "" en cours, "ants" (foret mangee a 98 %) ou "fungus" (colonie morte)
}

func NewWorld(w, h, ants int, seed int64) *World {
	wd := &World{W: w, H: h, rng: rand.New(rand.NewSource(seed)), histEvery: 10}
	wd.Food = newGridInt(w, h)
	wd.Mycelium = newGridInt(w, h)
	wd.Fungus = make([][]Fungus, h)
	for y := range wd.Fungus {
		wd.Fungus[y] = make([]Fungus, w)
	}
	wd.PherFood = make([]float64, w*h)
	wd.PherHome = make([]float64, w*h)
	wd.PherDanger = make([]float64, w*h)
	wd.pherSpare = make([]float64, w*h)
	wd.lifeCount = make([]int, w*h)
	wd.lifeVenom = make([]int, w*h)
	wd.Nests = []*Nest{{X: w / 2, Y: h / 2}}
	wd.Stock = StartStock

	wd.generateForest()
	wd.seedFungus()
	for i := 0; i < ants; i++ {
		wd.spawnAnt(i%7 == 0, wd.Nests[0]) // environ 15% de soldates
	}
	wd.Stats.Born = 0 // les fourmis de depart ne comptent pas comme naissances

	wd.countAll()
	wd.InitialForest = wd.ForestLeft
	wd.rebuildGrid() // V4 : grille prete avant le premier tick
	wd.recordHistory()
	return wd
}

// generateForest : bruit de valeur (deux octaves) pour dessiner des massifs.
func (w *World) generateForest() {
	coarse := func(step int) [][]float64 {
		g := newGridFloat(w.W/step+2, w.H/step+2)
		for y := range g {
			for x := range g[y] {
				g[y][x] = w.rng.Float64()
			}
		}
		return g
	}
	sample := func(g [][]float64, step, x, y int) float64 {
		gx, gy := x/step, y/step
		fx := float64(x%step) / float64(step)
		fy := float64(y%step) / float64(step)
		a := g[gy][gx]*(1-fx) + g[gy][gx+1]*fx
		b := g[gy+1][gx]*(1-fx) + g[gy+1][gx+1]*fx
		return a*(1-fy) + b*fy
	}

	big, small := coarse(24), coarse(7)
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			if w.distNest(x, y) < 14 {
				continue // clairiere autour du nid
			}
			v := 0.7*sample(big, 24, x, y) + 0.3*sample(small, 7, x, y)
			if v > 0.5 {
				w.Food[y][x] = min(MaxFood, 1+int((v-0.5)/0.3*MaxFood))
			}
		}
	}
}

// seedFungus : des colonies de champignons adultes loin du nid.
func (w *World) seedFungus() {
	const radius = 5
	colonies := max(8, w.W*w.H/4000)
	minDist := float64(min(w.W, w.H)) / 4
	for i := 0; i < colonies; i++ {
		var x, y int
		for {
			x, y = w.rng.Intn(w.W), w.rng.Intn(w.H)
			if w.distNest(x, y) > minDist {
				break
			}
		}
		kind := Mushroom
		if w.rng.Float64() < 0.35 {
			kind = Venomous
		}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				nx, ny := x+dx, y+dy
				if dx*dx+dy*dy <= radius*radius && w.inside(nx, ny) && w.rng.Float64() < 0.6 {
					f := w.newFungus(kind)
					f.Age = MatureAge + w.rng.Intn(300) // deja adulte
					w.Fungus[ny][nx] = f
				}
			}
		}
	}
}

// updateSimulation avance la simulation d'un tick. Elle ne depend d'aucune
// interface (ni serveur, ni navigateur) : c'est elle qui est mesuree par
// BenchmarkUpdateSimulation.
func (w *World) updateSimulation() {
	if w.Result != "" {
		return
	}
	w.Tick++
	w.updateWeather()
	w.stepAnts()
	w.stepFungus()
	w.stepPheromones()
	w.stepNest()
	w.countAll()
	if w.Tick%w.histEvery == 0 {
		w.recordHistory()
	}

	switch {
	case len(w.Ants) == 0 && len(w.Eggs) == 0:
		w.Result = "fungus"
	case float64(w.ForestLeft) <= float64(w.InitialForest)*VictoryForest/100:
		w.Result = "ants"
	}
}

// updateWeather : la pluie arrive au hasard et dure quelques centaines de ticks.
func (w *World) updateWeather() {
	if w.Raining {
		w.rainLeft--
		if w.rainLeft <= 0 {
			w.Raining = false
		}
	} else if w.rng.Float64() < RainChance {
		w.Raining = true
		w.rainLeft = 300 + w.rng.Intn(600)
	}
}

// countAll recompte tout en parcourant la grille et les fourmis.
func (w *World) countAll() {
	w.ForestLeft, w.Mushrooms, w.VenomousN, w.MyceliumN = 0, 0, 0, 0
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			w.ForestLeft += w.Food[y][x]
			switch w.Fungus[y][x].Kind {
			case Mushroom:
				w.Mushrooms++
			case Venomous:
				w.VenomousN++
			}
			if w.Mycelium[y][x] > 0 {
				w.MyceliumN++
			}
		}
	}

	w.Workers, w.Soldiers, w.Carrying = 0, 0, 0
	w.Infected, w.Zombies, w.ImmuneN = 0, 0, 0
	for _, a := range w.Ants {
		if a.Soldier {
			w.Soldiers++
		} else {
			w.Workers++
		}
		if a.Carry > 0 {
			w.Carrying++
		}
		if a.Virus > 0 {
			w.Infected++
		}
		if a.Zombie() {
			w.Zombies++
		}
		if a.Immune > 0 {
			w.ImmuneN++
		}
	}
}

func (w *World) recordHistory() {
	pct := 0.0
	if w.InitialForest > 0 {
		pct = 100 * float64(w.ForestLeft) / float64(w.InitialForest)
	}
	w.History = append(w.History, HistPoint{
		T: w.Tick, Workers: w.Workers, Soldiers: w.Soldiers,
		Mushrooms: w.Mushrooms, Venomous: w.VenomousN, Infected: w.Infected, ForestPct: pct, Stock: w.Stock,
	})
	// Historique plein : on garde un point sur deux et on espace les mesures.
	if len(w.History) >= HistoryMax {
		var kept []HistPoint
		for i := 0; i < len(w.History); i += 2 {
			kept = append(kept, w.History[i])
		}
		w.History = kept
		w.histEvery *= 2
	}
}

// --- Petits utilitaires ---

func (w *World) inside(x, y int) bool { return x >= 0 && y >= 0 && x < w.W && y < w.H }

// distNest : distance au nid le plus proche (on parcourt tous les nids).
func (w *World) distNest(x, y int) float64 {
	best := math.Inf(1)
	for _, n := range w.Nests {
		best = math.Min(best, dist(x, y, n.X, n.Y))
	}
	return best
}

func (w *World) inNest(x, y int) bool   { return w.distNest(x, y) <= NestRadius }
func (w *World) nearNest(x, y int) bool { return w.distNest(x, y) <= SafeRadius }

func dist(x1, y1, x2, y2 int) float64 {
	return math.Hypot(float64(x1-x2), float64(y1-y2))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func newGridInt(w, h int) [][]int {
	g := make([][]int, h)
	for y := range g {
		g[y] = make([]int, w)
	}
	return g
}

func newGridFloat(w, h int) [][]float64 {
	g := make([][]float64, h)
	for y := range g {
		g[y] = make([]float64, w)
	}
	return g
}
