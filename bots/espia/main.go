// Comando espia: Michi Espía, un bot que estudia a sus rivales.
//
// La técnica es el modelado de rivales. El bot mira cada acción de la mesa,
// también las de las manos en que no juega, y arma un perfil de cada rival:
// con qué frecuencia entra, cuánto apuesta y cuánto se retira cuando le
// apuestan. Con esos números cambia su juego según a quién tenga enfrente:
//
//   - Farolea solo contra quien se retira seguido.
//   - Apuesta más grande y con manos más flojas contra quien paga todo.
//   - Le cree poco al que apuesta siempre y mucho al que casi nunca apuesta.
//
// Los rivales se reconocen por su identificador, que en el torneo es un alias
// aleatorio pero estable: lo aprendido en una ronda sirve en la siguiente.
//
// Cada frecuencia se estima con una regla bayesiana (ver modelo.go), y la
// fuerza de la mano propia con una simulación Monte Carlo (ver equidad.go).
//
// Uso:
//
//	espia -addr localhost:9000 -token mi-token
package main

import (
	"context"
	"flag"
	"log"
	"math/rand/v2"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

var (
	silencioso = flag.Bool("silencioso", false, "no imprime lo que aprende de cada rival")
	muestras   = flag.Int("muestras", 400, "simulaciones por decisión para estimar la equidad")
)

const (
	// maxRivalesSimulados acota la simulación: más rivales cambian poco la
	// decisión y cuestan tiempo.
	maxRivalesSimulados = 3
	// umbralFarol es la probabilidad mínima de que todos se retiren para
	// farolear. Apostar medio pozo ya rinde con 1/3; el resto es margen,
	// porque la estimación sale de pocas manos.
	umbralFarol = 0.55
	// resumenCada es cada cuántas manos se imprime lo aprendido.
	resumenCada = 100
)

type michiEspia struct {
	modelo *modelo
	azar   *rand.Rand

	// La mano en curso, tal como se la fue viendo.
	idMano    string
	historial []botsdk.AccionRegistrada
	enMano    []string
	miID      string

	manos int
}

func (m *michiEspia) Nombre() string { return "espia" }

func (m *michiEspia) Iniciar() error {
	m.azar = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	m.modelo = nuevoModelo()
	return nil
}

func (m *michiEspia) ManoInicio(estado botsdk.EstadoPublico, _ botsdk.Mano) { m.recordar(estado) }

// EstadoActualizado llega con cada acción de la mesa, propia o ajena.
func (m *michiEspia) EstadoActualizado(estado botsdk.EstadoPublico) { m.recordar(estado) }

// recordar guarda la versión más completa del historial de la mano en curso.
func (m *michiEspia) recordar(estado botsdk.EstadoPublico) {
	if estado.IDMano != m.idMano {
		m.idMano = estado.IDMano
		m.historial = nil
	}
	if len(estado.HistorialAcciones) >= len(m.historial) {
		m.historial = estado.HistorialAcciones
	}
	m.enMano = m.enMano[:0]
	for _, j := range estado.Jugadores {
		if j.EnMano {
			m.enMano = append(m.enMano, j.ID)
		}
	}
}

// ManoFin pasa la mano terminada al modelo: es el momento en que aprende.
func (m *michiEspia) ManoFin(botsdk.ResultadoMano) {
	m.modelo.observar(m.historial, m.enMano, m.miID)
	m.historial = nil
	m.manos++
	if !*silencioso && m.manos%resumenCada == 0 {
		log.Printf("[espia] mano %d\n%s", m.manos, m.modelo.resumen(m.enMano, m.miID))
	}
}

func (m *michiEspia) Decidir(_ context.Context, t botsdk.Turno) botsdk.Accion {
	m.miID = t.IDJugador
	m.recordar(t.Estado)

	rivales := rivalesEnLaMano(t)
	if len(t.Estado.Comunitarias) < 3 {
		// Preflop se mide contra un solo rival: da un orden de manos que no
		// depende del tamaño de la mesa.
		return m.preflop(t, equidad(t.Mano, nil, 1, *muestras, m.azar), rivales)
	}
	simulados := min(len(rivales), maxRivalesSimulados)
	return m.postflop(t, equidad(t.Mano, t.Estado.Comunitarias, simulados, *muestras, m.azar), simulados, rivales)
}

func (m *michiEspia) preflop(t botsdk.Turno, eq float64, rivales []string) botsdk.Accion {
	ciega := t.Estado.CiegaGrande
	huboSubida := t.Estado.ApuestaActual > ciega
	// Con más rivales hace falta mejor mano para lo mismo.
	exigencia := 0.02 * float64(len(rivales)-1)

	switch {
	case eq >= 0.70:
		// Pares altos: sube, y si ya subieron, vuelve a subir.
		return t.Apostar(max(3*ciega, 3*t.Estado.ApuestaActual))
	case eq >= 0.58+exigencia && !huboSubida:
		return t.Apostar(3 * ciega)
	case !huboSubida && eq >= 0.42 && m.modelo.todosEntranPoco(rivales):
		// Robo de ciegas: quienes casi nunca entran las regalan.
		return t.Apostar(3 * ciega)
	case eq >= 0.50+exigencia && t.PorIgualar() <= 4*ciega:
		return pagarOPasar(t)
	}
	return pasarORetirarse(t)
}

func (m *michiEspia) postflop(t botsdk.Turno, eq float64, simulados int, rivales []string) botsdk.Accion {
	// parteJusta es la equidad de una mano promedio: 1/2 contra un rival,
	// 1/4 contra tres.
	parteJusta := 1 / float64(simulados+1)

	if t.PorIgualar() == 0 {
		seRetiran := m.modelo.seRetiranTodos(rivales)
		switch {
		case eq >= parteJusta+0.22:
			if seRetiran < 0.30 {
				return t.ApostarPozo(0.9) // No se van a retirar: que paguen caro.
			}
			return t.ApostarPozo(0.6)
		case eq >= parteJusta+0.10 && seRetiran < 0.35:
			// Apuesta fina: contra pagadores, una mano mediana ya cobra.
			return t.ApostarPozo(0.5)
		case seRetiran > umbralFarol && m.azar.Float64() < 0.7:
			return t.ApostarPozo(0.5)
		}
		return pasarORetirarse(t)
	}

	// Hay que pagar. Quien apuesta cuenta algo sobre su mano, y cuánto creerle
	// depende de quién sea: la apuesta de un rival pasivo pesa más que la de
	// uno que apuesta siempre.
	agresion := m.modelo.de(ultimoAgresor(t)).agresion()
	creible := 0.10 - 0.4*(agresion-agresionTipica)
	precio := float64(t.PorIgualar()) / float64(t.PozoSiPagas())
	switch {
	case eq >= parteJusta+0.35:
		return t.ApostarPozo(1)
	case eq-creible >= precio+0.03:
		return pagarOPasar(t)
	}
	return pasarORetirarse(t)
}

// rivalesEnLaMano lista a los que siguen en la mano, sin contarse.
func rivalesEnLaMano(t botsdk.Turno) []string {
	var ids []string
	for _, j := range t.Estado.Jugadores {
		if j.ID != t.IDJugador && j.Activo && j.EnMano {
			ids = append(ids, j.ID)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, "")
	}
	return ids
}

// ultimoAgresor devuelve quién hizo la última apuesta o subida de la ronda.
func ultimoAgresor(t botsdk.Turno) string {
	historial := t.Estado.HistorialAcciones
	for i := len(historial) - 1; i >= 0; i-- {
		h := historial[i]
		if h.Etapa != t.Estado.Etapa {
			break
		}
		if h.IDJugador != t.IDJugador && esAgresiva(h.Accion.Tipo) {
			return h.IDJugador
		}
	}
	return ""
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

func main() { botsdk.CorrerDesdeFlags(&michiEspia{}) }
