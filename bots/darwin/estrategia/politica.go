package estrategia

import (
	"math"
	"math/rand/v2"
)

// Situacion es todo lo que la política mira para decidir, ya reducido a
// números. Los montos van en ciegas grandes para que un mismo genoma sirva
// con cualquier tamaño de ciegas.
//
// La arma SituacionDesde (con el estado real de la mesa) o el simulador del
// entrenamiento. Que las dos fuentes produzcan exactamente los mismos campos
// es lo que hace que lo aprendido en la simulación valga en la arena.
type Situacion struct {
	Calle    int     // 0 preflop, 1 flop, 2 turn, 3 river.
	Fuerza   float64 // Probabilidad de ganarle a UNA mano al azar (0..1).
	Rivales  int     // Rivales que siguen en la mano.
	Posicion float64 // 0 = habla primero después del flop, 1 = habla último.
	Subidas  int     // Apuestas y subidas de cualquiera en la calle actual.

	Pozo          float64 // Todo lo que hay en el centro, incluida esta ronda.
	PorIgualar    float64 // Lo que falta para pagar. 0 = se puede pasar gratis.
	ApuestaActual float64 // La apuesta más alta de la ronda.
	SubidaMinima  float64 // Total mínimo para que una subida sea válida.
	Techo         float64 // Total máximo (el all-in).
	PuedeSubir    bool    // La mesa acepta bet o raise en este turno.
}

// TipoDecision es la intención de la política, antes de traducirla a una
// acción válida del protocolo.
type TipoDecision int

const (
	Retirarse   TipoDecision = iota // Fold, salvo que pasar sea gratis.
	Pasar                           // Check.
	Pagar                           // Call (o check si no hay nada que pagar).
	Subir                           // Bet o raise hasta Total.
	TodoAdentro                     // All-in.
)

// Decision es la jugada elegida. Total solo cuenta para Subir y es el TOTAL
// de la ronda en ciegas grandes, igual que el monto del protocolo.
type Decision struct {
	Tipo   TipoDecision
	Total  float64
	Motivo string // Para los logs: por qué se eligió.
}

// Politica es cualquier forma de decidir. La implementan el genoma y los
// rivales de práctica del entrenamiento.
type Politica interface {
	Decidir(s Situacion, azar *rand.Rand) Decision
}

// FuerzaEfectiva corrige la fuerza contra un rival por la cantidad de rivales,
// la posición y la agresión de la calle. Es el número que se compara contra
// los umbrales del genoma.
//
// Contra k rivales, la probabilidad de ganarles a todos es aproximadamente
// fuerza^k. El genético no recibe esa fórmula impuesta: el gen Multijugador
// decide cuánto pesa cada rival extra, porque los rivales reales no juegan
// manos al azar (los que siguen en la mano suelen tener algo).
func (g Genoma) FuerzaEfectiva(s Situacion) float64 {
	rivales := max(s.Rivales, 1)
	efectiva := math.Pow(s.Fuerza, 1+float64(rivales-1)*g[Multijugador])
	efectiva += g[BonoPosicion] * (2*s.Posicion - 1)
	efectiva -= g[Presion] * float64(s.Subidas)
	return efectiva
}

