package main

import (
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

// La fuerza es un número entre 0 y 1 que resume qué tan buena es la mano. Aquí
// se calcula con reglas simples y sin simular: lo interesante de este bot es
// cómo aprende, no cómo mide. Para una medida mejor (Monte Carlo), mirar
// bots/estadistico o bots/espia.

// fuerzaPreflop puntúa las dos cartas iniciales con la fórmula de Chen,
// llevada a 0..1: AA vale 1, AKs 0.62, un par de nueves 0.48 y 72o vale 0.
func fuerzaPreflop(mano botsdk.Mano) float64 {
	alta, baja := mano[0].Rango, mano[1].Rango
	if baja > alta {
		alta, baja = baja, alta
	}

	var puntaje float64
	switch alta {
	case botsdk.As:
		puntaje = 10
	case botsdk.Rey:
		puntaje = 8
	case botsdk.Reina:
		puntaje = 7
	case botsdk.Jota:
		puntaje = 6
	default:
		puntaje = float64(alta) / 2
	}

	hueco := int(alta) - int(baja) - 1
	switch {
	case alta == baja:
		puntaje = max(2*puntaje, 5)
	case hueco == 1:
		puntaje--
	case hueco == 2:
		puntaje -= 2
	case hueco == 3:
		puntaje -= 4
	case hueco >= 4:
		puntaje -= 5
	}
	if mano[0].Palo == mano[1].Palo {
		puntaje += 2
	}
	if alta != baja && hueco <= 1 && alta < botsdk.Reina {
		puntaje++ // Conectoras bajas: arman escalera por los dos lados.
	}

	return max(0, min(1, (puntaje+1)/21))
}

// fuerzaPostflop mira la jugada ya armada y cuánto de ella es propio. Un par
// que está entero en la mesa lo tienen todos, así que vale poco.
func fuerzaPostflop(mano botsdk.Mano, mesa []botsdk.Carta, rivales int) float64 {
	enMesa := map[botsdk.Rango]int{}
	var masAlta botsdk.Rango
	for _, c := range mesa {
		enMesa[c.Rango]++
		masAlta = max(masAlta, c.Rango)
	}
	a, b := mano[0].Rango, mano[1].Rango
	parEnMano := a == b
	liga := enMesa[a] > 0 || enMesa[b] > 0 // Alguna carta propia empareja la mesa.

	jugada := poker.Mejor(mano, mesa)
	var fuerza float64
	switch jugada.Categoria {
	case poker.CartaAlta:
		fuerza = 0.10
		if max(a, b) >= botsdk.Rey {
			fuerza = 0.18
		}
	case poker.Par:
		switch {
		case parEnMano && a > masAlta:
			fuerza = 0.68 // Par en mano mayor que toda la mesa.
		case parEnMano:
			fuerza = 0.42
		case (enMesa[a] > 0 && a == masAlta) || (enMesa[b] > 0 && b == masAlta):
			fuerza = 0.60 // Par con la carta más alta de la mesa.
		case liga:
			fuerza = 0.38
		default:
			fuerza = 0.15
		}
	case poker.DoblePar:
		switch {
		case !parEnMano && enMesa[a] > 0 && enMesa[b] > 0:
			fuerza = 0.78
		case liga || parEnMano:
			fuerza = 0.55
		default:
			fuerza = 0.18
		}
	case poker.Trio:
		fuerza = 0.25
		if liga || parEnMano {
			fuerza = 0.86
		}
	case poker.Escalera:
		fuerza = 0.90
	case poker.Color:
		fuerza = 0.93
	case poker.FullHouse:
		fuerza = 0.96
	default:
		fuerza = 0.99
	}

	if len(mesa) == 5 {
		// Si la mejor jugada es la de la mesa sola, no hay ventaja sobre nadie.
		if deLaMesa, err := poker.MejorDe(mesa); err == nil && deLaMesa.Puntaje == jugada.Puntaje {
			fuerza = min(fuerza, 0.30)
		}
	} else if proyectoDeColor(mano, mesa) {
		fuerza += 0.12
	}

	// Cada rival de más es otra mano que puede ser mejor.
	fuerza -= 0.04 * float64(rivales-1)
	return max(0, min(1, fuerza))
}

// proyectoDeColor indica si falta una sola carta para el color, usando al
// menos una carta propia.
func proyectoDeColor(mano botsdk.Mano, mesa []botsdk.Carta) bool {
	porPalo := map[botsdk.Palo]int{}
	for _, c := range mesa {
		porPalo[c.Palo]++
	}
	for _, c := range mano {
		porPalo[c.Palo]++
	}
	return porPalo[mano[0].Palo] == 4 || porPalo[mano[1].Palo] == 4
}
