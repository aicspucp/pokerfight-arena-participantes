package entrenamiento

import (
	"math/rand/v2"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

// Los rivales de práctica. Un genoma que solo juega contra copias de sí mismo
// aprende a ganarle a un único estilo; con rivales variados aprende algo que
// sirve contra lo que de verdad va a encontrar en el torneo.
//
// Varios imitan a los bots de sparring de la arena (conservador, aleatorio,
// base) y al bot fácil de Python (Michi Novato). Como aquí no hay cartas sino
// fuerzas precalculadas, la imitación es aproximada: "tiene un par o más" se
// traduce como "su fuerza supera ~0,62".

// Conservador nunca apuesta: paga hasta 3 ciegas grandes o se retira. Es el
// piso del ranking (bots/conservador en la arena).
type Conservador struct{}

func (Conservador) Decidir(s estrategia.Situacion, _ *rand.Rand) estrategia.Decision {
	switch {
	case s.PorIgualar <= 0:
		return estrategia.Decision{Tipo: estrategia.Pasar}
	case s.PorIgualar <= 3:
		return estrategia.Decision{Tipo: estrategia.Pagar}
	}
	return estrategia.Decision{Tipo: estrategia.Retirarse}
}

// Aleatorio elige al azar entre retirarse, pagar, subir y all-in, con un monto
// al azar entre la subida mínima y el all-in (bots/aleatorio en la arena).
type Aleatorio struct{}

func (Aleatorio) Decidir(s estrategia.Situacion, azar *rand.Rand) estrategia.Decision {
	opciones := 3
	if s.PuedeSubir {
		opciones = 4
	}
	switch azar.IntN(opciones) {
	case 0:
		return estrategia.Decision{Tipo: estrategia.Retirarse}
	case 1:
		return estrategia.Decision{Tipo: estrategia.Pagar}
	case 2:
		return estrategia.Decision{Tipo: estrategia.TodoAdentro}
	}
	return estrategia.Decision{Tipo: estrategia.Subir, Total: s.SubidaMinima + azar.Float64()*max(s.Techo-s.SubidaMinima, 0)}
}

// Base imita a bots/base: tabla de Chen preflop y jugada hecha después.
type Base struct{}

func (Base) Decidir(s estrategia.Situacion, _ *rand.Rand) estrategia.Decision {
	if s.Calle == 0 {
		switch {
		case s.Fuerza >= 0.63:
			return estrategia.Decision{Tipo: estrategia.Subir, Total: 3}
		case s.Fuerza >= 0.52 && s.PorIgualar <= 3:
			return estrategia.Decision{Tipo: estrategia.Pagar}
		}
		return estrategia.Decision{Tipo: estrategia.Retirarse}
	}
	switch {
	case s.Fuerza >= 0.88:
		if s.PuedeSubir {
			return estrategia.Decision{Tipo: estrategia.Subir, Total: s.ApuestaActual + 0.75*(s.Pozo+s.PorIgualar)}
		}
		return estrategia.Decision{Tipo: estrategia.Pagar}
	case s.Fuerza >= 0.62:
		if s.PorIgualar <= 0 && s.PuedeSubir {
			return estrategia.Decision{Tipo: estrategia.Subir, Total: 0.4 * s.Pozo}
		}
		if s.PorIgualar*2 <= s.Pozo {
			return estrategia.Decision{Tipo: estrategia.Pagar}
		}
	}
	return estrategia.Decision{Tipo: estrategia.Retirarse}
}

// Novato imita al bot fácil de Python: grupos de manos preflop, apuestas por
// valor con mano buena, y paga proyectos solo si las pot odds alcanzan.
type Novato struct{}

func (Novato) Decidir(s estrategia.Situacion, _ *rand.Rand) estrategia.Decision {
	nadieSubio := s.ApuestaActual <= 1
	if s.Calle == 0 {
		switch {
		case s.Fuerza >= 0.66:
			if nadieSubio {
				return estrategia.Decision{Tipo: estrategia.Subir, Total: 3}
			}
			return estrategia.Decision{Tipo: estrategia.Subir, Total: 3 * s.ApuestaActual}
		case s.Fuerza >= 0.60:
			if nadieSubio {
				return estrategia.Decision{Tipo: estrategia.Subir, Total: 3}
			}
			if s.PorIgualar <= 4 {
				return estrategia.Decision{Tipo: estrategia.Pagar}
			}
		case s.Fuerza >= 0.55 && s.PorIgualar <= 1:
			return estrategia.Decision{Tipo: estrategia.Pagar}
		}
		return estrategia.Decision{Tipo: estrategia.Retirarse}
	}
	potOdds := s.PorIgualar / max(s.Pozo+s.PorIgualar, 1)
	switch {
	case s.Fuerza >= 0.88:
		if s.PorIgualar <= 0 {
			return estrategia.Decision{Tipo: estrategia.Subir, Total: 0.75 * s.Pozo}
		}
		return estrategia.Decision{Tipo: estrategia.Subir, Total: 3 * s.ApuestaActual}
	case s.Fuerza >= 0.72:
		if s.PorIgualar <= 0 {
			return estrategia.Decision{Tipo: estrategia.Subir, Total: 0.5 * s.Pozo}
		}
		if s.PorIgualar <= 0.75*s.Pozo {
			return estrategia.Decision{Tipo: estrategia.Pagar}
		}
	case s.Fuerza >= 0.55 && potOdds <= 0.25:
		return estrategia.Decision{Tipo: estrategia.Pagar}
	}
	return estrategia.Decision{Tipo: estrategia.Retirarse}
}

// Maniaco sube muchísimo y casi nunca se retira. Castiga a los bots que se
// asustan; un buen bot aprende a dejarlo apostar cuando tiene mano.
type Maniaco struct{}

func (Maniaco) Decidir(s estrategia.Situacion, azar *rand.Rand) estrategia.Decision {
	if s.PuedeSubir && azar.Float64() < 0.55 {
		return estrategia.Decision{Tipo: estrategia.Subir, Total: s.ApuestaActual + s.Pozo + s.PorIgualar}
	}
	if s.Fuerza < 0.35 && s.PorIgualar > 0 && azar.Float64() < 0.3 {
		return estrategia.Decision{Tipo: estrategia.Retirarse}
	}
	return estrategia.Decision{Tipo: estrategia.Pagar}
}

// Estacion ("calling station") paga casi todo y nunca sube. Contra ella los
// faroles no sirven y las apuestas por valor rinden el doble.
type Estacion struct{}

func (Estacion) Decidir(s estrategia.Situacion, _ *rand.Rand) estrategia.Decision {
	if s.PorIgualar <= 0 || s.Fuerza >= 0.40 || s.PorIgualar <= 2 {
		return estrategia.Decision{Tipo: estrategia.Pagar}
	}
	return estrategia.Decision{Tipo: estrategia.Retirarse}
}
