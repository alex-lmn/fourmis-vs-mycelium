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

// V3 (zero allocation) : les grilles sont des tableaux plats de W*H cases
// (case (x, y) a l'indice y*W+x). Un seul tampon de rechange, alloue une fois,
// recoit le resultat de la diffusion ; on echange ensuite les deux tableaux
// (double tampon). Plus aucune grille n'est creee pendant la partie.
func (w *World) stepPheromones() {
	rain := 1.0
	if w.Raining {
		rain = 0.98
	}
	w.diffuse(w.PherFood, w.pherSpare, 0.05, 0.985*rain)
	w.PherFood, w.pherSpare = w.pherSpare, w.PherFood
	w.diffuse(w.PherHome, w.pherSpare, 0.03, 0.993*rain)
	w.PherHome, w.pherSpare = w.pherSpare, w.PherHome
	w.diffuse(w.PherDanger, w.pherSpare, 0.10, 0.96*rain)
	w.PherDanger, w.pherSpare = w.pherSpare, w.PherDanger
}

// diffuse ecrit dans next la grille g ou chaque case se rapproche de la
// moyenne de ses voisines, puis s'evapore. Toutes les cases de next sont
// ecrites : pas besoin de la remettre a zero.
func (w *World) diffuse(g, next []float64, rate, evap float64) {
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			sum, n := 0.0, 0
			for d := 0; d < 8; d++ {
				nx, ny := x+dx8[d], y+dy8[d]
				if w.inside(nx, ny) {
					sum += g[ny*w.W+nx]
					n++
				}
			}
			v := (g[y*w.W+x]*(1-rate) + rate*sum/float64(n)) * evap
			if v < 0.001 {
				v = 0
			}
			next[y*w.W+x] = v
		}
	}
}
