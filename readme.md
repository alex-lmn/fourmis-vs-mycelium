# Fourmis vs Mycélium

Simulation en Go : une colonie de fourmis doit manger toute la forêt pendant
qu'un champignon venimeux se propage et les empoisonne. Le rendu se fait dans
le navigateur.

Le code est volontairement **naïf, sans aucune optimisation**.

## Lancer le projet

```bash
go run .
```

Puis ouvrir http://localhost:8080

Options : `-w 600 -h 400` (taille de la carte, jusqu'à 2000×2000),
`-ants 5000` (fourmis au départ, jusqu'à 50 000), `-seed 42` (rejouer
exactement la même partie), `-addr localhost:8080`.

Pour une partie légère : `go run . -w 320 -h 200 -ants 1000`.

## Victoire

- **Les fourmis gagnent** quand il reste **2 % de forêt ou moins** (les
  dernières cases sont souvent sous des champignons, impossibles à récolter).
- **Le mycélium gagne** quand il n'y a plus aucune fourmi ni aucun œuf.

## Règles

### Fourmis
- Deux castes : les **ouvrières** récoltent la forêt, les **soldates** chassent
  les champignons.
- Une ouvrière récolte la forêt à côté d'elle (2 unités par voyage) et la
  ramène au nid le plus proche en laissant une **phéromone nourriture**. En
  explorant, elle laisse une **phéromone maison**.
- Une ouvrière qui ne voit plus de forêt part vers une forêt lointaine dont la
  **colonie se souvient** (la colonie repère des cases de forêt sur toute la
  carte tous les 100 ticks).
- Les fourmis peuvent **marcher sur les champignons** et **récolter la forêt
  en dessous**, mais elles risquent d'attraper le virus.
- Chaque fourmi consomme de l'énergie. Au nid, elle dépose sa récolte, mange
  dans la réserve et se fait soigner du virus.
- Une fourmi affamée **mange sur place** ce qu'elle récolte. Si la réserve est
  vide, elle ne rentre pas au nid pour rien : elle part chercher de la forêt
  (les soldates aussi).
- Une ouvrière **mourante de faim** (énergie < 10) mange un champignon en
  dernier recours : elle récupère de l'énergie mais attrape le virus.
- Une soldate qui touche un champignon l'attaque, et risque d'attraper le virus
  à chaque coup. Sa **carapace** divise par 2 ses risques de contamination par
  les champignons.
- Les **soldates chassent les zombies** en priorité et les éliminent proprement
  (ni nuage toxique, ni champignon).
- **Croque-mortes** : une fourmi morte près d'un nid est évacuée, son cadavre ne
  libère ni nuage toxique, ni champignon, ni spores.
- Une fourmi infectée donne l'**alarme** : les fourmis proches sont alertées,
  les ouvrières fuient le danger et les soldates y vont.
- Quand il y a trop de fourmis au même endroit, elles s'écartent.

### Virus
Le poison des champignons est un **virus contagieux**.
- Une fourmi est contaminée en mangeant un champignon, en l'attaquant ou en
  passant tout près. Deux souches : **normale** et **venimeuse** (deux fois
  plus rapide et deux fois plus contagieuse).
- La **charge virale** grandit à chaque tick et fait perdre des points de vie,
  de plus en plus vite.
- **Contagion** : une fourmi infectée peut contaminer chaque fourmi collée à
  elle, à chaque tick (1 % de risque, 2 % pour la souche venimeuse).
- **Toilettage social** : une fourmi peu infectée est nettoyée par les sœurs
  saines qui la touchent (jusqu'à 3 à la fois) ; elle peut guérir avant de
  tomber malade.
- **Malade** (charge virale > 20 %), elle arrête de travailler, rentre au nid et y reste jusqu'à
  guérison complète. **Quarantaine** : tant qu'elle se repose au nid, elle ne
  contamine personne.
- **Guérie**, elle est immunisée pendant 1500 ticks, mais seulement contre la
  contagion entre fourmis : **les champignons restent toujours toxiques**.
- Trop infectée, elle devient **zombie** (comme avec le cordyceps) : elle ne
  peut plus guérir, **chasse les fourmis saines** autour d'elle (sans proie,
  elle erre au hasard), n'approche pas des nids et contamine deux fois plus.
- **Une fourmi infectée qui meurt loin des nids** libère un **nuage toxique**
  (toutes les fourmis à 2 cases sont contaminées, même immunisées), puis **un
  champignon pousse sur son cadavre** (venimeux pour la souche venimeuse) et
  une salve de spores s'envole.

### Colonie (respawn)
- **Chaque nid a sa reine** : chacune pond un œuf tous les 4 ticks quand la
  réserve le permet. Les reines gardent une réserve pour nourrir les fourmis
  déjà nées (0,3 unité par fourmi). L'œuf **éclot après 300 ticks** : c'est le
  temps de respawn d'une fourmi.
- Plus il y a de champignons, plus la colonie fait naître de soldates (de 12 %
  à 40 %).
- Une colonie riche **fonde de nouveaux nids** (8 au maximum) loin des autres,
  là où ses ouvrières trouvent de la forêt. La réserve est commune à tous les
  nids.

### Champignons
- Deux espèces : **normal** (marron) et **venimeux** (violet). Ils ne bloquent
  pas le passage, mais leur contact est dangereux.
- Un champignon adulte lâche des **spores** autour de lui. Elles germent mieux
  sur la forêt que sur le sol nu, et sont écrasées si une fourmi passe juste à
  côté. Trois fois plus de spores sous la pluie.
- Un venimeux donne souvent des petits venimeux ; un normal peut muter.
- **Règle du jeu de la vie** : une case vide entourée d'exactement 3
  champignons peut voir naître un champignon ; un champignon entouré de 5
  voisins ou plus meurt étouffé.
- Un champignon vieillit et meurt en lâchant ses dernières spores.
- **Aura** : un champignon peut contaminer les fourmis qui passent tout près
  (rayon 2 et risque plus fort pour un venimeux).
- Un champignon détruit laisse souvent du **mycélium** dans le sol : il
  **repousse après 600 à 1200 ticks** (respawn), sauf si une fourmi est dessus.
- Aucun champignon ne pousse près d'un nid.

### Monde
- **Pluie** aléatoire : spores ×3 et phéromones effacées plus vite.
- Les phéromones s'évaporent et se diffusent à chaque tick.

## Organisation du code

| Fichier | Rôle |
|---|---|
| `world.go` | carte, génération de la forêt, météo, statistiques |
| `ant.go` | comportement des fourmis, nids, ponte |
| `fungus.go` | propagation des champignons |
| `virus.go` | virus contagieux, soins, zombies |
| `pheromone.go` | évaporation et diffusion des phéromones |
| `server.go` | serveur web, envoi de l'état au navigateur (SSE) |
| `web/index.html` | affichage de la carte et des statistiques |

## Ce qui est volontairement lent (pistes d'optimisation)

- Chaque fourmi est comparée à **toutes** les autres (foule, alarme,
  contagion du virus, toilettage, chasse des zombies).
- La mémoire de la colonie parcourt toute la grille tous les 100 ticks.
- Chaque champignon est comparé à **toutes** les fourmis (aura).
- Chaque spore et chaque repousse parcourt toutes les fourmis.
- Les 3 grilles de phéromones sont **réallouées** à chaque tick.
- La grille est entièrement recomptée à chaque tick.
- Tout l'état (4 grilles + toutes les fourmis + l'historique) est encodé en
  JSON **à chaque tick**, même si le navigateur n'en affiche qu'une partie.
- Le navigateur redessine toute la carte à chaque image.
