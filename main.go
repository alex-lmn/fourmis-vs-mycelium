// Fourmis vs Mycelium : lance la simulation et le site web.
//
//	go run .
//	puis ouvrir http://localhost:8080
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	width := flag.Int("w", 600, "largeur de la carte")
	height := flag.Int("h", 400, "hauteur de la carte")
	ants := flag.Int("ants", 5000, "nombre de fourmis au depart")
	seed := flag.Int64("seed", 0, "graine aleatoire (0 = aleatoire)")
	addr := flag.String("addr", "localhost:8080", "adresse du serveur web")
	flag.Parse()

	if err := Validate(*width, *height, *ants); err != nil {
		log.Fatal(err)
	}

	s := NewServer(Config{W: *width, H: *height, Ants: *ants}, *seed)
	go s.Run()

	fmt.Printf("Simulation lancee : http://%s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, s.Routes()))
}
