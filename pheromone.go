// Pheromones : trois couches de valeurs sur toute la carte.
//   - nourriture : laissee par les ouvrieres qui rapportent de la foret
//   - maison     : laissee par les ouvrieres qui explorent
//   - danger     : laissee par les fourmis empoisonnees, les cadavres et les
//     champignons (les ouvrieres l'evitent, les soldates la suivent)
//
// A chaque tick, chaque couche s'evapore et se diffuse vers les cases
// voisines. La pluie efface les pheromones plus vite.
package main

const PherMax = 10.0

func (w *World) stepPheromones() {
	rain := 1.0
	if w.Raining {
		rain = 0.98
	}
	w.PherFood = w.diffuse(w.PherFood, 0.05, 0.985*rain)
	w.PherHome = w.diffuse(w.PherHome, 0.03, 0.993*rain)
	w.PherDanger = w.diffuse(w.PherDanger, 0.10, 0.96*rain)
}

// diffuse cree une nouvelle grille ou chaque case se rapproche de la
// moyenne de ses voisines, puis s'evapore.
func (w *World) diffuse(g [][]float64, rate, evap float64) [][]float64 {
	next := newGridFloat(w.W, w.H)
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			sum, n := 0.0, 0
			for d := 0; d < 8; d++ {
				nx, ny := x+dx8[d], y+dy8[d]
				if w.inside(nx, ny) {
					sum += g[ny][nx]
					n++
				}
			}
			v := (g[y][x]*(1-rate) + rate*sum/float64(n)) * evap
			if v < 0.001 {
				v = 0
			}
			next[y][x] = v
		}
	}
	return next
}
