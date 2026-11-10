package main

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

// cadaCuantasMirarReloj es cada cuántas simulaciones se revisa el plazo. Mirar
// el reloj en cada vuelta costaría más que la simulación misma; cada 256 el
// desvío sobre el presupuesto es de unos pocos milisegundos.
const cadaCuantasMirarReloj = 256

// simulacion es lo que hace falta para estimar la equidad de una mano.
type simulacion struct {
	mano         botsdk.Mano
	comunitarias []botsdk.Carta
	// rivales es cuántos jugadores siguen en la mano además del bot.
	rivales int
	// maxSimulaciones corta la estimación aunque sobre tiempo: con 10 000 el
	// error típico ronda el medio punto porcentual, y más no cambia ninguna
	// decisión.
	maxSimulaciones int
	// presupuesto corta la estimación aunque falten simulaciones, para no
	// pasarse del plazo que da la mesa.
	presupuesto time.Duration
}

// equidad es el resultado de una estimación.
type equidad struct {
	// Valor es la parte del pozo que el bot se lleva en promedio, entre 0 y
	// 1. Un empate entre k jugadores cuenta 1/k, que es lo que cobra cada uno
	// al dividir el pozo.
	Valor float64
	// Simulaciones es cuántas manos se jugaron para estimarla. Si es menor que
	// maxSimulaciones, se cortó por tiempo.
	Simulaciones int
}

// estimarEquidad juega la mano muchas veces repartiendo al azar lo que el bot
// no ve (las cartas de los rivales y las comunitarias que faltan) y cuenta qué
// parte del pozo gana.
//
// Es la idea de la baraja finita: las cartas que el bot conoce no pueden estar
// en ningún otro lado, así que se sortea solo entre las que quedan. En el flop,
// por ejemplo, el bot conoce 5 cartas y se reparte de las 47 restantes.
//
// Los rivales se suponen con cualquier mano al azar. Eso sobreestima la equidad
// frente a quien subió (suele tener algo bueno); corregirlo con rangos es la
// fase 2 del bot.
//
// Siempre juega al menos cadaCuantasMirarReloj simulaciones, aunque el
// contexto ya esté vencido, para no devolver nunca una equidad sin dato.
func estimarEquidad(ctx context.Context, azar *rand.Rand, sim simulacion) equidad {
	if sim.rivales <= 0 {
		// Sin nadie más en la mano, el pozo es del bot.
		return equidad{Valor: 1}
	}

	restantes := cartasRestantes(sim.mano, sim.comunitarias)
	faltanEnMesa := 5 - len(sim.comunitarias)
	// En una mesa real nunca faltan cartas (10 jugadores usan 25 de 52), pero
	// se recorta igual para que un estado raro no termine en un índice fuera
	// de rango.
	if maximo := (len(restantes) - faltanEnMesa) / 2; sim.rivales > maximo {
		sim.rivales = maximo
	}
	necesarias := faltanEnMesa + 2*sim.rivales

	var tablero [5]botsdk.Carta
	copy(tablero[:], sim.comunitarias)

	limite := time.Now().Add(sim.presupuesto)
	var acumulado float64
	jugadas := 0
	for jugadas < sim.maxSimulaciones {
		if jugadas > 0 && jugadas%cadaCuantasMirarReloj == 0 {
			if ctx.Err() != nil || time.Now().After(limite) {
				break
			}
		}

		// Fisher-Yates parcial: solo se barajan las primeras `necesarias`
		// posiciones, que son las que se van a usar. No hace falta volver a
		// ordenar el mazo entre vueltas: barajar una permutación da otra
		// permutación igual de uniforme.
		for i := 0; i < necesarias; i++ {
			j := i + azar.IntN(len(restantes)-i)
			restantes[i], restantes[j] = restantes[j], restantes[i]
		}
		copy(tablero[len(sim.comunitarias):], restantes[:faltanEnMesa])

		acumulado += parteGanada(sim.mano, tablero[:], restantes[faltanEnMesa:necesarias])
		jugadas++
	}

	return equidad{Valor: acumulado / float64(jugadas), Simulaciones: jugadas}
}

// parteGanada resuelve un showdown simulado: 1 si el bot gana solo, 1/k si
// empata con k-1 rivales, 0 si pierde. manosRivales trae las cartas de los
// rivales de a pares.
func parteGanada(mia botsdk.Mano, tablero []botsdk.Carta, manosRivales []botsdk.Carta) float64 {
	propio := poker.Mejor(mia, tablero).Puntaje
	empatados := 0
	for i := 0; i+1 < len(manosRivales); i += 2 {
		rival := poker.Mejor(botsdk.Mano{manosRivales[i], manosRivales[i+1]}, tablero).Puntaje
		if rival > propio {
			// Con un solo rival por encima la mano está perdida: no hace falta
			// evaluar a los demás.
			return 0
		}
		if rival == propio {
			empatados++
		}
	}
	return 1 / float64(empatados+1)
}

// cartasRestantes devuelve el mazo sin las cartas que el bot ya conoce.
func cartasRestantes(mano botsdk.Mano, comunitarias []botsdk.Carta) []botsdk.Carta {
	var vistas [botsdk.As + 1][botsdk.Picas + 1]bool
	for _, c := range mano {
		vistas[c.Rango][c.Palo] = true
	}
	for _, c := range comunitarias {
		vistas[c.Rango][c.Palo] = true
	}

	restantes := make([]botsdk.Carta, 0, 52)
	for rango := botsdk.Dos; rango <= botsdk.As; rango++ {
		for palo := botsdk.Treboles; palo <= botsdk.Picas; palo++ {
			if !vistas[rango][palo] {
				restantes = append(restantes, botsdk.Carta{Rango: rango, Palo: palo})
			}
		}
	}
	return restantes
}

// manoValida indica si el bot tiene cartas de verdad. Si se conectó a mitad de
// una mano (una reconexión, por ejemplo) no recibió el mano_inicio y la mano
// queda en el cero, que no es una carta.
func manoValida(mano botsdk.Mano) bool {
	return mano[0].Rango >= botsdk.Dos && mano[1].Rango >= botsdk.Dos && mano[0] != mano[1]
}
