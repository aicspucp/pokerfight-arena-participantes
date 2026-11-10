package mesa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/internal/protocolo"
)

// Es la interfaz que representa la conexión de un jugador con la mesa. Cualquier
// tipo de conexión a implementar (websocket, TCP, de prueba, etc.) debe implementar
// esta interfaz.
// Esta interfaz es utilizada por la mesa para enviar mensajes a los jugadores y
// recibir mensajes (acciones) de los jugadores.
// NOTA: Al ser una interfaz, no es necesario que las estructuras que la usen la llamen
// como puntero. Esto se debe a que en Go, las interfaces se manejan internamente como
// punteros. Por lo tanto, no habrán conexiones duplicadas ni problemas de memoria o red.
type ConexionMesaJugador interface {
	EnviarMensaje(m protocolo.MensajeMesa, timeoutMs uint64) error // La mesa envía un mensaje al jugador. Debe tener un timeout si no recibe confirmación de la respuesta. En caso no reciba confirmación, se retira al jugador de la mesa.
	SolicitarAccion(timeoutMs uint64) (protocolo.Accion, error)    // La mesa solicita una acción al jugador cuando sea su turno. Debe tener un timeout si no recibe confirmación de la respuesta. En caso no reciba confirmación, se opta por la acción más segura (check o fold) según el estado de la partida. En caso de error, se opta por la acción más segura (check o fold) según el estado de la partida.
	Cerrar() error                                                 // Cerrar la conexión con el jugador. Se llama cuando se termina la partida o el jugador se retira voluntariamente.
}

// Errores que puede devolver cualquier implementación de ConexionMesaJugador.
// La mesa los usa para decidir qué hacer: ante ErrTiempoAgotado aplica
// protocolo.AccionSegura; ante ErrConexionCerrada o ErrJugadorAbandono levanta
// al jugador de la mesa.
var (
	ErrConexionCerrada = errors.New("mesa: la conexión está cerrada")
	ErrTiempoAgotado   = errors.New("mesa: el jugador no respondió a tiempo")
	ErrJugadorAbandono = errors.New("mesa: el jugador abandonó la mesa")
	ErrVersionInvalida = errors.New("mesa: versión de protocolo incompatible")
	ErrSaludoInvalido  = errors.New("mesa: se esperaba un mensaje de saludo")
	ErrTokenInvalido   = errors.New("mesa: token inválido")
	ErrServidorCerrado = errors.New("mesa: el servidor está cerrado")
	ErrSinAcciones     = errors.New("mesa: la conexión de prueba no tiene más acciones programadas")
)

// timeoutMaximoMs acota el plazo para evitar desbordes al convertir a
// time.Duration. Son ~24 días, muy por encima de cualquier partida.
const timeoutMaximoMs = uint64(1) << 31

// plazo traduce un timeout en milisegundos a un instante absoluto para
// net.Conn.SetDeadline. Un timeout de 0 significa "sin plazo".
func plazo(timeoutMs uint64) time.Time {
	if timeoutMs == 0 {
		return time.Time{}
	}
	if timeoutMs > timeoutMaximoMs {
		timeoutMs = timeoutMaximoMs
	}
	return time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
}

