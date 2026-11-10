package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// perfil junta lo que se le vio hacer a un rival.
type perfil struct {
	manos     int // Manos en las que recibió cartas.
	entro     int // Manos en las que puso fichas preflop por decisión propia.
	agresivas int // Apuestas y subidas.
	pasivas   int // Pagos.
	enfrento  int // Veces que le apostaron después del flop.
	seRetiro  int // De esas, cuántas se retiró.
}

// agresionTipica es la agresión que se le supone a un rival desconocido.
const agresionTipica = 0.35

// estimar calcula una frecuencia con un prior Beta: es como haber visto
// `aFavor` éxitos y `enContra` fracasos antes de empezar.
//
// Sin esto, un rival que se retiró en su única oportunidad tendría "100% de
// retiros" y el bot lo farolearía siempre. Con el prior, las primeras
// observaciones mueven poco la estimación y, a medida que se juntan, mandan
// los datos.
func estimar(exitos, intentos int, aFavor, enContra float64) float64 {
	return (float64(exitos) + aFavor) / (float64(intentos) + aFavor + enContra)
}

// entra estima con qué frecuencia juega una mano. Un rival ajustado ronda 0.15;
// uno suelto pasa de 0.40.
func (p perfil) entra() float64 { return estimar(p.entro, p.manos, 2, 4) }

// agresion estima qué parte de sus jugadas son apuestas o subidas.
func (p perfil) agresion() float64 {
	return estimar(p.agresivas, p.agresivas+p.pasivas, 10*agresionTipica, 10*(1-agresionTipica))
}

// seRetira estima con qué frecuencia se retira cuando le apuestan.
func (p perfil) seRetira() float64 { return estimar(p.seRetiro, p.enfrento, 2, 2) }

// modelo guarda un perfil por rival.
type modelo struct {
	rivales map[string]*perfil
}

func nuevoModelo() *modelo { return &modelo{rivales: map[string]*perfil{}} }

// de devuelve el perfil de un rival. De uno desconocido devuelve el perfil
// vacío, cuyas estimaciones son las del prior.
func (m *modelo) de(id string) perfil {
	if p, ok := m.rivales[id]; ok {
		return *p
	}
	return perfil{}
}

func (m *modelo) perfil(id string) *perfil {
	p, ok := m.rivales[id]
	if !ok {
		p = &perfil{}
		m.rivales[id] = p
	}
	return p
}

func esAgresiva(tipo botsdk.TipoAccion) bool {
	return tipo == botsdk.Bet || tipo == botsdk.Raise || tipo == botsdk.AllIn
}

// observar actualiza los perfiles con una mano terminada. `enMano` son los que
// recibieron cartas y `yo` el identificador propio, que no se modela.
func (m *modelo) observar(historial []botsdk.AccionRegistrada, enMano []string, yo string) {
	for _, id := range enMano {
		if id != yo {
			m.perfil(id).manos++
		}
	}

	yaEntro := map[string]bool{}
	huboApuesta := map[botsdk.Etapa]bool{}
	for _, h := range historial {
		tipo := h.Accion.Tipo
		if h.IDJugador != yo {
			p := m.perfil(h.IDJugador)
			switch {
			case esAgresiva(tipo):
				p.agresivas++
			case tipo == botsdk.Call:
				p.pasivas++
			}
			if h.Etapa == botsdk.PreFlop && (esAgresiva(tipo) || tipo == botsdk.Call) && !yaEntro[h.IDJugador] {
				yaEntro[h.IDJugador] = true
				p.entro++
			}
			// Las ciegas obligan a todos preflop: retirarse ahí dice poco. Lo
			// que se mide es la respuesta a una apuesta después del flop.
			if h.Etapa != botsdk.PreFlop && huboApuesta[h.Etapa] {
				p.enfrento++
				if tipo == botsdk.Fold {
					p.seRetiro++
				}
			}
		}
		if esAgresiva(tipo) {
			huboApuesta[h.Etapa] = true
		}
	}
}

// seRetiranTodos estima la probabilidad de que una apuesta se lleve el pozo:
// tienen que retirarse todos.
func (m *modelo) seRetiranTodos(rivales []string) float64 {
	probabilidad := 1.0
	for _, id := range rivales {
		probabilidad *= m.de(id).seRetira()
	}
	return probabilidad
}

// todosEntranPoco indica si todos los rivales son ajustados, con manos
// suficientes vistas como para creerlo.
func (m *modelo) todosEntranPoco(rivales []string) bool {
	for _, id := range rivales {
		if p := m.de(id); p.manos < 15 || p.entra() >= 0.22 {
			return false
		}
	}
	return len(rivales) > 0
}

// resumen describe lo que se sabe de los rivales de la mesa actual.
func (m *modelo) resumen(enMano []string, yo string) string {
	ids := make([]string, 0, len(enMano))
	for _, id := range enMano {
		if id != yo {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	lineas := make([]string, 0, len(ids))
	for _, id := range ids {
		p := m.de(id)
		lineas = append(lineas, fmt.Sprintf("    %-14s entra %2.0f%% | agresión %2.0f%% | se retira %2.0f%% | %d manos",
			id, 100*p.entra(), 100*p.agresion(), 100*p.seRetira(), p.manos))
	}
	return strings.Join(lineas, "\n")
}
