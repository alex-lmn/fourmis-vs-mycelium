// Serveur web : fait tourner la simulation le plus vite possible et sert le
// dernier etat au navigateur.
//
// V11 :
//   - RCU : l'etat encode est publie dans un atomic.Pointer ; les clients le
//     lisent sans verrou, la simulation n'attend jamais le reseau ;
//   - decouplage : l'etat n'est encode qu'au plus 60 fois par seconde, pas a
//     chaque tick ;
//   - format binaire : statistiques en JSON + grilles a 4 bits par case +
//     fourmis sur 5 octets, au lieu d'un JSON texte d'environ 1 Mo ;
//   - tampon de construction recycle avec sync.Pool.
package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web
var webFS embed.FS

type Config struct {
	W, H int
	Ants int
}

// PublishEvery : intervalle minimum entre deux encodages (60 par seconde).
const PublishEvery = time.Second / 60

// Snapshot : statistiques envoyees en JSON en tete de chaque etat binaire.
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
}

// frame : un etat publie. Jamais modifie apres publication (RCU) : un
// lecteur peut l'envoyer pendant que la simulation en prepare un autre.
type frame struct {
	version uint64
	data    []byte
	next    chan struct{} // ferme quand une version plus recente est publiee
}

type Server struct {
	mu sync.Mutex // protege world, paused et cfg (simulation, pause, restart)

	cfg    Config
	world  *World
	seed   int64
	tps    float64
	paused bool

	current     atomic.Pointer[frame] // dernier etat publie, lu sans verrou
	version     uint64
	lastPublish time.Time
}

// buildPool : tampons de construction recycles d'un encodage a l'autre.
var buildPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

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
	s := &Server{}
	s.current.Store(&frame{next: make(chan struct{})})
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

// publish encode l'etat du monde au format binaire et le publie.
// A appeler avec s.mu verrouille.
//
// Format (petit-boutiste) :
//
//	uint32  taille du JSON des statistiques, puis ce JSON
//	4 grilles W*H a 4 bits par case (2 cases par octet) :
//	  cases (0 sol, 1..5 foret, 6 champignon, 7 venimeux, 8 nid, 9 mycelium),
//	  pheromones nourriture, maison, danger (intensite 0..9)
//	5 octets par fourmi : x uint16, y uint16, type uint8
func (s *Server) publish() {
	w := s.world
	buf := buildPool.Get().(*bytes.Buffer)
	buf.Reset()

	head, err := json.Marshal(Snapshot{
		Seed: s.seed, W: w.W, H: w.H, Tick: w.Tick, TPS: s.tps, Paused: s.paused, Result: w.Result,
		Raining: w.Raining,
		Ants:    len(w.Ants), Workers: w.Workers, Soldiers: w.Soldiers, Carrying: w.Carrying,
		Eggs: len(w.Eggs), Stock: w.Stock, Nests: w.Nests,
		Infected: w.Infected, Zombies: w.Zombies, Immune: w.ImmuneN,
		ForestLeft: w.ForestLeft, InitialForest: w.InitialForest,
		Mushrooms: w.Mushrooms, Venomous: w.VenomousN, Mycelium: w.MyceliumN,
		Stats: w.Stats, History: w.History,
	})
	if err != nil {
		buildPool.Put(buf)
		return
	}
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(head)))
	buf.Write(u32[:])
	buf.Write(head)

	level := func(v float64) byte { return byte(min(9, int(math.Ceil(v*1.5)))) }
	cell := func(x, y int) byte {
		switch {
		case w.inNest(x, y):
			return 8
		case w.Fungus[y][x].Kind == Mushroom:
			return 6
		case w.Fungus[y][x].Kind == Venomous:
			return 7
		case w.Food[y][x] > 0:
			return byte(w.Food[y][x])
		case w.Mycelium[y][x] > 0:
			return 9
		}
		return 0
	}
	n := w.W * w.H
	// Une grille a 4 bits par case : case paire dans les 4 bits faibles.
	packed := func(get func(i int) byte) {
		for i := 0; i < n; i += 2 {
			b := get(i)
			if i+1 < n {
				b |= get(i+1) << 4
			}
			buf.WriteByte(b)
		}
	}
	packed(func(i int) byte { return cell(i%w.W, i/w.W) })
	packed(func(i int) byte { return level(w.PherFood[i]) })
	packed(func(i int) byte { return level(w.PherHome[i]) })
	packed(func(i int) byte { return level(w.PherDanger[i]) })

	var rec [5]byte
	for _, a := range w.Ants {
		kind := byte(0)
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
		binary.LittleEndian.PutUint16(rec[0:], uint16(a.X))
		binary.LittleEndian.PutUint16(rec[2:], uint16(a.Y))
		rec[4] = kind
		buf.Write(rec[:])
	}

	// La trame publiee doit rester immuable : on copie le tampon recycle.
	data := bytes.Clone(buf.Bytes())
	buildPool.Put(buf)

	s.version++
	old := s.current.Load()
	s.current.Store(&frame{version: s.version, data: data, next: make(chan struct{})})
	close(old.next) // reveille les clients en attente d'une nouvelle version
	s.lastPublish = time.Now()
}

// Run : boucle principale. La simulation tourne sans pause ; l'etat n'est
// encode qu'au plus 60 fois par seconde (V11).
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
		if time.Since(s.lastPublish) >= PublishEvery {
			s.publish()
		}
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
	mux.HandleFunc("GET /state", s.handleState)
	mux.HandleFunc("POST /pause", s.handlePause)
	mux.HandleFunc("POST /restart", s.handleRestart)
	return mux
}

// handleState renvoie le dernier etat binaire, sans verrou (RCU). Si le
// client a deja la version courante (?v=...), on attend la suivante (au plus
// 1 seconde) au lieu de renvoyer la meme.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	f := s.current.Load()
	if v, err := strconv.ParseUint(r.URL.Query().Get("v"), 10, 64); err == nil && v == f.version {
		select {
		case <-f.next:
			f = s.current.Load()
		case <-time.After(time.Second):
		case <-r.Context().Done():
			return
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Version", strconv.FormatUint(f.version, 10))
	w.Write(f.data)
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