// esTimeout indica si un error de red es un vencimiento de plazo.
func esTimeout(err error) bool {
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// --- Conexión real sobre TCP ----------------------------------------------

// ConexionTCP es la implementación real de ConexionMesaJugador: habla JSON
// Lines sobre una net.Conn (ver docs/protocolo.md). Es segura para uso
// concurrente: la mesa puede difundir un estado mientras otro jugador piensa.
type ConexionTCP struct {
	IDJugador string // Se completa tras el handshake. Vacío mientras no haya token validado.
	Nombre    string // Nombre a mostrar, devuelto por ValidarToken.

	conexion net.Conn
	codec    *protocolo.Codec

	// Tres candados, cada uno con un solo trabajo. Antes había uno solo que se
	// sostenía durante toda la espera de una lectura: Cerrar() (y con él el cierre
	// del servidor y las reconexiones) quedaba bloqueado hasta que el bot
	// respondiera, y para siempre si el plazo era 0. Ahora:
	//
	//   - muLectura serializa las lecturas (acción del turno, saludo).
	//   - muEscritura serializa las escrituras.
	//   - mu protege solo el estado pequeño de abajo y nunca se sostiene durante
	//     una operación de red, así que Cerrar() no espera a nadie.
	muLectura   sync.Mutex
	muEscritura sync.Mutex

	mu       sync.Mutex
	idMano   string // Última mano anunciada. Sirve para descartar acciones tardías.
	cerrada  bool
	timeouts uint64 // Cuántas veces el jugador no respondió a tiempo.

	// listo, si no es nil, retiene los mensajes de juego hasta que se mande la
	// bienvenida. Hace falta porque el jugador queda sentado (o reconectado)
	// un instante antes de recibirla: si justo entonces la mesa difunde un
	// estado, ese mensaje llegaría primero y el bot, que espera la bienvenida
	// como primer mensaje (docs/protocolo.md), fallaría el handshake. Lo activa
	// el servidor con exigirBienvenida; se cierra una sola vez (listoUnaVez).
	listo       chan struct{}
	listoUnaVez sync.Once
}

// exigirBienvenida activa la retención de mensajes de juego hasta la
// bienvenida. Se llama antes de que la conexión se comparta con la mesa.
func (c *ConexionTCP) exigirBienvenida() {
	c.listo = make(chan struct{})
}

// marcarLista libera los mensajes retenidos. Es idempotente.
func (c *ConexionTCP) marcarLista() {
	if c.listo != nil {
		c.listoUnaVez.Do(func() { close(c.listo) })
	}
}

// esperarBienvenida bloquea un mensaje de juego hasta que la bienvenida haya
// salido (o la conexión se cierre), como mucho timeoutMs (0 = sin tope).
func (c *ConexionTCP) esperarBienvenida(m protocolo.MensajeMesa, timeoutMs uint64) error {
	if c.listo == nil || m.Tipo == protocolo.MsgBienvenida || m.Tipo == protocolo.MsgError {
		return nil
	}
	if timeoutMs == 0 {
		<-c.listo
		return nil
	}
	t := time.NewTimer(time.Until(plazo(timeoutMs)))
	defer t.Stop()
	select {
	case <-c.listo:
		return nil
	case <-t.C:
		return ErrTiempoAgotado
	}
}

// Verificación en tiempo de compilación de que cumple el contrato.
var _ ConexionMesaJugador = (*ConexionTCP)(nil)

// NuevaConexionTCP envuelve una net.Conn ya establecida (la que devuelve
// Accept en el servidor, o Dial en un cliente).
func NuevaConexionTCP(c net.Conn) *ConexionTCP {
	return &ConexionTCP{
		conexion: c,
		codec:    protocolo.NuevoCodec(c, c),
	}
}

// ConectarTCP abre una conexión saliente hacia una mesa. Se usa en pruebas de
// humo y desde el lado del bot; la mesa como servidor usa NuevaConexionTCP.
func ConectarTCP(direccion string, timeoutMs uint64) (*ConexionTCP, error) {
	var (
		c   net.Conn
		err error
	)
	if timeoutMs == 0 {
		c, err = net.Dial("tcp", direccion)
	} else {
		c, err = net.DialTimeout("tcp", direccion, time.Until(plazo(timeoutMs)))
	}
	if err != nil {
		return nil, err
	}
	return NuevaConexionTCP(c), nil
}

// EnviarMensaje serializa el mensaje y lo escribe en la conexión. El timeout
// acota la escritura: si el jugador no lee (buffer lleno) el envío falla y la
// mesa puede levantarlo.
func (c *ConexionTCP) EnviarMensaje(m protocolo.MensajeMesa, timeoutMs uint64) error {
	if err := c.esperarBienvenida(m, timeoutMs); err != nil {
		return err
	}
	c.muEscritura.Lock()
	defer c.muEscritura.Unlock()

	if m.Version == "" {
		m.Version = protocolo.VersionProtocolo
	}
	// Recordamos la mano en curso para poder descartar acciones tardías.
	idMano := ""
	if m.Estado != nil {
		idMano = m.Estado.IDMano
	}
	if err := c.registrarEnvio(idMano); err != nil {
		return err
	}

	if err := c.conexion.SetWriteDeadline(plazo(timeoutMs)); err != nil {
		return err
	}
	defer c.conexion.SetWriteDeadline(time.Time{})

	if err := c.codec.Enviar(m); err != nil {
		if esTimeout(err) {
			c.sumarTimeout()
			return ErrTiempoAgotado
		}
		return err
	}
	return nil
}

// registrarEnvio comprueba que la conexión siga abierta y anota la mano en
// curso. Es todo el estado que toca EnviarMensaje, y va aparte para que mu no
// se sostenga durante la escritura.
func (c *ConexionTCP) registrarEnvio(idMano string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cerrada {
		return ErrConexionCerrada
	}
	if idMano != "" {
		c.idMano = idMano
	}
	return nil
}

func (c *ConexionTCP) estaCerrada() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cerrada
}

func (c *ConexionTCP) manoActual() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.idMano
}

