# LazyBW — Go + gocui

Port del prototipo Python/Textual de LazyBW a Go con gocui. Su objetivo es conservar el flujo funcional del prototipo y simplificar la TUI mediante un renderer de celdas, evitando reconstrucciones costosas de widgets.

**[Guía de instalación y compilación: notes.md](notes.md)** · **[Revisión arquitectónica: ARCHITECTURE.md](ARCHITECTURE.md)**

## Funciones

- Autenticación Bitwarden CLI (API credentials y contraseña maestra); carga opcional desde `.env`.
- JSON y JSONC para `settings` y `keybinds`, creando JSONC por defecto.
- Paneles `[1] Vault`, `[2] Items`, `[3] Folders`, `[4] Changes`, `[5] Name / Owner`.
- Paneles colindantes con títulos sólidos y separadores azules; selección cian y estado aplicado/abierto verde brillante.
- Navegación por teclado, clic sobre filas, búsqueda difusa solo por nombre y texto coincidente resaltado.
- Contenido por campo, revelado selectivo y editor de campos con cursor de barra. Details aparece únicamente durante la edición.
- Crear/eliminar items y carpetas, crear/eliminar campos personalizados y copiar valores al portapapeles cuando hay una herramienta compatible.
- Cola de staging `N/M/D`; guardar/descartar uno o todos los cambios con confirmación; spinner de estado y salida segura.
- Campos ocultos cifrados en memoria con AES-256-GCM, sin consulta adicional al revelarlos.

## Arranque rápido (WSL)

```bash
# Requiere Go >= 1.23 y bw en PATH
go mod tidy
go test ./...
mkdir -p bin
go build -o bin/lazybw ./cmd/lazybw
./bin/lazybw
```

## Teclado principal

| Atajo | Acción |
|---|---|
| `1`–`5` | Cambiar de panel |
| `↑` / `↓` | Mover selección |
| `Enter` / `→` | Abrir o aplicar selección |
| `←` / `Esc` | Volver/cerrar editor |
| `f` | Buscar por nombre |
| `n` | Crear item/carpeta/campo según panel |
| `d` | Eliminar o descartar según panel |
| `e` | Editar campo seleccionado |
| `r` | Revelar únicamente el campo oculto seleccionado |
| `c` | Copiar campo |
| `s` | Guardar cambio seleccionado en Changes |
| `ctrl+s` | Guardar todos los cambios |
| `ctrl+d` | Descartar todos los cambios |
| `ctrl+q` | Salir, solicitando confirmación si hay pendientes |
| `?` | Paginar hints o listado largo de confirmación |
| `Tab` | Cambiar tipo de campo al crear un campo personalizado |
| `ctrl+j` (`ctrl+enter` si la terminal lo envía así) | Salto de línea al editar Notes o campo de texto multilineal |

La presentación y la capacidad de detectar algunas teclas dependen del terminal. Los cambios pendientes no se almacenan en disco.

## Estado

Esta entrega incluye código fuente, pruebas y documentación de compilación. Las pruebas locales del núcleo y una comprobación de tipos con firmas de gocui han pasado, pero la dependencia gocui real y la interfaz interactiva **no han podido ejecutarse en el entorno donde se preparó el archivo**. La aplicación necesita validación de interacción y guardado real antes de un uso habitual con contraseñas de producción. Consulta `notes.md` y `ARCHITECTURE.md`.
