"""Turno: todo lo que el bot sabe en el momento de decidir, ya masticado.

Los ayudantes de esta clase son los mismos que trae el SDK de Go de la arena
(pkg/botsdk), para que la estrategia no tenga que hacer cuentas con el
protocolo y no pierda manos por un error de aritmetica.
"""

ETAPAS = ("preflop", "flop", "turn", "river")


class Turno:
    def __init__(self, conexion, mensaje, mano):
        self.estado = mensaje.get("estado") or {}
        self.validas = mensaje.get("acciones_validas") or []
        self.timeout_ms = mensaje.get("timeout_ms", 0)
        self.mano = mano
        self.mi_id = conexion.id_jugador

        # Nuestra propia entrada del estado. Se busca por id_jugador (el de la
        # bienvenida), nunca por el token.
        self.yo = {"saldo": 0, "apuesta_ronda": 0, "posicion_silla": conexion.silla}
        for jugador in self.estado.get("jugadores", []):
            if jugador.get("id") == self.mi_id:
                self.yo = jugador
                break

    # --- Lo que se ve en la mesa ----------------------------------------------

    @property
    def etapa(self):
        return self.estado.get("etapa", "preflop")

    @property
    def comunitarias(self):
        return self.estado.get("comunitarias") or []

    @property
    def pozo(self):
        """Todo lo que hay en el centro, incluidas las apuestas de esta ronda."""
        return self.estado.get("pozo", 0)

    @property
    def ciega_grande(self):
        return self.estado.get("ciega_grande", 20) or 20

    @property
    def apuesta_actual(self):
        return self.estado.get("apuesta_actual", 0)

    @property
    def rivales_en_mano(self):
        """Cuantos rivales siguen en la mano (no se retiraron). Con 5 bots en
        la mesa esto cambia todo: un par que gana contra uno pierde seguido
        contra cuatro."""
        cantidad = 0
        for jugador in self.estado.get("jugadores", []):
            if jugador.get("id") != self.mi_id and jugador.get("activo") and jugador.get("en_mano", True):
                cantidad += 1
        return cantidad

    @property
    def subidas_en_esta_calle(self):
        """Cuantas veces alguien aposto o subio en la calle actual."""
        cantidad = 0
        for registro in self.estado.get("historial_acciones", []):
            if registro.get("etapa") == self.etapa and registro.get("accion", {}).get("tipo") in ("bet", "raise", "allin"):
                cantidad += 1
        return cantidad

    @property
    def hablo_ultimo(self):
        """True si despues del flop somos los ultimos en hablar entre los que
        siguen en la mano. Hablar ultimo es una ventaja enorme: ves que hizo
        todo el mundo antes de decidir."""
        boton = self.estado.get("posicion_boton", 0)
        sillas = []
        for jugador in self.estado.get("jugadores", []):
            if jugador.get("activo") and jugador.get("en_mano", True):
                sillas.append(jugador.get("posicion_silla", 0))
        if not sillas:
            return False
        # Las sillas van de 0 a N-1 en sentido horario; alcanza con un modulo
        # mayor que la silla mas alta para ordenarlas a partir del boton.
        total = max(j.get("posicion_silla", 0) for j in self.estado.get("jugadores", [])) + 1
        # Distancia en sentido horario desde el boton: el que esta mas lejos
        # (el propio boton cuenta como el mas lejos) habla ultimo.
        mia = (self.yo.get("posicion_silla", 0) - boton - 1) % total
        return all(mia >= (silla - boton - 1) % total for silla in sillas)

    # --- Cuentas de apuestas ---------------------------------------------------

    def puede(self, tipo):
        return tipo in self.validas

    @property
    def por_igualar(self):
        """Fichas que faltan para igualar. 0 si se puede pasar gratis."""
        return max(0, self.apuesta_actual - self.yo.get("apuesta_ronda", 0))

    @property
    def techo(self):
        """Total maximo al que se puede llevar la apuesta: el all-in."""
        return self.yo.get("saldo", 0) + self.yo.get("apuesta_ronda", 0)

    @property
    def subida_minima(self):
        """Total minimo que la mesa acepta como bet o raise."""
        return self.apuesta_actual + self.estado.get("subida_minima", 0)

    @property
    def porcentaje_para_pagar(self):
        """Pot odds: que porcentaje del pozo final ponemos nosotros si pagamos.
        Si tu mano gana mas seguido que este porcentaje, pagar es rentable."""
        if self.por_igualar == 0:
            return 0
        return self.por_igualar * 100 / (self.pozo + self.por_igualar)

    # --- Acciones (siempre validas) --------------------------------------------

    def apostar(self, total):
        """Lleva la apuesta de la ronda a `total` fichas.

        OJO: en el protocolo `monto` es el TOTAL de la ronda, no el incremento.
        Esta funcion lo ajusta solo a lo que la mesa acepta: nunca por debajo
        de la subida minima ni por encima del all-in.
        """
        total = int(total)
        tipo = "bet" if self.apuesta_actual <= self.yo.get("apuesta_ronda", 0) else "raise"

        if total >= self.techo and self.puede("allin"):
            # Llegar al techo es exactamente un all-in. Declararlo asi hace que
            # la mesa aplique la regla de all-in corto en vez de rechazarlo.
            return {"tipo": "allin"}
        if not self.puede(tipo):
            return self.pagar()

        total = max(total, self.subida_minima)
        if total >= self.techo:
            return {"tipo": "allin"} if self.puede("allin") else self.pagar()
        return {"tipo": tipo, "monto": total}

    def apostar_pozo(self, fraccion):
        """Apuesta una fraccion del pozo (0.5 = medio pozo) por encima de lo
        que haya que igualar."""
        return self.apostar(self.apuesta_actual + (self.pozo + self.por_igualar) * fraccion)

    def pagar(self):
        """Iguala. Si no hay nada que igualar, pasa."""
        if self.puede("call"):
            return {"tipo": "call"}
        if self.puede("check"):
            return {"tipo": "check"}
        # Pagar no esta permitido y tampoco pasar: solo queda el all-in corto
        # (no alcanza para igualar entero) o retirarse.
        if self.puede("allin"):
            return {"tipo": "allin"}
        return {"tipo": "fold"}

    def todo(self):
        if self.puede("allin"):
            return {"tipo": "allin"}
        return self.pagar()

    def pasar_o_retirarse(self):
        """Nunca te retires si pasar es gratis: es regalar la mano."""
        if self.puede("check"):
            return {"tipo": "check"}
        return {"tipo": "fold"}
