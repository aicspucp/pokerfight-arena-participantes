package entrenamiento

import (
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

// --- Escenarios: contra quién se mide la aptitud -----------------------------

// Escenario es una mesa de práctica: el genoma evaluado se sienta en ella junto
// a los rivales.
type Escenario struct {
	Nombre  string
	Rivales []estrategia.Politica
	Peso    float64
	// StackBB es la profundidad de los stacks, en ciegas grandes. Se mezclan
	// mesas de 100 y de 200 ciegas porque no se sabe con qué ciegas se va a
	// jugar el torneo, y la misma mano se juega distinto con más o menos
	// fichas detrás.
	StackBB int64
	// ManosTorneo > 0 convierte el escenario en un mini-torneo con
	// eliminación: las fichas NO se reponen entre manos y quien se queda sin
	// fichas queda afuera. Es el formato de la clasificatoria de las bases
	// (mesas de 5 o 6, rondas de 50 manos, eliminado al quedarse sin fichas).
	ManosTorneo int
}

// Escenarios arma las mesas de práctica. `campeones` son los mejores genomas
// de generaciones anteriores: sentarlos en la mesa hace que la población
// también tenga que ganarle a versiones previas de sí misma (coevolución), y
// no solo a los rivales fijos, que terminaría explotando de memoria.
//
// La mayor parte del peso está en los mini-torneos con eliminación. Medir solo
// con stacks que se reponen en cada mano premia jugarse el stack con manos
// medianas contra rivales que pagan todo: suma fichas en promedio, pero en un
// torneo de verdad el bot queda eliminado antes de cobrarlas.
func Escenarios(campeones []estrategia.Genoma) []Escenario {
	campeon := func(atras int) estrategia.Politica {
		if len(campeones) == 0 {
			return estrategia.GenomaInicial()
		}
		return campeones[max(len(campeones)-atras, 0)]
	}
	return []Escenario{
		// Heads-up contra cada estilo, con stacks que se reponen.
		{"1v1 conservador", []estrategia.Politica{Conservador{}}, 0.5, 200, 0},
		{"1v1 aleatorio", []estrategia.Politica{Aleatorio{}}, 0.5, 200, 0},
		{"1v1 base", []estrategia.Politica{Base{}}, 1.5, 200, 0},
		{"1v1 novato", []estrategia.Politica{Novato{}}, 1.5, 200, 0},
		{"1v1 roca", []estrategia.Politica{Roca{}}, 1.5, 100, 0},
		{"1v1 maniaco", []estrategia.Politica{Maniaco{}}, 0.5, 100, 0},
		{"1v1 estacion", []estrategia.Politica{Estacion{}}, 0.5, 100, 0},
		// Mini-torneos de 50 manos con eliminación, como la clasificatoria.
		{"torneo de 5 tight", []estrategia.Politica{Base{}, Novato{}, Roca{}, Conservador{}}, 3, 100, 50},
		{"torneo de 5 variado", []estrategia.Politica{Base{}, Novato{}, Aleatorio{}, Maniaco{}}, 3, 100, 50},
		{"torneo de 5 campeones", []estrategia.Politica{campeon(1), campeon(2), Roca{}, Novato{}}, 3, 100, 50},
		{"torneo de 6", []estrategia.Politica{Base{}, Novato{}, Roca{}, Estacion{}, Aleatorio{}}, 3, 200, 50},
	}
}

// --- Evaluación --------------------------------------------------------------

// Evaluacion es el resultado de un genoma en cada escenario, en ciegas grandes
// ganadas cada 100 manos (bb/100), la unidad habitual en poker.
type Evaluacion struct {
	Aptitud      float64   // Promedio ponderado de los escenarios.
	PorEscenario []float64 // bb/100 en cada escenario, en el orden de Escenarios.
	ManosJugadas int
}

// Evaluar mide al genoma en cada escenario, con repartos de la biblioteca a
// partir de la posición `desde`:
//
//   - En los escenarios de manos sueltas juega `repartos` manos, y cada una
//     tantas veces como sillas tenga la mesa, rotando quién recibe cada mano:
//     así el genoma juega TODAS las manos del reparto, las buenas y las malas,
//     y la suerte de las cartas se cancela.
//   - En los mini-torneos juega repartos/2 torneos, cada uno desde una silla
//     distinta. El resultado es lo que ganó o perdió al final del torneo,
//     expresado por cada 100 manos previstas: quedar eliminado en la mano 10
//     cuenta como perder el stack entero, no como un mal promedio por mano.
func Evaluar(heroe estrategia.Politica, escenarios []Escenario, bib Biblioteca, desde, repartos int, mesa ConfigMesa, semilla uint64) Evaluacion {
	ev := Evaluacion{PorEscenario: make([]float64, len(escenarios))}
	var suma, pesos float64

	for e, esc := range escenarios {
		mesaEsc := mesa
		if esc.StackBB > 0 {
			mesaEsc.Stack = esc.StackBB * mesa.CiegaGrande
		}
		n := len(esc.Rivales) + 1

		var neto int64
		var manosPrevistas, manosJugadas int
		if esc.ManosTorneo > 0 {
			for t := 0; t < max(repartos/2, 1); t++ {
				inicio := desde + t*esc.ManosTorneo
				resultado, jugadas := jugarTorneo(heroe, esc.Rivales, bib, inicio, esc.ManosTorneo, mesaEsc, t%n, mezclar(semilla, uint64(e), uint64(t)))
				neto += resultado
				manosJugadas += jugadas
				manosPrevistas += esc.ManosTorneo
			}
		} else {
			politicas := append([]estrategia.Politica{heroe}, esc.Rivales...)
			asientos := make([]int, n)
			for k := 0; k < repartos; k++ {
				rep := &bib[(desde+k)%len(bib)]
				boton := k % n
				for rot := 0; rot < n; rot++ {
					for i := range asientos {
						asientos[i] = (i + rot) % n
					}
					semillaMano := mezclar(semilla, uint64(e), uint64(k), uint64(rot))
					neto += JugarMano(mesaEsc, rep, politicas, asientos, nil, boton, semillaMano)[0]
					manosJugadas++
				}
			}
			manosPrevistas = manosJugadas
		}

		bb100 := float64(neto) / float64(mesa.CiegaGrande) / float64(max(manosPrevistas, 1)) * 100
		ev.PorEscenario[e] = bb100
		ev.ManosJugadas += manosJugadas
		suma += bb100 * esc.Peso
		pesos += esc.Peso
	}
	ev.Aptitud = suma / pesos
	return ev
}

// jugarTorneo juega un mini-torneo con eliminación y devuelve el neto final
// del héroe (en fichas) y cuántas manos se jugaron. Termina cuando se cumplen
// las manos, cuando queda uno solo o cuando el héroe queda eliminado: lo que
// pase después ya no cambia su resultado.
func jugarTorneo(heroe estrategia.Politica, rivales []estrategia.Politica, bib Biblioteca, desde, manos int, mesa ConfigMesa, sillaHeroe int, semilla uint64) (int64, int) {
	n := len(rivales) + 1
	sillas := make([]estrategia.Politica, 0, n)
	for i, r := 0, 0; i < n; i++ {
		if i == sillaHeroe {
			sillas = append(sillas, heroe)
			continue
		}
		sillas = append(sillas, rivales[r])
		r++
	}
	stacks := make([]int64, n)
	for i := range stacks {
		stacks[i] = mesa.Stack
	}

	boton, jugadas := 0, 0
	for h := 0; h < manos && stacks[sillaHeroe] > 0; h++ {
		// Se sientan solo los que tienen fichas. El botón va al primero vivo
		// desde la silla que le toca: las de los eliminados se saltean.
		var politicas []estrategia.Politica
		var enMano []int64
		var vivos, asientos []int
		botonMano := -1
		for i := 0; i < n; i++ {
			s := (boton + i) % n
			if stacks[s] > 0 {
				botonMano = s
				break
			}
		}
		for s := 0; s < n; s++ {
			if stacks[s] == 0 {
				continue
			}
			if s == botonMano {
				botonMano = len(vivos)
			}
			vivos = append(vivos, s)
			politicas = append(politicas, sillas[s])
			enMano = append(enMano, stacks[s])
			asientos = append(asientos, len(asientos))
		}
		if len(vivos) < 2 {
			break
		}

		neto := JugarMano(mesa, &bib[(desde+h)%len(bib)], politicas, asientos, enMano, botonMano, mezclar(semilla, uint64(h)))
		for i, s := range vivos {
			stacks[s] += neto[i]
		}
		jugadas++
		boton = (vivos[botonMano] + 1) % n
	}
	return stacks[sillaHeroe] - mesa.Stack, jugadas
}

// mezclar combina varios números en una semilla (splitmix64). Sirve para que
// la semilla de cada mano dependa de qué mano es y no del orden en que la
// juegue cada goroutine.
func mezclar(valores ...uint64) uint64 {
	var h uint64 = 0x9E3779B97F4A7C15
	for _, v := range valores {
		h ^= v + 0x9E3779B97F4A7C15 + (h << 6) + (h >> 2)
		h ^= h >> 30
		h *= 0xBF58476D1CE4E5B9
		h ^= h >> 27
		h *= 0x94D049BB133111EB
		h ^= h >> 31
	}
	return h
}

// --- Algoritmo genético ------------------------------------------------------

// ConfigGenetico son los parámetros de la evolución.
type ConfigGenetico struct {
	Poblacion    int     // Individuos por generación.
	Generaciones int     // Cuántas generaciones.
	Elite        int     // Los mejores pasan intactos a la siguiente.
	TamTorneo    int     // Participantes de cada torneo de selección.
	ProbCruce    float64 // Probabilidad de cruzar dos padres (si no, se copia uno).
	ProbMutacion float64 // Probabilidad de mutar cada gen.
	Sigma        float64 // Tamaño de la mutación, como fracción del rango del gen.
	Repartos     int     // Repartos por escenario en cada evaluación.
	Semilla      uint64
	Hilos        int
}

// Individuo es un genoma con su aptitud en la generación en la que se evaluó.
type Individuo struct {
	Genoma     estrategia.Genoma
	Evaluacion Evaluacion
}

// Generacion es el informe de una generación terminada.
type Generacion struct {
	Numero   int
	Mejor    Individuo
	Promedio float64
	Peor     float64
}

// Evolucionar corre el genético sobre la biblioteca de entrenamiento y
// devuelve la última población (ordenada de mejor a peor) y los campeones de
// cada generación.
//
// El esquema es el clásico:
//
//  1. Población inicial: el genoma escrito a mano y el resto al azar.
//  2. Evaluar a todos jugando las mismas manos (aptitud = bb/100).
//  3. Los Elite mejores pasan intactos.
//  4. El resto se genera eligiendo padres por torneo, cruzándolos (BLX-α) y
//     mutando cada gen con ruido gaussiano.
//  5. Repetir con manos nuevas.
//
// Cada generación juega repartos distintos de la biblioteca. Si fueran siempre
// los mismos, el genético aprendería esas manos en particular y no a jugar.
func Evolucionar(cfg ConfigGenetico, bib Biblioteca, mesa ConfigMesa, informar func(Generacion)) ([]Individuo, []estrategia.Genoma) {
	azar := rand.New(rand.NewPCG(cfg.Semilla, cfg.Semilla^0x6E6E))
	hilos := cfg.Hilos
	if hilos <= 0 {
		hilos = runtime.NumCPU()
	}

	poblacion := make([]Individuo, cfg.Poblacion)
	poblacion[0].Genoma = estrategia.GenomaInicial()
	for i := 1; i < len(poblacion); i++ {
		poblacion[i].Genoma = estrategia.Aleatorio(azar)
	}

	var campeones []estrategia.Genoma
	for gen := 0; gen < cfg.Generaciones; gen++ {
		escenarios := Escenarios(campeones)
		desde := azar.IntN(len(bib))
		semilla := azar.Uint64()

		evaluarTodos(poblacion, escenarios, bib, desde, cfg.Repartos, mesa, semilla, hilos)
		slices.SortStableFunc(poblacion, func(a, b Individuo) int {
			switch {
			case a.Evaluacion.Aptitud > b.Evaluacion.Aptitud:
				return -1
			case a.Evaluacion.Aptitud < b.Evaluacion.Aptitud:
				return 1
			}
			return 0
		})

		campeones = append(campeones, poblacion[0].Genoma)
		if informar != nil {
			informar(resumen(gen+1, poblacion))
		}
		if gen == cfg.Generaciones-1 {
			break
		}

		// La mutación se achica con las generaciones: al principio conviene
		// explorar lejos, al final afinar cerca de lo que ya funciona.
		progreso := float64(gen) / float64(max(cfg.Generaciones-1, 1))
		sigma := cfg.Sigma * (1 - 0.75*progreso)

		siguiente := make([]Individuo, 0, cfg.Poblacion)
		for i := 0; i < cfg.Elite && i < len(poblacion); i++ {
			siguiente = append(siguiente, Individuo{Genoma: poblacion[i].Genoma})
		}
		for len(siguiente) < cfg.Poblacion {
			padre := torneo(poblacion, cfg.TamTorneo, azar)
			madre := torneo(poblacion, cfg.TamTorneo, azar)
			hijo := padre
			if azar.Float64() < cfg.ProbCruce {
				hijo = cruzar(padre, madre, azar)
			}
			hijo = mutar(hijo, cfg.ProbMutacion, sigma, azar)
			siguiente = append(siguiente, Individuo{Genoma: hijo})
		}
		poblacion = siguiente
	}
	return poblacion, campeones
}

func evaluarTodos(poblacion []Individuo, escenarios []Escenario, bib Biblioteca, desde, repartos int, mesa ConfigMesa, semilla uint64, hilos int) {
	trabajos := make(chan int)
	var espera sync.WaitGroup
	for w := 0; w < hilos; w++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			for i := range trabajos {
				// Todos con la misma semilla y los mismos repartos: la
				// diferencia de aptitud entre dos genomas sale solo de sus
				// decisiones.
				poblacion[i].Evaluacion = Evaluar(poblacion[i].Genoma, escenarios, bib, desde, repartos, mesa, semilla)
			}
		}()
	}
	for i := range poblacion {
		trabajos <- i
	}
	close(trabajos)
	espera.Wait()
}

