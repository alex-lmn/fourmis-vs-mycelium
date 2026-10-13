// Grille spatiale (V4) : les fourmis sont rangees par cellule de CellSize x
// CellSize cases. Une recherche de voisines ne lit que les cellules qui
// touchent le carre de recherche, au lieu de parcourir toutes les fourmis.
//
// Pour garder exactement la meme partie qu'avec les boucles naives :
//   - chaque fourmi connait son indice dans w.Ants (a.idx) ; quand l'ordre
//     compte (tirages aleatoires, egalites de distance), on traite les
//     voisines par indice croissant, comme la boucle naive ;
//   - la grille suit chaque deplacement (moveInGrid), car une fourmi qui joue
//     voit les fourmis deja deplacees pendant le meme tick ;
//   - les fourmis mortes restent dans la grille jusqu'au tri des survivantes,
//     comme elles restaient dans w.Ants.
package main

import "slices"

const CellSize = 8 // cote d'une cellule (en cases)

// rebuildGrid range toutes les fourmis dans leur cellule, dans l'ordre de
// w.Ants. A appeler quand w.Ants change (debut et fin de stepAnts).
func (w *World) rebuildGrid() {
	if w.cells == nil {
		w.gridW = (w.W + CellSize - 1) / CellSize
		w.gridH = (w.H + CellSize - 1) / CellSize
		w.cells = make([][]int32, w.gridW*w.gridH)
	}
	for i := range w.cells {
		w.cells[i] = w.cells[i][:0] // on garde la capacite : pas d'allocation
	}
	for i, a := range w.Ants {
		a.idx = int32(i)
		a.cell = int32(w.cellOf(a.X, a.Y))
		w.cells[a.cell] = append(w.cells[a.cell], int32(i))
	}
}

func (w *World) cellOf(x, y int) int { return (y/CellSize)*w.gridW + x/CellSize }

// moveInGrid met a jour la cellule d'une fourmi qui vient de bouger.
func (w *World) moveInGrid(a *Ant) {
	c := int32(w.cellOf(a.X, a.Y))
	if c == a.cell {
		return // cas le plus frequent : on reste dans la meme cellule
	}
	old := w.cells[a.cell]
	if i := slices.Index(old, a.idx); i >= 0 {
		w.cells[a.cell] = slices.Delete(old, i, i+1)
	}
	w.cells[c] = append(w.cells[c], a.idx)
	a.cell = c
}

// near remplit w.nearBuf avec les indices des fourmis des cellules qui
// touchent le carre [x-r, x+r] x [y-r, y+r]. C'est un pre-filtre : l'appelant
// verifie ensuite la vraie distance. Les indices ne sont pas tries.
// Le resultat est valable jusqu'au prochain appel.
func (w *World) near(x, y, r int) []int32 {
	buf := w.nearBuf[:0]
	cx0, cy0 := max(0, x-r)/CellSize, max(0, y-r)/CellSize
	cx1, cy1 := min(w.W-1, x+r)/CellSize, min(w.H-1, y+r)/CellSize
	for cy := cy0; cy <= cy1; cy++ {
		for cx := cx0; cx <= cx1; cx++ {
			buf = append(buf, w.cells[cy*w.gridW+cx]...)
		}
	}
	w.nearBuf = buf
	return buf
}
