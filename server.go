// Serveur web : fait tourner la simulation le plus vite possible et diffuse
// l'etat complet au navigateur via Server-Sent Events (SSE).
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

type Config struct {
	W, H int
	Ants int
}

// Snapshot : tout ce qui est envoye au navigateur a chaque tick.
type Snapshot struct {
	Seed   int64   `json:"seed"`
	W      int     `json:"w"`
	H      int     `json:"h"`
	Tick   int     `json:"tick"`
	TPS    float64 `json:"tps"`
	Paused bool    `json:"paused"`
	Result string  `json:"result"`

	Raining bool `json:"raining"`

	Ants          int     `json:"ants"`
	Workers       int     `json:"workers"`
	Soldiers      int     `json:"soldiers"`
	Carrying      int     `json:"carrying"`
	Eggs          int     `json:"eggs"`
	Nests         []*Nest `json:"nests"`
	Stock         float64 `json:"stock"`
	Infected      int     `json:"infected"`
	Zombies       int     `json:"zombies"`
	Immune        int     `json:"immune"`
	ForestLeft    int     `json:"forestLeft"`
	InitialForest int     `json:"initialForest"`
	Mushrooms     int     `json:"mushrooms"`
	Venomous      int     `json:"venomous"`
	Mycelium      int     `json:"mycelium"`
	Stats         Stats   `json:"stats"`

	History []HistPoint `json:"history"`

	// Un caractere par case, code 48 + valeur :
	//   cells    : 0 sol, 1..5 foret, 6 champignon, 7 venimeux, 8 nid, 9 mycelium
	//   phFood, phHome, phDanger : intensite 0..9
	Cells    string `json:"cells"`
	PhFood   string `json:"phFood"`
	PhHome   string `json:"phHome"`
	PhDanger string `json:"phDanger"`

	// 3 nombres par fourmi : x, y, type
	// (0 ouvriere, 1 ouvriere chargee, 2 soldate, 3 infectee, 4 infectee souche
	// venimeuse, 5 zombie, 6 immunisee)
	AntData []int `json:"antData"`
}

type Server struct {
	mu sync.Mutex

	cfg    Config
	world  *World
	seed   int64
	tps    float64
	paused bool

	latest []byte        // dernier snapshot encode en JSON
	notify chan struct{} // ferme puis recree a chaque nouveau snapshot
}

func Validate(w, h, ants int) error {
	if w < MinSize || w > MaxSize || h < MinSize || h > MaxSize {
		return fmt.Errorf("la largeur et la hauteur doivent etre entre %d et %d", MinSize, MaxSize)
	}
	if ants < 1 || ants > MaxStartAnts {
		return fmt.Errorf("le nombre de fourmis doit etre entre 1 et %d", MaxStartAnts)
	}
	return nil
}

func NewServer(cfg Config, seed int64) *Server {
	s := &Server{notify: make(chan struct{})}
	s.reset(seed, cfg)
	return s
}

// reset demarre une nouvelle partie. A appeler avec s.mu verrouille
// (ou avant le lancement du serveur).
func (s *Server) reset(seed int64, cfg Config) {
	if seed == 0 {
		// Seed courte : un int64 complet depasse la precision des nombres JS.
		seed = 1 + time.Now().UnixNano()%999_999_999
	}
	s.seed = seed
	s.cfg = cfg
	s.world = NewWorld(cfg.W, cfg.H, cfg.Ants, seed)
	s.publish()
}