// torneo elige k individuos al azar y devuelve el genoma del mejor. Da más
// chances de reproducirse a los buenos sin condenar del todo a los demás,
// que pueden llevar genes útiles.
func torneo(poblacion []Individuo, k int, azar *rand.Rand) estrategia.Genoma {
	mejor := poblacion[azar.IntN(len(poblacion))]
	for i := 1; i < k; i++ {
		otro := poblacion[azar.IntN(len(poblacion))]
		if otro.Evaluacion.Aptitud > mejor.Evaluacion.Aptitud {
			mejor = otro
		}
	}
	return mejor.Genoma
}

// cruzar es el cruce BLX-α: cada gen del hijo sale al azar de un intervalo
// que cubre los genes de los dos padres y un poco más (α = 0,3 de cada lado).
// Ese "poco más" deja explorar fuera de lo que ya tienen los padres.
func cruzar(a, b estrategia.Genoma, azar *rand.Rand) estrategia.Genoma {
	const alfa = 0.3
	var hijo estrategia.Genoma
	for i := range hijo {
		bajo, alto := math.Min(a[i], b[i]), math.Max(a[i], b[i])
		d := alto - bajo
		hijo[i] = bajo - alfa*d + azar.Float64()*(1+2*alfa)*d
	}
	return hijo.Acotar()
}