func (c *ConexionTCP) sumarTimeout() {
	c.mu.Lock()
	c.timeouts++
	c.mu.Unlock()
}

// alErrorDeLectura traduce el error de una lectura. Un plazo vencido es
// ErrTiempoAgotado. Una línea demasiado larga deja la conexión desincronizada
// (el resto de la línea sigue en el socket) y es una señal de un cliente que no
// es un bot: se cierra en el acto.
func (c *ConexionTCP) alErrorDeLectura(err error) error {
	switch {
	case esTimeout(err):
		c.sumarTimeout()
		return ErrTiempoAgotado
	case errors.Is(err, protocolo.ErrLineaDemasiadoLarga):
		c.Cerrar()
	}
	return err
}

// SolicitarAccion espera la respuesta del jugador a un MsgSolicitarAccion ya
// enviado. Descarta mensajes que no sean acciones y acciones de una mano
// anterior (llegadas tarde), siempre dentro del mismo plazo.
// Devuelve ErrTiempoAgotado si vence el plazo y ErrJugadorAbandono si el
// jugador se retira: en ambos casos la mesa debe aplicar protocolo.AccionSegura
// o levantar al jugador, según corresponda.
func (c *ConexionTCP) SolicitarAccion(timeoutMs uint64) (protocolo.Accion, error) {
	c.muLectura.Lock()
	defer c.muLectura.Unlock()

	if c.estaCerrada() {
		return protocolo.Accion{}, ErrConexionCerrada
	}
	if err := c.conexion.SetReadDeadline(plazo(timeoutMs)); err != nil {
		return protocolo.Accion{}, err
	}
	defer c.conexion.SetReadDeadline(time.Time{})

	for {
		m, err := c.codec.RecibirMensajeBot()
		if err != nil {
			return protocolo.Accion{}, c.alErrorDeLectura(err)
		}

		switch m.Tipo {
		case protocolo.MsgAccion:
			// El bot hace eco del id_mano: si no coincide, la acción
			// corresponde a una mano ya cerrada y se ignora.
			if idMano := c.manoActual(); idMano != "" && m.IDMano != "" && m.IDMano != idMano {
				continue
			}
			if m.Accion == nil {
				return protocolo.Accion{}, protocolo.ErrAccionAusente
			}
			return *m.Accion, nil
		case protocolo.MsgAbandono:
			return protocolo.Accion{}, ErrJugadorAbandono
		default:
			// Saludos repetidos o basura: se ignoran, el plazo sigue corriendo.
			continue
		}
	}
}

// Cerrar cierra la conexión subyacente. Es idempotente y no espera a ninguna
// lectura ni escritura en curso: cerrar el socket es justamente lo que las
// desbloquea (devuelven error y sueltan sus candados).
func (c *ConexionTCP) Cerrar() error {
	c.mu.Lock()
	if c.cerrada {
		c.mu.Unlock()
		return nil
	}
	c.cerrada = true
	c.mu.Unlock()
	c.marcarLista() // Que nadie quede esperando una bienvenida que ya no saldrá.
	return c.conexion.Close()
}

// cerrarConGracia cierra después de un rechazo sin que el mensaje se pierda. Si se
// cierra un socket TCP con datos del otro lado todavía sin leer (el saludo que
// mandó el cliente y que no llegamos a leer), el sistema responde con un RST que
// puede hacer que el cliente descarte el mensaje de error que acabamos de
// escribir: el bot ve "conexión abortada" en vez del motivo. Se cierra primero la
// escritura (el cliente recibe el mensaje y el fin de datos), se lee y descarta lo
// que traía el cliente durante un instante y recién entonces se cierra del todo.
func (c *ConexionTCP) cerrarConGracia() {
	if tc, ok := c.conexion.(*net.TCPConn); ok && !c.estaCerrada() {
		tc.CloseWrite()
		tc.SetReadDeadline(time.Now().Add(graciaCierre))
		io.Copy(io.Discard, io.LimitReader(tc, 64<<10))
	}
	c.Cerrar()
}

// Direccion devuelve la dirección remota, útil para los logs del servidor.
func (c *ConexionTCP) Direccion() string {
	if c.conexion == nil || c.conexion.RemoteAddr() == nil {
		return ""
	}
	return c.conexion.RemoteAddr().String()
}

// Timeouts es la cantidad de veces que el jugador no respondió a tiempo
// (requisito de Estadisticas.Timeouts en interfaces.md de la organización).
func (c *ConexionTCP) Timeouts() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timeouts
}

