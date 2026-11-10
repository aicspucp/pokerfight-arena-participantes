# Gato Darwin · bot de ejemplo (dificultad intermedia)

Bot de ejemplo para **PokerFight** (AICS PUCP), escrito en **Go**. Su estrategia
no está escrita a mano: los números que la controlan los encontró un
**algoritmo genético** que hizo evolucionar a miles de estrategias candidatas
jugando millones de manos simuladas.

| | |
| --- | --- |
| Dificultad | Intermedia |
| Lenguaje | Go 1.26+ (usa `pkg/botsdk` y `pkg/poker` de la arena) |
| Técnica | Algoritmo genético + Monte Carlo |
| Mesas | Heads-up y mesas de 3 a 8 jugadores |
| Tiempo por jugada | ~10-20 ms (presupuesto máximo: ¼ del plazo de la mesa) |

---

## Cómo correrlo

Es parte del módulo de la arena: `make build` (o
`go build -o bin/ ./cmd/... ./bots/...`) lo compila junto con todo lo demás en
`bin/darwin`.

Ya está en el `torneo.json` de la raíz, así que `make torneo-local` lo hace
jugar contra los demás bots de ejemplo. Para sumarlo a otro torneo:

```json
{
  "id": "darwin",
  "nombre": "Gato Darwin",
  "comando": ["./bin/darwin", "-silencioso"]
}
```

```bash
./bin/arena correr -config torneo.json
```

Sin `-silencioso`, y con la arena en `-verboso`, imprime lo que piensa en cada
jugada: la fuerza de su mano, la fuerza corregida por rivales y posición, y por
qué decidió lo que decidió.

```
[flop   ] Ah Kd | mesa Kc 7d 2h      | fuerza  86% (efectiva  79%) vs 1 | bet 112    | apuesta por valor
```

Flags propios (opcionales):

| Flag | Por defecto | Para qué |
| ---- | ----------- | -------- |
| `-genoma archivo.json` | el incluido | Jugar con otro genoma sin recompilar. |
| `-semilla N` | 7 | Semilla de los faroles y del Monte Carlo (reproducibilidad). |
| `-muestras N` | 1500 | Muestras de Monte Carlo por decisión después del flop. |
| `-silencioso` | no | No imprime cada decisión. |

### Pruebas

```bash
go test ./bots/darwin/...          # incluye un entrenamiento corto que verifica que la evolución mejora
go test -short ./bots/darwin/...   # sin ese entrenamiento
```

---

## Cómo decide en cada turno

```
  cartas + mesa ──► Monte Carlo (pkg/poker) ──► fuerza: P(ganarle a UNA mano al azar)
                                                    │
     rivales, posición, apuestas de la calle ──────►│  corrección (genes)
                                                    ▼
                                             fuerza efectiva
                                                    │
               pot odds, riesgo para el stack ─────►│  umbrales (genes)
                                                    ▼
                         pasar · pagar · apostar/subir (tamaño: genes) · all-in · retirarse
```

1. **Fuerza.** Simula 1500 veces lo que falta repartir y cuenta cuántas veces su
   mano le gana a una mano al azar, con `pkg/poker`, **el mismo evaluador que usa
   el crupier**. Antes del flop usa una tabla precalculada de las 169 manos
   iniciales (`preflop.json`).
2. **Fuerza efectiva.** La corrige por la cantidad de rivales (contra cuatro, un
   par vale mucho menos que contra uno), por la posición (hablar último ayuda) y
   por la agresión de la calle (cada subida que enfrenta resta).
3. **Decisión.** Compara la fuerza efectiva contra los umbrales del genoma y
   contra las *pot odds*. Exige más cuanto mayor es la parte del stack en juego.
   A veces farolea, con una probabilidad que también decide el genoma.

Todo esto está en [`estrategia/politica.go`](estrategia/politica.go): unas 100
líneas que se leen en cinco minutos.

---

## El algoritmo genético

La **forma** de la estrategia está escrita a mano; los **números** no. Cada
estrategia candidata es un **genoma** de 18 genes:

