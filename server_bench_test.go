package main

import "testing"

// BenchmarkPublish : cout d'un encodage de l'etat pour le navigateur,
// carte par defaut apres 200 ticks (comparaison V8 JSON / V11 binaire).
func BenchmarkPublish(b *testing.B) {
	s := NewServer(Config{W: 600, H: 400, Ants: 5000}, MasterSeed)
	for i := 0; i < 200; i++ {
		s.world.updateSimulation()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.publish()
	}
}
