#!/usr/bin/env python3
"""Michi Novato: bot de ejemplo de dificultad FACIL para pokerFight.

Juega con una lista de reglas escritas con if (ver estrategia.py). No usa
ninguna libreria fuera de la estandar de Python, asi que corre en cualquier
maquina con Python 3.8 o superior.

Uso (lo mismo que hace la arena cuando lo lanza):

    python bots/novato/bot.py -addr localhost:9000 -token mi-token

Archivos:
    bot.py         este archivo: el bucle de mensajes con la mesa.
    conexion.py    socket, saludo y JSON Lines. Cópialo tal cual a tu bot.
    turno.py       ayudantes para calcular apuestas validas.
    cartas.py      lectura de la mano con conteos.
    estrategia.py  LA ESTRATEGIA. Es lo que tienes que cambiar.
"""

import argparse
import sys

from cartas import texto_cartas
from conexion import Conexion, MesaRechazo, log
from estrategia import decidir
from turno import Turno

NOMBRE = "Michi Novato"

# Cuantas veces se intenta volver si se corta la conexion a mitad de partida.
# Mientras el bot no esta, la mesa juega por el (check o fold) y le anota un
# timeout, asi que conviene volver rapido.
REINTENTOS_RECONEXION = 5


def describir_accion(accion):
    if "monto" in accion:
        return f"{accion['tipo']} {accion['monto']}"
    return accion["tipo"]


def jugar(conexion, silencioso):
    """Atiende mensajes hasta que la mesa cierre. Devuelve True si la partida
    termino normalmente, False si se corto la conexion."""
    mano = []
    while True:
        try:
            mensaje = conexion.recibir()
        except OSError:
            return False
        if mensaje is None:
            return True

        tipo = mensaje.get("tipo")

        if tipo == "mano_inicio":
            mano = mensaje.get("cartas") or []

        elif tipo == "solicitar_accion":
            turno = Turno(conexion, mensaje, mano)
            accion, motivo = decidir(turno)
            try:
                conexion.responder(turno.estado, accion)
            except OSError:
                return False
            if not silencioso:
                log(f"[{turno.etapa:7}] {texto_cartas(mano)} | mesa {texto_cartas(turno.comunitarias)}"
                    f" | {describir_accion(accion):12} | {motivo}")

        elif tipo == "mano_fin":
            # Informativo. Si hubo showdown, resultado["mostradas"] trae las
            # cartas del rival: es lo unico legitimo para aprender de el.
            resultado = mensaje.get("resultado") or {}
            for reparto in resultado.get("repartos", []):
                if reparto.get("id_jugador") == conexion.id_jugador and not silencioso:
                    log(f"           gane {reparto.get('monto', 0)} fichas")

        elif tipo == "error":
            log(f"la mesa aviso un error: {mensaje.get('mensaje', '')}")

        # "estado" es una actualizacion tras la accion de otro: no se responde.


def main():
    parser = argparse.ArgumentParser(description=f"{NOMBRE}: bot facil de pokerFight")
    parser.add_argument("-addr", default="localhost:9000", help="direccion de la mesa (host:puerto)")
    parser.add_argument("-token", default="michi-novato", help="token de sesion")
    parser.add_argument("-silencioso", action="store_true", help="no imprime cada decision")
    argumentos = parser.parse_args()

    conexion = Conexion(argumentos.addr, argumentos.token)
    try:
        conexion.conectar()
    except (MesaRechazo, ConnectionError) as error:
        log(f"{NOMBRE}: {error}")
        sys.exit(1)
    log(f"{NOMBRE} conectado como {conexion.id_jugador} en la silla {conexion.silla}")

    for intento in range(REINTENTOS_RECONEXION + 1):
        if jugar(conexion, argumentos.silencioso):
            log(f"{NOMBRE}: la mesa cerro la partida")
            return
        if intento == REINTENTOS_RECONEXION:
            break
        # Se volvio a conectar con el MISMO token: la mesa reconoce al bot y
        # le devuelve su silla y su saldo.
        log(f"{NOMBRE}: se corto la conexion, reintentando ({intento + 1}/{REINTENTOS_RECONEXION})")
        conexion.cerrar()
        try:
            conexion.conectar(intentos=10, espera_s=0.5)
        except (MesaRechazo, ConnectionError) as error:
            log(f"{NOMBRE}: no se pudo volver: {error}")
            break
    sys.exit(1)


if __name__ == "__main__":
    main()
