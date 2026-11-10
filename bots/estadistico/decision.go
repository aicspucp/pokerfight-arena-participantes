package main

import (
	"math/rand/v2"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// Parámetros de la decisión. Están juntos para poder ajustarlos mirando los
// resultados de la arena sin tener que buscarlos por el código.
const (
	// umbralSubir es la ventaja sobre la parte justa (ver ventajaSobreLaParteJusta)
	// a partir de la cual se sube por valor. 0.25 es, heads-up, una equidad
	// del 62,5 %; contra 5 rivales, del 37,5 %.
	umbralSubir = 0.25
	// recargoPorSubida es cuánta ventaja más se pide por cada bet o raise que
	// ya hubo en la ronda. Sin esto, dos manos buenas se resuben hasta el
	// all-in aunque ninguna sea tan buena: la tercera subida de una ronda
	// casi siempre viene de una mano muy fuerte.
	recargoPorSubida = 0.15
	// margenPagar es la equidad que se pide por encima del precio para pagar.
	// Cubre que la equidad se calcula contra manos al azar y el que apostó
	// suele tener algo mejor que eso.
	margenPagar = 0.04
	// anchoMezcla es la mitad de la franja alrededor de cada umbral en la que
	// la decisión se sortea en vez de ser fija (ver superaUmbral).
	anchoMezcla = 0.05
	// fraccionMinima y fraccionMaxima acotan el tamaño de la apuesta por
	// valor, como fracción del pozo: media apuesta justo en el umbral, pozo
	// entero con la mano casi segura.
	fraccionMinima = 0.5
	fraccionMaxima = 1.0
	// probabilidadFarol es cada cuánto se apuesta sin mano cuando nadie
	// apostó. Es baja a propósito: alcanza para que pasar no signifique
	// siempre "no tengo nada", pero no regala fichas.
	probabilidadFarol = 0.08
	// fraccionFarol es el tamaño del farol: medio pozo, lo justo para que un
	// rival con poco se retire.
	fraccionFarol = 0.5
	// maxRivalesFarol limita el farol a manos heads-up: contra varios rivales
	// casi siempre alguno tiene algo y paga.
	maxRivalesFarol = 1
)

// decidir elige la acción del turno a partir de la equidad ya estimada. Es una
// función aparte de la simulación para poder probarla con equidades exactas.
//
// El orden importa:
//
//  1. Si la ventaja alcanza para subir, se sube. El tamaño crece con la
//     ventaja.
//  2. Si es gratis seguir, se pasa (o, de vez en cuando, se farolea).
//  3. Si la equidad paga el precio, se paga.
//  4. Si no, se retira.
//
// Cerca de cada umbral la decisión se sortea (superaUmbral): un bot que decide
// siempre igual con la misma equidad es fácil de leer.
func decidir(t botsdk.Turno, eq float64, rivales int, azar *rand.Rand) botsdk.Accion {
	ventaja := ventajaSobreLaParteJusta(eq, rivales)
	umbral := umbralSubir + recargoPorSubida*float64(subidasEnLaRonda(t))

	if puedeSubir(t) && superaUmbral(ventaja, umbral, azar) {
		return t.ApostarPozo(fraccionDeApuesta(ventaja, umbral))
	}

	if t.PorIgualar() == 0 {
		if puedeFarolear(t, rivales) && azar.Float64() < probabilidadFarol {
			return t.ApostarPozo(fraccionFarol)
		}
		if t.Puede(botsdk.Check) {
			return t.Check()
		}
	}

	if superaUmbral(eq, precioDePagar(t)+margenPagar, azar) {
		return pagar(t)
	}
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	return t.Fold()
}

// ventajaSobreLaParteJusta mide cuánto mejor que el promedio está la mano, en
// una escala que no depende de cuántos rivales hay.
//
// La parte justa es lo que se llevaría cada uno si todos fueran iguales:
// 1/(rivales+1). Con AA contra 5 rivales la equidad es ~49 %, que suena a
// moneda al aire pero es casi tres veces la parte justa (17 %). La escala va de
// 0 (justo la parte justa) a 1 (gana siempre); por debajo de la parte justa da
// negativo.
func ventajaSobreLaParteJusta(eq float64, rivales int) float64 {
	if rivales <= 0 {
		return 1
	}
	parteJusta := 1 / float64(rivales+1)
	return (eq - parteJusta) / (1 - parteJusta)
}

// precioDePagar es la equidad mínima para que pagar no pierda fichas en
// promedio: lo que se pone dividido por el pozo que se puede ganar.
//
// Si pagar cuesta más que el saldo, se paga con todo lo que queda y lo que el
// rival apostó por encima de eso no se puede ganar (vuelve a él, ver
// docs/reglas.md). Se descuenta del pozo para no sobreestimar el premio. Con
// varios rivales es una aproximación: otro jugador podría cubrir ese exceso,
// pero el bot no lo gana igual.
func precioDePagar(t botsdk.Turno) float64 {
	costo := min(t.PorIgualar(), t.Yo.Saldo)
	if costo <= 0 {
		return 0
	}
	noIgualable := t.PorIgualar() - costo
	pozoFinal := t.Estado.Pozo - noIgualable + costo
	if pozoFinal <= 0 {
		return 1
	}
	return float64(costo) / float64(pozoFinal)
}

// superaUmbral decide si `valor` pasa el `umbral`, sorteando en la franja de
// ±anchoMezcla alrededor de él. La probabilidad de "sí" sube en línea recta de
// 0 (en umbral-anchoMezcla) a 1 (en umbral+anchoMezcla).
//
// Es la regla "azar < probabilidad" llevada a donde sirve: lejos del umbral la
// decisión es clara y sortearla regala fichas; cerca, las dos opciones valen
// casi lo mismo y sortear cuesta poco y hace al bot impredecible.
func superaUmbral(valor, umbral float64, azar *rand.Rand) bool {
	switch {
	case valor >= umbral+anchoMezcla:
		return true
	case valor <= umbral-anchoMezcla:
		return false
	}
	probabilidad := (valor - (umbral - anchoMezcla)) / (2 * anchoMezcla)
	return azar.Float64() < probabilidad
}

// fraccionDeApuesta es el tamaño de la subida por valor, como fracción del
// pozo: más grande cuanto más lejos del umbral está la ventaja.
func fraccionDeApuesta(ventaja, umbral float64) float64 {
	if umbral >= 1 {
		return fraccionMaxima
	}
	avance := (ventaja - umbral) / (1 - umbral)
	avance = max(0, min(1, avance))
	return fraccionMinima + (fraccionMaxima-fraccionMinima)*avance
}

// subidasEnLaRonda cuenta los bet, raise y all-in de la calle en curso. Las
// ciegas no aparecen en el historial de la mesa, así que preflop la primera
// subida de verdad cuenta como la primera.
func subidasEnLaRonda(t botsdk.Turno) int {
	subidas := 0
	for _, a := range t.Estado.HistorialAcciones {
		if a.Etapa != t.Estado.Etapa {
			continue
		}
		switch a.Accion.Tipo {
		case botsdk.Bet, botsdk.Raise, botsdk.AllIn:
			subidas++
		}
	}
	return subidas
}

func puedeSubir(t botsdk.Turno) bool {
	return t.Puede(botsdk.Bet) || t.Puede(botsdk.Raise) || t.Puede(botsdk.AllIn)
}

// puedeFarolear limita el farol a donde tiene sentido: después del flop
// (preflop pasar es gratis solo para la ciega grande, y ahí no hay nada que
// robar), heads-up y con la apuesta disponible.
func puedeFarolear(t botsdk.Turno, rivales int) bool {
	return len(t.Estado.Comunitarias) >= 3 && rivales <= maxRivalesFarol && t.Puede(botsdk.Bet)
}

// pagar iguala la apuesta. La mesa ofrece call aunque no alcance el saldo (se
// paga con lo que hay), pero si alguna vez solo ofreciera all-in, es la misma
// jugada.
func pagar(t botsdk.Turno) botsdk.Accion {
	if t.Puede(botsdk.Call) {
		return t.Pagar()
	}
	if t.Puede(botsdk.AllIn) {
		return t.AllIn()
	}
	return t.Fold()
}
