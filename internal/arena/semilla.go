package arena

// Semilla secreta: cartas reproducibles para quien organiza, impredecibles para
// quien compite.
//
// El problema: con "semilla": N en el archivo del torneo, las cartas de todo el
// torneo se pueden calcular (el algoritmo es público y la semilla está en el
// repositorio). Un bot remoto, o uno local que lea el archivo, sabría qué cartas
// van a salir.
//
// La solución: con "semilla_secreta" el torneo sigue siendo reproducible, pero
// solo para quien conozca un secreto de 256 bits.
//
//   - "auto": la arena sortea el secreto al arrancar con crypto/rand. No se
//     escribe en ningún archivo mientras el torneo corre, así que ningún bot (ni
//     remoto, ni local leyendo el disco) puede verlo.
//   - Una cadena hexadecimal de 64 caracteres: sirve para repetir un torneo ya
//     jugado cuya semilla se reveló.
//
// Compromiso y revelación: al arrancar se publica el hash del secreto
// (semilla_compromiso). Al terminar se guarda el secreto en semilla_secreta.txt.
// Cualquiera puede comprobar que el hash coincide, y con eso, que quien organizó no
// eligió las cartas a conveniencia después de ver los resultados.
//
// Cada enfrentamiento recibe su propia clave, derivada del secreto y de quiénes
// juegan con HMAC-SHA256, y baraja con ChaCha8 (el generador criptográfico de la
// biblioteca estándar). La ida y la vuelta de un emparejamiento comparten
// participantes, así que comparten clave y reparten las mismas cartas, igual que
// con una semilla fija.
//
// Alcance, para no prometer de más: protege de lo que un participante puede ver
// desde afuera (el repositorio, la configuración, las cartas de sus propias manos).
// Un bot que corre como subproceso de la arena, con el mismo usuario del sistema,
// comparte máquina con ella y ningún secreto en memoria es seguro frente a él; para
// un torneo con bots que no son de confianza, lo correcto es que corran remotos o
// aislados.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/aicspucp/pokerfight-arena-participantes/internal/crupier"
)

// SemillaAuto pide que la arena sortee el secreto al arrancar.
const SemillaAuto = "auto"

const (
	dominioCompromiso = "pokerFight-semilla-compromiso-v1:"
	dominioReparto    = "pokerFight-reparto-v1"
)

// resolverSemillaSecreta deja listo el secreto de la configuración. Se llama desde
// Validar, así que toda configuración cargada ya lo tiene resuelto. Es idempotente:
// una vez resuelto, no se vuelve a sortear.
func (c *ConfigTorneo) resolverSemillaSecreta() error {
	if c.SemillaSecreta == "" {
		return nil
	}
	if c.secreta != nil {
		return nil
	}

	var s [32]byte
	if c.SemillaSecreta == SemillaAuto {
		if _, err := rand.Read(s[:]); err != nil {
			return fmt.Errorf("arena: no se pudo sortear la semilla secreta: %w", err)
		}
	} else {
		b, err := hex.DecodeString(strings.TrimSpace(c.SemillaSecreta))
		if err != nil || len(b) != len(s) {
			return fmt.Errorf(`arena: "semilla_secreta" debe ser "auto" o 64 caracteres hexadecimales (32 bytes)`)
		}
		copy(s[:], b)
		if c.SemillaCompromiso != "" && c.SemillaCompromiso != compromisoDe(s) {
			return fmt.Errorf(`arena: la "semilla_secreta" no coincide con "semilla_compromiso": no es la semilla que se publicó al empezar ese torneo`)
		}
	}
	c.secreta = &s
	c.SemillaCompromiso = compromisoDe(s)
	return nil
}

// EsSemillaSecreta dice si el torneo usa una semilla secreta (en vez de la fija).
func (c ConfigTorneo) EsSemillaSecreta() bool { return c.secreta != nil }

// RevelarSemilla devuelve el secreto en hexadecimal. Solo se llama cuando el
// torneo ya terminó: mientras corre, el secreto no sale de la memoria de la arena.
func (c ConfigTorneo) RevelarSemilla() (string, bool) {
	if c.secreta == nil {
		return "", false
	}
	return hex.EncodeToString(c.secreta[:]), true
}

// semillaDeGrupo es la semilla numérica de un enfrentamiento: la derivada de
// "semilla" con una semilla fija, y 0 con una secreta (la clave real es de 256 bits
// y no cabe en un número; ver CrupierPara). Un 0 no se escribe en los resultados.
func (c ConfigTorneo) semillaDeGrupo(ids []string) uint64 {
	if c.secreta != nil {
		return 0
	}
	return semillaDe(c.Semilla, ids)
}

// CrupierPara arma el crupier de un enfrentamiento: determinista con la semilla
// fija, o con la clave derivada del secreto si el torneo usa semilla secreta.
func (c ConfigTorneo) CrupierPara(e Enfrentamiento) crupier.Crupier {
	if c.secreta != nil {
		return crupier.NuevoSeguro(claveDeReparto(*c.secreta, e.Participantes))
	}
	return crupier.NuevoConSemilla(e.Semilla)
}

// claveDeReparto deriva la clave de un enfrentamiento del secreto y de quiénes
// juegan. Los IDs se ordenan: la ida y la vuelta (mismos jugadores, otras sillas)
// obtienen la misma clave, y por lo tanto las mismas cartas.
func claveDeReparto(secreta [32]byte, ids []string) [32]byte {
	ordenados := append([]string(nil), ids...)
	sort.Strings(ordenados)

	mac := hmac.New(sha256.New, secreta[:])
	mac.Write([]byte(dominioReparto))
	for _, id := range ordenados {
		mac.Write([]byte{0})
		mac.Write([]byte(id))
	}
	var clave [32]byte
	copy(clave[:], mac.Sum(nil))
	return clave
}

// compromisoDe es el hash que se publica al empezar: demuestra, al revelarse el
// secreto, que no cambió durante el torneo, sin permitir deducirlo antes.
func compromisoDe(secreta [32]byte) string {
	h := sha256.Sum256(append([]byte(dominioCompromiso), secreta[:]...))
	return hex.EncodeToString(h[:])
}

// AdvertenciaSemilla devuelve un aviso si el torneo usa una semilla fija y tiene
// participantes remotos: ellos pueden leer la semilla del repositorio y calcular
// las cartas. Vacío si no hay nada que advertir.
func (c ConfigTorneo) AdvertenciaSemilla() string {
	if c.secreta != nil {
		return ""
	}
	remotos := 0
	for _, p := range c.Participantes {
		if p.EsRemoto() {
			remotos++
		}
	}
	if remotos == 0 {
		return ""
	}
	return fmt.Sprintf(`la semilla es fija ("semilla": %d) y hay %d participante(s) remoto(s): con ella se pueden calcular todas las cartas del torneo. Para una competencia usa "semilla_secreta": "auto"`, c.Semilla, remotos)
}
