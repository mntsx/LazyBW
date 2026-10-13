# Notas: instalar Go, compilar y ejecutar LazyBW

Este documento está pensado para quien **nunca ha utilizado Go**. Las instrucciones principales se refieren a **WSL2 Ubuntu 24.04** con Alacritty, aunque también se incluyen notas para Windows.

## 1. ¿Go es compilado?

Sí. Go es un lenguaje compilado. El comando `go build` convierte el código fuente (`.go`) en un **ejecutable nativo** para el sistema operativo y la arquitectura elegidos. No necesitas instalar un intérprete de Go en los equipos donde ejecutes el binario. LazyBW sigue necesitando el **CLI de Bitwarden (`bw`)** en el `PATH`, porque lo utiliza para acceder al vault.

Go también proporciona:

- `go run`: compila y ejecuta el programa para pruebas rápidas.
- `go test`: compila y ejecuta las pruebas.
- `gofmt`: formatea automáticamente el código.
- `go mod`: gestiona dependencias externas.

## 2. Instalar Go en WSL Ubuntu

Comprueba si ya está instalado:

```bash
go version
```

Este repositorio indica `go 1.23` como versión mínima en `go.mod`. Si tienes una versión anterior, instala **Go 1.23 o superior**. La forma más sencilla de obtener una versión actual y oficial es visitar [go.dev/dl](https://go.dev/dl/), elegir el archivo Linux AMD64 (si tu equipo es x86-64) y seguir las instrucciones de instalación del sitio.

Si prefieres el paquete de tu distribución, puedes usar:

```bash
sudo apt update
sudo apt install golang-go
```

**Comprueba la versión que instala `apt`:** algunas versiones de Ubuntu ofrecen una versión anterior a la requerida. Si es anterior a 1.23, utiliza la distribución oficial de Go de go.dev/dl.

La distribución oficial de Go suele instalarse en `/usr/local/go`; añade su carpeta `bin` a tu `PATH` si no se ha configurado automáticamente:

```bash
export PATH="/usr/local/go/bin:$PATH"
go version
```

Para que este cambio sea permanente, incorpora la exportación a tus archivos de configuración de shell. Si ya gestionas el `PATH` desde tus dotfiles, hazlo ahí.

## 3. Requisitos de LazyBW

Además de Go, necesitas:

1. **Bitwarden CLI** instalado: `bw --version` debe funcionar.
2. Una cuenta Bitwarden y la sesión correspondiente (API key y contraseña maestra, o una sesión de CLI desbloqueada).
3. Una terminal con soporte de colores ANSI y Unicode; Alacritty es adecuada.
4. Preferiblemente una **Nerd Font** (por ejemplo, Lilex Nerd Font Mono), necesaria para los iconos.
5. Acceso a Internet **la primera vez que descargues las dependencias Go** y cuando Bitwarden necesite comunicarse con el servidor.

Si tienes `bw` instalado en Ubuntu/WSL, compila también el ejecutable para Linux/WSL. No uses el ejecutable de Windows para acceder a una instalación de `bw` que solo exista dentro de WSL.

## 4. Abrir el repositorio

Descomprime `LazyBW-Go.zip` y entra en su carpeta:

```bash
cd /ruta/al/proyecto/LazyBW-Go
```

Comprueba la organización:

```text
LazyBW-Go/
├── cmd/lazybw/main.go         # Punto de entrada
├── internal/
│   ├── config/                # JSON/JSONC, colores y keybinds
│   ├── bitwarden/             # Adaptador del CLI bw
│   ├── vault/                 # Items, campos, AES-GCM y staging
│   └── ui/                    # Estado, navegación y gocui
├── go.mod                     # Versión de Go y dependencias
├── README.md
├── ARCHITECTURE.md
└── notes.md                    # Este documento
```

## 5. Descargar dependencias

Desde la raíz del repositorio:

```bash
go mod tidy
```

Esto descarga gocui y sus dependencias, resuelve versiones y genera `go.sum`, que contiene checksums para verificar los módulos descargados. Necesitarás conectividad durante este primer paso.

**Importante:** el ZIP no incluye el código de gocui ni un binario ya compilado. La versión utilizada es `github.com/awesome-gocui/gocui v1.1.0`. Tras descargarla, la compilación podrá reutilizar la caché local de módulos.

## 6. Ejecutar las pruebas

```bash
go test ./...
```

Para ejecutar las pruebas con detección de carreras de datos:

```bash
go test -race ./...
```

Y para comprobar estilos y análisis estático:

```bash
gofmt -l ./cmd ./internal
go vet ./...
```

`gofmt -l` no debería listar archivos si todos están formateados.

## 7. Compilar el ejecutable Linux

```bash
mkdir -p bin
go build -trimpath -ldflags="-s -w" -o bin/lazybw ./cmd/lazybw
```

La opción `-o` indica el nombre del archivo generado; `-trimpath` elimina rutas locales de depuración y `-ldflags="-s -w"` reduce el tamaño omitiendo parte de los símbolos de depuración. Para depurar, puedes usar simplemente:

```bash
go build -o bin/lazybw ./cmd/lazybw
```

Ejecuta el resultado:

```bash
./bin/lazybw
```

O, durante el desarrollo, compila y ejecuta directamente:

```bash
go run ./cmd/lazybw
```

`go run` **también compila**, pero gestiona el ejecutable temporal automáticamente.

## 8. Instalarlo como comando `lazybw`

Puedes copiar el binario a `~/.local/bin`, sin necesidad de permisos de administrador:

```bash
mkdir -p ~/.local/bin
cp bin/lazybw ~/.local/bin/lazybw
chmod +x ~/.local/bin/lazybw
```

Asegúrate de que `~/.local/bin` está en el `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
lazybw
```

Cuando recompiles el proyecto, vuelve a copiar el archivo si quieres actualizar la versión instalada.

Alternativamente, Go permite instalar el binario en su directorio habitual de ejecutables con:

```bash
go install ./cmd/lazybw
```

El destino predeterminado suele ser `$(go env GOPATH)/bin`.

## 9. Configurar LazyBW

En el primer arranque, LazyBW crea:

```text
~/.config/lazybw/settings.jsonc
~/.config/lazybw/keybinds.jsonc
```

Acepta también archivos existentes `.json`, dando prioridad a `.jsonc` cuando están ambos. Admite `//`, `/*...*/` y comas finales.

Para cargar credenciales API desde un fichero de entorno sin introducirlas cada vez, en `settings.jsonc` puedes utilizar:

```jsonc
{
  "authentication": {
    "client_env_path": "$HOME/ruta/privada/bitwarden.env"
  }
}
```

Y el fichero de entorno debería contener (sin comillas angulares):

```bash
export BW_CLIENTID="user.<tu-client-id>"
export BW_CLIENTSECRET="<tu-client-secret>"
```

Protege el fichero:

```bash
chmod 600 "$HOME/ruta/privada/bitwarden.env"
```

LazyBW **lee** estas variables de un fichero; no lo ejecuta como un script. También puedes introducir manualmente las credenciales en la primera pantalla. No introduzcas credenciales reales en el repositorio ni en capturas públicas.

El archivo `keybinds.jsonc` se genera con los mismos grupos del prototipo. La implementación Go reconoce los atajos predeterminados y admite atajos adicionales personalizados para las acciones habituales; algunas combinaciones avanzadas del sistema de widgets original no tienen equivalente directo en gocui.

## 10. Compilar para Windows

Si quieres un ejecutable Windows desde WSL:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/lazybw.exe ./cmd/lazybw
```

El binario Windows requiere que `bw.exe` esté disponible en el `PATH` de Windows. La ejecución nativa en Windows debe verificarse por separado; para tu configuración de Alacritty + WSL, **recomiendo la compilación Linux**.

## 11. Modificar el código y recompilar

El flujo de trabajo habitual sería:

```bash
# Editar archivos .go desde VS Code o Zed
gofmt -w ./cmd ./internal
go test ./...
go build -o bin/lazybw ./cmd/lazybw
./bin/lazybw
```

No hay `pip install`, entorno virtual de Python ni archivos `.pyc`.

## 12. Limitaciones importantes

- Una terminal se dibuja sobre una rejilla de celdas de caracteres: **no existe realmente una altura de 1,5 filas**. Los botones utilizan un medio bloque Unicode para aproximar visualmente esa altura.
- `Ctrl+Enter` puede transmitirse como Enter o como `Ctrl+J` según la terminal. La implementación escucha `Ctrl+J` para insertar un salto de línea en Notes o campos personalizados de texto.
- `Ctrl+S` y `Ctrl+Q` pueden verse afectados por XON/XOFF del terminal. Si no llegan a la aplicación, comprueba `stty -a` y prueba `stty -ixon` antes de abrir LazyBW. No modifiques indiscriminadamente la configuración del terminal.
- La interacción del ratón y sus estados de hover dependen de los eventos que gocui y la terminal entreguen. El clic directo está implementado; el hover continuo sin eventos de ratón puede variar.
- Los cambios no guardados se mantienen **solo en memoria**. La salida normal solicita confirmación; un cierre forzado pierde el staging.
- La sesión utiliza AES-256-GCM para los campos ocultos en las estructuras persistentes. No garantiza borrar todas las copias temporales en la memoria administrada por Go ni impide que el proceso `bw` vea el texto en claro durante sus operaciones.

**Estado de verificación de este paquete:** se ejecutaron pruebas de los paquetes Go de lógica y se comprobó la compatibilidad de tipos de la interfaz con una implementación mínima de las firmas públicas de gocui. El entorno de generación no dispone de acceso de red para descargar la dependencia real, por lo que **no se ha compilado ni probado el renderizado interactivo contra la biblioteca gocui auténtica**. El primer `go mod tidy && go test ./... && go build ...` en tu equipo constituye la validación indispensable antes de considerar esta versión apta para uso habitual.
