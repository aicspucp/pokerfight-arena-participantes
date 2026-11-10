package main

import (
	"math/rand/v2"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

// equidad estima por Monte Carlo qué parte del pozo gana la mano en promedio
// contra `rivales` manos al azar: reparte muchas veces las cartas que faltan y
// cuenta. 0.5 contra un rival es una mano promedio; un empate vale medio pozo.
func equidad(mano botsdk.Mano, mesa []botsdk.Carta, rivales, muestras int, azar *rand.Rand) float64 {
	mazo := cartasRestantes(mano, mesa)
	faltan := 5 - len(mesa)
	necesarias := faltan + 2*rivales
	if muestras <= 0 || rivales <= 0 || necesarias > len(mazo) {
		return 0.5
	}

	tablero := make([]botsdk.Carta, 5)
	copy(tablero, mesa)

	total := 0.0
	for range muestras {
		// Barajado parcial: solo hace falta mezclar las cartas que se usan.
		for i := range necesarias {
			j := i + azar.IntN(len(mazo)-i)
			mazo[i], mazo[j] = mazo[j], mazo[i]
		}
		copy(tablero[len(mesa):], mazo[:faltan])

		mia := poker.Mejor(mano, tablero).Puntaje
		resultado := 1.0
		for r := range rivales {
			rival := botsdk.Mano{mazo[faltan+2*r], mazo[faltan+2*r+1]}
			puntaje := poker.Mejor(rival, tablero).Puntaje
			if puntaje > mia {
				resultado = 0
				break
			}
			if puntaje == mia {
				resultado = 0.5
			}
		}
		total += resultado
	}
	return total / float64(muestras)
}

// cartasRestantes arma el mazo sin las cartas ya vistas.
func cartasRestantes(mano botsdk.Mano, mesa []botsdk.Carta) []botsdk.Carta {
	vistas := map[botsdk.Carta]bool{mano[0]: true, mano[1]: true}
	for _, c := range mesa {
		vistas[c] = true
	}
	mazo := make([]botsdk.Carta, 0, 52)
	for palo := botsdk.Treboles; palo <= botsdk.Picas; palo++ {
		for rango := botsdk.Dos; rango <= botsdk.As; rango++ {
			if c := (botsdk.Carta{Rango: rango, Palo: palo}); !vistas[c] {
				mazo = append(mazo, c)
			}
		}
	}
	return mazo
}
