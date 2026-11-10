// Comando entrenar-darwin: evoluciona el genoma de Gato Darwin y lo guarda en
// bots/darwin/genoma.json, que el bot incluye al compilarse.
//
// Desde la raíz del repositorio:
//
//	go run ./bots/darwin/cmd/entrenar-darwin                     # por defecto (~1 min con la caché)
//	go run ./bots/darwin/cmd/entrenar-darwin -generaciones 150   # más largo
//	go run ./bots/darwin/cmd/entrenar-darwin -rapido             # prueba de humo en segundos
//
// Después hay que recompilar para que el bot tome el genoma nuevo:
//
//	make build
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/entrenamiento"
	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

func main() {
	log.SetFlags(0)

	poblacion := flag.Int("poblacion", 60, "individuos por generación")
	generaciones := flag.Int("generaciones", 80, "cantidad de generaciones")
	repartos := flag.Int("repartos", 60, "repartos por escenario en cada evaluación (cada uno se juega rotando las cartas por todas las sillas)")
	tamBiblioteca := flag.Int("biblioteca", 4000, "repartos precalculados (el 80% entrena, el 20% valida)")
	muestras := flag.Int("muestras", 300, "muestras de Monte Carlo por calle al precalcular la fuerza")
	stackBB := flag.Int64("stack-bb", 100, "stack inicial de cada mano, en ciegas grandes")
	semilla := flag.Uint64("semilla", 2026, "semilla: misma semilla, mismo resultado")
	dir := flag.String("dir", dirDelBot(), "carpeta del bot: ahí se escriben el genoma, la tabla preflop, la caché y el CSV")
	salida := flag.String("salida", "genoma.json", "dónde guardar el genoma campeón (relativo a -dir)")
	rutaPreflop := flag.String("preflop", "preflop.json", "dónde guardar la tabla de fuerza preflop que incluye el bot (relativo a -dir)")
	rutaCache := flag.String("cache", "biblioteca.cache", "caché de la biblioteca precalculada, relativa a -dir (vacío para no usar)")
	rutaCSV := flag.String("csv", "entrenamiento.csv", "historia de la evolución, una fila por generación, relativa a -dir (vacío para no guardar)")
	rapido := flag.Bool("rapido", false, "entrenamiento corto, para probar que todo funciona. Escribe en una carpeta temporal (salvo que pases -dir) para no pisar el genoma entrenado del bot")
	flag.Parse()

	// La prueba de humo produce un genoma malo a propósito (pocas
	// generaciones): no debe reemplazar el que trae el bot. Si no se pidió una
	// carpeta explícita, va a una temporal.
	if *rapido && !flagExplicito("dir") {
		tmp, err := os.MkdirTemp("", "entrenar-darwin-rapido-")
		if err != nil {
			log.Fatalf("no se pudo crear la carpeta temporal: %v", err)
		}
		*dir = tmp
		fmt.Printf("modo -rapido: los archivos van a %s (el genoma del bot no se toca)\n", tmp)
	}

	*salida, *rutaPreflop = enDir(*dir, *salida), enDir(*dir, *rutaPreflop)
	*rutaCache, *rutaCSV = enDir(*dir, *rutaCache), enDir(*dir, *rutaCSV)

	if *rapido {
		*poblacion, *generaciones, *repartos, *tamBiblioteca, *muestras = 16, 8, 12, 600, 100
	}

	mesa := entrenamiento.ConfigMesa{CiegaChica: 10, CiegaGrande: 20, Stack: 20 * *stackBB}
	inicio := time.Now()

	// --- 1. Biblioteca de repartos ------------------------------------------
	cfgBib := entrenamiento.ConfigBiblioteca{Repartos: *tamBiblioteca, Muestras: *muestras, Semilla: *semilla}
	ultimoAviso := time.Now()
	bib, deCache, err := entrenamiento.CargarOGenerar(*rutaCache, cfgBib, func(hechos, total int) {
		if time.Since(ultimoAviso) > time.Second || hechos == total {
			fmt.Printf("\r  precalculando repartos: %d/%d", hechos, total)
			ultimoAviso = time.Now()
		}
	})
	if err != nil {
		log.Printf("aviso: %v", err)
	}
	if deCache {
		fmt.Printf("biblioteca: %d repartos leídos de %s\n", len(bib), *rutaCache)
	} else {
		fmt.Printf("\nbiblioteca: %d repartos precalculados en %s\n", len(bib), time.Since(inicio).Round(time.Second))
	}

	corte := len(bib) * 8 / 10
	entreno, validacion := bib[:corte], bib[corte:]

	// --- 2. Evolución ---------------------------------------------------------
	cfg := entrenamiento.ConfigGenetico{
		Poblacion:    *poblacion,
		Generaciones: *generaciones,
		Elite:        max(2, *poblacion/20),
		TamTorneo:    3,
		ProbCruce:    0.9,
		ProbMutacion: 0.15,
		Sigma:        0.12,
		Repartos:     *repartos,
		Semilla:      *semilla,
	}
	fmt.Printf("evolucionando: población %d, %d generaciones, %d escenarios\n\n",
		cfg.Poblacion, cfg.Generaciones, len(entrenamiento.Escenarios(nil)))
	fmt.Println("  gen      mejor   promedio       peor   (bb/100)")

	var historia [][]string
	inicioEvolucion := time.Now()
	poblacionFinal, campeones := entrenamiento.Evolucionar(cfg, entreno, mesa, func(g entrenamiento.Generacion) {
		fmt.Printf("  %3d  %9.1f  %9.1f  %9.1f   %s\n", g.Numero, g.Mejor.Evaluacion.Aptitud, g.Promedio, g.Peor, barra(g.Mejor.Evaluacion.Aptitud))
		historia = append(historia, []string{
			strconv.Itoa(g.Numero),
			strconv.FormatFloat(g.Mejor.Evaluacion.Aptitud, 'f', 2, 64),
			strconv.FormatFloat(g.Promedio, 'f', 2, 64),
			strconv.FormatFloat(g.Peor, 'f', 2, 64),
		})
	})
	fmt.Printf("\nevolución terminada en %s\n", time.Since(inicioEvolucion).Round(time.Second))

	// --- 3. Validación con manos nunca vistas ----------------------------------
	candidatos := []estrategia.Genoma{}
	for i := 0; i < len(poblacionFinal) && i < 6; i++ {
		candidatos = append(candidatos, poblacionFinal[i].Genoma)
	}
	if len(campeones) > 0 {
		// Los campeones de las últimas generaciones también compiten: a veces
		// el mejor de verdad quedó segundo en la última tanda por mala suerte.
		desde := max(len(campeones)-4, 0)
		candidatos = append(candidatos, campeones[desde:]...)
	}
	repartosValidacion := min(len(validacion), *repartos*3)
	campeon, evCampeon := entrenamiento.ElegirCampeon(candidatos, campeones, validacion, repartosValidacion, mesa, *semilla+1)
	_, evInicial := entrenamiento.ElegirCampeon([]estrategia.Genoma{estrategia.GenomaInicial()}, campeones, validacion, repartosValidacion, mesa, *semilla+1)

	fmt.Printf("\nvalidación (%d manos nunca vistas por escenario):\n\n", evCampeon.ManosJugadas/len(evCampeon.PorEscenario))
	fmt.Printf("  %-22s %14s %14s\n", "escenario", "genoma a mano", "campeón")
	for i, esc := range entrenamiento.Escenarios(campeones) {
		fmt.Printf("  %-22s %14.1f %14.1f\n", esc.Nombre, evInicial.PorEscenario[i], evCampeon.PorEscenario[i])
	}
	fmt.Printf("  %-22s %14.1f %14.1f   (bb/100, ponderado)\n", "APTITUD", evInicial.Aptitud, evCampeon.Aptitud)

	// --- 4. Guardar ----------------------------------------------------------------
	archivo := estrategia.NuevoArchivo(campeon, cfg.Generaciones, evCampeon.Aptitud, *semilla)
	if err := archivo.Guardar(*salida); err != nil {
		log.Fatalf("no se pudo guardar %s: %v", *salida, err)
	}
	fmt.Printf("\ngenoma campeón guardado en %s\n", *salida)
	for i, def := range estrategia.Genes {
		fmt.Printf("  %-20s %7.3f   %s\n", def.Nombre, campeon[i], def.Descripcion)
	}

	if *rutaCSV != "" {
		if err := guardarCSV(*rutaCSV, historia); err != nil {
			log.Printf("aviso: no se pudo guardar %s: %v", *rutaCSV, err)
		}
	}

	// La tabla preflop sale de las mismas semillas que usó el entrenamiento:
	// el bot ve exactamente los números con los que se evolucionó el genoma.
	if err := guardarTablaPreflop(*rutaPreflop); err != nil {
		log.Printf("aviso: no se pudo guardar %s: %v", *rutaPreflop, err)
	}

	if *rapido && !flagExplicito("dir") {
		fmt.Printf("\ntotal: %s. Prueba de humo correcta; el genoma del bot quedó intacto.\n", time.Since(inicio).Round(time.Second))
		return
	}
	fmt.Printf("\ntotal: %s. Recompila para que el bot use el genoma nuevo:\n  make build   (o: go build -o bin/ ./bots/...)\n", time.Since(inicio).Round(time.Second))
}

