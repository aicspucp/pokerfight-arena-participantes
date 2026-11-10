# Servidor de pruebas

Un servidor de la organización, referencial al de la clasificatoria, donde puedes
conectar tu bot desde tu máquina para probarlo contra otros equipos y con latencia de
red de verdad. **No es la competencia**: no hay clasificación y lo que pase aquí no
cuenta para nada.

## Datos de conexión

Necesitas dos cosas, y las dos te las da la organización:

- **La dirección** (`host:puerto`): está en la guía de inicio que recibiste. Mírala
  siempre ahí, porque **el servidor puede cambiar** y este archivo no se actualiza.
- **Tu token**: te llega en un mensaje aparte. Es la credencial de tu equipo y es
  secreta: quien la tenga puede jugar en tu lugar. Es la misma el día del torneo.

## Cómo conectarte

Es el mismo bot y el mismo protocolo que en tu máquina; cambian la dirección y el
token.

```bash
./mi-bot -addr <host>:<puerto> -token <tu-token>
```

Para comprobar que llegas al servidor antes de tener tu bot, conecta uno de ejemplo:

```bash
./bin/conservador -addr <host>:<puerto> -token <tu-token>
```

Si el servidor lo acepta, el bot se queda corriendo y la conexión está bien. Si tu
token no es válido, termina con un error `credencial desconocida`.

## Cómo se juega aquí

Mientras haya algún equipo conectado, el servidor juega **rondas sin parar**:

| | |
| - | - |
| **Mesas** | Se sortean en cada ronda con quienes estén conectados, de 5 jugadores (algunas de 6). Si hay pocos, una sola mesa más pequeña |
| **Rondas** | 50 manos, con un tope de 6 minutos |
| **Fichas** | 10 000 al empezar cada ronda: aquí nunca quedas eliminado |
| **Ciegas** | 50 / 100, fijas, igual que en la competencia |
| **Plazo** | 5 segundos por jugada |
| **Rivales** | Los equipos conectados y bots de sparring de la organización, para que siempre tengas contra quién jugar |
| **Identidad** | Un alias aleatorio (`j-a1b2c3`), igual que en la competencia |

Si te conectas a mitad de una ronda, entras en la siguiente. Hasta que empiece, tu
socket queda **abierto y en silencio**: es normal, no lo cierres.

## Lo que tienes que saber

- **Un bot por equipo y por vez.** Si conectas dos veces con el mismo token, la conexión
  nueva **reemplaza** a la anterior. Si tu bot se desconecta solo, fíjate que no haya
  otra copia corriendo.
- **El servidor puede apagarse o reiniciarse.** Cuando pasa, tu conexión se corta: tu
  bot tiene que **reintentar** hasta que vuelva (ver
  [`guia-del-bot.md`](guia-del-bot.md#reconectar)).
- **No lances ráfagas de conexiones**: el servidor limita los intentos simultáneos por
  IP y te va a rechazar.
- Si algo se rompe o te parece un error del servidor, avisa a la organización con la
  hora y tu alias: ayuda mucho.

## Qué hace distinto a la competencia

| | Pruebas | Competencia |
| - | ------- | ----------- |
| Quién entra | Inscritos, con su token | Inscritos, con su token |
| Identidad | Un alias aleatorio | Un alias aleatorio, estable en todo el torneo |
| Fichas | 10 000 en cada ronda | 10 000 al empezar; las ganadas o perdidas se conservan entre rondas |
| Rondas | Sin fin | 10 rondas con mesas sorteadas de nuevo |
| Quedarte sin fichas | Vuelves en la ronda siguiente | Quedas eliminado |

Cómo será la competencia: [`competencia.md`](competencia.md).