// Decidir implementa Politica. Es la estrategia completa de Gato Darwin: la
// forma es fija y los números salen del genoma.
func (g Genoma) Decidir(s Situacion, azar *rand.Rand) Decision {
	f := g.FuerzaEfectiva(s)

	// Los genes vienen de a pares: uno para preflop y otro para después del
	// flop, porque con dos cartas y con cinco se juega distinto.
	umbralApuesta, umbralSubida, margenPago, farol := g[UmbralApuestaPre], g[UmbralSubidaPre], g[MargenPagoPre], g[FarolPre]
	if s.Calle > 0 {
		umbralApuesta, umbralSubida, margenPago, farol = g[UmbralApuestaPost], g[UmbralSubidaPost], g[MargenPagoPost], g[FarolPost]
	}

	// Jugarse todo el stack exige más que jugarse una ciega: en un torneo con
	// eliminación, perder el stack es perder el torneo, y una mano "buena"
	// contra una al azar no lo es tanto contra quien acepta ir all-in.
	if f >= g[UmbralAllIn]+g[AversionRiesgo] {
		return Decision{Tipo: TodoAdentro, Motivo: "mano enorme: all-in"}
	}

	if s.PorIgualar <= 0 {
		total := g.tamApuesta(s, g[TamValor])
		if f >= umbralApuesta+g[AversionRiesgo]*s.Riesgo(total) && s.PuedeSubir {
			return Decision{Tipo: Subir, Total: total, Motivo: "apuesta por valor"}
		}
		// Los faroles son más creíbles (y más baratos de que salgan mal)
		// hablando último: la probabilidad crece con la posición.
		if s.PuedeSubir && azar.Float64() < farol*(0.5+s.Posicion) {
			return Decision{Tipo: Subir, Total: g.tamApuesta(s, g[TamFarol]), Motivo: "farol"}
		}
		return Decision{Tipo: Pasar, Motivo: "paso"}
	}

	// Pot odds: la fracción del pozo final que ponemos al pagar. Si ganamos
	// más seguido que eso, pagar da fichas a la larga.
	potOdds := s.PorIgualar / (s.Pozo + s.PorIgualar)

	if subida := g.tamSubida(s); f >= umbralSubida+g[AversionRiesgo]*s.Riesgo(subida) && s.PuedeSubir {
		return Decision{Tipo: Subir, Total: subida, Motivo: "subo por valor"}
	}
	if f >= potOdds*margenPago+g[AversionRiesgo]*s.Riesgo(s.ApuestaActual) {
		return Decision{Tipo: Pagar, Motivo: "las pot odds dan para pagar"}
	}
	if s.PuedeSubir && azar.Float64() < g[FarolSubida] {
		return Decision{Tipo: Subir, Total: g.tamSubida(s), Motivo: "resubida de farol"}
	}
	return Decision{Tipo: Retirarse, Motivo: "no alcanza"}
}

// Riesgo es la fracción de las fichas que nos quedan que habría que poner para
// llevar la apuesta de la ronda a `total`: 0 si no cuesta nada, 1 si es el
// all-in.
func (s Situacion) Riesgo(total float64) float64 {
	miApuesta := s.ApuestaActual - s.PorIgualar
	restante := s.Techo - miApuesta
	if restante <= 0 {
		return 1
	}
	return min(max((total-miApuesta)/restante, 0), 1)
}

// tamApuesta es el total de una apuesta cuando nadie apostó todavía.
func (g Genoma) tamApuesta(s Situacion, fraccion float64) float64 {
	if s.Calle == 0 {
		// Preflop se abre a un múltiplo fijo de la ciega grande: el pozo es
		// pequeño y una fracción de él no alcanzaría para sacar a nadie.
		return max(g[TamApertura], s.ApuestaActual+g[TamApertura]-1)
	}
	return s.ApuestaActual + fraccion*s.Pozo
}

// tamSubida es el total de una subida sobre una apuesta ya hecha.
func (g Genoma) tamSubida(s Situacion) float64 {
	if s.Calle == 0 && s.ApuestaActual <= 1 {
		// Nadie subió todavía (solo están las ciegas): es una apertura.
		return g[TamApertura]
	}
	return s.ApuestaActual + g[TamSubida]*(s.Pozo+s.PorIgualar)
}

// Posicion devuelve 0 si el jugador de miSilla habla primero después del
// flop y 1 si habla último, entre las sillas que siguen en la mano. Después
// del flop habla primero el primero a la izquierda del botón; el botón, último.
//
// totalSillas es cualquier número mayor que la silla más alta: solo se usa
// para dar la vuelta a la mesa.
func Posicion(sillas []int, miSilla, boton, totalSillas int) float64 {
	if len(sillas) <= 1 || totalSillas <= 0 {
		return 1
	}
	distancia := func(silla int) int {
		return ((silla-boton-1)%totalSillas + totalSillas) % totalSillas
	}
	mia := distancia(miSilla)
	antes := 0
	for _, s := range sillas {
		if s != miSilla && distancia(s) < mia {
			antes++
		}
	}
	return float64(antes) / float64(len(sillas)-1)
}
