"""Conexion con la mesa de pokerFight: TCP + JSON Lines.

Este archivo es "lo aburrido": abrir el socket, saludar, mandar y recibir
mensajes. No hay estrategia aca. Si quieres escribir tu propio bot en Python,
puedes copiar este archivo tal cual y concentrarte en estrategia.py.

Referencia del protocolo: docs/protocolo.md.
"""

import json
import socket
import sys
import time

VERSION_PROTOCOLO = "1.0.0"


class MesaRechazo(Exception):
    """La mesa respondio al saludo con un error (version incorrecta, token
    invalido...). No tiene sentido reintentar: hay que arreglar algo."""


class Conexion:
    """Una conexion con la mesa. Guarda la identidad que la mesa nos asigno."""

    def __init__(self, direccion, token):
        host, puerto = direccion.rsplit(":", 1)
        self.host = host
        self.puerto = int(puerto)
        self.token = token

        # La identidad real llega en la bienvenida. El token NO sirve como
        # identificador cuando hay un Casino detras: si te buscas en
        # estado["jugadores"] por el token, vas a decidir con el saldo de otro.
        self.id_jugador = token
        self.silla = -1

        self.socket = None
        self.entrada = None

    # --- Conexion y saludo ---------------------------------------------------

    def conectar(self, intentos=20, espera_s=0.1):
        """Abre el socket y hace el handshake, reintentando si la mesa todavia
        no acepta conexiones (la arena lanza los bots casi a la vez que abre
        el puerto, y en un torneo remoto la red puede fallar)."""
        ultimo_error = None
        for _ in range(intentos):
            try:
                self.socket = socket.create_connection((self.host, self.puerto), timeout=5)
                # El timeout solo aplica al connect: despues hay que poder
                # esperar sin limite, porque entre turno y turno la mesa puede
                # tardar lo que tarden los demas bots en decidir.
                self.socket.settimeout(None)
                # makefile da un iterador por lineas, que es exactamente el
                # framing del protocolo. Sin esto habria que juntar los bytes
                # a mano: un recv puede devolver media linea o dos y media.
                self.entrada = self.socket.makefile("r", encoding="utf-8", newline="\n")
                self._saludar()
                return
            except MesaRechazo:
                raise
            except OSError as error:
                ultimo_error = error
                self.cerrar()
                time.sleep(espera_s)
        raise ConnectionError(f"no se pudo conectar a {self.host}:{self.puerto}: {ultimo_error}")

    def _saludar(self):
        self.enviar({"tipo": "saludo", "version": VERSION_PROTOCOLO, "token": self.token})
        bienvenida = self.recibir()

        if bienvenida is None:
            raise OSError("la mesa cerro la conexion durante el saludo")
        if bienvenida.get("tipo") == "error":
            raise MesaRechazo(bienvenida.get("mensaje", "sin detalle"))
        if bienvenida.get("tipo") != "bienvenida":
            raise MesaRechazo(f"se esperaba bienvenida y llego {bienvenida.get('tipo')!r}")

        self.id_jugador = bienvenida.get("id_jugador") or self.token
        self.silla = bienvenida.get("silla", -1)

    # --- Mensajes --------------------------------------------------------------

    def enviar(self, mensaje):
        self.socket.sendall((json.dumps(mensaje) + "\n").encode("utf-8"))

    def recibir(self):
        """Devuelve el siguiente mensaje, o None si la mesa cerro la conexion
        (que es el fin normal de una partida, no un error)."""
        linea = self.entrada.readline()
        if not linea:
            return None
        return json.loads(linea)

    def responder(self, estado, accion):
        """Responde a un solicitar_accion. El id_mano se devuelve tal cual:
        la mesa lo usa para descartar respuestas que llegan tarde, de una mano
        que ya termino."""
        self.enviar({
            "tipo": "accion",
            "version": VERSION_PROTOCOLO,
            "id_mano": estado.get("id_mano", ""),
            "accion": accion,
        })

    def cerrar(self):
        try:
            if self.socket is not None:
                self.socket.close()
        except OSError:
            pass
        self.socket = None
        self.entrada = None


def log(texto):
    """Los logs van a stderr: stdout queda libre y la arena los muestra con
    -verboso. flush para que no se pierdan si el proceso termina de golpe."""
    print(texto, file=sys.stderr, flush=True)
