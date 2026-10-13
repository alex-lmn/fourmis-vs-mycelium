// Index spatial (V5, echec tente expres) : meme decoupage en cellules que la
// grille plate de V4, mais range dans une table de hachage map[int][]*Ant
// (cle = numero de cellule) qui contient des pointeurs vers les fourmis.
// La map est recreee a chaque tick, comme on l'ecrirait naturellement.
//
// Pour garder exactement la meme partie qu'avec les boucles naives :
//   - chaque fourmi connait son indice dans w.Ants (a.idx) ; quand l'ordre
//     compte (tirages aleatoires, egalites de distance), on le reconstitue ;
//   - l'index suit chaque deplacement (moveInGrid) ;
//   - les fourmis mortes restent dans l'index jusqu'au tri des survivantes.
package main

import "slices"

const CellSize = 8 // cote d'une cellule (en cases)

// rebuildGrid range toutes les fourmis dans leur cellule, dans l'ordre de
// w.Ants. A appeler quand w.Ants change (debut et fin de stepAnts).
// V5 : une nouvelle map a chaque appel, ses tranches grandissent a l'ajout.
func (w *World) rebuildGrid() {
	if w.gridW == 0 {
		w.gridW = (w.W + CellSize - 1) / CellSize
		w.gridH = (w.H + CellSize - 1) / CellSize
	}
	w.cellMap = make(map[int][]*Ant)
	for i, a := range w.Ants {
		a.idx = int32(i)
		a.cell = int32(w.cellOf(a.X, a.Y))
		w.cellMap[int(a.cell)] = append(w.cellMap[int(a.cell)], a)
	}
}

func (w *World) cellOf(x, y int) int { return (y/CellSize)*w.gridW + x/CellSize }

// moveInGrid met a jour la cellule d'une fourmi qui vient de bouger.
func (w *World) moveInGrid(a *Ant) {
	c := int32(w.cellOf(a.X, a.Y))
	if c == a.cell {
		return
	}
	old := w.cellMap[int(a.cell)]
	if i := slices.Index(old, a); i >= 0 {
		w.cellMap[int(a.cell)] = slices.Delete(old, i, i+1)
	}
	w.cellMap[int(c)] = append(w.cellMap[int(c)], a)
	a.cell = c
}

// near remplit w.nearBuf avec les indices des fourmis des cellules qui
// touchent le carre [x-r, x+r] x [y-r, y+r]. V5 : chaque cellule est
// cherchee dans la map (calcul de hash), puis on suit le pointeur de chaque
// fourmi pour lire son indice.
func (w *World) near(x, y, r int) []int32 {
	buf := w.nearBuf[:0]
	cx0, cy0 := max(0, x-r)/CellSize, max(0, y-r)/CellSize
	cx1, cy1 := min(w.W-1, x+r)/CellSize, min(w.H-1, y+r)/CellSize
	for cy := cy0; cy <= cy1; cy++ {
		for cx := cx0; cx <= cx1; cx++ {
			for _, a := range w.cellMap[cy*w.gridW+cx] {
				buf = append(buf, a.idx)
			}
		}
	}
	w.nearBuf = buf
	return buf
}
