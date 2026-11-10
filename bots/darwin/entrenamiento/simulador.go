package entrenamiento

import (
	"math/rand/v2"
	"slices"

	"github.com/aicspucp/pokerfight-arena-participantes/bots/darwin/estrategia"
)

// ConfigMesa son las ciegas y el stack de la simulación, en fichas. Todos
// arrancan cada mano con Stack, como el modo reponer_stack de la arena.
type ConfigMesa struct {
	CiegaChica  int64
	CiegaGrande int64
	Stack       int64
}

type tipoAccion int

const (
	fold tipoAccion = iota
	check
	call
	bet
	raise
	allin
)

type jugadorSim struct {
	politica estrategia.Politica
	azar     *rand.Rand
	asiento  int   // De qué asiento del Reparto son sus cartas.
	saldo    int64 // Lo que le queda.
	apuesta  int64 // Lo puesto en la ronda actual.
	aportado int64 // Lo puesto en toda la mano.
	retirado bool
	allin    bool
}

type manoSim struct {
	cfg           ConfigMesa
	rep           *Reparto
	js            []*jugadorSim
	boton         int
	calle         int
	apuestaActual int64
	subidaMinima  int64
	subidasCalle  int
}

// JugarMano juega una mano completa y devuelve el neto en fichas de cada
// jugador (lo ganado menos lo puesto). La suma de los netos es siempre 0.
//
// politicas[i] se sienta en la silla i y juega las cartas del asiento
// asientos[i] del reparto. Separar silla y cartas es lo que permite jugar un
// mismo reparto varias veces rotando quién recibe qué mano.
//
// stacks[i] son las fichas con que empieza la mano cada uno; nil = todos con
// cfg.Stack (el modo reponer_stack de la arena). Con stacks distintos hay
// pozos laterales, y repartir los resuelve.
//
// Las reglas son las de internal/mesa de la arena (docs/reglas.md): en
// heads-up el botón paga la ciega chica y habla primero preflop; con 3 o más,
// las ciegas van a la izquierda del botón; la subida mínima es la última
// subida completa; un all-in corto no reabre la acción a quien ya habló.
func JugarMano(cfg ConfigMesa, rep *Reparto, politicas []estrategia.Politica, asientos []int, stacks []int64, boton int, semilla uint64) []int64 {
	n := len(politicas)
	m := &manoSim{cfg: cfg, rep: rep, boton: boton, js: make([]*jugadorSim, n)}
	for i, p := range politicas {
		stack := cfg.Stack
		if stacks != nil {
			stack = stacks[i]
		}
		m.js[i] = &jugadorSim{
			politica: p,
			// Un generador por jugador y por mano: así lo que sortea uno
			// (un farol, por ejemplo) no le cambia los sorteos a los demás.
			// Es lo que hace que dos genomas distintos enfrenten exactamente
			// los mismos rivales.
			azar:    rand.New(rand.NewPCG(semilla, uint64(i)+1)),
			asiento: asientos[i],
			saldo:   stack,
		}
	}

	var chica, grande, primero int
	if n == 2 {
		chica, grande, primero = boton, (boton+1)%2, boton
	} else {
		chica, grande, primero = (boton+1)%n, (boton+2)%n, (boton+3)%n
	}
	m.aportar(m.js[chica], cfg.CiegaChica)
	m.aportar(m.js[grande], cfg.CiegaGrande)
	m.apuestaActual = cfg.CiegaGrande
	m.subidaMinima = cfg.CiegaGrande
	m.ronda(primero)

	for calle := 1; calle <= 3 && m.enMano() > 1; calle++ {
		m.calle = calle
		for _, j := range m.js {
			j.apuesta = 0
		}
		m.apuestaActual = 0
		m.subidaMinima = cfg.CiegaGrande
		m.subidasCalle = 0
		m.ronda((boton + 1) % n)
	}

	ganado := m.repartir()
	neto := make([]int64, n)
	for i, j := range m.js {
		neto[i] = ganado[i] - j.aportado
	}
	return neto
}

func (m *manoSim) aportar(j *jugadorSim, monto int64) {
	monto = min(monto, j.saldo)
	j.saldo -= monto
	j.apuesta += monto
	j.aportado += monto
	if j.saldo == 0 {
		j.allin = true
	}
}

func (m *manoSim) enMano() int {
	n := 0
	for _, j := range m.js {
		if !j.retirado {
			n++
		}
	}
	return n
}

func (m *manoSim) puedenActuar() int {
	n := 0
	for _, j := range m.js {
		if !j.retirado && !j.allin {
			n++
		}
	}
	return n
}