// recibirMensajeBot lee un mensaje del bot con plazo. Lo usa el handshake, que
// ocurre antes de que la conexión se entregue a la mesa.
func (c *ConexionTCP) recibirMensajeBot(timeoutMs uint64) (protocolo.MensajeBot, error) {
	c.muLectura.Lock()
	defer c.muLectura.Unlock()

	if c.estaCerrada() {
		return protocolo.MensajeBot{}, ErrConexionCerrada
	}
	if err := c.conexion.SetReadDeadline(plazo(timeoutMs)); err != nil {
		return protocolo.MensajeBot{}, err
	}
	defer c.conexion.SetReadDeadline(time.Time{})

	m, err := c.codec.RecibirMensajeBot()
	if err != nil {
		// En el handshake un plazo vencido no cuenta como timeout del jugador
		// (todavía no es un jugador).
		if esTimeout(err) {
			return m, ErrTiempoAgotado
		}
		return m, c.alErrorDeLectura(err)
	}
	return m, nil
}

// EnviarMensajeBot escribe un mensaje del lado del bot. Solo se usa desde el
// cliente (pruebas de humo, bots de ejemplo); la mesa nunca lo llama.
func (c *ConexionTCP) EnviarMensajeBot(m protocolo.MensajeBot, timeoutMs uint64) error {
	c.muEscritura.Lock()
	defer c.muEscritura.Unlock()

	if c.estaCerrada() {
		return ErrConexionCerrada
	}
	if err := c.conexion.SetWriteDeadline(plazo(timeoutMs)); err != nil {
		return err
	}
	defer c.conexion.SetWriteDeadline(time.Time{})
	return c.codec.Enviar(m)
}

// RecibirMensajeMesa lee un mensaje de la mesa. Igual que EnviarMensajeBot,
// es la cara de cliente de la conexión.
func (c *ConexionTCP) RecibirMensajeMesa(timeoutMs uint64) (protocolo.MensajeMesa, error) {
	c.muLectura.Lock()
	defer c.muLectura.Unlock()

	if c.estaCerrada() {
		return protocolo.MensajeMesa{}, ErrConexionCerrada
	}
	if err := c.conexion.SetReadDeadline(plazo(timeoutMs)); err != nil {
		return protocolo.MensajeMesa{}, err
	}
	defer c.conexion.SetReadDeadline(time.Time{})

	m, err := c.codec.RecibirMensajeMesa()
	if err != nil && esTimeout(err) {
		return m, ErrTiempoAgotado
	}
	return m, err
}

// ParConexionesMemoria devuelve dos ConexionTCP conectadas entre sí mediante
// net.Pipe: una para la mesa y otra para el bot. Sirve para probar el
// protocolo completo (codec, handshake, plazos) sin abrir puertos.
func ParConexionesMemoria() (mesa *ConexionTCP, bot *ConexionTCP) {
	a, b := net.Pipe()
	return NuevaConexionTCP(a), NuevaConexionTCP(b)
}

// --- Servidor TCP ----------------------------------------------------------

// ValidarToken traduce el token emitido por el Casino a una identidad. La mesa
// no importa el paquete casino: recibe esta función por inyección
// (interfaces.md de la organización §4). Si es nil, el servidor trabaja en modo abierto y usa
// el token como identificador (solo para desarrollo).
type ValidarToken func(token string) (idJugador string, nombre string, err error)

