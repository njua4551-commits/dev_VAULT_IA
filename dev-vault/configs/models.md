# Modelos para Kaggle (llama.cpp)

El notebook `kaggle/notebooks/run-qwen-llamacpp.ipynb` usa la lista `ATTEMPTS` de su primera celda. Por defecto:

| Orden | Modelo | Contexto | Repos GGUF que prueba |
|---|---|---|---|
| 1 | Qwen3.8-27B | 65536 | `ggml-org/Qwen3.8-27B-GGUF` (verificado), `unsloth/...`, `bartowski/...` |
| 2 | Qwen3.8-27B | 32768 | igual |
| 3 | Qwen3.6-27B | 65536 | `ggml-org/Qwen3.6-27B-GGUF` (no verificado), `unsloth/...`, `bartowski/Qwen_Qwen3.6-27B-GGUF` (verificado, sin mmproj) |
| 4 | Qwen3.6-27B | 32768 | igual |

Los repos marcados como "verificado" existen y se listaron; los otros se prueban en orden y se descartan solos si no existen. Los nombres de archivo no estan fijos: el notebook los descubre y elige la cuantizacion y el `mmproj` (vision).

## Por que Qwen3.8-27B
Cifras reportadas por Qwen (sin reproducciones independientes encontradas):

| Benchmark | Qwen3.6-27B | Qwen3.8-27B |
|---|---|---|
| SWE-bench Pro | 53.5 | 61.7 |
| Terminal-Bench 2.1 | 63.4 | 73.0 |
| LiveCodeBench v6 | 83.9 | 90.3 |

Acepta imagenes, licencia Apache 2.0, contexto nativo de 262K. Los pesos oficiales son BF16/FP8; los GGUF cuantizados son conversiones (ggml-org publica Q4_K_M de 19 GB y Q8_0 de 28.6 GB, con `mmproj`).

## Memoria (2 x T4 = 32 GB de VRAM)
- Q4_K_M ~19 GB + mmproj ~0.6-0.9 GB.
- Cache KV: ~16 GiB a 262K tokens (dato de un analisis externo) -> orden de ~4 GiB a 64K. Estimacion, no medicion.
- Disco: el modelo (~19 GB) no cabe holgado en `/kaggle/working` (~20 GB); el notebook lo descarga a la carpeta con mas espacio libre.

## Otras opciones evaluadas
| Modelo | Nota |
|---|---|
| Gemma 4 31B | Cabe en 2xT4. En una comparacion contra Qwen3.6-27B rinde algo peor en codigo y agentes y levemente mejor en un benchmark de vision. |
| Qwen3-VL-8B | Para pruebas rapidas (GGUF oficial, ~5 GB). Mucho menos capaz para agente. |
| Qwen3.5-27B | Superado por 3.6 y 3.8. |