// mutar suma ruido gaussiano a cada gen con probabilidad p. El ruido es
// proporcional al rango del gen: un umbral (0..1) y un tamaño de apertura
// (2..5 ciegas) no se pueden mover con el mismo paso.
func mutar(g estrategia.Genoma, p, sigma float64, azar *rand.Rand) estrategia.Genoma {
	for i, def := range estrategia.Genes {
		if azar.Float64() < p {
			g[i] += azar.NormFloat64() * sigma * (def.Max - def.Min)
		}
	}
	return g.Acotar()
}

func resumen(numero int, poblacion []Individuo) Generacion {
	g := Generacion{Numero: numero, Mejor: poblacion[0], Peor: poblacion[len(poblacion)-1].Evaluacion.Aptitud}
	for _, ind := range poblacion {
		g.Promedio += ind.Evaluacion.Aptitud
	}
	g.Promedio /= float64(len(poblacion))
	return g
}

// --- Validación ----------------------------------------------------------------

// ElegirCampeon reevalúa a los candidatos con repartos que el genético nunca
// vio y devuelve el mejor. Sin este paso, el "mejor" de la última generación
// puede ser simplemente el que tuvo más suerte en esa tanda de manos.
func ElegirCampeon(candidatos []estrategia.Genoma, campeones []estrategia.Genoma, validacion Biblioteca, repartos int, mesa ConfigMesa, semilla uint64) (estrategia.Genoma, Evaluacion) {
	escenarios := Escenarios(campeones)
	resultados := make([]Evaluacion, len(candidatos))
	var espera sync.WaitGroup
	for i := range candidatos {
		espera.Add(1)
		go func(i int) {
			defer espera.Done()
			resultados[i] = Evaluar(candidatos[i], escenarios, validacion, 0, repartos, mesa, semilla)
		}(i)
	}
	espera.Wait()

	mejor := 0
	for i := range resultados {
		if resultados[i].Aptitud > resultados[mejor].Aptitud {
			mejor = i
		}
	}
	return candidatos[mejor], resultados[mejor]
}
