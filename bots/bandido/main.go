// Comando bandido: Gato Bandido, un bot que aprende mientras juega.
//
// La técnica es un bandido multibrazo (multi-armed bandit), la forma más
// simple de aprendizaje por refuerzo. El bot tiene varios estilos de juego
// escritos a mano (los "brazos"). Antes de cada mano elige uno, lo juega y
// mira cuántas fichas ganó o perdió: esa es la recompensa. Con el algoritmo
// UCB1 reparte las manos entre probar estilos poco usados (explorar) y repetir
// el que mejor viene rindiendo (explotar).
//
// No necesita entrenamiento previo: empieza sin saber nada y se adapta a los
// rivales que le tocan. Como en el torneo las mesas se sortean de nuevo en
// cada ronda, además olvida de a poco lo viejo (ver bandido.go).
//
// Uso:
//
//	bandido -addr localhost:9000 -token mi-token
package main

import (
	"context"
	"flag"
	"log"
	"math/rand/v2"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

var silencioso = flag.Bool("silencioso", false, "no imprime el resumen de lo aprendido")

// estilo es una forma de jugar: los umbrales que separan retirarse, pagar y
// apostar. Cada estilo es un brazo del bandido.
type estilo struct {
	nombre string
	entrar float64 // Preflop: fuerza mínima para pagar la ciega o una subida pequeña.
	abrir  float64 // Preflop: fuerza a partir de la cual sube.
	valor  float64 // Postflop: fuerza a partir de la cual apuesta por valor.
	farol  float64 // Probabilidad de apostar sin mano cuando nadie apostó.
	margen float64 // Cuánto por encima del precio de pagar exige para seguir.
}

// Los cuatro arquetipos clásicos de la mesa. Ninguno es el mejor siempre: cuál
// conviene depende de los rivales, y eso es justo lo que el bandido descubre.
var estilos = []estilo{
	{nombre: "roca", entrar: 0.47, abrir: 0.57, valor: 0.66, farol: 0.00, margen: 0.25},
	{nombre: "tiburon", entrar: 0.38, abrir: 0.50, valor: 0.58, farol: 0.12, margen: 0.18},
	{nombre: "maniaco", entrar: 0.28, abrir: 0.40, valor: 0.42, farol: 0.35, margen: 0.10},
	{nombre: "pagador", entrar: 0.24, abrir: 0.70, valor: 0.75, farol: 0.00, margen: 0.02},
}

// topeRecompensa acota la recompensa de una mano, en ciegas grandes. UCB1
// supone recompensas entre 0 y 1; sin tope, un solo all-in ganado o perdido
// pesaría más que cien manos normales.
const topeRecompensa = 20.0

// resumenCada es cada cuántas manos se imprime lo aprendido.
const resumenCada = 100

type gatoBandido struct {
	bandido *bandido
	azar    *rand.Rand

	// La mano en curso.
	brazo        int   // Estilo elegido para esta mano.
	decidio      bool  // Si llegó a tomar alguna decisión.
	saldoInicial int64 // Fichas al empezar la mano.
	saldoActual  int64 // Fichas después de la última decisión.
	ciegaGrande  int64
	miID         string

	manos int
}

func (g *gatoBandido) Nombre() string { return "bandido" }

func (g *gatoBandido) Iniciar() error {
	g.azar = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	g.bandido = nuevoBandido(len(estilos))
	return nil
}

// ManoInicio elige el estilo con el que se juega la mano entera. Cambiar de
// estilo a mitad de mano impediría saber a cuál atribuirle el resultado.
func (g *gatoBandido) ManoInicio(botsdk.EstadoPublico, botsdk.Mano) {
	g.brazo = g.bandido.elegir()
	g.decidio = false
}

func (g *gatoBandido) Decidir(_ context.Context, t botsdk.Turno) botsdk.Accion {
	if !g.decidio {
		// En la primera decisión de la mano, lo ya apostado en la ronda es la
		// ciega: sumarla da las fichas con las que empezó.
		g.saldoInicial = t.Yo.Saldo + t.Yo.ApuestaRonda
		g.ciegaGrande = t.Estado.CiegaGrande
		g.miID = t.IDJugador
		g.decidio = true
	}

	e := estilos[g.brazo]
	var accion botsdk.Accion
	if len(t.Estado.Comunitarias) < 3 {
		accion = e.preflop(t, fuerzaPreflop(t.Mano), g.azar)
	} else {
		accion = e.postflop(t, fuerzaPostflop(t.Mano, t.Estado.Comunitarias, rivalesEnLaMano(t)), g.azar)
	}
	g.saldoActual = saldoTras(t, accion)
	return accion
}

// ManoFin calcula la recompensa de la mano y se la anota al estilo que la jugó.
func (g *gatoBandido) ManoFin(resultado botsdk.ResultadoMano) {
	g.manos++
	if !g.decidio || g.ciegaGrande == 0 {
		// La mano terminó sin que le tocara decidir: no hay nada que aprender.
		return
	}
	var ganado int64
	for _, r := range resultado.Repartos {
		if r.IDJugador == g.miID {
			ganado += r.Monto
		}
	}
	neto := ganado - (g.saldoInicial - g.saldoActual)
	g.bandido.actualizar(g.brazo, normalizar(float64(neto)/float64(g.ciegaGrande)))

	if !*silencioso && g.manos%resumenCada == 0 {
		log.Printf("[bandido] mano %d | %s", g.manos, g.bandido.resumen(estilos))
	}
}

// normalizar lleva una recompensa en ciegas grandes al rango 0..1 de UCB1:
// 0.5 es no ganar ni perder.
func normalizar(ciegas float64) float64 {
	ciegas = max(-topeRecompensa, min(topeRecompensa, ciegas))
	return (ciegas + topeRecompensa) / (2 * topeRecompensa)
}

// saldoTras calcula las fichas que quedan después de jugar la acción.
func saldoTras(t botsdk.Turno, a botsdk.Accion) int64 {
	switch a.Tipo {
	case botsdk.Call:
		return t.Yo.Saldo - min(t.PorIgualar(), t.Yo.Saldo)
	case botsdk.Bet, botsdk.Raise:
		return t.Yo.Saldo - (a.Monto - t.Yo.ApuestaRonda)
	case botsdk.AllIn:
		return 0
	}
	return t.Yo.Saldo
}

// rivalesEnLaMano cuenta a los que siguen en la mano, sin contarse.
func rivalesEnLaMano(t botsdk.Turno) int {
	n := 0
	for _, j := range t.Estado.Jugadores {
		if j.ID != t.IDJugador && j.Activo && j.EnMano {
			n++
		}
	}
	return max(n, 1)
}

// --- Cómo juega un estilo -------------------------------------------------

func (e estilo) preflop(t botsdk.Turno, fuerza float64, azar *rand.Rand) botsdk.Accion {
	ciega := t.Estado.CiegaGrande
	huboSubida := t.Estado.ApuestaActual > ciega

	switch {
	case fuerza >= e.abrir+0.20:
		// Mano de primera: sube, y si ya subieron, vuelve a subir.
		return t.Apostar(max(3*ciega, 3*t.Estado.ApuestaActual))
	case fuerza >= e.abrir && !huboSubida:
		return t.Apostar(3 * ciega)
	case !huboSubida && azar.Float64() < e.farol/2:
		// Robo de ciegas: sube con cualquier cosa, de vez en cuando.
		return t.Apostar(3 * ciega)
	case fuerza >= e.entrar && t.PorIgualar() <= 4*ciega:
		return pagarOPasar(t)
	}
	return pasarORetirarse(t)
}

func (e estilo) postflop(t botsdk.Turno, fuerza float64, azar *rand.Rand) botsdk.Accion {
	if t.PorIgualar() == 0 {
		switch {
		case fuerza >= e.valor:
			return t.ApostarPozo(0.66)
		case azar.Float64() < e.farol:
			return t.ApostarPozo(0.5)
		}
		return pasarORetirarse(t)
	}

	// Hay que pagar. El precio es qué parte del pozo final pondría: pagar
	// conviene cuando la chance de ganar lo supera. El margen cubre que quien
	// apuesta suele tener más que una mano al azar.
	precio := float64(t.PorIgualar()) / float64(t.PozoSiPagas())
	switch {
	case fuerza >= 0.85:
		return t.ApostarPozo(1)
	case fuerza >= precio+e.margen:
		return pagarOPasar(t)
	}
	return pasarORetirarse(t)
}

func pagarOPasar(t botsdk.Turno) botsdk.Accion {
	switch {
	case t.Puede(botsdk.Check):
		return t.Check()
	case t.Puede(botsdk.Call):
		return t.Pagar()
	case t.Puede(botsdk.AllIn):
		// Pagar cuesta más de lo que queda: el all-in es la forma de pagar.
		return t.AllIn()
	}
	return t.Fold()
}

func pasarORetirarse(t botsdk.Turno) botsdk.Accion {
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	return t.Fold()
}

func main() { botsdk.CorrerDesdeFlags(&gatoBandido{}) }
