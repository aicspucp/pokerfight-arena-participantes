// Package estrategia es el cerebro de Gato Darwin: una política de juego
// controlada por un genoma de números reales.
//
// La idea es separar QUÉ decide el bot (la forma de la política, escrita a
// mano en politica.go) de CUÁNTO (los umbrales, tamaños de apuesta y
// frecuencias de farol, que son los genes). Los genes no se eligen a ojo: los
// encuentra el algoritmo genético de ../entrenamiento haciendo jugar miles de
// manos a cada candidato y quedándose con los que más fichas ganan.
//
// El paquete lo usan las dos mitades del proyecto, y eso es a propósito: el
// bot real (main.go) y el simulador de entrenamiento toman decisiones con
// exactamente el mismo código. Si no, el genético optimizaría una política
// distinta de la que después juega en la arena.
package estrategia

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"time"
)

// Gen identifica una posición del genoma.
type Gen int

const (
	UmbralApuestaPre  Gen = iota // Fuerza efectiva para abrir preflop.
	UmbralApuestaPost            // Fuerza efectiva para apostar por valor después del flop.
	UmbralSubidaPre              // Fuerza efectiva para resubir preflop.
	UmbralSubidaPost             // Fuerza efectiva para subir después del flop.
	MargenPagoPre                // Paga si fuerza >= pot odds × margen (preflop).
	MargenPagoPost               // Ídem después del flop.
	FarolPre                     // Probabilidad de abrir preflop sin mano.
	FarolPost                    // Probabilidad de apostar de farol si nadie apostó.
	TamApertura                  // Tamaño de la apertura preflop, en ciegas grandes.
	TamValor                     // Apuesta por valor, como fracción del pozo.
	TamFarol                     // Farol, como fracción del pozo.
	TamSubida                    // Lo que se agrega al subir, como fracción del pozo.
	BonoPosicion                 // Cuánto suma hablar último a la fuerza efectiva.
	Presion                      // Cuánto resta cada subida enfrentada en la calle.
	Multijugador                 // Cuánto pesa cada rival extra (exponente).
	UmbralAllIn                  // Fuerza efectiva para ir all-in.
	FarolSubida                  // Probabilidad de resubir de farol.
	AversionRiesgo               // Fuerza extra que se exige al jugarse buena parte del stack.

	CantidadGenes
)

// DefinicionGen describe un gen: su nombre en el JSON, su rango válido y qué
// significa. El rango acota la búsqueda del genético: fuera de él no hay
// estrategias razonables y explorarlo es tiempo perdido.
type DefinicionGen struct {
	Nombre      string
	Min, Max    float64
	Descripcion string
}

// Genes es la tabla de todos los genes, indexada por Gen.
var Genes = [CantidadGenes]DefinicionGen{
	UmbralApuestaPre:  {"umbral_apuesta_pre", 0.30, 0.85, "fuerza efectiva mínima para abrir preflop"},
	UmbralApuestaPost: {"umbral_apuesta_post", 0.30, 0.90, "fuerza efectiva mínima para apostar por valor después del flop"},
	UmbralSubidaPre:   {"umbral_subida_pre", 0.40, 0.95, "fuerza efectiva mínima para resubir preflop"},
	UmbralSubidaPost:  {"umbral_subida_post", 0.45, 0.97, "fuerza efectiva mínima para subir después del flop"},
	MargenPagoPre:     {"margen_pago_pre", 0.50, 1.80, "paga si la fuerza efectiva supera las pot odds por este factor (preflop)"},
	MargenPagoPost:    {"margen_pago_post", 0.50, 1.80, "ídem, después del flop"},
	FarolPre:          {"farol_pre", 0.00, 0.30, "probabilidad de abrir preflop sin mano"},
	FarolPost:         {"farol_post", 0.00, 0.40, "probabilidad de apostar de farol cuando nadie apostó"},
	TamApertura:       {"tam_apertura", 2.00, 5.00, "apertura preflop, en ciegas grandes"},
	TamValor:          {"tam_valor", 0.25, 1.25, "apuesta por valor, como fracción del pozo"},
	TamFarol:          {"tam_farol", 0.25, 1.10, "farol, como fracción del pozo"},
	TamSubida:         {"tam_subida", 0.40, 1.60, "lo que se agrega al subir, como fracción del pozo"},
	BonoPosicion:      {"bono_posicion", 0.00, 0.15, "cuánto suma hablar último a la fuerza efectiva"},
	Presion:           {"presion", 0.00, 0.15, "cuánto resta cada apuesta o subida de la calle"},
	Multijugador:      {"multijugador", 0.20, 1.20, "peso de cada rival extra: fuerza^(1 + (rivales-1)×m)"},
	UmbralAllIn:       {"umbral_allin", 0.75, 0.99, "fuerza efectiva para ir all-in"},
	FarolSubida:       {"farol_subida", 0.00, 0.15, "probabilidad de resubir de farol"},
	AversionRiesgo:    {"aversion_riesgo", 0.00, 0.50, "fuerza extra exigida por jugarse todo el stack (proporcional a lo que se arriesga)"},
}

