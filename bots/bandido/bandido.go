package main

import (
	"fmt"
	"math"
	"strings"
)

// bandido es un bandido multibrazo con UCB1 y olvido.
//
// UCB1 ("upper confidence bound") le pone a cada brazo un puntaje optimista:
// su recompensa media más un bono que crece cuanto menos se lo probó. Elegir
// siempre el de mayor puntaje hace que un brazo poco probado reciba
// oportunidades, y que deje de recibirlas en cuanto queda claro que rinde
// menos.
//
// El olvido multiplica todo lo aprendido por un factor apenas menor que 1
// después de cada mano. Así lo reciente pesa más: sirve cuando los rivales
// cambian, que es lo que pasa cada vez que se sortean las mesas.
type bandido struct {
	jugadas     []float64 // Veces que se jugó cada brazo, con olvido.
	recompensas []float64 // Suma de recompensas de cada brazo, con olvido.
	exploracion float64   // Peso del bono: más alto, más explora.
	olvido      float64   // 1 = no olvida nada.
}

// sinDatos es la cantidad de jugadas por debajo de la cual un brazo se prueba
// sin calcular nada. Es menor que 1 porque el olvido ya le descuenta un poco a
// un brazo recién jugado.
const sinDatos = 0.5

func nuevoBandido(brazos int) *bandido {
	return &bandido{
		jugadas:     make([]float64, brazos),
		recompensas: make([]float64, brazos),
		// El resultado de una mano de poker varía mucho más que la diferencia
		// entre dos estilos. Un bono pequeño evita explorar para siempre.
		exploracion: 0.2,
		olvido:      0.997, // Lo de hace ~300 manos pesa un tercio.
	}
}

// elegir devuelve el brazo a jugar.
func (b *bandido) elegir() int {
	total := 0.0
	for i, n := range b.jugadas {
		// Un brazo casi sin datos se prueba antes que cualquier cálculo. Con
		// el olvido, uno abandonado vuelve a caer aquí cada tanto: es la
		// segunda oportunidad que le permite notar que los rivales cambiaron.
		if n < sinDatos {
			return i
		}
		total += n
	}

	mejor, mejorPuntaje := 0, math.Inf(-1)
	for i, n := range b.jugadas {
		media := b.recompensas[i] / n
		bono := b.exploracion * math.Sqrt(math.Log(total)/n)
		if puntaje := media + bono; puntaje > mejorPuntaje {
			mejor, mejorPuntaje = i, puntaje
		}
	}
	return mejor
}

// actualizar anota la recompensa (entre 0 y 1) de haber jugado un brazo.
func (b *bandido) actualizar(brazo int, recompensa float64) {
	for i := range b.jugadas {
		b.jugadas[i] *= b.olvido
		b.recompensas[i] *= b.olvido
	}
	b.jugadas[brazo]++
	b.recompensas[brazo] += recompensa
}

// resumen describe lo aprendido: qué parte de las manos recientes jugó cada
// estilo y su resultado medio en ciegas grandes por mano.
func (b *bandido) resumen(estilos []estilo) string {
	total := 0.0
	for _, n := range b.jugadas {
		total += n
	}
	partes := make([]string, 0, len(estilos))
	for i, e := range estilos {
		if b.jugadas[i] == 0 {
			partes = append(partes, e.nombre+" sin probar")
			continue
		}
		media := b.recompensas[i]/b.jugadas[i]*2*topeRecompensa - topeRecompensa
		partes = append(partes, fmt.Sprintf("%s %.0f%% (%+.1f ciegas/mano)", e.nombre, 100*b.jugadas[i]/total, media))
	}
	return strings.Join(partes, " | ")
}
