package estrategia

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

// MuestrasPreflop es cuántas manos se simulan para estimar la fuerza de una
// clase de mano inicial. Se calcula una sola vez por clase y se guarda.
const MuestrasPreflop = 4000

// Fuerza estima por Monte Carlo la probabilidad de que `mano` le gane a UNA
// mano al azar, completando al azar las comunitarias que faltan. Un empate
// cuenta como media victoria.
//
// Se mide contra un solo rival a propósito: así el número depende solo de las
// cartas y no de la mesa, y el ajuste por cantidad de rivales lo hace el
// genoma (ver FuerzaEfectiva).
//
// Usa pkg/poker, el mismo evaluador del crupier: lo que el bot cree que gana
// es exactamente lo que la mesa va a puntuar.
func Fuerza(mano botsdk.Mano, comunitarias []botsdk.Carta, muestras int, azar *rand.Rand) float64 {
	return fuerzaHasta(mano, comunitarias, muestras, time.Time{}, azar)
}

// FuerzaConPlazo es Fuerza, pero corta antes si se acerca el límite. Es la
// que usa el bot en la arena, donde pasarse del plazo significa que la mesa
// juega por él.
func FuerzaConPlazo(mano botsdk.Mano, comunitarias []botsdk.Carta, muestras int, limite time.Time, azar *rand.Rand) float64 {
	if len(comunitarias) == 0 {
		return FuerzaPreflop(mano)
	}
	return fuerzaHasta(mano, comunitarias, muestras, limite, azar)
}

func fuerzaHasta(mano botsdk.Mano, comunitarias []botsdk.Carta, muestras int, limite time.Time, azar *rand.Rand) float64 {
	mazo := mazoSin(mano[:], comunitarias)
	faltan := 5 - len(comunitarias)

	// Buffers reutilizados en cada muestra: Monte Carlo es un bucle caliente y
	// asignar memoria en cada vuelta lo haría varias veces más lento.
	mia := make([]botsdk.Carta, 0, 7)
	suya := make([]botsdk.Carta, 0, 7)

	// Con la mesa completa nuestra jugada no cambia entre muestras.
	var fija int
	if faltan == 0 {
		ev, _ := poker.MejorDe(append(append(mia[:0], mano[:]...), comunitarias...))
		fija = ev.Puntaje
	}

	ganadas, jugadas := 0.0, 0
	for jugadas < muestras {
		if !limite.IsZero() && jugadas%64 == 0 && time.Now().After(limite) {
			break
		}
		// Fisher-Yates parcial: solo hace falta mezclar las primeras
		// 2 + faltan cartas del mazo.
		for i := 0; i < 2+faltan; i++ {
			j := i + azar.IntN(len(mazo)-i)
			mazo[i], mazo[j] = mazo[j], mazo[i]
		}
		rival := mazo[:2]
		resto := mazo[2 : 2+faltan]

		puntajeMio := fija
		if faltan > 0 {
			mia = append(append(append(mia[:0], mano[:]...), comunitarias...), resto...)
			ev, _ := poker.MejorDe(mia)
			puntajeMio = ev.Puntaje
		}
		suya = append(append(append(suya[:0], rival...), comunitarias...), resto...)
		ev, _ := poker.MejorDe(suya)

		switch {
		case puntajeMio > ev.Puntaje:
			ganadas++
		case puntajeMio == ev.Puntaje:
			ganadas += 0.5
		}
		jugadas++
	}
	if jugadas == 0 {
		return 0.5
	}
	return ganadas / float64(jugadas)
}

// mazoSin devuelve las cartas del mazo que no están en ninguna de las listas.
func mazoSin(listas ...[]botsdk.Carta) []botsdk.Carta {
	usadas := make(map[botsdk.Carta]bool, 7)
	for _, l := range listas {
		for _, c := range l {
			usadas[c] = true
		}
	}
	mazo := make([]botsdk.Carta, 0, 52)
	for palo := botsdk.Treboles; palo <= botsdk.Picas; palo++ {
		for rango := botsdk.Dos; rango <= botsdk.As; rango++ {
			c := botsdk.Carta{Rango: rango, Palo: palo}
			if !usadas[c] {
				mazo = append(mazo, c)
			}
		}
	}
	return mazo
}

