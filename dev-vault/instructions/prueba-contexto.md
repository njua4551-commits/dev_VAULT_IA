# Prueba de contexto grande (Qwen3.8-27B en 2xT4)

Objetivo: saber cuanto contexto aguanta de verdad la GPU y a que velocidad. Todo esto esta SIN PROBAR en tu Kaggle.

## Que cambia el notebook
- Prueba contextos de 131072, 98304, 65536 y 32768 (baja solo al siguiente si no carga).
- Cache de contexto comprimida: `-ctk q8_0 -ctv q8_0 -fa on -np 1`.
- Imprime `nvidia-smi` al terminar de cargar (fase `vram`).
- Ajustes arriba del notebook (celda 1): `KV_TYPE` ("q8_0", "f16", "q4_0") y `KV_ON_CPU` (True = cache en RAM, mas lento).

## Pasos
1. `python client/vault.py up` (el notebook nuevo se sube solo).
2. Cuando diga IA lista, mira en `python client/vault.py logs` la linea `LISTO -> {... 'ctx': N}` y la tabla de nvidia-smi.
3. Anota: ctx que cargo, memory.used / memory.total de las dos GPUs.
4. Pide en el chat: `escribe una funcion en Python que ordene una lista` y cronometra los segundos.
5. Pega un texto largo (~50.000 palabras no; unas 20 paginas) y pide un resumen: anota si responde y cuanto tarda.
6. `python client/vault.py down`.

## Que hacer con el resultado
- Sobra VRAM (mas de ~4 GB libres por GPU): en la celda 1 pon 196608 o 262144 de primero y repite.
- Cargo pero va lento con contexto largo: baja un escalon o prueba `KV_TYPE = "q4_0"`.
- No cargo el de 131072 y bajo a 98304 o menos: pasame las lineas del log antes del fallo. Ultimo recurso: `KV_ON_CPU = True` (mide la velocidad).
- Datos a enviarme: ctx final, nvidia-smi, segundos de la prueba corta y de la larga. Nunca envies tu API key ni authtoken.

## Resultados medidos (2026-11-11, Qwen3.8-27B Q4_K_M, KV q8_0, -fa on, -np 1)
- 131072: carga directa, ~2,3 + ~2,5 GiB de VRAM libre por GPU.
- 196608: carga directa, 0,74 + 0,92 GiB libres. Prueba larga: 14.668 tokens de entrada en 60,3 s.
- Costo de contexto: ~50 KiB por token con q8_0 (calculado de dos nvidia-smi).
- 262144 no cabe con q8_0 (haría falta KV q4_0, sin probar).
- Recomendado: 131072 por defecto; LONG_MODE = True solo para pruebas.
- Pendiente: prueba de estrés con ~100K tokens y medir velocidad de lectura/escritura.
