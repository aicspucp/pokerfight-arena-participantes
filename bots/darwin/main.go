// Gato Darwin: bot de ejemplo de dificultad INTERMEDIA para pokerFight.
//
// Juega con una política cuyos números (umbrales, tamaños de apuesta, faroles)
// salieron de un algoritmo genético: cientos de estrategias candidatas se
// enfrentaron a rivales de práctica durante miles de manos simuladas, y las
// que más fichas ganaron se cruzaron y mutaron generación tras generación.
// El resultado está en genoma.json, que queda incluido en el binario.
//
// En cada turno el bot:
//
//  1. Estima la fuerza de su mano por Monte Carlo con pkg/poker, el mismo
//     evaluador que usa el crupier.
//  2. La corrige por cantidad de rivales, posición y agresión de la calle.
//  3. Compara contra los umbrales del genoma y elige pasar, pagar, subir o
//     retirarse (estrategia/politica.go).
//
// Uso (lo mismo que hace la arena cuando lo lanza):
//
//	gato-darwin -addr localhost:9000 -token mi-token
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

// genomaIncluido es el genoma entrenado. Se incluye en el binario para que el
// bot funcione aunque la arena lo lance desde otro directorio.
//
//go:embed genoma.json
var genomaIncluido []byte

// tablaPreflop es la fuerza de las 169 manos iniciales, precalculada por el
// entrenamiento. Sin ella, cada proceso la recalcularía de cero.
//
//go:embed preflop.json
var tablaPreflop []byte

var (
	rutaGenoma = flag.String("genoma", "", "genoma.json a usar en lugar del incluido en el binario")
	semilla    = flag.Uint64("semilla", 7, "semilla de los faroles y del Monte Carlo")
	muestras   = flag.Int("muestras", 1500, "muestras de Monte Carlo por decisión después del flop")
	silencioso = flag.Bool("silencioso", false, "no imprime cada decisión")
)

type gatoDarwin struct {
	genoma estrategia.Genoma
	azar   *rand.Rand
	log    *log.Logger
}

func (*gatoDarwin) Nombre() string { return "gato-darwin" }

// Iniciar implementa botsdk.Inicializable: carga el genoma y siembra el
// generador. El SDK lo llama después de parsear los flags.
func (b *gatoDarwin) Iniciar() error {
	datos := genomaIncluido
	if *rutaGenoma != "" {
		leidos, err := os.ReadFile(*rutaGenoma)
		if err != nil {
			return err
		}
		datos = leidos
	}
	genoma, archivo, err := estrategia.LeerGenoma(datos)
	if err != nil {
		return err
	}
	b.genoma = genoma

	var tabla map[string]float64
	if err := json.Unmarshal(tablaPreflop, &tabla); err != nil {
		return fmt.Errorf("preflop.json ilegible: %w", err)
	}
	estrategia.CargarTablaPreflop(tabla)
	// Una fuente propia y sembrada en vez de la global de math/rand, que
	// cambia en cada proceso: así una partida del bot es reproducible.
	b.azar = rand.New(rand.NewPCG(*semilla, *semilla^0xDA8F1))
	b.log = log.New(os.Stderr, "", 0)
	b.log.Printf("Gato Darwin listo: genoma de la generación %d (aptitud %.1f bb/100 en entrenamiento)", archivo.Generacion, archivo.Aptitud)
	return nil
}

// Decidir implementa botsdk.Bot.
func (b *gatoDarwin) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	// Se reserva como mucho un cuarto del plazo para pensar: el resto es
	// margen para la red. Pasarse significa que la mesa juega por nosotros.
	presupuesto := min(t.Timeout/4, 400*time.Millisecond)
	if presupuesto <= 0 {
		presupuesto = 200 * time.Millisecond
	}
	limite := time.Now().Add(presupuesto)
	if plazo, ok := ctx.Deadline(); ok && plazo.Before(limite) {
		limite = plazo
	}

	fuerza := estrategia.FuerzaConPlazo(t.Mano, t.Estado.Comunitarias, *muestras, limite, b.azar)
	situacion := estrategia.SituacionDesde(t, fuerza)
	decision := b.genoma.Decidir(situacion, b.azar)
	accion := estrategia.AAccion(decision, t)

	if !*silencioso {
		b.log.Printf("[%-7s] %s %s | mesa %-14s | fuerza %3.0f%% (efectiva %3.0f%%) vs %d | %-10s | %s",
			t.Estado.Etapa, t.Mano[0], t.Mano[1], cartas(t.Estado.Comunitarias),
			fuerza*100, b.genoma.FuerzaEfectiva(situacion)*100, situacion.Rivales,
			describir(accion), decision.Motivo)
	}
	return accion
}

func cartas(cs []botsdk.Carta) string {
	if len(cs) == 0 {
		return "-"
	}
	texto := ""
	for i, c := range cs {
		if i > 0 {
			texto += " "
		}
		texto += c.String()
	}
	return texto
}

func describir(a botsdk.Accion) string {
	if a.Tipo == botsdk.Bet || a.Tipo == botsdk.Raise {
		return fmt.Sprintf("%s %d", a.Tipo, a.Monto)
	}
	return string(a.Tipo)
}

func main() { botsdk.CorrerDesdeFlags(&gatoDarwin{}) }
