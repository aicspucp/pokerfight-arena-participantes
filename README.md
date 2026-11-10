# PokerFight · Kit de inicio

Todo lo que necesitas para desarrollar y probar tu bot del **Poker Fight de Bots**
en tu propia máquina: una arena que juega Texas Hold'em No-Limit, bots de ejemplo
y de sparring, y la documentación de la API.

Tu bot puede estar escrito en **cualquier lenguaje**: habla con la mesa por TCP,
con un mensaje JSON por línea. Hay un SDK para Go y ejemplos completos en Python y
JavaScript.

> **Qué es este kit y qué no.** Es la arena para practicar. **No** incluye el
> servidor de la competencia (sorteo de mesas, rondas, clasificación): ese corre en
> la organización y se conecta con el mismo protocolo. Lo que funciona aquí funciona
> allá.

## Arranque rápido

Necesitas [Go](https://go.dev/dl/) para compilar la arena, aunque tu bot esté en otro
lenguaje. Si tu versión es anterior a la que dice `go.mod`, Go descarga sola la que
hace falta.

```bash
git clone https://github.com/aicspucp/pokerfight-arena-participantes.git
cd pokerfight-arena-participantes

go build -o bin/ ./cmd/... ./bots/...       # compila la arena y los bots de ejemplo
./bin/arena correr -config torneo-go.json   # un torneo de prueba entre los bots de ejemplo
```

Si eso imprime una tabla de posiciones, todo funciona. (Con `make` es `make build` y
`make torneo-local`. En Windows con PowerShell los binarios se llaman
`.\bin\arena.exe`, `.\bin\mesa.exe`, etc.)

Para traer las correcciones que publique la organización: `git pull` y volver a compilar.

## Probar tu bot

Hay tres caminos, y conviene usar los tres:

**1. En tu máquina, contra los bots de ejemplo.** Es lo más rápido para iterar.

```bash
# Terminal 1: una mesa con el plazo de la competencia (5 segundos)
./bin/mesa -addr :9000 -jugadores 6 -min-jugadores 3 -rondas 200 -timeout-ms 5000

# Terminal 2, 3 y 4: tres bots de sparring
./bin/conservador -addr localhost:9000 -token conservador
./bin/base        -addr localhost:9000 -token base
./bin/aleatorio   -addr localhost:9000 -token aleatorio

# Terminal 5: tu bot
./mi-bot -addr localhost:9000 -token mi-bot
```

La mesa arranca cuando se conectan `-min-jugadores`, y al terminar imprime las
posiciones finales. Cualquier bot de `bin/` sirve de rival: prueba también `espia`,
`bandido`, `estadistico` y `darwin`. En una mesa local el token es solo un nombre.

**2. En un torneo local, contra todos a la vez.** La arena hace jugar a tu bot contra
cada uno de los demás y arma una tabla. Agrega el tuyo a la lista `participantes` de
`torneo-go.json`, con el comando que lo arranca (la arena le suma `-addr` y `-token`):

```json
{ "id": "mi-bot", "nombre": "Mi Bot", "comando": ["python", "mi_bot.py"] }
```

```bash
./bin/arena correr -config torneo-go.json
```

En `resultados/` quedan la tabla y el historial de cada mano, con las cartas de todos:
es lo mejor para entender por qué perdiste una mano. `torneo.json` suma los bots en
Python y JavaScript (necesita Python y Node). Formatos y puntuación:
[`docs/arena.md`](docs/arena.md).

**3. En el servidor de pruebas de la organización.** Con otros equipos y latencia de
red de verdad. Ver [`docs/servidor-practica.md`](docs/servidor-practica.md).

## Escribir tu bot

| Lenguaje | Por dónde empezar |
| -------- | ----------------- |
| **Python** | [`bots/novato`](bots/novato): un bot sencillo y comentado, con reglas simples y cada parte en su archivo. Versión mínima en [`bots/ejemplos/python/bot.py`](bots/ejemplos/python/bot.py). Sin dependencias |
| **Go** | [`pkg/botsdk`](pkg/botsdk/botsdk.go): implementas `Nombre()` y `Decidir()` y el SDK resuelve el resto. Ejemplo mínimo: [`bots/conservador`](bots/conservador/main.go). Con aprendizaje: [`bots/bandido`](bots/bandido), [`bots/espia`](bots/espia) y [`bots/darwin`](bots/darwin) |
| **JavaScript** | [`bots/ejemplos/javascript/bot.js`](bots/ejemplos/javascript/bot.js): con E/S asíncrona |
| **Otro** | [`docs/protocolo.md`](docs/protocolo.md): es todo lo que hace falta; se puede probar a mano con `nc` |

Los bots del kit sirven de sparring y de punto de partida. Cada uno muestra una
técnica distinta:

| Bot | Lenguaje | Técnica |
| --- | -------- | ------- |
| `aleatorio` | Go | Juega cualquier acción válida |
| `conservador` | Go | Casi nunca apuesta |
| `novato` | Python | Reglas simples, con condicionales |
| `base` | Go | Fórmula de Chen y la jugada ya armada |
| `estadistico` | Go | Equidad por simulación Monte Carlo |
| `bandido` | Go | Aprendizaje por refuerzo: bandido multibrazo (UCB1) |
| `espia` | Go | Modelado de rivales con estimación bayesiana |
| `darwin` | Go | Algoritmo genético |

Un bot que no le gana a `conservador` tiene un problema. Los más elaborados traen un
README con la idea y por dónde mejorarlos.

Puedes escribir tu bot dentro de esta carpeta o en tu propio repositorio. En Go, lo más
simple es crear `bots/mi-bot/` aquí adentro: importas `github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk` y ya
compila. Desde tu propio módulo, trae el SDK con `go get github.com/aicspucp/pokerfight-arena-participantes@main`.

## Lo que más sorprende (léelo antes de empezar)

La guía completa está en [`docs/guia-del-bot.md`](docs/guia-del-bot.md). Lo esencial:

- **Una sola conexión TCP.** Te conectas una vez y te quedas conectado.
- **El plazo es de 5 segundos por jugada.** Si no respondes a tiempo, la mesa juega
  por ti `check` o `fold` y te anota un **timeout**.
- **Una acción inválida cuesta lo mismo que no responder.**
- **`monto` es el total de tu apuesta, no el incremento.**
- **Si te caes, vuelve con el mismo token** y retomas tu silla y tus fichas. El SDK
  de Go **no** reconecta solo: hay que hacerlo (hay un ejemplo en la guía).
- **No te identificas por nombre**, sino por el `id_jugador` que te da la mesa.

## Documentación

| Documento | Para qué |
| --------- | -------- |
| [`docs/guia-del-bot.md`](docs/guia-del-bot.md) | Interfaces, SDK y las cosas importantes, en una página |
| [`docs/protocolo.md`](docs/protocolo.md) | La API: el contrato mesa ↔ bot, mensaje por mensaje (v1.0.0) |
| [`docs/reglas.md`](docs/reglas.md) | Las reglas de poker que aplica la mesa: ciegas, subidas mínimas, pozos laterales |
| [`docs/arena.md`](docs/arena.md) | La arena local: torneos entre bots, formatos, puntuación, reproducibilidad |
| [`docs/torneo.md`](docs/torneo.md) | Cómo competir y cómo se conecta un bot el día del torneo |
| [`docs/competencia.md`](docs/competencia.md) | Cómo es la competencia: formato, ciegas, fichas y fechas |
| [`docs/servidor-practica.md`](docs/servidor-practica.md) | El servidor de pruebas de la organización |

## Problemas y dudas

Si el protocolo y el código difieren, es un error del proyecto: avisa a la
organización. Reportar una falla suma; explotarla descalifica. Dudas y reportes:
**aicspucp@gmail.com**.
