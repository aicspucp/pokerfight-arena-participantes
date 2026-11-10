"""Lectura de cartas con conteos y condicionales.

No hay evaluador de manos ni simulaciones: solo contar cuantas cartas hay de
cada rango y de cada palo, y preguntar con if. Es suficiente para saber si
tenemos un par, un color o un proyecto de escalera, que es todo lo que
necesita un bot facil.

Una carta llega del protocolo como {"rango": 2..14, "palo": 0..3}:
  rango 11=J, 12=Q, 13=K, 14=A
  palo  0=treboles (c), 1=diamantes (d), 2=corazones (h), 3=picas (s)
"""

LETRA_RANGO = {10: "T", 11: "J", 12: "Q", 13: "K", 14: "A"}
LETRA_PALO = "cdhs"

# Categorias de jugada, de menor a mayor. Es el mismo orden que usa el
# crupier de la arena para decidir el showdown.
CARTA_ALTA, PAR, DOBLE_PAR, TRIO, ESCALERA, COLOR, FULL, POKER, ESCALERA_COLOR = range(9)
NOMBRES = ["carta alta", "par", "doble par", "trio", "escalera", "color",
           "full house", "poker", "escalera de color"]

# Que tan buena es nuestra jugada, en cuatro niveles. La estrategia decide
# mirando solo esto (y los proyectos), no la categoria exacta.
NADA, MEDIA, BUENA, MONSTRUO = "nada", "media", "buena", "monstruo"


def texto(carta):
    """{"rango": 14, "palo": 3} -> "As". Solo para los logs."""
    return f"{LETRA_RANGO.get(carta['rango'], str(carta['rango']))}{LETRA_PALO[carta['palo']]}"


def texto_cartas(cartas):
    return " ".join(texto(c) for c in cartas) or "-"


# --- Preflop ------------------------------------------------------------------

def grupo_preflop(mano):
    """Clasifica las dos cartas privadas en cinco grupos, de mejor a peor.

    Es una tabla de manos iniciales escrita con if: la forma mas simple de
    no jugar basura. Con dos cartas casi todo depende de tres preguntas:
    son pareja? son altas? son del mismo palo?
    """
    alta = max(mano[0]["rango"], mano[1]["rango"])
    baja = min(mano[0]["rango"], mano[1]["rango"])
    mismo_palo = mano[0]["palo"] == mano[1]["palo"]
    pareja = alta == baja

    if pareja and alta >= 11:
        return "premium"            # JJ, QQ, KK, AA
    if alta == 14 and baja == 13:
        return "premium"            # AK
    if pareja and alta >= 8:
        return "fuerte"             # 88, 99, TT
    if alta == 14 and baja >= 11:
        return "fuerte"             # AQ, AJ
    if alta == 13 and baja == 12 and mismo_palo:
        return "fuerte"             # KQ del mismo palo
    if pareja:
        return "jugable"            # 22 a 77: buscan trio
    if alta == 14 and (mismo_palo or baja >= 10):
        return "jugable"            # AT, o un as con cualquiera del mismo palo
    if alta >= 11 and baja >= 10:
        return "jugable"            # KQ, KJ, QJ, KT, QT, JT
    if mismo_palo and alta - baja == 1 and baja >= 5:
        return "especulativa"       # conectoras del mismo palo: 65s, 76s...
    return "basura"


# --- Postflop -----------------------------------------------------------------

def hay_escalera(rangos):
    """True si en el conjunto de rangos hay cinco seguidos. El as vale 14 y
    tambien 1, por la escalera A-2-3-4-5."""
    if 14 in rangos:
        rangos = rangos | {1}
    seguidas = 0
    for rango in range(1, 15):
        if rango in rangos:
            seguidas += 1
            if seguidas >= 5:
                return True
        else:
            seguidas = 0
    return False


def categoria(cartas):
    """La mejor jugada que se puede armar con estas cartas (2 a 7)."""
    rangos = {}
    palos = {}
    for carta in cartas:
        rangos[carta["rango"]] = rangos.get(carta["rango"], 0) + 1
        palos[carta["palo"]] = palos.get(carta["palo"], 0) + 1

    palo_color = None
    for palo, cuantas in palos.items():
        if cuantas >= 5:
            palo_color = palo

    if palo_color is not None:
        rangos_del_color = {c["rango"] for c in cartas if c["palo"] == palo_color}
        if hay_escalera(rangos_del_color):
            return ESCALERA_COLOR

    repeticiones = sorted(rangos.values(), reverse=True) + [0]
    if repeticiones[0] == 4:
        return POKER
    if repeticiones[0] == 3 and repeticiones[1] >= 2:
        return FULL
    if palo_color is not None:
        return COLOR
    if hay_escalera(set(rangos)):
        return ESCALERA
    if repeticiones[0] == 3:
        return TRIO
    if repeticiones[0] == 2 and repeticiones[1] == 2:
        return DOBLE_PAR
    if repeticiones[0] == 2:
        return PAR
    return CARTA_ALTA


