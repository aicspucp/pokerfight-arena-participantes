# Michi Novato · bot de ejemplo (dificultad fácil)

Bot de ejemplo para **PokerFight** (AICS PUCP), escrito en **Python** y sin
ninguna librería fuera de la estándar. Toda su inteligencia son **condicionales**:
una tabla de manos iniciales, una lectura de la jugada con conteos y un puñado
de reglas de poker escritas con `if`.

Es el primer rival contra el que conviene probar tu bot. Si no le ganas de forma
consistente, tu bot todavía no está listo para el torneo.

| | |
| --- | --- |
| Dificultad | Fácil |
| Lenguaje | Python 3.8+ (solo librería estándar) |
| Técnica | Reglas con condicionales, sin azar |
| Mesas | Heads-up y mesas de 3 a 8 jugadores |
| Tiempo por jugada | Instantáneo |

---

## Cómo correrlo

### Contra la arena, en un torneo local

Ya está en el `torneo.json` de la raíz, así que `make torneo-local` lo hace
jugar contra los demás bots de ejemplo. Para sumarlo a otro torneo, agrégalo a
la lista de `participantes` (las rutas son relativas a la raíz del repositorio):

```json
{
  "id": "novato",
  "nombre": "Michi Novato",
  "comando": ["python", "bots/novato/bot.py", "-silencioso"]
}
```

y corre la arena como siempre:

```bash
./bin/arena correr -config torneo.json
```

La arena le agrega `-addr <host:puerto> -token <token>` al final del comando: el
bot ya los entiende. Quita `-silencioso` y corre la arena con `-verboso` para ver
qué piensa en cada jugada:

```
[flop   ] Ah Kd | mesa Kc 7d 2h | bet 60       | par (buena): apuesto medio pozo
[turn   ] Ah Kd | mesa Kc 7d 2h 9s | call         | par (buena): pago
```

### Contra una mesa suelta, a mano

```bash
./bin/mesa -addr :9000 -jugadores 2 -min-jugadores 2
python bots/novato/bot.py -addr localhost:9000 -token michi
./bin/aleatorio -addr localhost:9000 -token rival   # en otra terminal
```

### Pruebas

```bash
cd bots/novato
python -m unittest -v
```

### Usarlo como punto de partida en tu propio repositorio

No depende de nada de la arena: copia esta carpeta a tu repositorio, cambia
`estrategia.py` y en el `torneo.json` apunta el comando a donde lo tengas:

```json
{ "id": "mi-bot", "comando": ["python", "../mi-repo/bot.py"] }
```

---

## Cómo está armado

| Archivo | Qué hace | ¿Lo tienes que tocar? |
| ------- | -------- | -------------------- |
| `bot.py` | Bucle principal: recibe mensajes y responde los turnos. Reconecta si se corta la red. | No |
| `conexion.py` | Socket TCP, saludo y JSON Lines (el protocolo de la arena). | No: cópialo tal cual a tu bot |
| `turno.py` | Ayudantes: `apostar(total)`, `pagar()`, `por_igualar`, `rivales_en_mano`, `hablo_ultimo`... Nunca devuelven una acción inválida. | Rara vez |
| `cartas.py` | Lectura de la mano con conteos: categoría de la jugada, si usa tus cartas y cuántos *outs* tienes. | Si quieres leer mejor la mano |
| `estrategia.py` | **La estrategia.** `decidir(turno)` devuelve la acción y el motivo. | **Sí: aquí empieza tu bot** |
| `test_bot.py` | Pruebas sin mesa ni red. | Agrega las tuyas |

---

## La estrategia, regla por regla

### Antes del flop

Las dos cartas se clasifican en cinco grupos con tres preguntas: ¿son pareja?
¿son altas? ¿son del mismo palo?

| Grupo | Manos | Qué hace |
| ----- | ----- | -------- |
| Premium | JJ+, AK | Abre a 3 ciegas. Si ya subieron, resube al triple (o all-in si la subida es grande). |
| Fuerte | 88-TT, AQ, AJ, KQ del mismo palo | Abre a 3 ciegas. Paga subidas de hasta 4 ciegas. |
| Jugable | 22-77, AT, Ax del mismo palo, figuras | Si nadie entró y habla último, intenta robar las ciegas. Si no, entra solo barato. |
| Especulativa | Conectoras del mismo palo (65s+) | Entra barato y solo con varios rivales: ganan pozos grandes cuando ligan. |
| Basura | Todo lo demás | Pasa si es gratis; si no, se retira. |

### Después del flop

`cartas.py` resume la mano en un **nivel**:

- **Monstruo**: trío o mejor armado *con sus cartas* (un trío que está entero en
  la mesa lo tienen todos, no cuenta). → Apuesta 3/4 del pozo o resube.
- **Buena**: doble par propio, pareja mayor que la mesa (*overpair*) o pareja con
  la carta más alta de la mesa (*top pair*). → Apuesta medio pozo (un tercio con
  3+ rivales) y paga apuestas razonables.
- **Media**: cualquier otro par. → Pasa y paga solo apuestas chicas.
- **Proyecto**: le falta una carta para color o escalera. → Paga si las **pot
  odds** lo justifican (ver abajo). En posición y contra uno, semi-farolea.
- **Nada**: pasa o se retira. Su único farol: en el river, solo contra uno,
  hablando último y si nadie apostó.

### La regla del 2 y el 4

Para decidir si pagar un proyecto, el bot cuenta sus **outs** (cartas que lo
mejoran: 9 para un color, 8 para una escalera abierta, 4 para una interna) y
estima su probabilidad de ligar:

- con **dos** cartas por venir (flop): outs × 4 %
- con **una** carta por venir (turn): outs × 2 %

Si esa probabilidad supera lo que pone en juego al pagar
(`por_igualar / (pozo + por_igualar)`), paga. Si no, se retira.

---

## Cómo ganarle (y por dónde mejorarlo)

Michi Novato es **predecible a propósito**. Lo que no hace:

1. **No varía**: frente a la misma situación hace siempre lo mismo. Un bot que
   lo observe aprende rápido que cuando apuesta tiene algo.
2. **No lee al rival**: `estado["historial_acciones"]` y `mano_fin.mostradas`
   traen todo lo que hicieron los demás, y los ignora.
3. **Farolea casi nunca**: contra él, retirarse ante sus apuestas es casi
   siempre correcto, y robarle las ciegas también.
4. **Pondera mal el kicker**: A-K y K-2 en una mesa con rey son "buena" por igual.

Tu bot debería explotar al menos dos de estas cuatro cosas. Y si quieres un rival
más duro, prueba contra **Gato Darwin**, el bot intermedio en Go, cuya estrategia
fue entrenada con un algoritmo genético.

---

## Requisitos del torneo que ya cumple

- Acepta `-addr` y `-token` (lo único que pasa la arena).
- Se identifica con el `id_jugador` de la bienvenida, nunca con el token.
- Manda `monto` como el **total** de la ronda y respeta la subida mínima.
- Nunca se retira cuando pasar es gratis.
- Responde al instante: muy lejos del límite de 5 segundos por jugada.
- Si se corta la conexión, vuelve con el mismo token y recupera su silla.

---

Hecho por AICS PUCP para la competencia PokerFight · [@aicspucp](https://instagram.com/aicspucp)
