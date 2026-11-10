// Comando bot-estadistico: un bot que decide con probabilidades. En cada turno
// estima qué parte del pozo gana su mano (su equidad) jugando la mano miles de
// veces con las cartas que no ve repartidas al azar, y compara ese número con
// lo que le cuesta seguir.
//
// La estrategia, calle por calle:
//
//   - Se cuenta cuántos rivales siguen en la mano y se estima la equidad por
//     Monte Carlo (equidad.go). Con las cartas que el bot conoce fuera del
//     mazo: en el flop se sortea entre las 47 restantes.
//   - La equidad se compara con la parte justa, 1/(rivales+1). Si la supera
//     con holgura, se sube por valor, más fuerte cuanto mayor es la ventaja.
//   - Si no, se paga solo si la equidad cubre el precio: lo que cuesta pagar
//     dividido por el pozo que se gana. Si no lo cubre, se retira.
//   - Cerca de cada umbral la decisión se sortea, y de vez en cuando se
//     farolea heads-up, para no ser un bot que se lee solo (decision.go).
//
// Lo que NO hace todavía, y es el plan de las fases siguientes: suponer que
// quien subió tiene una mano mejor que una al azar (rangos), mirar la posición,
// cuidar el stack en un torneo con eliminación ni recordar cómo juega cada
// rival.
package main

import (
	"context"
	"flag"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// Los flags se declaran a nivel de paquete porque CorrerDesdeFlags es quien
// parsea: sus valores recién existen cuando el SDK llama a Iniciar.
var (
	semilla      = flag.Uint64("semilla", 1, "semilla del generador. Con la misma semilla el bot repite sus decisiones")
	simulaciones = flag.Int("simulaciones", 10000, "manos simuladas por decisión. Más es más preciso y más lento")
	detalle      = flag.Bool("detalle", false, "escribe en stderr la equidad y la acción de cada turno")
)

const (
	// fraccionDelPlazo es qué parte del timeout de la mesa se usa para
	// simular. El resto queda de margen para la red y para que una máquina
	// cargada (la arena corre varias partidas en paralelo) no provoque un
	// timeout.
	fraccionDelPlazo = 4
	// presupuestoMaximo acota la simulación aunque la mesa dé mucho tiempo
	// (el campeonato da 5 s): pasadas las 10 000 simulaciones no se gana
	// precisión que cambie una decisión.
	presupuestoMaximo = 500 * time.Millisecond
	// presupuestoSinPlazo se usa si la mesa no informa timeout.
	presupuestoSinPlazo = 300 * time.Millisecond
)

type botEstadistico struct {
	azar            *rand.Rand
	maxSimulaciones int
	registro        *log.Logger
}

// Iniciar implementa botsdk.Inicializable. El bot lleva su propia fuente
// sembrada para que un torneo que lo incluya sea reproducible: la global de
// math/rand arranca con una semilla distinta en cada proceso.
//
// La reproducibilidad vale mientras la simulación no se corte por tiempo: si
// la máquina está tan cargada que no llega a las simulaciones pedidas, el
// número de sorteos cambia y con él las decisiones siguientes.
func (b *botEstadistico) Iniciar() error {
	b.azar = rand.New(rand.NewPCG(*semilla, *semilla^0x9E3779B97F4A7C15))
	b.maxSimulaciones = max(1, *simulaciones)
	if *detalle {
		b.registro = log.New(os.Stderr, "estadistico: ", log.Lmsgprefix)
	}
	return nil
}

func (*botEstadistico) Nombre() string { return "estadistico" }

func (b *botEstadistico) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	if !manoValida(t.Mano) {
		// Sin cartas no hay nada que estimar: se juega lo que jugaría la
		// mesa por el bot, sin el timeout.
		if t.Puede(botsdk.Check) {
			return t.Check()
		}
		return t.Fold()
	}

	rivales := contarRivales(t)
	eq := estimarEquidad(ctx, b.azar, simulacion{
		mano:            t.Mano,
		comunitarias:    t.Estado.Comunitarias,
		rivales:         rivales,
		maxSimulaciones: b.maxSimulaciones,
		presupuesto:     presupuesto(t.Timeout),
	})
	accion := decidir(t, eq.Valor, rivales, b.azar)

	if b.registro != nil {
		b.registro.Printf("%s %s%s: equidad %.3f contra %d rivales (%d simulaciones), precio %.3f -> %s %d",
			t.Estado.Etapa, t.Mano[0], t.Mano[1], eq.Valor, rivales, eq.Simulaciones, precioDePagar(t), accion.Tipo, accion.Monto)
	}
	return accion
}

// contarRivales cuenta los jugadores que siguen en la mano además del bot.
// Activo pasa a false con el fold, y EnMano deja afuera a los que no
// recibieron cartas (los que se sentaron a mitad de la mano).
func contarRivales(t botsdk.Turno) int {
	rivales := 0
	for _, j := range t.Estado.Jugadores {
		if j.ID != t.IDJugador && j.Activo && j.EnMano {
			rivales++
		}
	}
	return rivales
}

// presupuesto es cuánto tiempo se le da a la simulación según el plazo de la
// mesa.
func presupuesto(plazo time.Duration) time.Duration {
	if plazo <= 0 {
		return presupuestoSinPlazo
	}
	return min(plazo/fraccionDelPlazo, presupuestoMaximo)
}

func main() { botsdk.CorrerDesdeFlags(&botEstadistico{}) }
