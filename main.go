// Fourmis vs Mycelium : lance la simulation et le site web.
//
//	go run .
//	puis ouvrir http://localhost:8080
//
// Mode mesure, sans interface (utilise par hyperfine) :
//
//	go run . -headless -ticks 1000 -seed 42
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // profils pprof, exposes seulement sur le port d'administration (-pprof)
	"runtime"
	"slices"
	"time"
)

func main() {
	width := flag.Int("w", 600, "largeur de la carte")
	height := flag.Int("h", 400, "hauteur de la carte")
	ants := flag.Int("ants", 5000, "nombre de fourmis au depart")
	seed := flag.Int64("seed", 0, "graine aleatoire (0 = aleatoire)")
	addr := flag.String("addr", "localhost:8080", "adresse du serveur web")
	pprofAddr := flag.String("pprof", "", "adresse d'administration pprof, ex : localhost:6060 (vide = desactive)")
	headless := flag.Bool("headless", false, "sans serveur ni navigateur : joue -ticks ticks puis s'arrete")
	ticks := flag.Int("ticks", 1000, "nombre de ticks a jouer en mode -headless")
	flag.Parse()

	if err := Validate(*width, *height, *ants); err != nil {
		log.Fatal(err)
	}

	if *headless {
		runHeadless(*width, *height, *ants, *seed, *ticks)
		return
	}

	// Port d'administration separe : les profils ne sont jamais exposes sur
	// le port du jeu (le jeu utilise son propre routeur, pas celui par defaut).
	if *pprofAddr != "" {
		go func() {
			fmt.Printf("pprof : http://%s/debug/pprof/\n", *pprofAddr)
			log.Println(http.ListenAndServe(*pprofAddr, nil))
		}()
	}

	s := NewServer(Config{W: *width, H: *height, Ants: *ants}, *seed)
	go s.Run()

	fmt.Printf("Simulation lancee : http://%s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, s.Routes()))
}

// runHeadless joue une partie sans aucun affichage. Il chronometre chaque
// tick pour donner la latence P50 / P95 / P99 / max, et compte les passages
// du ramasse-miettes (GC) pendant la partie.
func runHeadless(w, h, ants int, seed int64, ticks int) {
	if seed == 0 {
		seed = 42
	}
	world := NewWorld(w, h, ants, seed)
	durations := make([]time.Duration, 0, ticks) // prealloue : pas d'allocation pendant la mesure

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	for i := 0; i < ticks && world.Result == ""; i++ {
		t0 := time.Now()
		world.updateSimulation()
		durations = append(durations, time.Since(t0))
	}
	el := time.Since(start)
	runtime.ReadMemStats(&after)

	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	pct := func(p float64) float64 {
		if len(sorted) == 0 {
			return 0
		}
		i := min(len(sorted)-1, int(float64(len(sorted))*p))
		return float64(sorted[i].Microseconds()) / 1000
	}

	n := max(1, len(durations))
	fmt.Printf("%d ticks en %v (%.2f ms/tick en moyenne) - fourmis=%d champignons=%d\n",
		world.Tick, el.Round(time.Millisecond), el.Seconds()*1000/float64(n),
		len(world.Ants), world.Mushrooms+world.VenomousN)
	fmt.Printf("latence par tick (ms) : P50=%.2f P95=%.2f P99=%.2f max=%.2f\n",
		pct(0.50), pct(0.95), pct(0.99), pct(1))
	fmt.Printf("GC : %d passages, pause totale %.2f ms, %.1f Mo alloues\n",
		after.NumGC-before.NumGC, float64(after.PauseTotalNs-before.PauseTotalNs)/1e6,
		float64(after.TotalAlloc-before.TotalAlloc)/1e6)
	// Memoire reellement demandee au systeme (tas + piles + runtime).
	fmt.Printf("RAM : %.1f Mo obtenus du systeme, tas en usage %.1f Mo\n",
		float64(after.Sys)/1e6, float64(after.HeapInuse)/1e6)
}