// Genoma es un individuo: un valor por gen.
type Genoma [CantidadGenes]float64

// GenomaInicial es una estrategia escrita a mano, "tight-agresiva" de manual.
// Es el punto de partida del genético (entra en la primera generación) y la
// línea de base contra la que se mide cuánto mejoró la evolución.
func GenomaInicial() Genoma {
	var g Genoma
	g[UmbralApuestaPre] = 0.55
	g[UmbralApuestaPost] = 0.60
	g[UmbralSubidaPre] = 0.70
	g[UmbralSubidaPost] = 0.75
	g[MargenPagoPre] = 1.10
	g[MargenPagoPost] = 1.10
	g[FarolPre] = 0.05
	g[FarolPost] = 0.10
	g[TamApertura] = 3.0
	g[TamValor] = 0.66
	g[TamFarol] = 0.50
	g[TamSubida] = 0.80
	g[BonoPosicion] = 0.05
	g[Presion] = 0.04
	g[Multijugador] = 0.60
	g[UmbralAllIn] = 0.92
	g[FarolSubida] = 0.02
	g[AversionRiesgo] = 0.15
	return g
}

// Aleatorio devuelve un genoma con cada gen uniforme en su rango.
func Aleatorio(azar *rand.Rand) Genoma {
	var g Genoma
	for i, def := range Genes {
		g[i] = def.Min + azar.Float64()*(def.Max-def.Min)
	}
	return g
}

// Acotar devuelve el genoma con cada gen recortado a su rango. Lo usan el
// cruce y la mutación, que pueden empujar un gen fuera de los límites.
func (g Genoma) Acotar() Genoma {
	for i, def := range Genes {
		g[i] = min(max(g[i], def.Min), def.Max)
	}
	return g
}

// --- Archivo genoma.json -----------------------------------------------------

// Archivo es el formato en disco de un genoma entrenado. Los genes van por
// nombre y no como un arreglo para que el archivo se pueda leer y retocar a
// mano sin tener que contar posiciones.
type Archivo struct {
	Bot        string             `json:"bot"`
	Generacion int                `json:"generacion"`
	Aptitud    float64            `json:"aptitud_bb_100"`
	Semilla    uint64             `json:"semilla"`
	Fecha      string             `json:"fecha"`
	Genes      map[string]float64 `json:"genes"`
}

// NuevoArchivo arma el archivo de un genoma con sus metadatos.
func NuevoArchivo(g Genoma, generacion int, aptitud float64, semilla uint64) Archivo {
	genes := make(map[string]float64, CantidadGenes)
	for i, def := range Genes {
		genes[def.Nombre] = redondear(g[i])
	}
	return Archivo{
		Bot:        "Gato Darwin",
		Generacion: generacion,
		Aptitud:    redondear(aptitud),
		Semilla:    semilla,
		Fecha:      time.Now().Format("2006-01-02 15:04"),
		Genes:      genes,
	}
}

// Genoma reconstruye el genoma del archivo. Un gen que falta es un error: un
// genoma a medias jugaría con ceros en sus umbrales sin avisar.
func (a Archivo) Genoma() (Genoma, error) {
	var g Genoma
	for i, def := range Genes {
		v, ok := a.Genes[def.Nombre]
		if !ok {
			return g, fmt.Errorf("estrategia: al genoma le falta el gen %q", def.Nombre)
		}
		g[i] = v
	}
	return g.Acotar(), nil
}

// LeerGenoma interpreta el contenido de un genoma.json.
func LeerGenoma(datos []byte) (Genoma, Archivo, error) {
	var a Archivo
	if err := json.Unmarshal(datos, &a); err != nil {
		return Genoma{}, a, fmt.Errorf("estrategia: genoma.json ilegible: %w", err)
	}
	g, err := a.Genoma()
	return g, a, err
}

// Guardar escribe el archivo con sangría, para que se pueda leer.
func (a Archivo) Guardar(ruta string) error {
	datos, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}

func redondear(v float64) float64 {
	return math.Round(v*10000) / 10000
}