| Gen | Rango | Qué controla |
| --- | ----- | ------------ |
| `umbral_apuesta_pre` / `_post` | 0,30-0,90 | Fuerza mínima para apostar (antes / después del flop) |
| `umbral_subida_pre` / `_post` | 0,40-0,97 | Fuerza mínima para subir sobre una apuesta |
| `margen_pago_pre` / `_post` | 0,5-1,8 | Cuánto por encima de las pot odds tiene que estar para pagar |
| `farol_pre` / `_post` / `farol_subida` | 0-0,40 | Probabilidad de farolear |
| `tam_apertura` | 2-5 ciegas | Tamaño de la apertura preflop |
| `tam_valor` / `tam_farol` / `tam_subida` | fracción del pozo | Tamaños de apuesta |
| `bono_posicion` | 0-0,15 | Cuánto vale hablar último |
| `presion` | 0-0,15 | Cuánto resta cada subida que enfrenta |
| `multijugador` | 0,2-1,2 | Cuánto pesa cada rival extra |
| `umbral_allin` | 0,75-0,99 | Fuerza para ir all-in |
| `aversion_riesgo` | 0-0,5 | Cuánto más exige para jugarse el stack |

El genoma campeón está en [`genoma.json`](genoma.json), con los genes por
nombre: lo puedes abrir y retocar a mano.

### La evolución, paso a paso

```
población inicial (60 genomas: 1 escrito a mano + 59 al azar)
   │
   ▼
┌─► evaluar: cada genoma juega ~11.000 manos simuladas  ──► aptitud (bb/100)
│      · heads-up contra 7 estilos de rival
│      · mini-torneos de 5 y 6 jugadores, 50 manos, con eliminación
│   seleccionar por torneo (k = 3)
│   cruzar (BLX-α, α = 0,3)
│   mutar (ruido gaussiano, se achica con las generaciones)
│   los 3 mejores pasan intactos (elitismo)
└── 80 generaciones
   │
   ▼
validación con manos que el genético nunca vio ──► genoma.json
```

Decisiones de diseño que vale la pena mirar en el código:

- **Repartos precalculados** ([`entrenamiento/biblioteca.go`](entrenamiento/biblioteca.go)).
  Evaluar manos es lo caro; apostar no. La fuerza de cada asiento en cada calle
  se calcula una vez y se guarda en `biblioteca.cache`. Después, los genomas
  juegan miles de manos por segundo sin evaluar una carta.
- **Las mismas cartas para todos** (números aleatorios comunes). Todos los
  genomas de una generación juegan exactamente los mismos repartos, y cada
  reparto se juega rotando las cartas por todas las sillas. Si un genoma gana
  más que otro es porque decidió mejor, no porque le tocaron ases.
- **Un simulador con las reglas de la arena** ([`entrenamiento/simulador.go`](entrenamiento/simulador.go)):
  ciegas, orden de turnos, subida mínima, all-in corto y pozos laterales,
  calcados de `internal/mesa`.
- **Rivales variados** ([`entrenamiento/rivales.go`](entrenamiento/rivales.go),
  [`entrenamiento/replicas.go`](entrenamiento/replicas.go)): réplicas exactas de
  Bot Base y de Michi Novato, más conservador, aleatorio, maníaco, "estación" y
  roca. Y **coevolución**: los campeones de generaciones anteriores también se
  sientan a la mesa.
- **Mini-torneos con eliminación.** Una primera versión medía la aptitud con
  stacks que se reponen en cada mano: el genético aprendió a jugarse el stack
  con manos medianas, sumaba fichas en promedio… y en una mesa de 6 con
  eliminación quedaba afuera temprano. Medir con el formato real de las bases lo
  corrigió solo: el umbral de all-in subió de 0,75 a 0,89.

### Resultados del último entrenamiento

Aptitud en **manos nunca vistas** (bb/100, más es mejor):