// ConfigServidor define cómo escucha la mesa.
type ConfigServidor struct {
	Direccion          string // Dirección de escucha, por ejemplo ":9000". ":0" pide un puerto libre.
	TimeoutHandshakeMs uint64 // Plazo para que el bot envíe su saludo. 0 = sin plazo.
	// MaxConexiones es cuántos jugadores distintos pueden tener conexión
	// registrada. 0 = sin límite. Un jugador que ya está registrado y vuelve a
	// conectarse (reconexión) no cuenta como uno nuevo: si contara, un bot que
	// pierde el socket en una mesa llena no podría volver nunca.
	MaxConexiones int
	// MaxHandshakes es cuántos handshakes pueden estar en curso a la vez. Antes
	// este tope era MaxConexiones (como mucho 8): una ráfaga de más de 8 bots
	// conectándose juntos, normal al abrir un torneo, recibía "demasiadas
	// conexiones" aunque la mesa estuviera vacía. Ahora es un tope propio, pensado
	// para acotar las goroutines que acumulan conexiones que no saludan, no para
	// limitar jugadores.
	//
	// 0 usa el valor por defecto: el mayor entre 64 y 8×MaxConexiones, o sin
	// límite si MaxConexiones también es 0. Un valor negativo es sin límite.
	MaxHandshakes int
	// MaxHandshakesPorIP es cuántos handshakes puede tener en curso a la vez una
	// misma dirección IP. Sin esto, una sola máquina que abra cientos de
	// conexiones mudas ocupa todos los lugares de MaxHandshakes y nadie más puede
	// saludar hasta que venza el plazo. 0 usa 16; un valor negativo es sin límite.
	// Es un tope de saludos simultáneos, no de bots: un aula entera detrás de la
	// misma IP puede jugar, y a quien llegue en el mismo instante que los demás
	// se le pide reintentar (el SDK lo hace solo).
	MaxHandshakesPorIP int
	Log                *log.Logger

	// SillaDe, si no es nil, decide en qué silla se sienta cada jugador. Se
	// consulta con el ID que devolvió ValidarToken; si responde ok=false, el
	// jugador va a la primera silla libre, como siempre.
	//
	// Sin esto, la silla la reparte el orden de conexión, que entre bots
	// lanzados en paralelo es una carrera. Es lo que usa la arena para fijar
	// las posiciones de cada enfrentamiento.
	SillaDe func(idJugador string) (silla int, ok bool)

	// AlConectar, si no es nil, se llama cuando un jugador terminó el handshake:
	// ya está sentado y ya recibió su bienvenida. Es el primer momento en que es
	// seguro mandarle otra cosa por el socket.
	//
	// Importa para quien reparte las conexiones entre varias mesas (el lobby de
	// un campeonato): si empezara a hablarle al bot apenas lo sienta, un mensaje
	// de juego podría llegar antes que la bienvenida, y el bot esperaría otra cosa.
	AlConectar func(idJugador string)
}

// Servidor acepta conexiones TCP de bots, hace el handshake y sienta a los
// jugadores validados en la mesa.
type Servidor struct {
	cfg     ConfigServidor
	mesa    MesaInterface
	validar ValidarToken

	mu         sync.Mutex
	escucha    net.Listener
	conexiones map[string]*ConexionTCP
	enCurso    int            // Conexiones aceptadas que todavía no terminaron el handshake.
	porIP      map[string]int // Lo mismo, por dirección IP (ver MaxHandshakesPorIP).
	rechazando int            // Rechazos que se están escribiendo (ver rechazarEnSegundoPlano).
	cerrado    bool
	espera     sync.WaitGroup
}

const (
	// handshakesMinimos es el piso del tope de handshakes simultáneos por defecto.
	handshakesMinimos = 64
	// rechazosMaximos acota las goroutines que avisan "mesa llena" o "demasiadas
	// conexiones". Pasado el tope se cierra el socket sin explicar: ante una
	// avalancha, ahorrar recursos importa más que ser amable.
	rechazosMaximos = 64
	// plazoRechazoMs es cuánto se espera para escribir un rechazo si la mesa no
	// tiene plazo de handshake (0). Sin plazo, un cliente que no lee dejaría la
	// escritura colgada para siempre.
	plazoRechazoMs = 1000

	// Reintento de Accept: espera inicial y tope de la espera creciente, y cuántos
	// errores seguidos se toleran antes de rendirse.
	// handshakesPorIPPorDefecto es el valor de MaxHandshakesPorIP cuando vale 0.
	handshakesPorIPPorDefecto = 16
	// graciaCierre es cuánto se espera, al rechazar, a que el otro lado termine de
	// mandar lo que ya venía en camino antes de cerrar el socket.
	graciaCierre = 300 * time.Millisecond

	esperaAcceptMin = 5 * time.Millisecond
	esperaAcceptMax = time.Second
	fallosAcceptMax = 60
)

// maxHandshakesPorIP resuelve el tope efectivo por IP. 0 es sin límite.
func (s *Servidor) maxHandshakesPorIP() int {
	switch {
	case s.cfg.MaxHandshakesPorIP < 0:
		return 0
	case s.cfg.MaxHandshakesPorIP > 0:
		return s.cfg.MaxHandshakesPorIP
	}
	return handshakesPorIPPorDefecto
}

// hostDe devuelve la dirección IP del otro lado de una conexión, sin el puerto.
func hostDe(c net.Conn) string {
	if c.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return c.RemoteAddr().String()
	}
	return host
}

// maxHandshakes resuelve el tope efectivo de handshakes en curso. 0 es sin límite.
func (s *Servidor) maxHandshakes() int {
	switch {
	case s.cfg.MaxHandshakes < 0:
		return 0
	case s.cfg.MaxHandshakes > 0:
		return s.cfg.MaxHandshakes
	case s.cfg.MaxConexiones > 0:
		return max(handshakesMinimos, 8*s.cfg.MaxConexiones)
	}
	return 0
}

