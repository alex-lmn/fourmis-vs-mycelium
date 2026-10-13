#!/usr/bin/env bash
# Toutes les mesures de performance en une commande.
#
#   ./run_benchmarks.sh            mesure la version actuelle sous le nom "baseline"
#   ./run_benchmarks.sh v3         mesure sous le nom "v3" et la compare a la baseline
#
# Prerequis : go, benchstat (go install golang.org/x/perf/cmd/benchstat@latest)
# et hyperfine. Fermer le jeu et les applications lourdes avant de lancer.
#
# Resultats dans results/ :
#   bench_<nom>.txt        10 series du benchmark Go (100 ticks chacune)
#   benchstat_<nom>.txt    moyenne et dispersion calculees par benchstat
#   compare_<nom>.txt      comparaison avec la baseline (test de Mann-Whitney)
#   cpu_<nom>.pprof        profil CPU      (go tool pprof -http=:8081 results/cpu_<nom>.pprof)
#   mem_<nom>.pprof        profil memoire
#   pprof_top_<nom>.txt    fonctions les plus couteuses (CPU et memoire)
#   hyperfine_<nom>.md     mesure du binaire complet : moyenne, mediane, ecart-type
#   latency_<nom>.txt      latence par tick P50 / P95 / P99 / max et passages du GC
set -euo pipefail

LABEL="${1:-baseline}"
OUT="results"
mkdir -p "$OUT"

GOBIN="$(go env GOPATH)/bin"
BENCHSTAT="$(command -v benchstat || echo "$GOBIN/benchstat")"
HYPERFINE="$(command -v hyperfine || true)"
EXT="$(go env GOEXE)"

echo "== 1/4 Benchmark Go : 2 scenarios x 100 ticks x 10 series (MasterSeed = 42) =="
go test -run='^$' -bench=BenchmarkUpdateSimulation -benchmem -benchtime=100x -count=10 \
	| tee "$OUT/bench_$LABEL.txt"

echo "== 2/4 Statistiques benchstat =="
"$BENCHSTAT" "$OUT/bench_$LABEL.txt" | tee "$OUT/benchstat_$LABEL.txt"
if [ "$LABEL" != "baseline" ] && [ -f "$OUT/bench_baseline.txt" ]; then
	"$BENCHSTAT" "$OUT/bench_baseline.txt" "$OUT/bench_$LABEL.txt" | tee "$OUT/compare_$LABEL.txt"
fi

echo "== 3/4 Profils pprof (scenario Defaut) =="
go test -run='^$' -bench='BenchmarkUpdateSimulation/Defaut' -benchmem -benchtime=100x \
	-cpuprofile "$OUT/cpu_$LABEL.pprof" -memprofile "$OUT/mem_$LABEL.pprof" \
	-o "$OUT/bench$EXT" > /dev/null
{
	echo "### CPU"
	go tool pprof -top -nodecount=15 "$OUT/cpu_$LABEL.pprof"
	echo
	echo "### Memoire (octets alloues)"
	go tool pprof -top -nodecount=10 -sample_index=alloc_space "$OUT/mem_$LABEL.pprof"
} > "$OUT/pprof_top_$LABEL.txt" 2>&1
head -n 25 "$OUT/pprof_top_$LABEL.txt"

echo "== 4/4 hyperfine : binaire complet, 1000 ticks, 3 tours de chauffe + 10 mesures =="
go build -o "$OUT/jeudevie$EXT" .
if [ -n "$HYPERFINE" ]; then
	"$HYPERFINE" -N --warmup 3 --runs 10 \
		--export-markdown "$OUT/hyperfine_$LABEL.md" --export-json "$OUT/hyperfine_$LABEL.json" \
		"$OUT/jeudevie$EXT -headless -w 320 -h 200 -ants 1000 -ticks 1000 -seed 42"
else
	echo "hyperfine introuvable : etape ignoree"
fi

echo "== 5/5 Latence par tick (P50 / P95 / P99 / max) et GC =="
{
	echo "### Petite carte, 1000 ticks"
	"$OUT/jeudevie$EXT" -headless -w 320 -h 200 -ants 1000 -ticks 1000 -seed 42
	echo
	echo "### Carte par defaut, 500 ticks"
	"$OUT/jeudevie$EXT" -headless -w 600 -h 400 -ants 5000 -ticks 500 -seed 42
} | tee "$OUT/latency_$LABEL.txt"

echo "Termine. Resultats dans $OUT/"