| Escenario | Genoma escrito a mano | Campeón evolucionado |
| --------- | --------------------: | -------------------: |
| 1v1 contra Bot Base | 201 | **326** |
| 1v1 contra Michi Novato | −59 | **22** |
| 1v1 contra Roca | 13 | **30** |
| Torneo de 5 (rivales tight) | 17 | **51** |
| Torneo de 6 | 64 | **270** |
| **Aptitud ponderada** | **87** | **208** |

Y en la arena real (round-robin de 400 manos por cruce y 12 mesas únicas de 6
jugadores con eliminación):

| | Gato Darwin |
| --- | --- |
| Round-robin contra los 5 bots de ejemplo | 1.º, +173.524 fichas, 0 timeouts |
| Mesas únicas de 6 | ganó 7 de 12 (Michi Novato 3, Bot Base 2) |

### Entrenar tu propia versión

Desde la raíz del repositorio:

```bash
make entrenar-darwin                                   # entrena y recompila (~1 minuto con la caché; ~3 la primera vez)
go run ./bots/darwin/cmd/entrenar-darwin -generaciones 200 -poblacion 100
go run ./bots/darwin/cmd/entrenar-darwin -rapido       # prueba de humo en segundos (no toca el genoma del bot)
make build                                             # recompila con el genoma nuevo
```

El entrenamiento escribe en `bots/darwin/` el `genoma.json`, el `preflop.json` y
`entrenamiento.csv` (la curva de la evolución, para graficarla), **reemplazando**
los que trae el bot. Si quieres volver al original: `git checkout bots/darwin`.
La caché de repartos (`biblioteca.cache`) no se versiona.

Con `-rapido` los archivos van a una carpeta temporal, porque el genoma que sale
de tan pocas generaciones es malo a propósito. Si de verdad quieres guardarlo,
pasa `-dir` con la carpeta que quieras.

### Usarlo como punto de partida en tu propio repositorio

1. Copia esta carpeta a tu módulo de Go.
2. Cambia los imports de `bots/darwin/...` por la ruta de tu módulo.
3. Trae el SDK: con la arena en una carpeta al lado, agrega a tu `go.mod` un
   `require` y un `replace` hacia esa carpeta, con el nombre de módulo que dice
   su propio `go.mod`.
4. Cambia la política, agrega rivales y vuelve a entrenar.

---

## Cómo ganarle (y por dónde mejorarlo)

1. **No lee a sus rivales.** Mide la fuerza contra una mano *al azar*, y la
   corrige igual contra todos. Un bot que lleve estadísticas de cada rival
   (cuánto sube, con qué mostró en el showdown) puede explotarlo.
2. **Su forma es fija.** El genético solo ajusta números. Puedes darle genes
   nuevos (por ejemplo, tamaños distintos por calle) o cambiar la forma de la
   política y volver a entrenar.
3. **Aprendió contra estos rivales.** Si los bots del torneo juegan distinto,
   agrégalos a `entrenamiento/rivales.go` y vuelve a evolucionar.
4. **No entiende la textura de la mesa**: un 86 % en una mesa con cuatro
   corazones no es lo mismo que un 86 % en una mesa seca.

---

## Estructura

| Ruta | Qué hay |
| ---- | ------- |
| `main.go` | El bot: conecta con el SDK y decide cada turno. |
| `estrategia/` | Genoma, política, fuerza por Monte Carlo y traducción a acciones del protocolo. La comparten el bot y el entrenamiento. |
| `entrenamiento/` | Biblioteca de repartos, simulador de mesa, rivales y algoritmo genético. |
| `cmd/entrenar-darwin/` | El comando que evoluciona y guarda el genoma. |
| `genoma.json`, `preflop.json` | Lo que produce el entrenamiento. Van incluidos en el binario. |

## Requisitos del torneo que ya cumple

- Acepta `-addr` y `-token` (los define el SDK).
- Usa el SDK: handshake, reconexión, eco del `id_mano` y montos válidos.
- Nunca se pasa del plazo: piensa como mucho ¼ del `timeout_ms` y corta el
  Monte Carlo si se le acaba el tiempo.
- Reproducible con `-semilla`.

---

Hecho por AICS PUCP para la competencia PokerFight · [@aicspucp](https://instagram.com/aicspucp)