// NuevoServidor construye el servidor. No abre el puerto: eso lo hace Abrir.
func NuevoServidor(cfg ConfigServidor, m MesaInterface, validar ValidarToken) *Servidor {
	if cfg.Log == nil {
		cfg.Log = log.Default()
	}
	return &Servidor{
		cfg:        cfg,
		mesa:       m,
		validar:    validar,
		conexiones: make(map[string]*ConexionTCP),
		porIP:      make(map[string]int),
	}
}

// Abrir reserva el puerto sin empezar a aceptar. Separarlo de Servir permite
// conocer la dirección real (útil con ":0") antes de que llegue nadie, y evita
// carreras en las pruebas. Es idempotente.
func (s *Servidor) Abrir() error {
	// Se mantiene el lock durante el Listen: es una operación corta y así dos
	// llamadas concurrentes no reservan dos puertos distintos.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cerrado {
		return ErrServidorCerrado
	}
	if s.escucha != nil {
		return nil
	}

	escucha, err := net.Listen("tcp", s.cfg.Direccion)
	if err != nil {
		return err
	}
	s.escucha = escucha
	s.cfg.Log.Printf("mesa: escuchando en %s", escucha.Addr())
	return nil
}

// Escuchar abre el puerto y atiende conexiones hasta que se cancele el
// contexto o se llame a Cerrar. Bloquea. Devuelve nil en un cierre ordenado.
func (s *Servidor) Escuchar(ctx context.Context) error {
	if err := s.Abrir(); err != nil {
		return err
	}
	return s.Servir(ctx)
}

// Servir corre el bucle de aceptación sobre un puerto ya abierto por Abrir.
// Bloquea hasta que se cancele el contexto o se llame a Cerrar.
func (s *Servidor) Servir(ctx context.Context) error {
	if err := s.Abrir(); err != nil {
		return err
	}

	s.mu.Lock()
	escucha := s.escucha
	s.mu.Unlock()

	// El cierre del listener es lo que desbloquea Accept.
	hecho := make(chan struct{})
	defer close(hecho)
	go func() {
		select {
		case <-ctx.Done():
			s.Cerrar()
		case <-hecho:
		}
	}()

	var espera time.Duration
	fallos := 0
	for {
		conexion, err := escucha.Accept()
		if err != nil {
			s.mu.Lock()
			cerrado := s.cerrado
			s.mu.Unlock()
			if cerrado {
				s.espera.Wait()
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			// Un error de Accept bajo carga casi nunca es el fin del servidor:
			// se acabaron los descriptores de archivo, un cliente cortó antes de
			// que lo aceptáramos, el sistema está saturado. Antes esto hacía
			// volver a Servir, y con eso nadie más podía conectarse ni
			// reconectarse hasta reiniciar la mesa. Se espera (cada vez más) y se
			// sigue; solo se rinde si el error no se va.
			fallos++
			if fallos > fallosAcceptMax {
				return fmt.Errorf("mesa: demasiados errores seguidos aceptando conexiones: %w", err)
			}
			espera = min(max(2*espera, esperaAcceptMin), esperaAcceptMax)
			s.cfg.Log.Printf("mesa: error aceptando una conexión (%v); reintento en %s", err, espera)
			select {
			case <-ctx.Done():
			case <-time.After(espera):
			}
			continue
		}
		fallos, espera = 0, 0

		// Antes del handshake no se sabe quién es, así que no se puede decidir
		// todavía si entra (una reconexión entra aunque la mesa esté llena).
		// Lo único que se limita aquí es cuántos pueden estar saludando a la vez,
		// para que conexiones que no saludan no acumulen goroutines sin techo.
		host := hostDe(conexion)
		if !s.reservarHandshake(host) {
			s.rechazarEnSegundoPlano(conexion, protocolo.MotivoMesaSaturada)
			continue
		}

		s.espera.Add(1)
		go func() {
			defer s.espera.Done()
			defer s.liberarHandshake(host)
			s.atender(conexion)
		}()
	}
}

// Direccion devuelve la dirección real de escucha. Es la forma de conocer el
// puerto cuando se configuró ":0".
func (s *Servidor) Direccion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.escucha == nil {
		return s.cfg.Direccion
	}
	return s.escucha.Addr().String()
}

// Cerrar deja de aceptar conexiones y cierra las que estén abiertas. Es
// idempotente y seguro de llamar desde otra goroutine.
func (s *Servidor) Cerrar() error {
	s.mu.Lock()
	if s.cerrado {
		s.mu.Unlock()
		return nil
	}
	s.cerrado = true
	escucha := s.escucha
	conexiones := make([]*ConexionTCP, 0, len(s.conexiones))
	for _, c := range s.conexiones {
		conexiones = append(conexiones, c)
	}
	s.conexiones = make(map[string]*ConexionTCP)
	s.mu.Unlock()

	var err error
	if escucha != nil {
		err = escucha.Close()
	}
	for _, c := range conexiones {
		c.Cerrar()
	}
	return err
}

