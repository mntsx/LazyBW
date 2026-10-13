# Revisión arquitectónica del prototipo y reorganización Go

## Diagnóstico del prototipo

Se tomó como referencia `LazyBW-jsonc-config.zip` (última versión Python de la conversación). Su `screens.py` concentraba múltiples responsabilidades: inicialización, movimiento de foco, autenticación, listas, estilos activos, edición, sincronización visual, diálogos y envío de cambios. Esto dificultaba razonar sobre regresiones de selección o de staging. Además, el renderizado de widgets Textual implicaba actualizaciones y reconstrucciones frecuentes para cambios triviales.

Otras oportunidades de simplificación detectadas: código de teclado de pantallas similares duplicado, UI acoplada a los comandos `bw`, popups con varias implementaciones de botones, estado de selección mezclado con estado aplicado, y valores de configuración predeterminados dispersos.

## Estructura de la implementación

| Paquete | Responsabilidad | Dependencias principales |
|---|---|---|
| `internal/config` | JSONC, defaults y keybinds | Biblioteca estándar |
| `internal/vault` | Modelo de items, campos, rutas, cifrado, búsqueda y staging | Biblioteca estándar |
| `internal/bitwarden` | `bw` como subproceso, sesión y persistencia remota | `internal/vault` |
| `internal/ui` | Estado, eventos de teclado/ratón, estilo, layout, popups | gocui, dominio y cliente |
| `cmd/lazybw` | Inicializar y ejecutar | Los paquetes internos |

Esto permite probar el comportamiento del dominio y el cliente CLI sin abrir una terminal interactiva.

## Correspondencia aproximada con Python

| Python | Go |
|---|---|
| `models.py` | `vault/item.go` |
| `secret_store.py` | `vault/secret.go` |
| `staging.py` | `vault/staging.go` |
| `bw.py`, `bw_stage.py` | `bitwarden/client.go` |
| `config.py` | `config/config.go` |
| `search.py` | `vault/staging.go` (`FuzzyMatch`) |
| `screens.py`, `widgets.py`, `styles.tcss` | `ui/state.go`, `ui/render.go`, `ui/app.go`, `ui/keys.go`, `ui/theme.go` |
| `dialogs.py`, `change_dialog.py` | `ui/app.go` (`Popup` y formularios) |
| `clipboard.py` | `ui/app.go` (clipboard por stdin) |
| `app.py` | `cmd/lazybw/main.go`, `ui/app.go` |

## Reglas de comportamiento conservadas

- Autenticación CLI con API key y contraseña maestra, con posibilidad de reutilizar la sesión existente.
- Cargar el vault sin un `bw sync` forzoso; leer items y folders en paralelo.
- Cifrar en memoria los campos ocultos, con descifrado bajo demanda.
- Búsqueda difusa **solo** por el nombre, con coincidencias visuales.
- Filtros de vault, tipo y carpeta; diferenciación entre selección de teclado y filtro aplicado.
- Operaciones `N/M/D` consolidadas por objeto, con guardado explícito por elemento o en lote.
- Confirmación de acciones destructivas y confirmación al salir con cambios pendientes.
- Edición local de un campo y guardado posterior al servidor.
- Mantener selección, marcadores y navegación de los cinco paneles numerados.

## Cambios de implementación

1. **Renderizado**: gocui recibe un buffer de celdas coloreadas que constituye el lienzo completo; no se montan/desmontan cientos de widgets para reconstruir listas.
2. **Procesos de Bitwarden**: `os/exec` sin shell; la sesión y las credenciales viajan por el entorno del subproceso. Los JSON codificados para `bw create/edit` viajan por **stdin**, no en el argumento visible del proceso.
3. **Cifrado**: AES-256-GCM con nonce aleatorio y clave de sesión. Para comparar secretos que se han recifrado con nonces diferentes, se utiliza un tag de igualdad HMAC separado, sin descifrar las copias persistentes.
4. **Staging**: el modelo conserva snapshots originales independientes de la UI y las transiciones se prueban sin gocui.
5. **Diseño**: títulos de fondo sólido azul oscuro, buscador con fondo sólido, separadores contiguos y franjas de selección de dos columnas.
6. **Botones**: se simula una altura de 1,5 filas con un medio bloque Unicode, sin intentar alterar la geometría de las celdas del terminal.

## Pruebas migradas

Las pruebas Python dependientes de nombres de clases Textual, CSS y declaraciones internas no pueden trasladarse literalmente: esas implementaciones ya no existen. Las pruebas Go se centran en los invariantes funcionales: validación JSONC y precedencia, cifrado y serialización, campos, edición y cursores, filtros y ordenación, búsqueda, staging y descarte, creación y guardado por CLI simulado, confirmaciones y colores renderizados. Revisa `*_test.go` en cada paquete.

## Diferencias y validación pendiente

La primera entrega de Go no afirma paridad visual píxel a píxel con Textual. Los siguientes detalles requieren comprobación y eventualmente ajuste en una terminal real: compatibilidad de secuencias de teclas no estándar, recepción continua de movimiento de ratón, tamaño efectivo de vistas y colores truecolor con la biblioteca gocui real, sesiones ya desbloqueadas por otras aplicaciones y guardado contra la cuenta auténtica de Bitwarden.

Las personalizaciones de `keybinds.jsonc` se tratan actualmente como **atajos adicionales** para las acciones habituales; los atajos originales permanecen habilitados como fallback. Las acciones no reconocidas no se reinterpretan arbitrariamente.

**No debe utilizarse el resultado como gestor de contraseñas de producción sin probarlo en el terminal y verificar las operaciones de creación/edición/borrado con una cuenta de pruebas.**
