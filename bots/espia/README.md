# Michi Espía · bot de ejemplo (modelado de rivales)

Bot de ejemplo para **PokerFight** (AICS PUCP), escrito en **Go**. Los demás bots
del kit juegan igual contra cualquiera. Este **estudia a cada rival** y cambia su
juego según a quién tenga enfrente.

| | |
| --- | --- |
| Dificultad | Intermedia |
| Lenguaje | Go (usa `pkg/botsdk` y `pkg/poker` de la arena) |
| Técnica | Modelado de rivales con estimación bayesiana + Monte Carlo |
| Entrenamiento previo | Ninguno: aprende durante la partida |
| Tiempo por jugada | Unos pocos milisegundos (400 simulaciones) |

## La idea

El protocolo le manda a cada bot todas las acciones de la mesa, también las de
las manos en que no juega. Con eso el bot lleva tres números por rival:

| Número | Qué mide | Para qué lo usa |
| --- | --- | --- |
| `entra` | En qué parte de las manos pone fichas por decisión propia | Robar las ciegas a quienes casi nunca entran |
| `agresión` | Qué parte de sus jugadas son apuestas o subidas | Creerle poco al que apuesta siempre y mucho al que casi nunca |
| `se retira` | Con qué frecuencia se retira cuando le apuestan | Farolear solo contra quien se retira; cobrarle caro al que paga todo |

En el torneo los rivales son alias aleatorios (`j-a1b2c3`), pero **estables**: lo
que aprende de un rival en una ronda le sirve cuando se lo vuelve a cruzar.

**Por qué bayesiano.** Con pocas manos, las frecuencias engañan: quien se retiró
en su única oportunidad tendría "100%". Cada número arranca de una suposición
razonable (un *prior*) y se corrige con lo que se ve. Está en `modelo.go`.

## Cómo correrlo

Se compila con todo lo demás (`go build -o bin/ ./cmd/... ./bots/...`):

```bash
./bin/espia -addr localhost:9000 -token espia
```

Cada 100 manos imprime el perfil de los rivales de la mesa (`-silencioso` lo apaga):

```
[espia] mano 200
    conservador    entra  9% | agresión 12% | se retira 71% | 200 manos
    aleatorio      entra 62% | agresión 48% | se retira 30% | 200 manos
```

## Dónde está cada cosa

| Archivo | Qué tiene |
| --- | --- |
| `main.go` | El bot y cómo usa los perfiles para decidir |
| `modelo.go` | Los perfiles y la estimación de cada frecuencia |
| `equidad.go` | La simulación Monte Carlo de la mano propia |

## Por dónde mejorarlo

1. **Supone que el rival tiene una mano al azar.** Si alguien solo apuesta con
   manos fuertes, simular contra ese rango y no contra cualquier mano.
2. **No mira el showdown.** `mano_fin` trae las cartas mostradas: sirven para
   saber con qué apuesta cada uno.
3. **No distingue rondas.** Un rival puede retirarse mucho en el flop y nunca en
   el river.
4. **No mira los tamaños.** Una apuesta de todo el pozo y una mínima se leen igual.
5. **No recuerda entre partidas.** Guardar los perfiles en un archivo los
   conservaría si el bot se reinicia.