// Conexiones devuelve las conexiones actualmente registradas, por id de jugador.
func (s *Servidor) Conexiones() map[string]*ConexionTCP {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make(map[string]*ConexionTCP, len(s.conexiones))
	for id, c := range s.conexiones {
		copia[id] = c
	}
	return copia
}

// reservarHandshake anota una conexión más en curso de handshake, si hay
// lugar. Ver Servir.
func (s *Servidor) reservarHandshake(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tope := s.maxHandshakes(); tope > 0 && s.enCurso >= tope {
		return false
	}
	if tope := s.maxHandshakesPorIP(); tope > 0 && s.porIP[host] >= tope {
		return false
	}
	s.enCurso++
	s.porIP[host]++
	return true
}

func (s *Servidor) liberarHandshake(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enCurso--
	if s.porIP[host] <= 1 {
		delete(s.porIP, host) // El mapa no crece con cada IP que pasó alguna vez.
	} else {
		s.porIP[host]--
	}
}

// admite decide, ya con la identidad validada, si el jugador puede entrar: un
// jugador ya registrado siempre (es una reconexión), uno nuevo solo si queda
// lugar.
//
// Dos jugadores nuevos que saludan a la vez pueden pasar los dos este control
// con un solo lugar libre. No es un problema: la silla la reparte la Mesa bajo
// su propio lock, y el segundo recibe "mesa llena" de SentarJugador.
func (s *Servidor) admite(id string) bool {
	if s.cfg.MaxConexiones <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, registrado := s.conexiones[id]; registrado {
		return true
	}
	return len(s.conexiones) < s.cfg.MaxConexiones
}

// atender corre en su propia goroutine por conexión: handshake y, si todo va
// bien, entrega el jugador a la mesa.
func (s *Servidor) atender(conexion net.Conn) {
	cx := NuevaConexionTCP(conexion)
	cx.exigirBienvenida()

	// La mesa todavía tiene tareas sin implementar (ver interfaces.md de la organización).
	// Un panic en SentarJugador no debe tumbar al servidor entero.
	defer func() {
		if r := recover(); r != nil {
			s.cfg.Log.Printf("mesa: pánico atendiendo a %s: %v", cx.Direccion(), r)
			cx.Cerrar()
		}
	}()

	id, nombre, err := s.saludar(cx)
	if err != nil {
		s.cfg.Log.Printf("mesa: handshake fallido con %s: %v", cx.Direccion(), err)
		s.rechazarConexion(cx, err.Error())
		return
	}

	cx.IDJugador = id
	cx.Nombre = nombre

	if !s.admite(id) {
		s.cfg.Log.Printf("mesa: no se pudo sentar a %s: mesa llena", id)
		s.rechazarConexion(cx, "mesa llena")
		return
	}

	// Primero se intenta sentar. Si ese ID ya tiene silla, es una reconexión
	// (el bot se cayó y volvió): se le cambia la conexión sin tocarle el saldo.
	// Sin esto, un bot que pierde el socket queda fuera de la partida para
	// siempre y sus turnos se resuelven con la acción segura hasta quedar en 0.
	reconexion := false
	if err := s.sentar(id, nombre, cx); err != nil {
		if errReconexion := s.mesa.ReconectarJugador(id, cx); errReconexion != nil {
			// No pudo entrar ni como jugador nuevo ni como reconexión: se le
			// informa el motivo original (mesa llena, normalmente).
			s.cfg.Log.Printf("mesa: no se pudo sentar a %s: %v", id, err)
			s.rechazarConexion(cx, err.Error())
			return
		}
		reconexion = true
	}

	// La bienvenida se manda recién ahora, con la silla ya asignada: es la
	// única forma que tiene el bot de saber cuál de EstadoPublico.Jugadores es
	// él mismo (el token no sirve de ID salvo en modo abierto).
	if err := s.darBienvenida(cx, id, nombre); err != nil {
		s.cfg.Log.Printf("mesa: no se pudo saludar a %s: %v", id, err)
		cx.Cerrar()
		return
	}

	s.registrar(id, cx)
	if s.cfg.AlConectar != nil {
		s.cfg.AlConectar(id)
	}
	if reconexion {
		s.cfg.Log.Printf("mesa: jugador %s (%s) reconectado desde %s", id, nombre, cx.Direccion())
		return
	}
	s.cfg.Log.Printf("mesa: jugador %s (%s) conectado desde %s", id, nombre, cx.Direccion())
}