func guardarTablaPreflop(ruta string) error {
	datos, err := json.MarshalIndent(estrategia.TablaPreflop(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}

// barra dibuja la aptitud como una barrita de texto, para ver la curva de la
// evolución en la terminal sin graficar nada.
func barra(aptitud float64) string {
	largo := int(aptitud / 25)
	if largo < 0 {
		return "-"
	}
	if largo > 40 {
		largo = 40
	}
	b := make([]rune, largo)
	for i := range b {
		b[i] = '#'
	}
	return string(b)
}

func guardarCSV(ruta string, filas [][]string) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"generacion", "mejor_bb100", "promedio_bb100", "peor_bb100"})
	w.WriteAll(filas)
	return w.Error()
}

// dirDelBot es la carpeta donde el bot espera sus archivos. Se corre casi
// siempre desde la raíz del repositorio (go run ./bots/darwin/cmd/...), y ahí
// escribir genoma.json en el directorio actual lo dejaría fuera del bot: el
// binario seguiría incluyendo el genoma viejo sin avisar.
func dirDelBot() string {
	enRepo := filepath.Join("bots", "darwin")
	if _, err := os.Stat(filepath.Join(enRepo, "genoma.json")); err == nil {
		return enRepo
	}
	return "."
}

// enDir resuelve una ruta relativa contra la carpeta del bot. Las absolutas y
// las vacías (que apagan la caché o el CSV) quedan como están.
func enDir(dir, ruta string) string {
	if ruta == "" || filepath.IsAbs(ruta) {
		return ruta
	}
	return filepath.Join(dir, ruta)
}

// flagExplicito dice si el flag se pasó en la línea de comandos (y no quedó
// con su valor por defecto).
func flagExplicito(nombre string) bool {
	visto := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == nombre {
			visto = true
		}
	})
	return visto
}
