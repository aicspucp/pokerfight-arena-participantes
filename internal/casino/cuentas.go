// Este archivo contiene el registro de cuentas, el login y la validación de
// tokens de sesión. Ver casino.go para los tipos y la interfaz Casino.
package casino

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/internal/almacen"
)

// esquema es todo lo que el Casino persiste en disco. No se expone fuera del
// paquete: los métodos de Casino siempre devuelven copias de Cuenta/Bot/etc,
// nunca punteros a lo que hay adentro de esquema.
type esquema struct {
	Cuentas      map[string]Cuenta       `json:"cuentas"`      // por ID de cuenta.
	PorUsuario   map[string]string       `json:"por_usuario"`  // usuario en minusculas -> ID de cuenta.
	Tokens       map[string]string       `json:"tokens"`       // token -> ID de cuenta.
	Bots         map[string][]Bot        `json:"bots"`         // ID de cuenta -> versiones subidas, en orden.
	Estadisticas map[string]Estadisticas `json:"estadisticas"` // por ID de cuenta.
}

func esquemaVacio() esquema {
	return esquema{
		Cuentas:      make(map[string]Cuenta),
		PorUsuario:   make(map[string]string),
		Tokens:       make(map[string]string),
		Bots:         make(map[string][]Bot),
		Estadisticas: make(map[string]Estadisticas),
	}
}

// motor es la implementación concreta de Casino: mantiene todo en memoria y
// persiste en almacen después de cada escritura.
type motor struct {
	almacen *almacen.JSON

	mu    sync.Mutex
	datos esquema

	// Índices de solo lectura para ValidarToken, que corre en el handshake de
	// cada bot que se conecta. Antes tomaba mu, el mismo candado que sostienen
	// Registrar y Login mientras escriben el archivo en disco: con muchos bots
	// conectándose a la vez, cada saludo esperaba a que terminara cualquier
	// escritura, se vencía el plazo de handshake y la mesa rechazaba a todos.
	// Las cuentas y los tokens nunca se borran, así que alcanza con agregarlos
	// aquí una vez persistidos.
	tokensIdx  sync.Map // token -> ID de cuenta
	cuentasIdx sync.Map // ID de cuenta -> Cuenta
}

// Verificación en tiempo de compilación de que cumple el contrato.
var _ Casino = (*motor)(nil)

// Nuevo construye un Casino que persiste en el archivo JSON de rutaAlmacen.
// Si el archivo ya existe, carga su contenido; si no, arranca vacío (se crea
// recién con la primera escritura).
func Nuevo(rutaAlmacen string) (Casino, error) {
	m := &motor{
		almacen: almacen.NuevoJSON(rutaAlmacen),
		datos:   esquemaVacio(),
	}
	if err := m.almacen.Cargar(&m.datos); err != nil {
		return nil, fmt.Errorf("casino: no se pudo cargar %s: %w", rutaAlmacen, err)
	}
	// Un archivo viejo puede traer algún mapa en null (json.Unmarshal no
	// toca lo que ya no aparece en el JSON): nos aseguramos de no operar
	// nunca sobre un mapa nil.
	if m.datos.Cuentas == nil {
		m.datos.Cuentas = make(map[string]Cuenta)
	}
	if m.datos.PorUsuario == nil {
		m.datos.PorUsuario = make(map[string]string)
	}
	if m.datos.Tokens == nil {
		m.datos.Tokens = make(map[string]string)
	}
	if m.datos.Bots == nil {
		m.datos.Bots = make(map[string][]Bot)
	}
	if m.datos.Estadisticas == nil {
		m.datos.Estadisticas = make(map[string]Estadisticas)
	}
	for id, cuenta := range m.datos.Cuentas {
		m.cuentasIdx.Store(id, cuenta)
	}
	for token, id := range m.datos.Tokens {
		m.tokensIdx.Store(token, id)
	}
	return m, nil
}

