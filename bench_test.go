package main

import "testing"

// Banc d'essai deterministe de updateSimulation(), sans serveur ni navigateur.
//
//   - 1 operation = 1 tick : ns/op, B/op et allocs/op sont des valeurs par tick.
//   - MasterSeed fixe : chaque partie mesuree est identique d'un lancement a
//     l'autre.
//   - Le monde est cree hors chronometre (b.ResetTimer), puis recree tous les
//     benchTicks ticks : on mesure toujours les ticks 1 a benchTicks.
//
// Commande de reference (100 ticks, 10 repetitions pour benchstat) :
//
//	go test -run=^$ -bench=BenchmarkUpdateSimulation -benchmem -benchtime=100x -count=10

const (
	MasterSeed = 42
	benchTicks = 100
)

// BenchmarkSink : resultat ecrit dans une variable globale pour que le
// compilateur ne puisse pas supprimer le calcul (piege du Dead Code Elimination).
var BenchmarkSink int

var benchScenarios = []struct {
	name       string
	w, h, ants int
}{
	{"Petite_320x200_1000f", 320, 200, 1000},
	{"Defaut_600x400_5000f", 600, 400, 5000}, // reglage par defaut du jeu
}

func BenchmarkUpdateSimulation(b *testing.B) {
	for _, sc := range benchScenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			w := NewWorld(sc.w, sc.h, sc.ants, MasterSeed)
			b.ResetTimer() // la creation du monde n'est pas mesuree

			for i := 0; i < b.N; i++ {
				if i > 0 && i%benchTicks == 0 {
					b.StopTimer()
					w = NewWorld(sc.w, sc.h, sc.ants, MasterSeed)
					b.StartTimer()
				}
				w.updateSimulation()
			}
			b.StopTimer()
			BenchmarkSink = w.Tick + len(w.Ants)

			msPerTick := b.Elapsed().Seconds() * 1000 / float64(b.N)
			b.ReportMetric(msPerTick, "ms/tick")
			b.ReportMetric(1000/msPerTick, "TPS")
		})
	}
}
