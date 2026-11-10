// Package almacen persiste datos en disco. Es deliberadamente genérico: no
// conoce el modelo de dominio de quien lo usa (hoy, Casino). La idea es que
// el día que se cambie a SQLite (ver README, "Estructura del repositorio"),
// el paquete casino no tenga que cambiar cómo lo usa, solo qué implementación
// de este paquete recibe.
package almacen

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// JSON persiste un único valor serializable en un archivo JSON. Es segura
// para uso concurrente.
type JSON struct {
	ruta string
	mu   sync.Mutex
}

// NuevoJSON construye un almacén sobre el archivo en ruta. No lo crea ni lo
// toca todavía: eso lo hace la primera llamada a Guardar. Cargar sobre un
// archivo que no existe no es un error (primer uso).
func NuevoJSON(ruta string) *JSON {
	return &JSON{ruta: ruta}
}

// Cargar deserializa el archivo en v. Si el archivo no existe o está vacío,
// deja v tal cual está y no devuelve error.
func (a *JSON) Cargar(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	datos, err := os.ReadFile(a.ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(datos) == 0 {
		return nil
	}
	return json.Unmarshal(datos, v)
}

// Guardar serializa v y lo escribe de forma atómica (archivo temporal +
// rename) para no dejar el archivo a medio escribir si el proceso se cae en
// el medio.
func (a *JSON) Guardar(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if dir := filepath.Dir(a.ruta); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}

	datos, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	// El archivo guarda tokens de sesión: quien lo lea puede hacerse pasar por
	// cualquier jugador. Solo el dueño lo lee y lo escribe (0600), y se fuerza a
	// disco (Sync) antes del rename para que un corte de luz no deje un archivo
	// renombrado pero vacío.
	tmp := a.ruta + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(datos); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, a.ruta)
}