// publish encode tout l'etat du monde et reveille les clients connectes.
// A appeler avec s.mu verrouille.
func (s *Server) publish() {
	w := s.world
	n := w.W * w.H
	var cells, phFood, phHome, phDanger strings.Builder
	cells.Grow(n)
	phFood.Grow(n)
	phHome.Grow(n)
	phDanger.Grow(n)

	level := func(v float64) byte {
		return byte('0' + min(9, int(math.Ceil(v*1.5))))
	}
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			var c int
			switch {
			case w.inNest(x, y):
				c = 8
			case w.Fungus[y][x].Kind == Mushroom:
				c = 6
			case w.Fungus[y][x].Kind == Venomous:
				c = 7
			case w.Food[y][x] > 0:
				c = w.Food[y][x]
			case w.Mycelium[y][x] > 0:
				c = 9
			}
			cells.WriteByte(byte('0' + c))
			phFood.WriteByte(level(w.PherFood[y*w.W+x]))
			phHome.WriteByte(level(w.PherHome[y*w.W+x]))
			phDanger.WriteByte(level(w.PherDanger[y*w.W+x]))
		}
	}

	ants := make([]int, 0, len(w.Ants)*3)
	for _, a := range w.Ants {
		kind := 0
		switch {
		case a.Zombie():
			kind = 5
		case a.Virus > 0 && a.Venom:
			kind = 4
		case a.Virus > 0:
			kind = 3
		case a.Immune > 0:
			kind = 6
		case a.Soldier:
			kind = 2
		case a.Carry > 0:
			kind = 1
		}
		ants = append(ants, a.X, a.Y, kind)
	}

	snap := Snapshot{
		Seed: s.seed, W: w.W, H: w.H, Tick: w.Tick, TPS: s.tps, Paused: s.paused, Result: w.Result,
		Raining: w.Raining,
		Ants:    len(w.Ants), Workers: w.Workers, Soldiers: w.Soldiers, Carrying: w.Carrying,
		Eggs: len(w.Eggs), Stock: w.Stock, Nests: w.Nests,
		Infected: w.Infected, Zombies: w.Zombies, Immune: w.ImmuneN,
		ForestLeft: w.ForestLeft, InitialForest: w.InitialForest,
		Mushrooms: w.Mushrooms, Venomous: w.VenomousN, Mycelium: w.MyceliumN,
		Stats: w.Stats, History: w.History,
		Cells: cells.String(), PhFood: phFood.String(), PhHome: phHome.String(), PhDanger: phDanger.String(),
		AntData: ants,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	s.latest = data
	close(s.notify)
	s.notify = make(chan struct{})
}

// Run : boucle principale, sans aucune pause : un tick puis on publie, en boucle.
func (s *Server) Run() {
	ticks := 0
	last := time.Now()

	for {
		s.mu.Lock()
		running := !s.paused && s.world.Result == ""
		if running {
			s.world.updateSimulation()
			ticks++
		}
		if el := time.Since(last); el >= time.Second {
			s.tps = float64(ticks) / el.Seconds()
			ticks = 0
			last = time.Now()
		}
		s.publish()
		s.mu.Unlock()

		if !running {
			time.Sleep(50 * time.Millisecond) // en pause : inutile de tourner a vide
		}
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("POST /pause", s.handlePause)
	mux.HandleFunc("POST /restart", s.handleRestart)
	return mux
}

// handleEvents envoie le dernier etat en flux SSE, au maximum ~60 fois/s.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming non supporte", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	for {
		s.mu.Lock()
		data, ch := s.latest, s.notify
		s.mu.Unlock()

		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		select {
		case <-r.Context().Done():
			return
		case <-time.After(16 * time.Millisecond):
		}
		select {
		case <-r.Context().Done():
			return
		case <-ch:
		}
	}
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.paused = !s.paused
	s.mu.Unlock()
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	seed, _ := strconv.ParseInt(r.FormValue("seed"), 10, 64) // vide ou invalide = aleatoire
	width, err1 := strconv.Atoi(r.FormValue("w"))
	height, err2 := strconv.Atoi(r.FormValue("h"))
	ants, err3 := strconv.Atoi(r.FormValue("ants"))
	if err1 != nil || err2 != nil || err3 != nil {
		http.Error(w, "w, h et ants doivent etre des nombres", http.StatusBadRequest)
		return
	}
	if err := Validate(width, height, ants); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.paused = false
	s.reset(seed, Config{W: width, H: height, Ants: ants})
	s.mu.Unlock()
}
