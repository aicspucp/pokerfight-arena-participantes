package protocolo

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
)

// El transporte es JSON delimitado por saltos de linea (JSON Lines) sobre TCP.
// Motivos: es depurable con netcat, es agnostico del lenguaje del bot y no
// requiere dependencias externas.

// Tamaños máximos de una línea. Sin tope, quien conecta y manda bytes sin salto
// de línea hace crecer la memoria del servidor sin límite (un solo socket llena
// cientos de MB en el plazo del handshake, y con muchas conexiones tumba el
// proceso). Un mensaje legítimo del bot es una acción: pesa decenas de bytes. El
// de la mesa lleva el estado y el historial de la mano, bastante más grande.
const (
	MaxLineaBot  = 64 << 10 // 64 KiB: saludo, acción, abandono.
	MaxLineaMesa = 1 << 20  // 1 MiB: estado público con historial, resultado de mano.
)

// ErrLineaDemasiadoLarga indica que el otro lado mandó una línea más larga que el
// máximo permitido. La conexión queda desincronizada: quien la reciba tiene que
// cerrarla.
var ErrLineaDemasiadoLarga = errors.New("protocolo: línea demasiado larga")

// Codec serializa y deserializa mensajes sobre una conexion.
type Codec struct {
	lector    *bufio.Reader
	escritor  *bufio.Writer
	codificar *json.Encoder
}

// NuevoCodec construye un Codec sobre un lector y un escritor cualesquiera
// (una net.Conn cumple ambos, pero asi es testeable con buffers).
func NuevoCodec(r io.Reader, w io.Writer) *Codec {
	escritor := bufio.NewWriter(w)
	return &Codec{
		lector:    bufio.NewReader(r),
		escritor:  escritor,
		codificar: json.NewEncoder(escritor),
	}
}

// Enviar serializa v como una linea JSON y hace flush.
func (c *Codec) Enviar(v any) error {
	if err := c.codificar.Encode(v); err != nil {
		return err
	}
	return c.escritor.Flush()
}

// leerLinea lee hasta el próximo salto de línea sin acumular más de max bytes.
// Devuelve lo leído y el error de la lectura (EOF, plazo vencido...), como
// bufio.Reader.ReadBytes, pero cortando en max.
func (c *Codec) leerLinea(max int) ([]byte, error) {
	var linea []byte
	for {
		fragmento, err := c.lector.ReadSlice('\n')
		linea = append(linea, fragmento...)
		if len(linea) > max {
			return nil, ErrLineaDemasiadoLarga
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue // La línea sigue: se junta el próximo trozo.
		}
		return linea, err
	}
}

// RecibirMensajeMesa lee una linea y la interpreta como MensajeMesa.
func (c *Codec) RecibirMensajeMesa() (MensajeMesa, error) {
	var m MensajeMesa
	linea, err := c.leerLinea(MaxLineaMesa)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(linea, &m)
}

// RecibirMensajeBot lee una linea y la interpreta como MensajeBot.
func (c *Codec) RecibirMensajeBot() (MensajeBot, error) {
	var m MensajeBot
	linea, err := c.leerLinea(MaxLineaBot)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(linea, &m)
}
