package protocolo

import "encoding/json"

// MarshalJSON escribe el mensaje como siempre, salvo un detalle de la
// bienvenida: "silla" va SIEMPRE, también cuando vale 0.
//
// El campo tiene omitempty porque en el resto de los mensajes no existe, y eso
// hacía que el bot de la silla 0 recibiera una bienvenida sin "silla". Los
// ejemplos leen el campo con -1 por defecto ("silla desconocida", ver
// docs/protocolo.md), así que ese bot creía no saber dónde estaba sentado.
func (m MensajeMesa) MarshalJSON() ([]byte, error) {
	type sinMetodos MensajeMesa // Evita que json.Marshal vuelva a llamar a este método.
	if m.Tipo != MsgBienvenida {
		return json.Marshal(sinMetodos(m))
	}
	return json.Marshal(struct {
		sinMetodos
		Silla int `json:"silla"` // Sin omitempty: tapa al campo embebido.
	}{sinMetodos(m), m.Silla})
}