// --- Preflop ----------------------------------------------------------------

// Antes del flop solo importan los rangos y si las cartas son del mismo palo:
// A♠K♠ y A♥K♥ valen lo mismo. Eso deja 169 clases de manos iniciales, y la
// fuerza de cada una se calcula una vez y se guarda.
var (
	cachePreflop   = make(map[string]float64, 169)
	cachePreflopMu sync.Mutex
)

// ClasePreflop devuelve el nombre canónico de una mano inicial: "AA", "AKs",
// "T9o"...
func ClasePreflop(mano botsdk.Mano) string {
	alta, baja := mano[0], mano[1]
	if baja.Rango > alta.Rango {
		alta, baja = baja, alta
	}
	letra := func(r botsdk.Rango) string { return string("--23456789TJQKA"[r]) }
	switch {
	case alta.Rango == baja.Rango:
		return letra(alta.Rango) + letra(baja.Rango)
	case alta.Palo == baja.Palo:
		return letra(alta.Rango) + letra(baja.Rango) + "s"
	default:
		return letra(alta.Rango) + letra(baja.Rango) + "o"
	}
}

// FuerzaPreflop es la fuerza de la clase de la mano, calculada la primera vez
// que aparece. La semilla sale del nombre de la clase, así que el valor es el
// mismo en el bot y en el entrenamiento: los dos ven el mismo número para AKs.
func FuerzaPreflop(mano botsdk.Mano) float64 {
	clase := ClasePreflop(mano)

	cachePreflopMu.Lock()
	f, ok := cachePreflop[clase]
	cachePreflopMu.Unlock()
	if ok {
		return f
	}

	h := fnv.New64a()
	fmt.Fprint(h, clase)
	semilla := h.Sum64()
	f = Fuerza(representante(clase), nil, MuestrasPreflop, rand.New(rand.NewPCG(semilla, semilla^0xA1C5)))

	cachePreflopMu.Lock()
	cachePreflop[clase] = f
	cachePreflopMu.Unlock()
	return f
}

// TablaPreflop calcula la fuerza de las 169 clases de manos iniciales (en
// paralelo) y la devuelve por nombre de clase. La genera el entrenamiento y el
// bot la incluye en el binario: calcularla de cero en cada partida le costaba
// unos segundos a cada proceso que lanza la arena.
func TablaPreflop() map[string]float64 {
	var clases []string
	letras := "23456789TJQKA"
	for i := len(letras) - 1; i >= 0; i-- {
		for j := i; j >= 0; j-- {
			if i == j {
				clases = append(clases, string(letras[i])+string(letras[j]))
				continue
			}
			clases = append(clases, string(letras[i])+string(letras[j])+"s", string(letras[i])+string(letras[j])+"o")
		}
	}

	var espera sync.WaitGroup
	for _, clase := range clases {
		espera.Add(1)
		go func() {
			defer espera.Done()
			FuerzaPreflop(representante(clase))
		}()
	}
	espera.Wait()

	cachePreflopMu.Lock()
	defer cachePreflopMu.Unlock()
	tabla := make(map[string]float64, len(cachePreflop))
	for clase, f := range cachePreflop {
		tabla[clase] = f
	}
	return tabla
}

// CargarTablaPreflop precarga la caché con una tabla ya calculada.
func CargarTablaPreflop(tabla map[string]float64) {
	cachePreflopMu.Lock()
	defer cachePreflopMu.Unlock()
	for clase, f := range tabla {
		cachePreflop[clase] = f
	}
}

// representante arma una mano concreta de la clase, siempre la misma, para
// que el cálculo no dependa de qué palos tocaron.
func representante(clase string) botsdk.Mano {
	rango := func(b byte) botsdk.Rango {
		for r := botsdk.Dos; r <= botsdk.As; r++ {
			if "--23456789TJQKA"[r] == b {
				return r
			}
		}
		return botsdk.Dos
	}
	alta := botsdk.Carta{Rango: rango(clase[0]), Palo: botsdk.Picas}
	baja := botsdk.Carta{Rango: rango(clase[1]), Palo: botsdk.Corazones}
	if len(clase) == 3 && clase[2] == 's' {
		baja.Palo = botsdk.Picas
	}
	return botsdk.Mano{alta, baja}
}