class Lectura:
    """Lo que el bot entiende de su mano despues del flop."""

    def __init__(self, categoria_jugada, nivel, outs, proyecto):
        self.categoria = categoria_jugada
        self.nombre = NOMBRES[categoria_jugada]
        self.nivel = nivel
        self.outs = outs          # Cartas que nos mejoran a una jugada buena.
        self.proyecto = proyecto  # "color", "escalera", "color+escalera" o "".


def leer_jugada(mano, comunitarias):
    mia = categoria(mano + comunitarias)
    de_la_mesa = categoria(comunitarias)

    mis_rangos = [c["rango"] for c in mano]
    rangos_mesa = [c["rango"] for c in comunitarias]
    mas_alta_mesa = max(rangos_mesa)
    pareja_en_mano = mis_rangos[0] == mis_rangos[1]

    # Lo que importa no es la jugada, sino si la arman NUESTRAS cartas. Si la
    # mesa tiene un trio, todos tienen ese trio: no vale nada.
    if mia >= TRIO and mia > de_la_mesa:
        nivel = MONSTRUO
    elif mia == DOBLE_PAR and de_la_mesa == CARTA_ALTA:
        nivel = BUENA        # Los dos pares usan nuestras cartas.
    elif pareja_en_mano and mis_rangos[0] > mas_alta_mesa:
        nivel = BUENA        # "Overpair": pareja mas alta que toda la mesa.
    elif mas_alta_mesa in mis_rangos:
        nivel = BUENA        # "Top pair": pareja con la carta mas alta de la mesa.
    elif pareja_en_mano or any(rango in rangos_mesa for rango in mis_rangos):
        nivel = MEDIA        # Un par, pero no el mejor posible.
    else:
        nivel = NADA

    outs = 0
    proyectos = []
    # Proyectos solo con cartas por venir (flop y turn) y si no estamos ya
    # hechos: con un trio no hace falta ir a buscar el color.
    if len(comunitarias) < 5 and nivel != MONSTRUO:
        if proyecto_de_color(mano, comunitarias):
            outs += 9   # 13 del palo - 4 que ya vemos.
            proyectos.append("color")
        tipo = proyecto_de_escalera(mano, comunitarias)
        if tipo == "abierta":
            outs += 8   # Dos rangos completan, 4 cartas de cada uno.
            proyectos.append("escalera")
        elif tipo == "interna":
            outs += 4   # Un solo rango completa.
            proyectos.append("escalera interna")

    return Lectura(mia, nivel, min(outs, 15), "+".join(proyectos))


def proyecto_de_color(mano, comunitarias):
    """Cuatro cartas del mismo palo, al menos una nuestra."""
    for palo in range(4):
        en_total = sum(1 for c in mano + comunitarias if c["palo"] == palo)
        mias = sum(1 for c in mano if c["palo"] == palo)
        if en_total == 4 and mias >= 1:
            return True
    return False


def proyecto_de_escalera(mano, comunitarias):
    """Devuelve "abierta" (completa por dos lados), "interna" (por uno solo)
    o "" si no hay proyecto. Tiene que usar al menos una carta nuestra."""
    rangos = {c["rango"] for c in mano + comunitarias}
    mios = {c["rango"] for c in mano}
    if 14 in rangos:
        rangos.add(1)
    if 14 in mios:
        mios.add(1)

    resultado = ""
    # Se miran todas las ventanas de 5 rangos seguidos (A-5, 2-6, ..., T-A).
    for bajo in range(1, 11):
        ventana = [bajo + i for i in range(5)]
        presentes = [r for r in ventana if r in rangos]
        if len(presentes) != 4 or not any(r in mios for r in presentes):
            continue
        falta = [r for r in ventana if r not in rangos][0]
        if falta in (ventana[0], ventana[4]):
            # Son cuatro seguidas: es abierta si se puede completar por abajo
            # Y por arriba (J-Q-K-A o A-2-3-4 solo se completan por un lado).
            if presentes[0] - 1 >= 1 and presentes[-1] + 1 <= 14:
                return "abierta"
            resultado = "interna"
        else:
            resultado = "interna"  # Falta una del medio: 5-6-_-8-9.
    return resultado
