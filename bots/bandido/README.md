# Gato Bandido · bot de ejemplo (aprendizaje por refuerzo)

Bot de ejemplo para **PokerFight** (AICS PUCP), escrito en **Go**. No trae una
estrategia fija: tiene cuatro estilos de juego y **aprende mientras juega** cuál
le rinde más contra los rivales que le tocaron.

| | |
| --- | --- |
| Dificultad | Intermedia |
| Lenguaje | Go (usa `pkg/botsdk` y `pkg/poker` de la arena) |
| Técnica | Bandido multibrazo (UCB1 con olvido), aprendizaje por refuerzo en línea |
| Entrenamiento previo | Ninguno |
| Tiempo por jugada | Menos de 1 ms |

## La idea

Un **bandido multibrazo** es el problema de una persona frente a varias máquinas
tragamonedas que pagan distinto, sin saber cuál paga más. Cada tirada sirve para
ganar o para averiguar, y hay que repartirlas bien.

Aquí las máquinas son estilos de juego:

| Estilo | Cómo juega |
| --- | --- |
| `roca` | Pocas manos, casi nunca farolea, se retira ante la presión |
| `tiburon` | Manos buenas y apuestas firmes, algún farol |
| `maniaco` | Muchas manos y muchos faroles |
| `pagador` | Paga casi todo y casi nunca sube |

Antes de cada mano el bot elige un estilo con **UCB1**: se queda con el de mayor
"recompensa media + bono por haberlo probado poco". Al terminar la mano anota
cuántas ciegas ganó o perdió. Además **olvida** de a poco lo viejo, porque en el
torneo las mesas se sortean de nuevo en cada ronda y el mejor estilo cambia.

## Cómo correrlo

Se compila con todo lo demás (`go build -o bin/ ./cmd/... ./bots/...`):

```bash
./bin/bandido -addr localhost:9000 -token bandido
```

Cada 100 manos imprime lo que aprendió (`-silencioso` lo apaga):

```
[bandido] mano 300 | roca 18% (-0.2 ciegas/mano) | tiburon 55% (+1.4 ciegas/mano) | ...
```

## Dónde está cada cosa

| Archivo | Qué tiene |
| --- | --- |
| `main.go` | El bot, los estilos y el cálculo de la recompensa |
| `bandido.go` | El algoritmo UCB1 con olvido |
| `fuerza.go` | Una medida simple de la mano, sin simular |

## Por dónde mejorarlo

1. **Aprende lento.** El resultado de una mano varía mucho más que la diferencia
   entre estilos: necesita cientos de manos. Probar otra regla de elección
   (muestreo de Thompson) o recompensas con menos ruido.
2. **No mira el contexto.** Elige el estilo sin saber cuántos rivales hay ni
   cuántas fichas tiene. Un *bandido contextual* aprendería un estilo por situación.
3. **Los estilos son fijos.** Se pueden agregar brazos, o dejar que el bot ajuste
   los umbrales de cada uno.
4. **La fuerza es tosca.** Cambiarla por una simulación como la de `bots/espia`.
