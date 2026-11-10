# Cómo competir en pokerFight

Guía del participante. Si vienes a organizar el torneo, lo que buscas es
[`competencia.md`](competencia.md) (el torneo de las bases) o [`arena.md`](arena.md)
(el round-robin).

Escribes un bot que juega Texas Hold'em No Limit. Se conecta a la arena por una
API y decide solo, sin intervención tuya durante las partidas. Vale cualquier
enfoque: reglas, probabilidades, Machine Learning, aprendizaje por refuerzo, IA
generativa o decisiones al azar.

- [El torneo de las bases](#el-torneo-de-las-bases)
- [El día del torneo](#el-día-del-torneo)
- [En 5 minutos](#en-5-minutos)
- [Qué tienes que entregar](#qué-tienes-que-entregar)
- [Escribir un bot en Go](#escribir-un-bot-en-go)
- [Escribir un bot en otro lenguaje](#escribir-un-bot-en-otro-lenguaje)
- [Probar tu bot](#probar-tu-bot)
- [Reglas del torneo](#reglas-del-torneo)
- [Cómo se puntúa](#cómo-se-puntúa)
- [Los errores que más cuestan](#los-errores-que-más-cuestan)
- [Por dónde mejorar](#por-dónde-mejorar)

---

## El torneo de las bases

El torneo oficial (detalle en [`competencia.md`](competencia.md)) tiene dos etapas:

1. **Clasificatoria.** Cada bot empieza con **10 000 fichas**. Los bots se reparten
   por sorteo en **mesas de 5** (si la división no es exacta, algunas de 6) y juegan
   **10 rondas de 50 manos, o hasta 6 minutos por mesa** (o hasta que quede uno solo),
   con **ciegas fijas de 50 / 100**. El que se queda sin fichas queda eliminado.
   Tras cada ronda **se sortean las mesas de nuevo** y cada bot **conserva sus
   fichas**. Al final pasan los **6 con más fichas** (si quedan menos de 6 con
   fichas, entran los últimos en ser eliminados).
2. **Final en vivo.** Los 6 mejores juegan una mesa final, con 10 000 fichas cada
   uno: gana el último en pie, o el que tenga más fichas luego de **1 hora**.

Lo que importa para escribir tu bot:

- **Juegan más de dos.** Hay mesas de 2 a 6. Un bot pensado solo para heads-up pierde
  información: en `estado.jugadores` están todos los rivales de la mesa.
- **Tus fichas son una decisión.** Con 10 000 fichas y ciegas fijas de 50 / 100
  (100 ciegas grandes de stack), hay que elegir cuándo arriesgarlas: quebrar te saca
  del torneo, y una mesa se cierra a los 6 minutos aunque no se hayan jugado las 50
  manos. Lee las ciegas de `estado.ciega_grande` en vez de suponerlas.
- **No ves nombres.** Tus rivales son alias aleatorios (`j-a1b2c3`), estables durante
  todo el campeonato. Puedes aprender cómo juega uno que se te cruza en varias rondas,
  sin saber de quién es.
- **El plazo por jugada es de 5 segundos.** Se permiten APIs externas (por ejemplo
  LLMs) con el mismo límite y su costo a cargo del equipo. Pasado el plazo, la arena
  juega check (o fold si hay que pagar) y te anota un timeout.

Las secciones siguientes explican cómo escribir y probar el bot; valen igual para
el torneo oficial y para el round-robin de la arena. Lo que dice "heads-up" o
"neto" en [Reglas del torneo](#reglas-del-torneo) y [Cómo se puntúa](#cómo-se-puntúa)
es del round-robin local.

---

## El día del torneo

Las bases: *"En la clasificatoria y la final, el bot se conecta al servidor del
torneo en sesiones con horario fijo, y basta con conectarlo y dejarlo
ejecutándose."*

**Qué hacer.** Arranca tu bot con la dirección del servidor y **tu token**, antes
del horario de la sesión:

```bash
./mi-bot -addr servidor-del-torneo:9000 -token <tu-token>
```

**Qué esperar.**

- Tu bot se conecta **una sola vez** y se queda conectado toda la sesión. La arena
  lo va sentando en una mesa distinta en cada ronda. Entre ronda y ronda tu socket
  queda abierto **y en silencio**: no es una caída, no cierres.
- Si te eliminan, tampoco llega nada. Puedes dejarlo corriendo.
- Cuando la arena cierra el socket, terminó la sesión (o el torneo). Si el torneo
  sigue otro día, vuelve a conectar.

**Si te desconectas** (corte de red, el bot se cae, lo reinicias): la arena **juega
por ti** con check, o fold si hay que pagar, y tu silla sigue ocupada, así que
vas perdiendo las ciegas. Cuando vuelves a conectarte **con el mismo token**
retomas la misma silla con las mismas fichas. Cuanto antes, mejor.

> **El SDK de Go no reconecta solo.** Si el socket se cae, `botsdk.Correr`
> termina. Para un torneo conviene un bucle que lo vuelva a lanzar:
>
> ```go
> for {
> 	err := botsdk.Correr(ctx, miBot, opciones)
> 	if errors.Is(err, botsdk.ErrMesaCerro) && torneoTerminado() { return }
> 	time.Sleep(time.Second) // y reintentar con el mismo token
> }
> ```
>
> Ojo: `ErrMesaCerro` es lo que ves tanto si la arena terminó la sesión como si se
> cayó el servidor. Piensa cuándo darte por vencido.

**Cosas que conviene saber**

- Si respondes algo que no es JSON, **no te desconectan**: se toma como una acción
  inválida (la arena juega la segura) y sigues en pie.
- Si tardas más del plazo, la respuesta llega tarde y se descarta... **salvo que
  llegue dentro de la misma mano pero para otra jugada**: hoy una respuesta tardía
  puede tomarse como la de la jugada siguiente (ver
  [`guia-del-bot.md`](guia-del-bot.md#lo-importante)). Si usas un LLM,
  **responde antes del plazo y con margen**: una respuesta tardía no solo se pierde,
  puede aplicarse donde no corresponde.
- Dos procesos con el mismo token se pisan: el último en conectar se queda con el
  asiento.

---

## En 5 minutos

```bash
cd pokerfight-kit
go build -o bin/ ./cmd/... ./bots/...

# Corre un torneo de prueba entre los bots de ejemplo
./bin/arena correr -config torneo-go.json
```

Copia [`bots/base/main.go`](../bots/base/main.go) (Go) o
[`bots/ejemplos/python/bot.py`](../bots/ejemplos/python/bot.py) (cualquier otro
lenguaje), cámbiale la estrategia y agrégate al `torneo.json` para medirte
contra los de ejemplo.

---

## Qué tienes que entregar

Cada equipo entrega **un solo bot**, hasta el **sábado 21 de noviembre**:

1. **El código del bot**, que el jurado puede revisar.
2. **Una descripción breve** (máximo una página o un README), que es lo que se
   usa para la categoría del jurado.

El día del torneo el bot **corre en tu máquina** y se conecta solo al servidor:

```
<tu comando> -addr <host:puerto> -token <tu-token>
```

Conviene que tu bot acepte esos dos flags, como hacen todos los ejemplos. La
dirección y tu token te los da la organización.

---

## Escribir un bot en Go

Usa [`pkg/botsdk`](../pkg/botsdk/botsdk.go). Resuelve el handshake, la
reconexión, el eco del `id_mano` y el problema de saber cuál de los jugadores
del estado eres tú.

Un bot completo:

```go
package main

import (
	"context"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

type MiBot struct{}

func (MiBot) Nombre() string { return "mi-bot" }

func (MiBot) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	// poker.Mejor es el mismo evaluador que usa el crupier para decidir el
	// showdown: lo que aquí te dice que tienes es exactamente lo que la mesa va
	// a puntuar.
	jugada := poker.Mejor(t.Mano, t.Estado.Comunitarias)

	if jugada.Categoria >= poker.Trio {
		return t.ApostarPozo(0.75) // Apuesta 3/4 del pozo.
	}
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	if t.PorIgualar() <= t.Estado.CiegaGrande {
		return t.Pagar()
	}
	return t.Fold()
}

func main() { botsdk.CorrerDesdeFlags(MiBot{}) }
```

`CorrerDesdeFlags` define `-addr` y `-token`, así que tu bot ya cumple el
contrato de la arena.

Todos los tipos del protocolo que tu bot necesita nombrar (`botsdk.Accion`,
`botsdk.EstadoPublico`, `botsdk.Mano`, `botsdk.Raise`, `botsdk.As`, ...) salen
de `pkg/botsdk`. No importes `internal/protocolo`: Go no deja importar paquetes
`internal/` desde otro módulo, y tu bot no compilaría fuera de este repo.

### La API de `Turno`

Todo lo que sabes al decidir:

| | |
| --- | --- |
| `t.Mano` | Tus dos cartas privadas. |
| `t.Yo` | Tu propia entrada del estado: saldo, apuesta de la ronda, silla. |
| `t.Estado` | La mesa: pozo, comunitarias, rivales, historial de acciones de la mano. |
| `t.Validas` | Las únicas acciones que la mesa va a aceptar ahora. |
| `t.Timeout` | Cuánto tiempo tienes. |

Consultas:

| | |
| --- | --- |
| `t.Puede(botsdk.Raise)` | ¿Está permitida esa acción? |
| `t.PorIgualar()` | Fichas que faltan para igualar. 0 si puedes pasar gratis. |
| `t.Techo()` | Total máximo al que puedes llevar tu apuesta: el all-in. |
| `t.SubidaMinima()` | Total mínimo que la mesa acepta como subida. |
| `t.PozoSiPagas()` | Cuánto habría en el pozo si igualas. Para pot odds. |

Acciones (usa estos constructores, no armes structs a mano):

| | |
| --- | --- |
| `t.Fold()` `t.Check()` `t.Pagar()` `t.AllIn()` | Las simples. |
| `t.Apostar(total)` | Lleva tu apuesta a `total`, ajustando al rango que la mesa acepta. Elige sola entre `bet` y `raise`, y convierte a all-in si corresponde. |
| `t.ApostarPozo(0.5)` | Media apuesta del pozo. Azúcar sobre `Apostar`. |

`Apostar` existe para que no pierdas manos por aritmética: recorta el total a la
subida mínima y a tu techo, y si en ese turno no se puede subir, cae a la mejor
acción pasiva disponible en lugar de devolver algo que la mesa va a descartar.

### Preparar algo antes de jugar

Si tu bot necesita cargar un archivo, inicializar un modelo o sembrar su
generador de números al azar, implementa `Iniciar`. El SDK lo llama una vez,
**después de parsear los flags** y antes de conectarse:

```go
var semilla = flag.Uint64("semilla", 1, "semilla del generador")

func (b *MiBot) Iniciar() error {
	b.azar = rand.New(rand.NewPCG(*semilla, *semilla))
	return nil
}
```

> Si defines flags propios, **no llames a `flag.Parse()` tú mismo**: en ese momento el
> SDK todavía no definió `-addr` ni `-token`, y el parseo falla. Define tus flags
> y deja que `CorrerDesdeFlags` parsee; lee sus valores en `Iniciar`.

Ojo con la aleatoriedad: desde Go 1.22, el generador global de `math/rand`
arranca con una semilla distinta en cada proceso. Un bot que lo use no es
reproducible, y eso hace que el torneo tampoco lo sea.

### Enterarte de más cosas

Implementa estas interfaces opcionales y el SDK te llama solo:

```go
// Principio y fin de cada mano.
func (b *MiBot) ManoInicio(estado botsdk.EstadoPublico, mias botsdk.Mano) {}
func (b *MiBot) ManoFin(resultado botsdk.ResultadoMano) {
	// resultado.Mostradas trae las cartas del rival, si hubo showdown.
	// Es el mejor lugar para aprender a leerlo.
}

// Cada actualización de estado, incluidos los turnos ajenos.
func (b *MiBot) EstadoActualizado(estado botsdk.EstadoPublico) {}
```

### El evaluador

[`pkg/poker`](../pkg/poker/evaluador.go) es el mismo evaluador que usa el
crupier. Úsalo: reimplementarlo es trabajo perdido y arriesgas que tu idea de
qué mano gana no coincida con la de la mesa.

```go
ev := poker.Mejor(t.Mano, t.Estado.Comunitarias)
ev.Categoria     // poker.Par, poker.Color, poker.FullHouse...
ev.Descripcion   // "doble par"
ev.Mejores5      // las 5 cartas que forman la jugada
ev.Puntaje       // entero monótono: comparar manos es comparar enteros
```

Compáralo por `Categoria` para decidir. `Puntaje` sirve para comparar dos manos
entre sí, pero no tiene significado fuera de esa comparación: no lo interpretes
ni lo persistas.

---

## Escribir un bot en otro lenguaje

El protocolo es TCP + JSON Lines, sin dependencias. La referencia completa está
en [`protocolo.md`](protocolo.md).

Empieza copiando un ejemplo:

- [`bots/ejemplos/python/bot.py`](../bots/ejemplos/python/bot.py) — 200 líneas, solo `socket` y `json`.
- [`bots/ejemplos/javascript/bot.js`](../bots/ejemplos/javascript/bot.js) — lo mismo con E/S asíncrona, solo el módulo `net`.
- [`bots/novato/`](../bots/novato/) — un bot de Python completo con la estrategia separada del protocolo: cambias `estrategia.py` y el resto queda igual.

Los dos ya resuelven lo aburrido: el buffer de líneas, el handshake, encontrarte
en el estado, y una clase `Turno` con los mismos ayudantes que el SDK de Go. Tú
solo cambias la función `decidir`.

El bucle mínimo es:

```
conectar
enviar   {"tipo":"saludo","version":"1.0.0","token":"..."}
recibir  bienvenida  → guarda id_jugador
repetir:
    recibir mensaje
    si mano_inicio       → guarda cartas
    si solicitar_accion  → envía {"tipo":"accion","id_mano":..., "accion":{...}}
    si mano_fin, estado  → informativos, no respondas
    si el socket cierra  → la partida terminó
```

---

## Probar tu bot

**Contra los de ejemplo**, que es lo que importa. Agrégate a `torneo.json`:

```json
{
  "id": "mi-bot", "nombre": "Mi Bot",
  "comando": ["python", "mi_bot.py"]
}
```

```bash
./bin/arena correr -config torneo.json -verboso
```

`-verboso` muestra el stdout y stderr de tu bot en la terminal: es donde vas a
ver por qué se cayó.

**Los rivales de referencia**, de más fácil a más difícil:

| Bot | Cómo juega | Qué significa perder contra él |
| --- | ---------- | ------------------------------ |
| `conservador` | Nunca apuesta. Paga barato o se retira. | Tu bot está regalando fichas. |
| `aleatorio` | Elige al azar entre las acciones válidas. | Tu bot está peor que el azar. |
| `base` | Chen preflop, jugada hecha postflop. | Normal al principio. Ganarle de forma consistente es el piso para competir. |
| `novato` | Michi Novato, en Python: tabla de manos iniciales, lectura de la jugada y pot odds, todo con condicionales. | Tu bot no aprovecha lo predecible que es: nunca varía y casi no farolea. |
| `estadistico` | Estima su equidad por Monte Carlo contra los rivales que siguen en la mano y la compara con el precio de pagar. | Tu bot todavía no le saca ventaja a leer al rival. Es el que hay que superar con rangos, posición o lectura de rivales. |
| `darwin` | Gato Darwin, en Go: Monte Carlo y una estrategia evolucionada con un algoritmo genético. | Es el más fuerte de los ejemplos. Ganarle es buena señal para la clasificatoria. |

Michi Novato y Gato Darwin tienen su propio README en
[`bots/novato`](../bots/novato/README.md) y [`bots/darwin`](../bots/darwin/README.md),
con su estrategia explicada paso a paso y por dónde ganarles.

**A mano**, para depurar el protocolo:

```bash
./bin/mesa -addr :9000 -jugadores 2 -min-jugadores 2
# en otra terminal
./bin/aleatorio -addr localhost:9000 -token rival
# en otra
python mi_bot.py -addr localhost:9000 -token yo
```

**El historial**, para entender qué pasó:

```bash
jq . resultados/manos/r001-mi-bot-vs-base.jsonl | less
```

Trae las cartas de todos, todas las acciones y los repartos de cada mano.

---

## Reglas del torneo

Las reglas de poker están en [`reglas.md`](reglas.md). Las del torneo:

1. **Formato:** round-robin heads-up. Cada par juega dos partidas intercambiando
   las posiciones, **con las mismas cartas**. Si le ganas a alguien en las dos,
   le ganaste jugando mejor.
2. **Un token por bot.** Dos bots no pueden compartir token: el segundo en
   conectarse se toma como una reconexión del primero.
3. **Tienes un plazo por turno** (`timeout_ms`: 5000 ms en `torneo-go.json` y en la competencia). Si te
   pasas, la mesa juega por ti la acción segura y te anota un timeout.
4. **Solo se juega a través del protocolo.** No leas los archivos del torneo, no
   te conectes a las mesas de otros, no toques los procesos de los demás. La
   arena no lo impide técnicamente; es una regla, y romperla es descalificación.
5. **Tu bot corre en la máquina del torneo** con recursos compartidos. Si
   consumes toda la CPU, les provocas timeouts a los demás.

---

## Cómo se puntúa

Por defecto, `neto`: **las fichas que ganas o pierdes, sumadas sobre todos los
enfrentamientos**.

No se puntúa por partidas ganadas. Mide *cuánto* mejor juegas, no solo si ganas:
un bot que gana ajustado no queda igual que uno que arrasa.

Con `reponer_stack` activo (lo normal en round-robin), todos vuelven al stack
inicial antes de cada mano, así que se juegan las 200 manos completas y quedar
sin fichas en una mano no te elimina.

Cada timeout puede descontar puntos, según cómo lo configure la organización.

Desempates: puntos → fichas netas → menos timeouts → id.

---

## Los errores que más cuestan

Estos cuatro explican casi todos los bots que "no funcionan":

**1. `monto` es el TOTAL, no el incremento.**
Si ya pusiste 20, `apuesta_actual` es 40 y quieres subir a 100, mandas
`monto: 100`, no `monto: 80`. En Go, usa `t.Apostar(100)` y no pienses más en
esto.

**2. Buscarte en `estado.jugadores` por el token.**
Tu identificador es el `id_jugador` que vino en la **bienvenida**. Con un Casino
detrás, no es el token. Si te equivocas aquí, tu bot decide con el saldo de otro y
no falla nunca de forma visible: solo juega mal.

**3. Responder algo inválido "a ver si pasa".**
Cuesta exactamente lo mismo que no responder: la mesa descarta la acción, juega
la segura y te anota un timeout. No hay ninguna ventaja.

**4. Leer del socket sin buffer de líneas.**
Un `read` puede devolver media línea o dos y media. Acumula hasta el `\n`.

Hay una tabla más completa en
[`protocolo.md`](protocolo.md#errores-frecuentes).

---

## Por dónde mejorar

El bot `base` es deliberadamente flojo. Lo que **no** hace, en orden aproximado
de cuánto rinde arreglarlo:

1. **Posición.** No mira si actúa primero o último. En heads-up eso es enorme:
   con el botón ves actuar al rival antes de decidir.
2. **Proyectos.** No distingue un color hecho de uno a una carta. Cuatro cartas
   del mismo palo en el flop valen mucho más que una carta alta, y `base` las
   trata igual.
3. **Leer al rival.** `estado.historial_acciones` trae todo lo que hizo el rival
   en la mano, y `mano_fin.mostradas` sus cartas cuando hubo showdown. `base`
   ignora las dos cosas. Un bot que ajusta según si el rival sube mucho o poco le
   gana a uno que juega siempre igual.
4. **Farolear.** `base` nunca apuesta sin mano. Contra un rival que se retira
   seguido, eso es plata regalada.
5. **Tamaño de apuesta.** `base` usa 3/4 del pozo siempre. Variar según la
   fuerza de la mano y la textura del flop es de lo que más rinde.

Y si tienes tiempo: `estado.historial_acciones` más el historial de manos del
torneo son suficientes para entrenar algo de verdad contra los bots de ejemplo,
antes de medirte con los demás.

---

## Ver también

- [`protocolo.md`](protocolo.md) — el contrato completo Mesa ↔ Bot.
- [`reglas.md`](reglas.md) — las reglas de poker que aplica la mesa.
- [`arena.md`](arena.md) — cómo se corre el torneo.
