# Recolector y Reenviador de Ofensas de QRadar

Un microservicio de grado de producción escrito en Go que actúa como puente entre IBM QRadar SIEM y una API de ingesta externa.

## Arquitectura

1. **Consulta QRadar SIEM**: Consulta periódicamente el endpoint `GET /siem/offenses` para buscar ofensas nuevas o actualizadas desde la última consulta exitosa.
2. **Enriquecimiento de Eventos**: Para cada ofensa activa, genera una búsqueda AQL asíncrona a través de `POST /ariel/searches` para recuperar los detalles asociados del evento, consultando hasta que se complete.
3. **Transformación de Datos**: Mapea las respuestas de la API de QRadar en un esquema JSON personalizado predefinido.
4. **Reenvío**: Envía el payload JSON estructurado a la API de destino externa mediante solicitudes `POST`.
5. **Gestión de Estado**: Rastrea persistentemente la última marca de tiempo procesada en `state.json` para garantizar la capacidad de reanudación precisa después de reinicios.

## Características

- Procesamiento de eventos altamente concurrente utilizando un patrón de grupo de trabajadores (worker pool).
- Cliente HTTP robusto con agrupación de conexiones (connection pooling) y tiempos de espera (timeouts).
- Lógica de reintento con retroceso exponencial (exponential backoff) para resiliencia frente a fallas transitorias de QRadar o la API de destino.
- Cero dependencias fuera de `go.uber.org/zap` (registro) y `gopkg.in/yaml.v3` (configuración).
- Eficiente en memoria (RAM `< 50MB`) e imagen de contenedor ligera (`~15MB`).

---

## Configuración

La configuración se proporciona de forma nativa a través de un archivo `config.yaml`, pero todos los valores se pueden sobrescribir de forma segura mediante variables de entorno:

| Variable de Entorno     | Descripción                                                                            |
| ----------------------- | -------------------------------------------------------------------------------------- |
| `QRADAR_BASE_URL`       | URL base de la API de QRadar (ej., `https://qradar.company.local/api`)                 |
| `QRADAR_API_TOKEN`      | Token de Servicio Autorizado (SEC)                                                     |
| `QRADAR_VERSION`        | Versión de API objetivo (por defecto: `20.0`)                                          |
| `QRADAR_TLS_INSECURE`   | Establécelo en `true` para omitir la validación del certificado (por defecto: `false`) |
| `DESTINATION_URL`       | Endpoint de la API de ingesta externa                                                  |
| `DESTINATION_API_KEY`   | Clave de API enviada en el encabezado `x-api-key`                                      |
| `POLL_INTERVAL_SECONDS` | Con qué frecuencia verificar ofensas (por defecto: `60`)                               |
| `STATE_FILE`            | Archivo local para persistir la marca de tiempo (por defecto: `./state.json`)          |
| `LOG_LEVEL`             | Nivel de registro: `debug`, `info`, `warn`, `error`                                    |

---

## Inicio Rápido y Despliegue en Producción (Docker Compose)

El método de despliegue recomendado y totalmente compatible para este recolector es a través de **Docker Compose**. Esto garantiza un aislamiento de entorno perfecto, reinicios automáticos y cero dependencias del host.

### Prerrequisitos

- Motor de Docker instalado en el servidor objetivo.
- Docker Compose v2 instalado.

### Guía Paso a Paso

1. **Clonar o transferir el proyecto** a tu servidor Ubuntu:

   ```bash
   git clone https://github.com/zerok06/agentes-ai-soc-collector.git /opt/qradar-collector
   cd /opt/qradar-collector
   ```

2. **Configurar tus Secretos (`.env`)**
   Copia el archivo de entorno de ejemplo y completa tus tokens de producción reales.
   ```bash
   cp .env.example .env
   nano .env
   ```
   ```bash
   # Copiar archivo .env
   QRADAR_BASE_URL=
   QRADAR_API_TOKEN=
   QRADAR_VERSION=20.0
   QRADAR_TLS_INSECURE=false
   DESTINATION_URL=https://soc-ingest-api-269253729593.us-central1.run.app/api/v1/ingest/offense
   DESTINATION_API_KEY=Qradar2025agentsAi
   POLL_INTERVAL_SECONDS=60
   HTTP_TIMEOUT_SECONDS=30
   WORKER_COUNT=5
   LOG_LEVEL=info
   ```

_Nota: Nunca confirmes (commit) `.env` en el control de código fuente._

3. **Iniciar el Recolector en Segundo Plano**
   Construye la imagen ligera basada en Alpine e inicia el contenedor en modo desconectado (detached):

```bash
docker compose up -d --build
```

_La bandera `--build` garantiza que el binario de Go sea compilado nuevamente._

4. **Verificar el Despliegue**
   Revisa los registros para asegurarte de que se conectó exitosamente a QRadar y cargó el estado:

   ```bash
   docker compose logs -f
   ```

   _(Presiona `Ctrl+C` para salir de la vista de registros)_

5. **Gestión del Servicio**
   - **Detener el recolector:** `docker compose down`
   - **Reiniciar el recolector:** `docker compose restart`
   - **Verificar estado:** `docker compose ps`

### Solución de Problemas y Registros

Todos los registros JSON estructurados y errores de conexión se envían a `stdout` y son capturados por Docker. Puedes filtrar los registros nativamente:

```bash
docker compose logs --tail=100 -f
```