// sentar coloca al jugador en la silla que le corresponda: la asignada por
// ConfigServidor.SillaDe si hay una, o la primera libre si no.
func (s *Servidor) sentar(id, nombre string, cx *ConexionTCP) error {
	if s.cfg.SillaDe != nil {
		if silla, ok := s.cfg.SillaDe(id); ok {
			return s.mesa.SentarJugadorEnSilla(id, nombre, silla, cx)
		}
	}
	return s.mesa.SentarJugador(id, nombre, cx)
}

// darBienvenida cierra el handshake informando al bot su identidad y su silla.
// La silla se busca en el estado público porque MesaInterface no expone el
// arreglo de sillas: si no aparece (una implementación de prueba que devuelve
// un estado vacío), se manda -1, que el bot debe leer como "desconocida".
func (s *Servidor) darBienvenida(cx *ConexionTCP, id, nombre string) error {
	silla := -1
	for _, j := range s.mesa.Estado().Jugadores {
		if j.ID == id {
			silla = j.PosicionSilla
			break
		}
	}
	err := cx.EnviarMensaje(protocolo.MensajeMesa{
		Tipo:      protocolo.MsgBienvenida,
		Version:   protocolo.VersionProtocolo,
		IDJugador: id,
		Silla:     silla,
		Mensaje:   "bienvenido " + nombre,
	}, s.cfg.TimeoutHandshakeMs)
	// Recién ahora pueden salir los mensajes de juego que la mesa haya querido
	// mandarle mientras tanto (ver ConexionTCP.listo).
	cx.marcarLista()
	return err
}

// saludar ejecuta la primera mitad del handshake: espera el saludo del bot y
// verifica versión y token. No responde: la bienvenida se manda después de
// sentar al jugador (ver darBienvenida), porque recién ahí se conoce su silla.
func (s *Servidor) saludar(cx *ConexionTCP) (idJugador string, nombre string, err error) {
	m, err := cx.recibirMensajeBot(s.cfg.TimeoutHandshakeMs)
	if err != nil {
		return "", "", err
	}
	if m.Tipo != protocolo.MsgSaludo {
		return "", "", ErrSaludoInvalido
	}
	if m.Version != protocolo.VersionProtocolo {
		return "", "", ErrVersionInvalida
	}

	if s.validar != nil {
		idJugador, nombre, err = s.validar(m.Token)
		if err != nil {
			return "", "", err
		}
	} else {
		// Modo abierto (desarrollo): el token es el identificador.
		idJugador, nombre = m.Token, m.Token
	}
	if idJugador == "" {
		return "", "", ErrTokenInvalido
	}
	if nombre == "" {
		nombre = idJugador
	}
	return idJugador, nombre, nil
}

func (s *Servidor) registrar(id string, cx *ConexionTCP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cerrado {
		cx.Cerrar()
		return
	}
	if anterior, ok := s.conexiones[id]; ok && anterior != cx {
		anterior.Cerrar() // Reconexión: se descarta el socket viejo.
	}
	s.conexiones[id] = cx
}

// rechazarConexion avisa del motivo y cierra. Escribir tiene plazo siempre: un
// cliente que no lee no puede dejar esto colgado.
func (s *Servidor) rechazarConexion(cx *ConexionTCP, motivo string) {
	plazoMs := s.cfg.TimeoutHandshakeMs
	if plazoMs == 0 {
		plazoMs = plazoRechazoMs
	}
	cx.EnviarMensaje(protocolo.MensajeMesa{
		Tipo:    protocolo.MsgError,
		Version: protocolo.VersionProtocolo,
		Mensaje: motivo,
	}, plazoMs)
	cx.cerrarConGracia()
}

// rechazarEnSegundoPlano avisa el rechazo sin frenar el bucle de aceptación. Hacerlo
// ahí mismo era un cuello de botella: cada rechazo escribe en el socket de un
// desconocido, y mientras tanto no se aceptaba a nadie más, ni a los jugadores
// legítimos. Si ya hay muchos rechazos escribiéndose, se cierra sin explicar.
func (s *Servidor) rechazarEnSegundoPlano(conexion net.Conn, motivo string) {
	s.mu.Lock()
	if s.rechazando >= rechazosMaximos {
		s.mu.Unlock()
		conexion.Close()
		return
	}
	s.rechazando++
	s.mu.Unlock()

	s.espera.Add(1)
	go func() {
		defer s.espera.Done()
		defer func() {
			s.mu.Lock()
			s.rechazando--
			s.mu.Unlock()
		}()
		s.rechazarConexion(NuevaConexionTCP(conexion), motivo)
	}()
}