// ronda es una ronda de apuestas. Calca rondaApuestas de la mesa de la arena.
func (m *manoSim) ronda(primero int) {
	n := len(m.js)
	pendientes := make([]bool, n)
	cuantos := 0
	for i, j := range m.js {
		if !j.retirado && !j.allin {
			pendientes[i] = true
			cuantos++
		}
	}
	// vista[i] es la apuesta actual que tenía delante el jugador i la última
	// vez que actuó: decide si puede volver a subir tras un all-in corto.
	vista := make([]int64, n)
	actuo := make([]bool, n)

	silla := primero
	for cuantos > 0 {
		if m.enMano() <= 1 {
			return
		}
		j := m.js[silla]
		debe := pendientes[silla] && !j.retirado && !j.allin
		if debe && j.apuesta >= m.apuestaActual && m.puedenActuar() <= 1 {
			debe = false // Nadie más puede apostar: no queda nada que decidir.
		}
		if pendientes[silla] {
			pendientes[silla] = false
			cuantos--
		}
		if debe {
			apuestaAntes := m.apuestaActual
			m.turno(silla, vista, actuo)
			if m.apuestaActual > apuestaAntes {
				for i, o := range m.js {
					if i != silla && !o.retirado && !o.allin && o.apuesta < m.apuestaActual && !pendientes[i] {
						pendientes[i] = true
						cuantos++
					}
				}
			}
		}
		silla = (silla + 1) % n
	}
}

func (m *manoSim) turno(silla int, vista []int64, actuo []bool) {
	j := m.js[silla]
	puedeSubir := !actuo[silla] || m.apuestaActual-vista[silla] >= m.subidaMinima
	pendiente := m.apuestaActual - j.apuesta

	// Las mismas acciones válidas que accionesValidas en la mesa.
	v := validas{
		check: pendiente <= 0,
		call:  pendiente > 0,
		subir: puedeSubir && ((pendiente <= 0 && j.saldo > 0) || (pendiente > 0 && j.saldo > pendiente)),
		allin: j.saldo > 0 && (puedeSubir || j.saldo <= pendiente),
	}

	s := m.situacion(silla, v.subir)
	var d estrategia.Decision
	if conCartas, ok := j.politica.(politicaConCartas); ok {
		visibles := [4]int{0, 3, 4, 5}[m.calle]
		d = conCartas.DecidirConCartas(s, m.rep.Privadas[j.asiento], m.rep.Comunitarias[:visibles], j.azar)
	} else {
		d = j.politica.Decidir(s, j.azar)
	}
	tipo, monto := m.traducir(d, j, v)

	m.procesar(j, tipo, monto)
	vista[silla] = m.apuestaActual
	actuo[silla] = true
	if tipo == bet || tipo == raise || tipo == allin {
		m.subidasCalle++
	}
}

type validas struct{ check, call, subir, allin bool }

// situacion arma lo mismo que estrategia.SituacionDesde con una mesa real.
func (m *manoSim) situacion(silla int, puedeSubir bool) estrategia.Situacion {
	j := m.js[silla]
	cg := float64(m.cfg.CiegaGrande)

	var pozo int64
	sillas := make([]int, 0, len(m.js))
	for i, o := range m.js {
		pozo += o.aportado
		if !o.retirado {
			sillas = append(sillas, i)
		}
	}
	return estrategia.Situacion{
		Calle:         m.calle,
		Fuerza:        m.rep.Fuerza[j.asiento][m.calle],
		Rivales:       len(sillas) - 1,
		Posicion:      estrategia.Posicion(sillas, silla, m.boton, len(m.js)),
		Subidas:       m.subidasCalle,
		Pozo:          float64(pozo) / cg,
		PorIgualar:    float64(max(m.apuestaActual-j.apuesta, 0)) / cg,
		ApuestaActual: float64(m.apuestaActual) / cg,
		SubidaMinima:  float64(m.apuestaActual+m.subidaMinima) / cg,
		Techo:         float64(j.saldo+j.apuesta) / cg,
		PuedeSubir:    puedeSubir,
	}
}

