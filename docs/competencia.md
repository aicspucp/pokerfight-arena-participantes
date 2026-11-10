# La competencia

Cómo va a ser el Poker Fight de Bots y qué todavía está por definir. Lo que sigue
**es lo que la organización ya fijó**; lo que no está fijado está dicho como tal,
para que no cuentes con algo que puede cambiar.

> **Estado:** la fase de práctica está en marcha. Este documento se actualiza cuando
> se decida cada punto pendiente.

## Fechas

| Hito | Fecha |
| ---- | ----- |
| Sesión de dudas y preguntas | Domingo 15 de noviembre |
| Entrega final del bot | Sábado 21 de noviembre |
| Clasificatoria, publicación de finalistas, final en vivo y premiación | Domingo 22 de noviembre |

Las horas de cada sesión se avisan aparte.

## Fase clasificatoria

- Cada bot empieza con **10 000 fichas**.
- Los bots se reparten por sorteo en **mesas de 5** jugadores (si la división no es
  exacta, algunas mesas tienen 6).
- Las **ciegas son fijas: 50 / 100** durante todo el torneo. No suben.
- Cada mesa juega **hasta 50 manos** y se cierra con lo primero que pase: que se
  completen las 50 manos, que **pasen 6 minutos** o que **quede un solo bot con
  fichas**.
- El bot que **se queda sin fichas queda eliminado** y no juega más rondas.
- Después de cada ronda **se vuelven a sortear las mesas** y cada bot **conserva sus
  fichas** de la ronda anterior.
- Son **10 rondas** en total: cada bot juega como máximo en 10 mesas, siempre con
  rivales distintos que sigan en competencia.
- **No hay descanso fijo entre rondas:** la siguiente empieza apenas la organización
  la configura. Mientras tanto tu conexión queda abierta y en silencio.
- Al terminar, **pasan a la final los 6 bots con más fichas**. Si quedan menos de 6
  con fichas, pasan **los últimos en ser eliminados**.
- Cada equipo juega con **un solo bot**.

El juego es **Texas Hold'em No-Limit**: las reglas están en [`reglas.md`](reglas.md).

## Final en vivo

Los **6 mejores** de la clasificatoria juegan una mesa final. Cada uno entra con
**10 000 fichas** y las ciegas siguen fijas en **50 / 100**. Gana el **último en pie**,
o el bot con **más fichas luego de 1 hora**.

## Cómo juega tu bot

- **Te conectas una sola vez** y tu conexión dura todo el torneo. Entre ronda y ronda
  tu socket queda **abierto y en silencio**: no es una caída, no lo cierres.
- Si te eliminan, tampoco llega nada.
- **Tu identificador es un alias aleatorio** (`j-a1b2c3`), lo mismo que el de tus
  rivales: no hay nombres de equipo ni de bot. Es estable durante todo el
  torneo, así que puedes reconocer a un rival que se te cruza en varias rondas, pero no
  saber quién es.
- **Plazo de 5 segundos por jugada.** Si no respondes, la mesa juega `check` o `fold`
  por ti y te anota un timeout.
- Si te desconectas, **la mesa juega por ti** y tu silla sigue ocupada; al volver con
  tu token retomas **la misma silla con las mismas fichas**. Cuanto antes, mejor.
- Tu bot tiene que **reconectar por su cuenta** si se cae: ver
  [`guia-del-bot.md`](guia-del-bot.md#reconectar).

El detalle de la conexión en la competencia: [`torneo.md`](torneo.md) y
[`protocolo.md`](protocolo.md#en-el-campeonato-una-conexión-muchas-mesas).

## Todavía por definir

| Tema | Qué falta |
| ---- | --------- |
| **Hora de inicio** | A qué hora empiezan la clasificatoria y la final del domingo 22 |

Se avisa a todos los equipos en cuanto se decida.

## Cómo prepararte

1. Haz que tu bot **responda siempre con margen** de tiempo (el plazo es de 5 s, pero
   apunta a 2 o 3).
2. Pruébalo **contra los bots de ejemplo** en tu máquina y en el
   [servidor de pruebas](servidor-practica.md).
3. Prueba que **reconecta** si lo cortas a mano (cierra el socket o apaga la red un
   momento).
4. Prueba que aguanta **horas conectado en silencio**.
5. Revisa que **no se rompe** con mesas de 5 y de 6 y con tus fichas cambiando entre rondas.
