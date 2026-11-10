// Package entrenamiento evoluciona el genoma de Gato Darwin con un algoritmo
// genético.
//
// Las piezas:
//
//	biblioteca.go  Repartos precalculados: cartas, fuerza por calle y puntaje final.
//	simulador.go   Una mesa de Hold'em No Limit con las mismas reglas que la arena.
//	rivales.go     Los rivales de práctica: conservador, aleatorio, maníaco...
//	genetico.go    Población, selección, cruce, mutación y elitismo.
//
// # Por qué una biblioteca de repartos
//
// Lo caro de simular poker no es apostar, es evaluar manos: estimar la fuerza
// de una mano por Monte Carlo cuesta miles de evaluaciones. Pero la fuerza
// depende solo de las cartas, no de cómo se apostó. Así que se calcula una
// vez por reparto y se guarda: después, cada uno de los miles de genomas que
// prueba el genético juega esos repartos sin evaluar una sola carta.
//
// Y de paso se consigue algo mejor: todos los genomas de una generación juegan
// exactamente las mismas cartas. Si uno gana más fichas que otro, es porque
// decidió mejor, no porque le tocaron ases (la misma idea de las "mismas
// cartas en la ida y en la vuelta" de la arena).
package entrenamiento

import (
	"encoding/gob"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"sync"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

// MaxAsientos es el tamaño de mesa más grande que se simula. Las bases del
// torneo usan mesas de 5 y algunas de 6.
const MaxAsientos = 6

// Reparto es una mano ya repartida para MaxAsientos jugadores, con todo lo que
// la simulación necesita saber de las cartas.
type Reparto struct {
	Privadas     [MaxAsientos]botsdk.Mano
	Comunitarias [5]botsdk.Carta
	// Fuerza[asiento][calle]: probabilidad de ganarle a una mano al azar con
	// lo que se ve en esa calle (0 preflop .. 3 river).
	Fuerza [MaxAsientos][4]float64
	// Puntaje es el valor de la mejor jugada final de cada asiento. Comparar
	// puntajes es decidir el showdown, igual que el crupier.
	Puntaje [MaxAsientos]int
}

// Biblioteca es una lista de repartos.
type Biblioteca []Reparto

// ConfigBiblioteca define cómo se genera una biblioteca.
type ConfigBiblioteca struct {
	Repartos int    // Cuántas manos distintas.
	Muestras int    // Muestras de Monte Carlo por asiento y calle.
	Semilla  uint64 // Determina las cartas: misma semilla, misma biblioteca.
}

// NuevaBiblioteca reparte y precalcula. Usa todos los núcleos: es la parte
// lenta del entrenamiento (del orden de segundos a un minuto).
func NuevaBiblioteca(cfg ConfigBiblioteca, progreso func(hechos, total int)) Biblioteca {
	biblioteca := make(Biblioteca, cfg.Repartos)

	// Las cartas se reparten en serie con un solo generador, para que la
	// biblioteca no dependa de cuántos núcleos tenga la máquina.
	azar := rand.New(rand.NewPCG(cfg.Semilla, cfg.Semilla^0x5EED))
	for i := range biblioteca {
		biblioteca[i] = repartir(azar)
	}

	trabajos := make(chan int)
	var espera sync.WaitGroup
	var mu sync.Mutex
	hechos := 0
	for w := 0; w < runtime.NumCPU(); w++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			for i := range trabajos {
				// Cada reparto tiene su propio generador: el resultado es el
				// mismo sin importar qué goroutine lo procese.
				azarReparto := rand.New(rand.NewPCG(cfg.Semilla+uint64(i), uint64(i)))
				precalcular(&biblioteca[i], cfg.Muestras, azarReparto)
				if progreso != nil {
					mu.Lock()
					hechos++
					progreso(hechos, len(biblioteca))
					mu.Unlock()
				}
			}
		}()
	}
	for i := range biblioteca {
		trabajos <- i
	}
	close(trabajos)
	espera.Wait()
	return biblioteca
}

func repartir(azar *rand.Rand) Reparto {
	mazo := make([]botsdk.Carta, 0, 52)
	for palo := botsdk.Treboles; palo <= botsdk.Picas; palo++ {
		for rango := botsdk.Dos; rango <= botsdk.As; rango++ {
			mazo = append(mazo, botsdk.Carta{Rango: rango, Palo: palo})
		}
	}
	azar.Shuffle(len(mazo), func(i, j int) { mazo[i], mazo[j] = mazo[j], mazo[i] })

	var r Reparto
	for a := 0; a < MaxAsientos; a++ {
		r.Privadas[a] = botsdk.Mano{mazo[2*a], mazo[2*a+1]}
	}
	copy(r.Comunitarias[:], mazo[2*MaxAsientos:2*MaxAsientos+5])
	return r
}

func precalcular(r *Reparto, muestras int, azar *rand.Rand) {
	// Cuántas comunitarias se ven en cada calle.
	visibles := [4]int{0, 3, 4, 5}
	for a := 0; a < MaxAsientos; a++ {
		r.Fuerza[a][0] = estrategia.FuerzaPreflop(r.Privadas[a])
		for calle := 1; calle < 4; calle++ {
			r.Fuerza[a][calle] = estrategia.Fuerza(r.Privadas[a], r.Comunitarias[:visibles[calle]], muestras, azar)
		}
		ev := poker.Mejor(r.Privadas[a], r.Comunitarias[:])
		r.Puntaje[a] = ev.Puntaje
	}
}

// --- Caché en disco ---------------------------------------------------------

// cabeceraCache identifica con qué parámetros se generó un archivo de caché:
// si no coinciden, el archivo no sirve y se regenera.
type cabeceraCache struct {
	Version int
	Config  ConfigBiblioteca
}

const versionCache = 1

// CargarOGenerar lee la biblioteca de `ruta` si fue generada con la misma
// configuración, y si no la genera y la guarda. Regenerarla en cada corrida
// del entrenamiento sería esperar lo mismo una y otra vez.
func CargarOGenerar(ruta string, cfg ConfigBiblioteca, progreso func(hechos, total int)) (Biblioteca, bool, error) {
	if ruta != "" {
		if b, err := cargar(ruta, cfg); err == nil {
			return b, true, nil
		}
	}
	b := NuevaBiblioteca(cfg, progreso)
	if ruta != "" {
		if err := guardar(ruta, cfg, b); err != nil {
			return b, false, fmt.Errorf("entrenamiento: no se pudo guardar la caché %s: %w", ruta, err)
		}
	}
	return b, false, nil
}

func cargar(ruta string, cfg ConfigBiblioteca) (Biblioteca, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)
	var cab cabeceraCache
	if err := dec.Decode(&cab); err != nil {
		return nil, err
	}
	if cab.Version != versionCache || cab.Config != cfg {
		return nil, fmt.Errorf("entrenamiento: la caché es de otra configuración")
	}
	var b Biblioteca
	if err := dec.Decode(&b); err != nil {
		return nil, err
	}
	return b, nil
}

func guardar(ruta string, cfg ConfigBiblioteca, b Biblioteca) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	if err := enc.Encode(cabeceraCache{Version: versionCache, Config: cfg}); err != nil {
		return err
	}
	return enc.Encode(b)
}
