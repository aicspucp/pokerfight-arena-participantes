package main

// Lo que la línea de comandos le dice al organizador sobre la semilla del torneo.
// La lógica (sorteo, derivación, compromiso) está en internal/arena/semilla.go.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aicspucp/pokerfight-arena-participantes/internal/arena"
)

// archivoSemillaSecreta es donde queda el secreto al terminar el torneo.
const archivoSemillaSecreta = "semilla_secreta.txt"

// anunciarSemilla avisa, al arrancar, cómo se reparten las cartas: con semilla
// secreta publica el compromiso; con una fija y participantes remotos advierte que
// las cartas se pueden calcular.
func anunciarSemilla(cfg arena.ConfigTorneo) {
	if cfg.EsSemillaSecreta() {
		log.Printf("semilla secreta activa. Compromiso (sha256): %s", cfg.SemillaCompromiso)
		log.Printf("  El secreto no se escribe en disco mientras el torneo corre; al terminar queda en %s",
			filepath.Join(cfg.Salida, archivoSemillaSecreta))
		return
	}
	if adv := cfg.AdvertenciaSemilla(); adv != "" {
		log.Printf("AVISO DE SEGURIDAD: %s", adv)
	}
}

// guardarSemillaSecreta escribe el secreto junto a los resultados. Se llama recién
// cuando el torneo terminó: revelarlo antes dejaría a un bot ver las cartas que
// todavía no salieron. Con semilla fija no hace nada.
func guardarSemillaSecreta(cfg arena.ConfigTorneo) {
	secreto, ok := cfg.RevelarSemilla()
	if !ok {
		return
	}
	texto := fmt.Sprintf(`Semilla secreta del torneo %q

Secreto:     %s
Compromiso:  %s   (se publicó al empezar el torneo)

Para verificar que no cambió: el sha256 de "pokerFight-semilla-compromiso-v1:"
seguido de los 32 bytes del secreto tiene que dar el compromiso.

Para repetir el torneo con las mismas cartas, en su configuración:
  "semilla_secreta": "%s"

Publícalo recién cuando el torneo haya terminado.
`, cfg.Nombre, secreto, cfg.SemillaCompromiso, secreto)

	ruta := filepath.Join(cfg.Salida, archivoSemillaSecreta)
	// 0600: es el secreto que predice todas las cartas.
	if err := os.WriteFile(ruta, []byte(texto), 0o600); err != nil {
		log.Printf("aviso: no se pudo guardar %s: %v", ruta, err)
		return
	}
	log.Printf("semilla secreta revelada en %s", ruta)
}

// describirSemilla es la línea de "semilla" de `arena validar`.
func describirSemilla(cfg arena.ConfigTorneo) string {
	if cfg.EsSemillaSecreta() {
		return "secreta (compromiso " + cfg.SemillaCompromiso[:16] + "...)"
	}
	return fmt.Sprintf("%d", cfg.Semilla)
}
