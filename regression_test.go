package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"testing"
)

/*
Test de non-regression : une optimisation ne doit pas changer le jeu.
   Meme graine -> meme partie : apres regressionTicks ticks, l'empreinte de
   l'etat du monde doit etre identique a celle de la version naive (V0).

	go test -run TestNonRegression
*/

const (
	regressionTicks       = 500
	regressionFingerprint = "dcd32e588bb620bd"
)

// fingerprint resume tout l'etat du monde : fourmis, grilles, reserve, stats.
func fingerprint(w *World) string {
	h := fnv.New64a()
	put := func(vals ...any) { fmt.Fprint(h, vals...) }

	put(w.Tick, w.Stock, len(w.Ants), len(w.Eggs), len(w.Nests), w.Stats)
	for _, a := range w.Ants {
		put(a.ID, a.X, a.Y, a.Dir, a.Carry, a.Age, a.Alarm, a.Immune, a.Resting,
			math.Float64bits(a.HP), math.Float64bits(a.Energy), math.Float64bits(a.Virus))
	}
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			f := w.Fungus[y][x]
			put(w.Food[y][x], f.Kind, f.Age, w.Mycelium[y][x],
				math.Float64bits(w.PherFood[y*w.W+x]), math.Float64bits(w.PherDanger[y*w.W+x]))
		}
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func TestNonRegression(t *testing.T) {
	w := NewWorld(320, 200, 1000, MasterSeed)
	for i := 0; i < regressionTicks; i++ {
		w.updateSimulation()
	}
	got := fingerprint(w)
	if got != regressionFingerprint {
		t.Fatalf("le jeu a change : empreinte %s, attendue %s (fourmis=%d champignons=%d)",
			got, regressionFingerprint, len(w.Ants), w.Mushrooms+w.VenomousN)
	}
}
