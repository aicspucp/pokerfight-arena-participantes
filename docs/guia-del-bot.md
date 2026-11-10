# Guía del bot

Las interfaces y las cosas importantes para escribir un bot, en una página. El
detalle mensaje por mensaje está en [`protocolo.md`](protocolo.md); aquí está lo que
conviene saber **antes**.

- [El contrato en 30 segundos](#el-contrato-en-30-segundos)
- [Escribir un bot en Go](#escribir-un-bot-en-go)
- [Escribir un bot en otro lenguaje](#escribir-un-bot-en-otro-lenguaje)
- [Lo importante](#lo-importante)
- [Reconectar](#reconectar)
- [Probar](#probar)
- [Qué mira la mesa y qué ve tu bot](#qué-mira-la-mesa-y-qué-ve-tu-bot)

---

## El contrato en 30 segundos

TCP, **un mensaje JSON por línea** (`\n` al final), UTF-8.

```
bot  ── saludo ───────────►  mesa      {"tipo":"saludo","version":"1.0.0","token":"mi-bot"}
bot  ◄── bienvenida ───────  mesa      tu id_jugador y tu silla
bot  ◄── mano_inicio ──────  mesa      tus 2 cartas privadas
bot  ◄── solicitar_accion ─  mesa      es tu turno (trae acciones_validas)
bot  ── accion ───────────►  mesa      {"tipo":"accion","id_mano":"...","accion":{"tipo":"call"}}
bot  ◄── estado ───────────  mesa      alguien jugó (informativo, no se responde)
bot  ◄── mano_fin ─────────  mesa      repartos y cartas mostradas
```

| Mensaje | Quién lo manda | Se responde |
| ------- | -------------- | ----------- |
| `saludo` | bot | — |
| `bienvenida`, `mano_inicio`, `estado`, `mano_fin`, `error` | mesa | no |
| `solicitar_accion` | mesa | **sí**, con `accion` |
| `accion`, `abandono` | bot | — |

Las acciones son `fold`, `check`, `call`, `bet`, `raise` y `allin`. Solo puedes
mandar las que vienen en `acciones_validas` del turno.

La **versión** del saludo tiene que ser exacta (hoy `1.0.0`). Un bot
desactualizado falla al conectar, no a mitad del torneo.

---

## Escribir un bot en Go

El mínimo es un tipo con dos métodos:

```go
package main

import (
	"context"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

type MiBot struct{}

func (MiBot) Nombre() string { return "mi-bot" }

func (MiBot) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	return t.Fold()
}

func main() { botsdk.CorrerDesdeFlags(MiBot{}) }
```

Se compila y se conecta con `-addr` y `-token`:

```bash
go build -o bin/ ./bots/mi-bot
./bin/mi-bot -addr localhost:9000 -token mi-bot
```

Con `-o bin/` (terminado en `/`) Go nombra el binario solo y en Windows le agrega
`.exe`. No uses `-o bin/mi-bot`: en Windows deja un archivo sin `.exe` que ni
PowerShell ni la arena pueden ejecutar.

### Qué trae `Turno`

| Campo o método | Qué es |
| -------------- | ------ |
| `t.Mano` | Tus dos cartas privadas |
| `t.Estado` | La foto pública: pozo, cartas comunitarias, jugadores, historial de acciones de la mano |
| `t.Yo` | Tu propia entrada de `Estado.Jugadores` (saldo, apuesta de la ronda), ya resuelta |
| `t.Validas`, `t.Puede(tipo)` | Las acciones que la mesa va a aceptar ahora |
| `t.Timeout` | El plazo para responder |
| `t.PorIgualar()` | Cuánto te falta para igualar la apuesta actual |
| `t.Techo()` | Lo máximo a lo que puedes llegar (tu all-in) |
| `t.SubidaMinima()` | El total mínimo de una subida válida |
| `t.PozoSiPagas()` | El pozo si igualas |

### Cómo responder

Arma la acción con los constructores: calculan bien los montos y evitan mandar una
acción inválida por un error de aritmética.

| Constructor | Resultado |
| ----------- | --------- |
| `t.Fold()`, `t.Check()`, `t.Pagar()`, `t.AllIn()` | La acción simple |
| `t.Apostar(total)` | Apuesta o sube **hasta ese total**, recortado a lo que la mesa acepta |
| `t.ApostarPozo(fraccion)` | Apuesta una fracción del pozo (`0.5` es media apuesta) |

### Hooks opcionales

Si tu bot quiere enterarse de más, implementa cualquiera de estas interfaces (el
SDK las detecta solo):

| Interfaz | Para qué |
| -------- | -------- |
| `Inicializable` (`Iniciar() error`) | Preparar algo una vez antes de conectar: cargar un modelo, abrir un archivo. Se llama **después** de parsear los flags |
| `ObservaManos` (`ManoInicio`, `ManoFin`) | Saber cuándo empieza y termina cada mano y qué se mostró en el showdown |
| `ObservaEstado` (`EstadoActualizado`) | Ver cada actualización de estado, incluidas las jugadas de los rivales |

> **Flags propios:** defínelos antes de llamar a `CorrerDesdeFlags`, pero **no
> llames a `flag.Parse`**: el SDK agrega `-addr` y `-token` y recién entonces
> parsea. Para leer el valor de un flag propio, usa `Iniciar()`.

Ejemplos reales: [`bots/conservador`](../bots/conservador/main.go) (el más corto),
[`bots/estadistico`](../bots/estadistico) (simula manos para calcular su equidad),
[`bots/bandido`](../bots/bandido) (aprende mientras juega qué estilo le rinde más),
[`bots/espia`](../bots/espia) (estudia a cada rival) y [`bots/darwin`](../bots/darwin)
(estrategia hallada con un algoritmo genético).

**En tu propio repositorio.** No hace falta escribir el bot dentro de esta carpeta. En
tu módulo de Go, trae el SDK y usa los mismos imports:

```bash
go get github.com/aicspucp/pokerfight-arena-participantes@main
```

---

## Escribir un bot en otro lenguaje

Parte de [`bots/ejemplos/python/bot.py`](../bots/ejemplos/python/bot.py) o de
[`bots/ejemplos/javascript/bot.js`](../bots/ejemplos/javascript/bot.js): tienen todo
el ciclo, comentado. Si usas otro lenguaje, [`protocolo.md`](protocolo.md) es todo
lo que hace falta.

Tres cosas que salen mal seguido:

1. **Acumular en un buffer hasta el `\n`.** Un `read` puede traer media línea o dos.
2. **Buscarte por `id_jugador`**, el de la `bienvenida`, no por el token.
3. **Devolver el `id_mano` de la solicitud** en cada `accion`.

---

## Lo importante

| Tema | Qué pasa |
| ---- | -------- |
| **Plazo** | 5 s por jugada en la competencia y en el servidor de pruebas (2 s por defecto en una mesa local: usa `-timeout-ms 5000` para igualarlo). Pasado el plazo la mesa juega `check` (si es gratis) o `fold` |
| **Timeout** | Cada acción segura que la mesa juega por ti suma un timeout, y **resta en el ranking** |
| **Acción inválida** | Cuesta exactamente lo mismo que no responder. No hay ventaja en "probar suerte" |
| **`monto`** | Es el **total** de tu apuesta en la ronda, no el incremento |
| **Subida mínima** | Con stack corto, usa `allin` |
| **Responder a `estado` o `mano_fin`** | La mesa lo descarta, y si mandas el `id_mano` equivocado te cuesta timeouts |
| **Una línea que no es JSON** | No te desconecta: se toma como acción inválida |
| **Dos procesos con el mismo token** | El segundo **reemplaza** al primero |
| **Fin de la partida** | La mesa cierra el socket. Un `read` que devuelve EOF es el final normal, no un error |
| **Cartas ajenas** | Nunca las recibes, salvo las que se muestran en un showdown (`mano_fin.mostradas`) |

> **Si usas un modelo de lenguaje, responde con margen.** Una respuesta que llega
> pasado el plazo no solo se pierde: hoy, dentro de una **misma mano**, puede
> tomarse como la respuesta a la **siguiente** solicitud (ver
> [`torneo.md`](torneo.md#el-día-del-torneo)). Deja un colchón de varios segundos.

---

## Reconectar

Si tu bot pierde la conexión y vuelve con el **mismo token**, la mesa le devuelve su
silla y su saldo. Mientras estuvo fuera, la mesa jugó por él con la acción segura.

**El SDK de Go no reconecta solo**: si el socket se cae, `botsdk.Correr` termina. Un
bucle simple alcanza:

```go
for {
	err := botsdk.Correr(ctx, MiBot{}, botsdk.Opciones{Direccion: addr, Token: token})
	if ctx.Err() != nil {
		return
	}
	// ErrMesaCerro es lo que ves tanto si la partida terminó como si se cayó el servidor.
	log.Printf("desconectado (%v); reintento en 1 s", err)
	time.Sleep(time.Second)
}
```

En la competencia, tu conexión **dura todo el torneo** y queda **en silencio entre
una ronda y la siguiente**: no es una caída, no la cierres. Está explicado en
[`torneo.md`](torneo.md#el-día-del-torneo) y en [`competencia.md`](competencia.md).

---

## Probar

```bash
./bin/mesa -addr :9000 -jugadores 2 -min-jugadores 2     # una mesa
nc localhost 9000                                         # y se le habla a mano
```

Windows no trae `nc`. Esta línea de Python hace lo mismo (sirve en PowerShell y en
Git Bash):

```bash
python -c "import socket,sys,threading as t;s=socket.create_connection(('localhost',9000));t.Thread(target=lambda:[print(l.decode(),end='',flush=True) for l in s.makefile('rb')],daemon=True).start();[s.sendall(l.lstrip('﻿').encode()) for l in sys.stdin]"
```

Pega `{"tipo":"saludo","version":"1.0.0","token":"demo"}` y la mesa contesta con
la bienvenida: así se ve el protocolo sin escribir código. El detalle, en
[`protocolo.md`](protocolo.md#probar-a-mano).

Un torneo entero entre varios bots (el tuyo incluido), con ida y vuelta y las mismas
cartas para todos: [`arena.md`](arena.md). Agregas tu bot al `torneo-go.json` con su
comando:

```json
{ "id": "mi-bot", "nombre": "Mi bot", "comando": ["./bin/mi-bot"] }
```

---

## Qué mira la mesa y qué ve tu bot

- La mesa **no confía en tu bot**: valida cada acción contra tu saldo, la apuesta
  actual y la subida mínima. No se puede apostar lo que no se tiene.
- Las **fichas se conservan**: la suma de todos los jugadores no cambia por una
  mano.
- Las reglas de poker (ciegas, botón, pozos laterales, empates) están en
  [`reglas.md`](reglas.md).