// traducir convierte la decisión en una acción válida, con la misma lógica
// que estrategia.AAccion + botsdk.Turno.Apostar en el bot real.
func (m *manoSim) traducir(d estrategia.Decision, j *jugadorSim, v validas) (tipoAccion, int64) {
	pasiva := func() tipoAccion {
		if v.check {
			return check
		}
		return fold
	}
	switch d.Tipo {
	case estrategia.TodoAdentro:
		if v.allin {
			return allin, 0
		}
		return m.apostar(j.saldo+j.apuesta, j, v)
	case estrategia.Subir:
		return m.apostar(int64(d.Total*float64(m.cfg.CiegaGrande)+0.5), j, v)
	case estrategia.Pagar:
		switch {
		case v.call:
			return call, 0
		case v.check:
			return check, 0
		case v.allin:
			return allin, 0
		}
		return fold, 0
	default:
		return pasiva(), 0
	}
}

// apostar es botsdk.Turno.Apostar: lleva la apuesta a `total`, recortando a
// la subida mínima y al all-in, y si no se puede subir cae a lo pasivo.
func (m *manoSim) apostar(total int64, j *jugadorSim, v validas) (tipoAccion, int64) {
	tipo := raise
	if m.apuestaActual <= j.apuesta {
		tipo = bet
	}
	techo := j.saldo + j.apuesta
	if !v.subir {
		switch {
		case v.allin && total >= techo:
			return allin, 0
		case v.check:
			return check, 0
		case v.call:
			return call, 0
		}
		return fold, 0
	}
	if total >= techo {
		if v.allin {
			return allin, 0
		}
		total = techo
	}
	total = max(total, m.apuestaActual+m.subidaMinima)
	if total > techo {
		switch {
		case v.allin:
			return allin, 0
		case v.call:
			return call, 0
		}
		return fold, 0
	}
	return tipo, total
}

// procesar aplica la acción. Calca procesarAccion de la mesa.
func (m *manoSim) procesar(j *jugadorSim, tipo tipoAccion, monto int64) {
	switch tipo {
	case fold:
		j.retirado = true
	case call:
		m.aportar(j, m.apuestaActual-j.apuesta)
	case bet, raise:
		incremento := monto - m.apuestaActual
		m.aportar(j, monto-j.apuesta)
		if incremento >= m.subidaMinima {
			m.subidaMinima = incremento
		}
		m.apuestaActual = j.apuesta
	case allin:
		previa := m.apuestaActual
		m.aportar(j, j.saldo)
		if j.apuesta > previa {
			if incremento := j.apuesta - previa; incremento >= m.subidaMinima {
				m.subidaMinima = incremento
			}
			m.apuestaActual = j.apuesta
		}
	}
}

// repartir decide el showdown con pozos laterales: cada "nivel" de aporte es
// un pozo que solo pueden ganar los que pusieron al menos eso.
func (m *manoSim) repartir() []int64 {
	n := len(m.js)
	ganado := make([]int64, n)

	vivos := make([]int, 0, n)
	var total int64
	for i, j := range m.js {
		total += j.aportado
		if !j.retirado {
			vivos = append(vivos, i)
		}
	}
	if len(vivos) == 1 {
		ganado[vivos[0]] = total
		return ganado
	}

	niveles := make([]int64, 0, len(vivos))
	for _, i := range vivos {
		niveles = append(niveles, m.js[i].aportado)
	}
	slices.Sort(niveles)
	niveles = slices.Compact(niveles)

	var previo, repartido int64
	for _, nivel := range niveles {
		var pozo int64
		for _, j := range m.js {
			pozo += min(j.aportado, nivel) - min(j.aportado, previo)
		}
		mejor := -1
		var ganadores []int
		// Se recorre desde la izquierda del botón: la ficha que sobra de un
		// pozo que no divide exacto va al primero en ese orden, como en la mesa.
		for k := 1; k <= n; k++ {
			i := (m.boton + k) % n
			j := m.js[i]
			if j.retirado || j.aportado < nivel {
				continue
			}
			p := m.rep.Puntaje[j.asiento]
			switch {
			case p > mejor:
				mejor, ganadores = p, []int{i}
			case p == mejor:
				ganadores = append(ganadores, i)
			}
		}
		parte := pozo / int64(len(ganadores))
		for _, g := range ganadores {
			ganado[g] += parte
		}
		ganado[ganadores[0]] += pozo - parte*int64(len(ganadores))
		repartido += pozo
		previo = nivel
	}
	// Lo que puso por encima del último nivel alguien que se retiró no tiene
	// a quién volver: se lo lleva el mejor de los que quedan.
	if resto := total - repartido; resto > 0 {
		mejor := vivos[0]
		for _, i := range vivos[1:] {
			if m.rep.Puntaje[m.js[i].asiento] > m.rep.Puntaje[m.js[mejor].asiento] {
				mejor = i
			}
		}
		ganado[mejor] += resto
	}
	return ganado
}
