"""Estrategia de Michi Novato: solo condicionales.

Toda la inteligencia del bot esta en este archivo y son if encadenados. No
hay azar, simulaciones ni aprendizaje: frente a la misma situacion, el bot
hace siempre lo mismo. Eso lo hace facil de leer y de explotar, que es justo
lo que se espera de un rival de practica.

decidir() devuelve la accion y un motivo en texto, que se imprime en los logs
para que puedas seguir lo que piensa el bot mano a mano.

Ideas de poker que usa (todas explicadas donde aparecen):
  - Tabla de manos iniciales: con que dos cartas vale la pena entrar.
  - Posicion: hablar ultimo es una ventaja.
  - Cantidad de rivales: una mano buena contra uno es mediocre contra cuatro.
  - Pot odds y la "regla del 2 y el 4" para pagar proyectos.
"""

from cartas import BUENA, MEDIA, MONSTRUO, grupo_preflop, leer_jugada


def decidir(turno):
    if len(turno.comunitarias) < 3:
        return preflop(turno)
    return postflop(turno)


# --- Antes del flop -------------------------------------------------------------

def preflop(t):
    grupo = grupo_preflop(t.mano)
    cg = t.ciega_grande
    # Si la apuesta mas alta es la ciega grande, nadie subio todavia.
    nadie_subio = t.apuesta_actual <= cg

    if grupo == "premium":
        if nadie_subio:
            return t.apostar(3 * cg), "premium: abro a 3 ciegas"
        if t.apuesta_actual * 3 >= t.techo:
            return t.todo(), "premium contra subida grande: all-in"
        return t.apostar(3 * t.apuesta_actual), "premium: resubo al triple"

    if grupo == "fuerte":
        if nadie_subio:
            return t.apostar(3 * cg), "fuerte: abro a 3 ciegas"
        if t.por_igualar <= 4 * cg:
            return t.pagar(), "fuerte: pago una subida normal"
        return t.pasar_o_retirarse(), "fuerte, pero la subida es enorme"

    if grupo == "jugable":
        if nadie_subio and t.hablo_ultimo:
            # En el boton, si nadie entro, subir roba las ciegas seguido.
            return t.apostar(3 * cg), "jugable en buena posicion: intento robar las ciegas"
        if t.por_igualar <= cg:
            return t.pagar(), "jugable: entro barato"
        return t.pasar_o_retirarse(), "jugable, pero no vale tanto"

    if grupo == "especulativa":
        # Las conectoras del mismo palo ganan pozos grandes cuando ligan, y
        # para eso tiene que haber gente en el pozo. Solo entramos baratas.
        if t.por_igualar <= cg and t.rivales_en_mano >= 2:
            return t.pagar(), "especulativa: entro barato con varios rivales"
        return t.pasar_o_retirarse(), "especulativa: no paga la entrada"

    return t.pasar_o_retirarse(), "basura: me voy (o paso si es gratis)"


# --- Despues del flop -------------------------------------------------------------

def postflop(t):
    lectura = leer_jugada(t.mano, t.comunitarias)
    nombre = lectura.nombre
    muchos_rivales = t.rivales_en_mano >= 3

    if lectura.nivel == MONSTRUO:
        if t.por_igualar == 0:
            return t.apostar_pozo(0.75), f"{nombre}: apuesto 3/4 del pozo"
        if t.puede("raise"):
            return t.apostar(t.apuesta_actual * 3), f"{nombre}: resubo"
        return t.pagar(), f"{nombre}: pago"

    if lectura.nivel == BUENA:
        if t.por_igualar == 0:
            if muchos_rivales:
                # Contra muchos, apostar poco cobra a las manos peores sin
                # inflar un pozo que puede ganar otro.
                return t.apostar_pozo(0.33), f"{nombre} (buena) con muchos rivales: apuesto poco"
            return t.apostar_pozo(0.5), f"{nombre} (buena): apuesto medio pozo"
        limite = 0.5 if muchos_rivales else 0.75
        if t.por_igualar <= t.pozo * limite:
            return t.pagar(), f"{nombre} (buena): pago"
        return t.pasar_o_retirarse(), f"{nombre} (buena), pero la apuesta es muy grande"

    if lectura.outs > 0:
        # Regla del 2 y el 4: cada out vale ~4% con dos cartas por venir
        # (flop) y ~2% con una (turn). Si ese porcentaje supera lo que pone
        # en juego pagar (pot odds), pagar es rentable a la larga.
        cartas_por_venir = 5 - len(t.comunitarias)
        probabilidad = lectura.outs * (4 if cartas_por_venir == 2 else 2)
        if t.por_igualar == 0:
            if t.hablo_ultimo and t.rivales_en_mano == 1:
                return t.apostar_pozo(0.5), f"proyecto de {lectura.proyecto} en posicion: semi-farol"
            return t.pagar(), f"proyecto de {lectura.proyecto}: paso gratis"
        if probabilidad >= t.porcentaje_para_pagar:
            return t.pagar(), f"proyecto de {lectura.proyecto} ({probabilidad}% vs {t.porcentaje_para_pagar:.0f}%): pago"
        return t.pasar_o_retirarse(), f"proyecto de {lectura.proyecto}: no dan las cuentas"

    if lectura.nivel == MEDIA:
        if t.por_igualar == 0:
            return t.pagar(), f"{nombre} (media): paso"
        if t.por_igualar <= t.pozo / 3 and not muchos_rivales:
            return t.pagar(), f"{nombre} (media): pago una apuesta chica"
        return t.pasar_o_retirarse(), f"{nombre} (media): no aguanta la apuesta"

    # No tenemos nada. El unico farol del bot: en el river, si nadie aposto,
    # quedamos solos contra uno y hablamos ultimos, la mano se gana apostando.
    if (t.por_igualar == 0 and t.etapa == "river" and t.rivales_en_mano == 1
            and t.hablo_ultimo and t.subidas_en_esta_calle == 0):
        return t.apostar_pozo(0.5), "nada, pero el rival paso en el river: farol"
    return t.pasar_o_retirarse(), "nada: paso o me retiro"
