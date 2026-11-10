package estrategia

import (
	"math"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// SituacionDesde traduce el turno real de la mesa a la Situacion que mira la
// política. Es el espejo de lo que hace el simulador del entrenamiento con sus
// mesas falsas: si se agrega un campo aquí, hay que agregarlo allá.
func SituacionDesde(t botsdk.Turno, fuerza float64) Situacion {
	cg := float64(max(t.Estado.CiegaGrande, 1))

	var sillas []int
	maxSilla := 0
	rivales := 0
	for _, j := range t.Estado.Jugadores {
		maxSilla = max(maxSilla, j.PosicionSilla)
		// Activo es "sigue en la mano" (no se retiró). Un rival all-in sigue
		// en la mano: también hay que ganarle.
		if !j.Activo || !j.EnMano {
			continue
		}
		sillas = append(sillas, j.PosicionSilla)
		if j.ID != t.IDJugador {
			rivales++
		}
	}

	subidas := 0
	for _, a := range t.Estado.HistorialAcciones {
		if a.Etapa == t.Estado.Etapa && (a.Accion.Tipo == botsdk.Bet || a.Accion.Tipo == botsdk.Raise || a.Accion.Tipo == botsdk.AllIn) {
			subidas++
		}
	}

	return Situacion{
		Calle:         calle(len(t.Estado.Comunitarias)),
		Fuerza:        fuerza,
		Rivales:       rivales,
		Posicion:      Posicion(sillas, t.Yo.PosicionSilla, t.Estado.PosicionBoton, maxSilla+1),
		Subidas:       subidas,
		Pozo:          float64(t.Estado.Pozo) / cg,
		PorIgualar:    float64(t.PorIgualar()) / cg,
		ApuestaActual: float64(t.Estado.ApuestaActual) / cg,
		SubidaMinima:  float64(t.SubidaMinima()) / cg,
		Techo:         float64(t.Techo()) / cg,
		PuedeSubir:    t.Puede(botsdk.Bet) || t.Puede(botsdk.Raise),
	}
}

func calle(comunitarias int) int {
	switch {
	case comunitarias >= 5:
		return 3
	case comunitarias == 4:
		return 2
	case comunitarias == 3:
		return 1
	default:
		return 0
	}
}

// AAccion traduce la decisión de la política a una acción que la mesa acepta.
// Las cuentas de montos las hace el SDK (t.Apostar), que nunca devuelve algo
// inválido: recorta a la subida mínima y al all-in, y si no se puede subir cae
// a la mejor acción pasiva.
func AAccion(d Decision, t botsdk.Turno) botsdk.Accion {
	switch d.Tipo {
	case TodoAdentro:
		if t.Puede(botsdk.AllIn) {
			return t.AllIn()
		}
		return t.Apostar(t.Techo())
	case Subir:
		cg := float64(max(t.Estado.CiegaGrande, 1))
		return t.Apostar(int64(math.Round(d.Total * cg)))
	case Pagar:
		if t.Puede(botsdk.Call) {
			return t.Pagar()
		}
		if t.Puede(botsdk.Check) {
			return t.Check()
		}
		if t.Puede(botsdk.AllIn) {
			// No alcanza para igualar entero: el único pago posible es el
			// all-in corto.
			return t.AllIn()
		}
		return t.Fold()
	default:
		// Retirarse o Pasar: nunca se tira una mano si pasar es gratis.
		if t.Puede(botsdk.Check) {
			return t.Check()
		}
		return t.Fold()
	}
}