// guardarSinLock persiste el estado actual. Requiere que el llamador ya
// tenga m.mu tomado.
func (m *motor) guardarSinLock() error {
	return m.almacen.Guardar(&m.datos)
}

// claveUsuario normaliza un nombre de usuario para usarlo como clave: el
// registro y el login no distinguen mayúsculas ni espacios de más.
func claveUsuario(usuario string) string {
	return strings.ToLower(strings.TrimSpace(usuario))
}

// Registrar crea una cuenta nueva. Si no se puede persistir, deshace el
// cambio en memoria: la API nunca deja el estado en memoria y el disco
// desincronizados.
func (m *motor) Registrar(usuario string) (Cuenta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clave := claveUsuario(usuario)
	if clave == "" {
		return Cuenta{}, fmt.Errorf("casino: el usuario no puede estar vacío")
	}
	if _, existe := m.datos.PorUsuario[clave]; existe {
		return Cuenta{}, fmt.Errorf("casino: el usuario %q ya está registrado", usuario)
	}

	id := nuevoID("c")
	cuenta := Cuenta{ID: id, Usuario: usuario, Creada: time.Now().UTC()}

	m.datos.Cuentas[id] = cuenta
	m.datos.PorUsuario[clave] = id
	m.datos.Estadisticas[id] = Estadisticas{IDCuenta: id}

	if err := m.guardarSinLock(); err != nil {
		delete(m.datos.Cuentas, id)
		delete(m.datos.PorUsuario, clave)
		delete(m.datos.Estadisticas, id)
		return Cuenta{}, fmt.Errorf("casino: no se pudo guardar la cuenta nueva: %w", err)
	}
	m.cuentasIdx.Store(id, cuenta)
	return cuenta, nil
}

// Login emite un token de sesión nuevo para una cuenta ya registrada. No
// invalida tokens anteriores: una cuenta puede tener varias sesiones activas
// a la vez (por ejemplo, un bot corriendo y el CLI de ranking en paralelo).
func (m *motor) Login(usuario string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clave := claveUsuario(usuario)
	id, existe := m.datos.PorUsuario[clave]
	if !existe {
		return "", fmt.Errorf("casino: no existe el usuario %q", usuario)
	}

	token := nuevoID("tok")
	m.datos.Tokens[token] = id

	if err := m.guardarSinLock(); err != nil {
		delete(m.datos.Tokens, token)
		return "", fmt.Errorf("casino: no se pudo guardar el token nuevo: %w", err)
	}
	m.tokensIdx.Store(token, id)
	return token, nil
}

// ValidarToken traduce un token a la identidad de la cuenta. Tiene la misma
// forma que mesa.ValidarToken (interfaces.md de la organización §4): un *motor se puede
// pasar directo a mesa.NuevoServidor sin envoltorios.
//
// No toma m.mu: lee los índices de solo lectura (ver motor), así que no espera a
// las escrituras a disco de Registrar, Login ni RegistrarResultado.
func (m *motor) ValidarToken(token string) (idCuenta, nombre string, err error) {
	id, existe := m.tokensIdx.Load(token)
	if !existe {
		return "", "", fmt.Errorf("casino: token inválido")
	}
	v, existe := m.cuentasIdx.Load(id)
	if !existe {
		// No debería pasar (los tokens se borran junto con la cuenta si
		// alguna vez se implementa borrado), pero cubrimos el caso.
		return "", "", fmt.Errorf("casino: la cuenta del token ya no existe")
	}
	cuenta := v.(Cuenta)
	return cuenta.ID, cuenta.Usuario, nil
}

// Cuenta devuelve los datos de una cuenta por su ID.
func (m *motor) Cuenta(idCuenta string) (Cuenta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cuenta, existe := m.datos.Cuentas[idCuenta]
	if !existe {
		return Cuenta{}, fmt.Errorf("casino: no existe la cuenta %q", idCuenta)
	}
	return cuenta, nil
}
