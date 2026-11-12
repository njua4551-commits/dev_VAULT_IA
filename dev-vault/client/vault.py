#!/usr/bin/env python3
"""
vault - controla tu IA de Kaggle desde la terminal (version gratuita).

  python vault.py init --user TU_USUARIO_KAGGLE   # una sola vez
  python vault.py up                              # enciende la IA y espera la URL
  python vault.py chat                            # chatea desde la terminal
  python vault.py down                            # apaga y deja de gastar GPU

Solo usa la libreria estandar de Python + el paquete `kaggle` (pip install kaggle).
Lee la URL del tunel desde los logs en vivo del notebook (kaggle kernels logs -f).
"""
import argparse
import base64
import hashlib
import hmac
import http.server
import json
import mimetypes
import os
import queue
import re
import secrets
import shutil
import shlex
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import webbrowser
from pathlib import Path

HOME = Path(os.environ.get("VAULT_HOME") or Path.home() / ".dev-vault")
CFG_FILE = HOME / "config.json"
STATE_FILE = HOME / "state.json"
PUSH_DIR = HOME / "kaggle-notebook"
DATA_DIR = HOME / "kaggle-config-dataset"
DEFAULT_NB = Path(__file__).resolve().parent.parent / "kaggle" / "notebooks" / "run-qwen-llamacpp.ipynb"
NB_NAME = "run-qwen-llamacpp.ipynb"
DATASET_SLUG = "vault-config"
BINARY_SLUG = "llama-bin-t4"
NTFY_BASE = os.environ.get("VAULT_NTFY_BASE", "https://ntfy.sh")
KAGGLE_CMD = shlex.split(os.environ["VAULT_KAGGLE_CMD"]) if os.environ.get("VAULT_KAGGLE_CMD") \
    else [sys.executable, "-m", "kaggle"]
POLL = float(os.environ.get("VAULT_POLL", 10))   # segundos entre consultas de estado
HTTP_HEADERS = {"User-Agent": "dev-vault-client/1.0", "ngrok-skip-browser-warning": "1"}

RE_URL = re.compile(r"VAULT_URL=(https?://[^\s\"'\\]+)")
RE_URLS = re.compile(r"VAULT_URL_([A-Z]+)=(https?://[^\s\"'\\]+)")
PRIORITY = ["ngrok", "tailscale", "cloudflared"]      # orden de preferencia (los dos primeros soportan streaming)
NO_STREAM = {"cloudflared"}                           # Cloudflare Quick Tunnel no soporta SSE
RETRY_CODES = {402, 403, 429, 500, 502, 503, 504}    # errores del tunel/cupo: probar otro tunel
RE_MODEL = re.compile(r"VAULT_MODEL=([^\s\"'\\]+)")
RE_PHASE = re.compile(r"\[VAULT\]\s*(.+)")
RE_STATUS = re.compile(r"\b(QUEUED|RUNNING|COMPLETE|ERROR|CANCEL_REQUESTED|CANCEL_ACKNOWLEDGED|NEW_SCRIPT)\b")
ACTIVE = ("QUEUED", "RUNNING")
FINISHED = ("COMPLETE", "ERROR", "CANCEL_ACKNOWLEDGED")


# ----------------------------------------------------------------- utilidades
def say(msg=""):
    print(msg, flush=True)


def die(msg, code=1):
    print(f"\nERROR: {msg}", file=sys.stderr, flush=True)
    sys.exit(code)


def load_cfg():
    if not CFG_FILE.exists():
        die("Aun no hay configuracion. Primero ejecuta:\n  python vault.py init --user TU_USUARIO_KAGGLE")
    return json.loads(CFG_FILE.read_text(encoding="utf-8"))


def save_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")
    try:
        os.chmod(path, 0o600)
    except Exception:
        pass


def load_state():
    try:
        return json.loads(STATE_FILE.read_text(encoding="utf-8"))
    except Exception:
        return {}


def ref(cfg):
    return f"{cfg['user']}/{cfg['slug']}"


def kaggle(*args):
    try:
        return subprocess.run(KAGGLE_CMD + list(args), capture_output=True, text=True,
                              encoding="utf-8", errors="replace")
    except FileNotFoundError:
        die("No se pudo ejecutar Kaggle. Instala el paquete:  pip install kaggle")


def check_auth():
    r = kaggle("kernels", "list", "--page-size", "1")
    out = (r.stdout + r.stderr)
    if "No module named kaggle" in out:
        die("Falta el paquete de Kaggle. Instala:  pip install kaggle")
    if r.returncode != 0 and "uthentication" in out:
        die("Kaggle no tiene tus credenciales. Ejecuta:\n  python -m kaggle auth login\n"
            "(o crea un token en https://www.kaggle.com/settings/api y sigue las instrucciones que muestra).")


def kernel_status(cfg):
    r = kaggle("kernels", "status", ref(cfg))
    out = (r.stdout + r.stderr).strip()
    m = RE_STATUS.search(out)
    return (m.group(1) if m else None), out


def http_get(url, headers=None, timeout=15):
    req = urllib.request.Request(url, headers={**HTTP_HEADERS, **(headers or {})})
    return urllib.request.urlopen(req, timeout=timeout)


def url_alive(url, tries=6, pause=5):
    for i in range(tries):
        try:
            if http_get(url.rstrip("/") + "/health", timeout=10).status == 200:
                return True
        except Exception:
            pass
        if i < tries - 1:
            time.sleep(pause)
    return False


def ordered(urls, first=None):
    """[(nombre, url)] por prioridad; `first` (nombre) va primero si existe."""
    def key(n):
        return PRIORITY.index(n) if n in PRIORITY else len(PRIORITY)
    names = sorted(urls, key=key)
    if first in urls:
        names.remove(first)
        names.insert(0, first)
    return [(n, urls[n]) for n in names]


def pick_alive(urls, first=None, tries=2, pause=3):
    for n, u in ordered(urls, first):
        if url_alive(u, tries=tries, pause=pause):
            return n, u
    return None, None


def state_urls(state):
    urls = dict(state.get("urls") or {})
    if state.get("url") and state["url"] not in urls.values():
        urls["principal"] = state["url"]
    return urls


def name_of(state, url):
    for n, u in state_urls(state).items():
        if u == url:
            return n
    return "principal"


def has_binary(cfg):
    """Acepta tanto binary_dataset (este cliente) como bin_dataset (versiones anteriores)."""
    return bool(cfg.get("binary_dataset") or cfg.get("bin_dataset"))


def binary_slug(cfg):
    if cfg.get("binary_slug"):
        return cfg["binary_slug"]
    return "llama-bin" if cfg.get("bin_dataset") and not cfg.get("binary_dataset") else BINARY_SLUG


def stop_signature(api_key):
    return hmac.new(api_key.encode(), b"vault-stop", hashlib.sha256).hexdigest()


# ------------------------------------------- archivos para Kaggle (notebook + config)
def write_push_files(cfg):
    nb_src = Path(cfg.get("notebook") or DEFAULT_NB)
    if not nb_src.exists():
        die(f"No encuentro el notebook: {nb_src}\nUsa  --notebook RUTA  en el comando init.")
    PUSH_DIR.mkdir(parents=True, exist_ok=True)
    (PUSH_DIR / NB_NAME).write_bytes(nb_src.read_bytes())
    meta = {
        "id": ref(cfg),
        "title": cfg["slug"].replace("-", " ").title(),
        "code_file": NB_NAME,
        "language": "python",
        "kernel_type": "notebook",
        "is_private": "true",
        "enable_gpu": "true",
        "enable_tpu": "false",
        "enable_internet": "true",
        "machine_shape": "",
        "dataset_sources": [f"{cfg['user']}/{DATASET_SLUG}"] + ([f"{cfg['user']}/{binary_slug(cfg)}"] if has_binary(cfg) else []),
        "competition_sources": [],
        "kernel_sources": [],
        "model_sources": [],
    }
    save_json(PUSH_DIR / "kernel-metadata.json", meta)


def write_dataset_files(cfg):
    payload = {
        "API_KEY": cfg["api_key"],
        "NTFY_TOPIC": cfg["ntfy_topic"],
        "TUNNEL": cfg.get("tunnel", "auto"),
        "IDLE_MINUTES": cfg.get("idle_minutes", 30),
        "MAX_HOURS": cfg.get("max_hours", 8),
    }
    if cfg.get("ngrok_token"):
        payload["NGROK_AUTHTOKEN"] = cfg["ngrok_token"]
    if cfg.get("ngrok_domain"):
        payload["NGROK_DOMAIN"] = cfg["ngrok_domain"]
    if cfg.get("ts_authkey"):
        payload["TS_AUTHKEY"] = cfg["ts_authkey"]
        payload["TS_HOSTNAME"] = cfg.get("ts_hostname") or "kaggle-ia"
        payload["TS_MODE"] = cfg.get("ts_mode") or "funnel"
    payload["CLOUDFLARED_ALSO"] = "false" if cfg.get("no_cloudflared") else "true"
    save_json(DATA_DIR / "vault-config.json", payload)
    save_json(DATA_DIR / "dataset-metadata.json", {
        "title": DATASET_SLUG,
        "id": f"{cfg['user']}/{DATASET_SLUG}",
        "licenses": [{"name": "CC0-1.0"}],
    })


def publish_config(cfg):
    """Sube vault-config.json como Dataset PRIVADO (Kaggle crea los datasets privados por defecto)."""
    write_dataset_files(cfg)
    if cfg.get("dataset_created"):
        r = kaggle("datasets", "version", "-p", str(DATA_DIR), "-m", "actualiza configuracion", "-q")
    else:
        r = kaggle("datasets", "create", "-p", str(DATA_DIR), "-q")
    out = (r.stdout + r.stderr).strip()
    if r.returncode != 0:
        die("No pude subir la configuracion a Kaggle:\n" + out)
    cfg["dataset_created"] = True
    save_json(CFG_FILE, cfg)
    say("Configuracion subida como Dataset privado: " + f"{cfg['user']}/{DATASET_SLUG}")
    say("(Kaggle tarda ~1 minuto en procesarlo; `up` espera lo necesario.)")


# ------------------------------------------------------------------- comandos
def cmd_init(args):
    HOME.mkdir(parents=True, exist_ok=True)
    cfg = json.loads(CFG_FILE.read_text(encoding="utf-8")) if CFG_FILE.exists() else {}
    cfg["user"] = args.user or cfg.get("user") or die("Falta --user TU_USUARIO_KAGGLE")
    cfg["slug"] = args.slug or cfg.get("slug") or "dev-vault-qwen"
    if args.reset_key or not cfg.get("api_key"):
        cfg["api_key"] = secrets.token_urlsafe(36)
    cfg.setdefault("ntfy_topic", "vault-" + secrets.token_urlsafe(18).replace("_", "x").replace("-", "y"))
    cfg.setdefault("tunnel", "auto")
    cfg["idle_minutes"] = args.idle_minutes if args.idle_minutes is not None else cfg.get("idle_minutes", 30)
    cfg["max_hours"] = args.max_hours if args.max_hours is not None else cfg.get("max_hours", 8)
    if args.notebook:
        cfg["notebook"] = str(Path(args.notebook).resolve())

    token = args.ngrok_token if args.ngrok_token is not None else cfg.get("ngrok_token", "")
    domain = args.ngrok_domain if args.ngrok_domain is not None else cfg.get("ngrok_domain", "")
    if sys.stdin.isatty():
        if not token:
            token = input("NGROK_AUTHTOKEN (dashboard.ngrok.com/get-started/your-authtoken; Enter para omitir): ").strip()
        if token and not domain:
            domain = input("Dominio fijo gratis de ngrok (ej. mi-ia.ngrok-free.dev; Enter para omitir): ").strip()
    cfg["ngrok_token"], cfg["ngrok_domain"] = token, domain.replace("https://", "").strip("/ ")

    ts_key = args.ts_authkey if args.ts_authkey is not None else cfg.get("ts_authkey", "")
    if sys.stdin.isatty() and not ts_key and args.ts_authkey is None:
        ts_key = input("Tailscale auth key (opcional, 2o tunel gratis; ver guia; Enter para omitir): ").strip()
    cfg["ts_authkey"] = ts_key
    if args.ts_hostname:
        cfg["ts_hostname"] = args.ts_hostname
    if args.ts_mode:
        cfg["ts_mode"] = args.ts_mode
    if args.no_cloudflared is not None:
        cfg["no_cloudflared"] = args.no_cloudflared
    save_json(CFG_FILE, cfg)

    write_push_files(cfg)
    say(f"Configuracion guardada en {CFG_FILE}")
    tuneles = [n for n, ok in (("ngrok", cfg["ngrok_token"]), ("tailscale", cfg["ts_authkey"]),
                               ("cloudflared", not cfg.get("no_cloudflared"))) if ok]
    say("Tuneles que se abriran a la vez: " + ", ".join(tuneles or ["ninguno"]))
    if not cfg["ngrok_token"] and not cfg["ts_authkey"]:
        say("AVISO: solo Cloudflare Quick Tunnel: NO soporta streaming (SSE) y la URL cambia en cada sesion. "
            "Recomendado: crea la cuenta gratis de ngrok y/o Tailscale y vuelve a ejecutar init.")
    check_auth()
    if not args.no_publish:
        publish_config(cfg)
    say("\nListo. Siguiente paso:  python vault.py up")
    say(f"Tu API key (para la web): ejecuta  python vault.py key")


def cmd_key(args):
    say(load_cfg()["api_key"])


def wait_for_url(cfg, timeout_s):
    """Lee los logs en vivo del notebook hasta ver VAULT_URL y comprueba que el servidor responde."""
    deadline = time.time() + timeout_s
    model, shown, urls = None, set(), {}
    while time.time() < deadline:
        p = subprocess.Popen(KAGGLE_CMD + ["kernels", "logs", "-f", ref(cfg)], stdout=subprocess.PIPE,
                             stderr=subprocess.STDOUT, text=True, encoding="utf-8", errors="replace", bufsize=1)
        q = queue.Queue()

        def reader(proc=p, out=q):
            for line in proc.stdout:
                out.put(line.rstrip("\n"))
            out.put(None)

        threading.Thread(target=reader, daemon=True).start()
        try:
            while time.time() < deadline:
                try:
                    line = q.get(timeout=5)
                except queue.Empty:
                    continue
                if line is None:
                    break
                m = RE_PHASE.search(line)
                if m and m.group(1) not in shown:
                    shown.add(m.group(1))
                    say("  " + m.group(1))
                m = RE_MODEL.search(line)
                if m:
                    model = m.group(1)
                m = RE_URLS.search(line)
                if m:
                    urls[m.group(1).lower()] = m.group(2)
                m = RE_URL.search(line)
                if m:                      # VAULT_URL= se imprime despues de todos los VAULT_URL_<TUNEL>=
                    found = dict(urls) or {"principal": m.group(1)}
                    n, u = pick_alive(found, tries=4)
                    if u:
                        return found, model
        finally:
            if p.poll() is None:
                p.terminate()
        st, _ = kernel_status(cfg)
        if st in FINISHED:
            return None, model
        time.sleep(POLL)
    return None, model


def save_state(urls, model, current=None):
    n, u = (current, urls.get(current)) if current in urls else (ordered(urls)[0] if urls else (None, None))
    save_json(STATE_FILE, {"url": u, "urls": urls, "model": model, "time": time.time()})
    return n, u


def cmd_up(args):
    cfg = load_cfg()
    check_auth()
    st, _ = kernel_status(cfg)
    if st in ACTIVE:
        say(f"El notebook ya esta encendido ({st}). Busco la URL...")
    else:
        write_push_files(cfg)
        if args.sync_config:
            publish_config(cfg)
        timeout = int(float(cfg.get("max_hours", 8)) * 3600 + 3600)
        say("Encendiendo (GPU T4 x2, Internet On)...")
        r = kaggle("kernels", "push", "-p", str(PUSH_DIR), "--accelerator", "NvidiaTeslaT4", "-t", str(timeout))
        out = (r.stdout + r.stderr).strip()
        say(out)
        if r.returncode != 0 or re.search(r"\b(error|invalid)\b", out, re.I) and "successfully" not in out.lower():
            die("Kaggle rechazo el push. Revisa el mensaje de arriba (Dataset aun procesandose? cuenta sin telefono verificado?).")
        for _ in range(int(180 / POLL)):         # espera a que la nueva ejecucion aparezca
            time.sleep(POLL)
            st, _ = kernel_status(cfg)
            if st in ACTIVE:
                break
        say(f"Estado: {st or 'desconocido'}")
    if args.no_wait:
        return
    say("Esperando a que el modelo cargue y el tunel abra (la primera vez: compilacion ~25 min + descarga ~19 GB)...")
    urls, model = wait_for_url(cfg, int(args.timeout * 60))
    if not urls:
        die("No obtuve la URL. Mira los logs:  python vault.py logs   o la pagina del notebook en Kaggle.")
    model = model or "Qwen3.8-27B"
    best, url = pick_alive(urls, tries=2)
    save_state(urls, model, best)
    say("\n" + "=" * 60)
    say(f"  IA lista: {model}")
    say(f"  Web  : {url}   (pega tu API key; ver con: python vault.py key)")
    say(f"  API  : {url}/v1")
    if len(urls) > 1:
        say("  Tuneles de respaldo (el cliente cambia solo si uno falla):")
        for n, u in ordered(urls, best):
            say(f"    - {n}: {u}")
    say("=" * 60)
    say("Siguiente: python vault.py chat   |   python vault.py web   |   python vault.py down")


def cmd_url(args):
    cfg = load_cfg()
    check_auth()
    st, _ = kernel_status(cfg)
    if st not in ACTIVE:
        die(f"La IA esta apagada (estado: {st or 'sin ejecuciones'}). Enciendela con: python vault.py up")
    state = load_state()
    if state_urls(state):
        n, u = pick_alive(state_urls(state), first=name_of(state, state.get("url")), tries=2)
        if u:
            save_state(state_urls(state), state.get("model", "Qwen3.8-27B"), n)
            say(u)
            return
    urls, model = wait_for_url(cfg, 60)
    if not urls:
        die("Esta encendida pero aun no veo la URL (puede seguir cargando). Prueba: python vault.py logs")
    n, u = save_state(urls, model or state.get("model", "Qwen3.8-27B"), pick_alive(urls, tries=2)[0])
    say(u)


def cmd_urls(args):
    cfg, state = need_state()
    cur = state.get("url")
    for n, u in ordered(state_urls(state), name_of(state, cur)):
        ok = url_alive(u, tries=1)
        say(f"{'*' if u == cur else ' '} {n:<12} {'VIVO ' if ok else 'caido'}  {u}"
            + ("   (sin streaming)" if n in NO_STREAM else ""))
    say("(* = el que usa el cliente ahora)")


def cmd_status(args):
    cfg = load_cfg()
    check_auth()
    st, raw = kernel_status(cfg)
    say(f"Notebook {ref(cfg)}: {st or 'sin ejecuciones / no existe'}")
    state = load_state()
    if st in ACTIVE and state.get("url"):
        say(f"URL guardada: {state['url']}")
        if len(state_urls(state)) > 1:
            say("Respaldos: " + ", ".join(f"{n}" for n, _ in ordered(state_urls(state))))
    if args.quota:
        cmd_quota(args)


def cmd_save_binary(args):
    """Guarda el llama-server compilado como Dataset privado para no recompilar (~27 min de GPU) en cada encendido."""
    cfg = load_cfg()
    check_auth()
    st, _ = kernel_status(cfg)
    if st in ACTIVE:
        die("La sesion sigue encendida. Apagala primero (python vault.py down) y espera a que termine (estado COMPLETE): "
            "Kaggle solo guarda los archivos de salida cuando la ejecucion termina sola.")
    if st != "COMPLETE":
        die(f"La ultima ejecucion termino como {st or 'desconocido'}. Hace falta una que termine normalmente (COMPLETE): "
            "ejecuta  up  hasta ver 'IA lista', luego  down  y repite este comando. (Una ejecucion cancelada a mano pierde los archivos.)")
    import tempfile
    tmp = Path(tempfile.mkdtemp(prefix="vault-bin-"))
    say("Descargando la salida del notebook (binario compilado)...")
    r = kaggle("kernels", "output", ref(cfg), "-p", str(tmp), "-q")
    found = [p for p in tmp.rglob("llama-server") if p.is_file()]
    if r.returncode != 0 or not found:
        die("No encontre llama-server en la salida del notebook:\n" + (r.stdout + r.stderr).strip()[-600:]
            + "\nSi la ejecucion fue cancelada, Kaggle no guarda esos archivos.")
    src = found[0].parent
    ds = HOME / "kaggle-binary-dataset"
    if ds.exists():
        shutil.rmtree(ds)
    ds.mkdir(parents=True)
    for f in src.iterdir():
        if f.is_file():
            shutil.copy2(f, ds / f.name)
    slug = binary_slug(cfg)
    save_json(ds / "dataset-metadata.json", {"title": slug, "id": f"{cfg['user']}/{slug}",
                                             "licenses": [{"name": "MIT"}]})
    mb = sum(f.stat().st_size for f in ds.iterdir()) / 1e6
    say(f"Subiendo {mb:.0f} MB como Dataset privado {cfg['user']}/{slug}...")
    cmd = ["datasets", "version", "-p", str(ds), "-m", "binario llama-server T4", "-q", "-r", "skip"] \
        if (cfg.get("binary_dataset_created") or cfg.get("bin_dataset")) else ["datasets", "create", "-p", str(ds), "-q", "-r", "skip"]
    r = kaggle(*cmd)
    if r.returncode != 0:
        die("No pude subir el Dataset:\n" + (r.stdout + r.stderr).strip()[-600:])
    cfg["binary_dataset"] = cfg["binary_dataset_created"] = True
    cfg["binary_slug"] = slug
    save_json(CFG_FILE, cfg)
    write_push_files(cfg)
    shutil.rmtree(tmp, ignore_errors=True)
    say("Listo. Espera ~2 min a que Kaggle lo procese; los siguientes  up  saltan la compilacion (~27 min menos).")
    say("El binario esta hecho para GPU T4 (sm_75): si Kaggle cambia de GPU, borra el Dataset y recompila.")


def cmd_logs(args):
    cfg = load_cfg()
    check_auth()
    r = subprocess.run(KAGGLE_CMD + ["kernels", "logs"] + (["-f"] if args.follow else []) + [ref(cfg)])
    sys.exit(r.returncode)


def cmd_down(args):
    cfg = load_cfg()
    check_auth()
    st, _ = kernel_status(cfg)
    if st not in ACTIVE:
        say(f"Ya esta apagada (estado: {st or 'sin ejecuciones'}).")
        STATE_FILE.unlink(missing_ok=True)
        return
    if st == "QUEUED":
        say("Aun esta en cola; todavia no escucha la senal de apagado.")
    msg = "STOP " + stop_signature(cfg["api_key"])
    req = urllib.request.Request(f"{NTFY_BASE}/{cfg['ntfy_topic']}", data=msg.encode(), method="POST",
                                 headers=HTTP_HEADERS)
    try:
        urllib.request.urlopen(req, timeout=20)
    except Exception as e:
        die(f"No pude enviar la senal de apagado ({e}). Detén la sesion desde la web de Kaggle.")
    say("Senal de apagado enviada. Esperando a que la sesion termine...")
    for _ in range(int(300 / POLL)):
        time.sleep(POLL)
        st, _ = kernel_status(cfg)
        if st not in ACTIVE:
            STATE_FILE.unlink(missing_ok=True)
            say(f"Apagada (estado: {st}). Ya no gasta GPU.")
            return
    say("La sesion sigue activa tras 5 minutos. Detenla desde la web de Kaggle (Session options -> Stop session / "
        "o el boton de parar del notebook) para no gastar cuota. El notebook tambien se apaga solo por inactividad.")
    sys.exit(2)


def cmd_quota(args):
    try:
        from kaggle.api.kaggle_api_extended import KaggleApi
        from kagglesdk.kernels.types.kernels_api_service import ApiGetAcceleratorQuotaStatisticsRequest
        api = KaggleApi()
        api.authenticate()
        with api.build_kaggle_client() as k:
            resp = k.kernels.kernels_api_client.get_accelerator_quota_statistics(
                ApiGetAcceleratorQuotaStatisticsRequest())
        g = resp.gpu_quota
        hrs = lambda td: td.total_seconds() / 3600 if td is not None else 0.0
        used, total, reserved = hrs(g.time_used), hrs(g.total_time_allowed), hrs(g.time_reserved)
        say(f"GPU: usadas {used:.1f} h de {total:.1f} h "
            f"(reservadas por sesiones activas: {reserved:.1f} h)")
        # Lineas que lee el cliente Nosecode para pintar la cuota en la barra.
        say(f"QUOTA_GPU_USED_H={used:.2f}")
        say(f"QUOTA_GPU_TOTAL_H={total:.2f}")
        say(f"QUOTA_GPU_RESERVED_H={reserved:.2f}")
        say(f"QUOTA_GPU_REMAINING_H={max(0.0, total - used):.2f}")
    except Exception as e:
        say(f"No pude consultar la cuota por API ({str(e)[:120]}). Mirala en kaggle.com/settings (seccion Quotas).")


def need_state():
    cfg = load_cfg()
    state = load_state()
    if not state.get("url"):
        die("No hay URL guardada. Enciende la IA con:  python vault.py up")
    return cfg, state


def usable_state():
    """Como need_state, pero comprueba que el tunel actual responde y si no cambia al siguiente."""
    cfg, state = need_state()
    urls = state_urls(state)
    cur = name_of(state, state["url"])
    if not url_alive(state["url"], tries=1):
        n, u = pick_alive(urls, first=cur, tries=2)
        if not u:
            die("Ningun tunel responde. La IA sigue encendida?  python vault.py status   (o  up  para encenderla)")
        if u != state["url"]:
            say(f"(el tunel {cur} no responde; uso {n})")
            save_state(urls, state["model"], n)
            state = load_state()
    return cfg, state


def cmd_web(args):
    cfg, state = usable_state()
    say(f"Abriendo {state['url']}\nCuando la web pida la API key, pega:\n  {cfg['api_key']}")
    webbrowser.open(state["url"])


def img_part(path):
    p = Path(path.strip().strip('"'))
    if not p.exists():
        raise FileNotFoundError(path)
    mime = mimetypes.guess_type(p.name)[0] or "image/png"
    b64 = base64.b64encode(p.read_bytes()).decode()
    return {"type": "image_url", "image_url": {"url": f"data:{mime};base64,{b64}"}}


def stream_chat(cfg, model, url, messages, stream=True):
    body = json.dumps({"model": model, "messages": messages, "stream": stream}).encode()
    req = urllib.request.Request(url.rstrip("/") + "/v1/chat/completions", data=body, method="POST",
                                 headers={**HTTP_HEADERS, "Content-Type": "application/json",
                                          "Authorization": f"Bearer {cfg['api_key']}"})
    answer = []
    with urllib.request.urlopen(req, timeout=900) as resp:
        if not stream:                      # tuneles sin SSE (Cloudflare): respuesta completa de una vez
            text = json.loads(resp.read().decode("utf-8", "replace"))["choices"][0]["message"].get("content") or ""
            print(text)
            return text
        for raw in resp:
            line = raw.decode("utf-8", "replace").strip()
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if payload == "[DONE]":
                break
            try:
                delta = json.loads(payload)["choices"][0].get("delta", {})
            except Exception:
                continue
            piece = delta.get("content")
            if piece:
                answer.append(piece)
                print(piece, end="", flush=True)
    print()
    return "".join(answer)


def chat_with_failover(cfg, state, messages):
    """Prueba el tunel actual y, si falla (conexion o error del tunel/cupo), pasa al siguiente."""
    cur = name_of(state, state["url"])
    last = None
    for n, u in ordered(state_urls(state), cur):
        try:
            reply = stream_chat(cfg, state["model"], u, messages, stream=n not in NO_STREAM)
        except urllib.error.HTTPError as e:
            if e.code not in RETRY_CODES:
                raise                        # 401 clave mala, 400 peticion mala... no se arregla cambiando de tunel
            last = f"HTTP {e.code}"
        except (urllib.error.URLError, OSError, ValueError) as e:
            last = str(e)
        else:
            if u != state["url"]:
                save_state(state_urls(state), state["model"], n)
                state["url"] = u
            return reply
        say(f"\n  [{n}] fallo ({last}); pruebo otro tunel...")
        print("ia> ", end="", flush=True)
    raise RuntimeError(f"ningun tunel respondio (ultimo error: {last})")


def cmd_chat(args):
    cfg, state = usable_state()
    say(f"Chat con {state['model']}  |  /img RUTA adjunta una imagen al siguiente mensaje  |  /exit sale")
    messages, pending = [], []
    while True:
        try:
            text = input("\ntu> ").strip()
        except (EOFError, KeyboardInterrupt):
            say()
            break
        if not text:
            continue
        if text in ("/exit", "/quit"):
            break
        if text.startswith("/img "):
            try:
                pending.append(img_part(text[5:]))
                say(f"  (imagen adjunta: {len(pending)})")
            except Exception as e:
                say(f"  No pude leer la imagen: {e}")
            continue
        content = ([{"type": "text", "text": text}] + pending) if pending else text
        pending = []
        messages.append({"role": "user", "content": content})
        print("ia> ", end="", flush=True)
        try:
            reply = chat_with_failover(cfg, state, messages)
        except urllib.error.HTTPError as e:
            messages.pop()
            say(f"\nError HTTP {e.code}: {'API key incorrecta' if e.code == 401 else e.reason}")
            continue
        except Exception as e:
            messages.pop()
            say(f"\nNo pude hablar con la IA ({e}). Esta encendida? python vault.py status")
            continue
        messages.append({"role": "assistant", "content": reply})


def cmd_proxy(args):
    """Servidor local que reenvia a tu IA con la API key puesta y cambia de tunel si uno falla.
    Sirve para opencode o cualquier cliente: ellos hablan con http://127.0.0.1:PUERTO y no se enteran del cambio."""
    cfg, state = usable_state()
    skip = {"transfer-encoding", "connection", "content-length", "keep-alive", "host", "authorization"}

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def handle_any(self):
            n_body = int(self.headers.get("Content-Length") or 0)
            body = self.rfile.read(n_body) if n_body else None
            st = load_state()
            urls = state_urls(st)
            cur = name_of(st, st.get("url"))
            for n, u in ordered(urls, cur):
                hdr = {k: v for k, v in self.headers.items() if k.lower() not in skip}
                hdr.update(HTTP_HEADERS)
                hdr["Authorization"] = f"Bearer {cfg['api_key']}"
                req = urllib.request.Request(u.rstrip("/") + self.path, data=body, method=self.command, headers=hdr)
                try:
                    resp = urllib.request.urlopen(req, timeout=900)
                    code = resp.status
                except urllib.error.HTTPError as e:
                    if e.code in RETRY_CODES:
                        say(f"  [{n}] HTTP {e.code}: pruebo otro tunel")
                        continue
                    resp, code = e, e.code
                except OSError as e:
                    say(f"  [{n}] sin conexion ({e}): pruebo otro tunel")
                    continue
                if u != st.get("url"):
                    save_state(urls, st.get("model", "Qwen3.8-27B"), n)
                self.send_response(code)
                for k, v in resp.headers.items():
                    if k.lower() not in skip and k.lower() != "transfer-encoding":
                        self.send_header(k, v)
                self.send_header("Connection", "close")
                self.end_headers()
                read = getattr(resp, "read1", resp.read)
                try:
                    while True:
                        chunk = read(8192)
                        if not chunk:
                            break
                        self.wfile.write(chunk)
                        self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    pass
                return
            self.send_response(502)
            self.end_headers()
            self.wfile.write(b'{"error":"ningun tunel respondio"}')

        do_GET = do_POST = do_PUT = do_DELETE = do_OPTIONS = handle_any

    srv = http.server.ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    say(f"Proxy local en http://127.0.0.1:{args.port}/v1  (reenvia con tu API key y cambia de tunel si uno falla)")
    say("Usalo como baseURL en opencode:  python vault.py opencode --proxy      Ctrl+C para parar.")
    say("Ojo: cualquier programa de ESTE equipo que use ese puerto podra usar tu IA (no se expone a la red).")
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        say("\nProxy detenido.")


def cmd_opencode(args):
    cfg, state = usable_state()
    out = Path(args.out)
    data = json.loads(out.read_text(encoding="utf-8")) if out.exists() else {}
    data.setdefault("$schema", "https://opencode.ai/config.json")
    base = f"http://127.0.0.1:{args.port}/v1" if args.proxy else state["url"].rstrip("/") + "/v1"
    data.setdefault("provider", {})["kaggle"] = {
        "npm": "@ai-sdk/openai-compatible",
        "name": "Kaggle Qwen",
        "options": {"baseURL": base},
        "models": {state["model"]: {"name": f"{state['model']} (Kaggle)"}},
    }
    out.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")
    say(f"Escrito {out} con {base}")
    if args.proxy:
        say(f"Deja corriendo en otra terminal:  python vault.py proxy --port {args.port}")
        say("(con el proxy la API key la pone vault.py; en opencode puedes registrar cualquier clave, ej. 'x')")
    else:
        say("Sin proxy, opencode usa solo este tunel y no cambia solo si falla. Para cambio automatico:  opencode --proxy")
        say("Registra tu clave una vez:  opencode auth login   (elige 'Other', id de proveedor: kaggle)")
        say("Tu API key:  python vault.py key")


def main():
    ap = argparse.ArgumentParser(prog="vault", description="Controla tu IA de Kaggle (gratis) desde la terminal.")
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("init", help="configura todo (una sola vez)")
    p.add_argument("--user", help="tu usuario de Kaggle")
    p.add_argument("--slug", help="nombre del notebook (por defecto dev-vault-qwen)")
    p.add_argument("--ngrok-token", default=None)
    p.add_argument("--ngrok-domain", default=None)
    p.add_argument("--ts-authkey", default=None, help="auth key de Tailscale (2o tunel, opcional)")
    p.add_argument("--ts-hostname", default=None, help="nombre de la maquina en Tailscale (por defecto kaggle-ia)")
    p.add_argument("--ts-mode", choices=["funnel", "serve"], default=None,
                   help="funnel = publico con API key; serve = solo dentro de tu red Tailscale")
    p.add_argument("--no-cloudflared", dest="no_cloudflared", action="store_true", default=None,
                   help="no abrir Cloudflare como tercer tunel")
    p.add_argument("--idle-minutes", type=int, default=None, help="apagar tras N min sin uso (por defecto 30)")
    p.add_argument("--max-hours", type=float, default=None, help="tope de horas por sesion (por defecto 8)")
    p.add_argument("--notebook", help="ruta del notebook (por defecto el del repo)")
    p.add_argument("--reset-key", action="store_true", help="genera una API key nueva")
    p.add_argument("--no-publish", action="store_true", help="no subir el Dataset de configuracion")
    p.set_defaults(fn=cmd_init)

    p = sub.add_parser("up", help="enciende la IA y espera la URL")
    p.add_argument("--timeout", type=float, default=100, help="minutos maximos de espera (por defecto 100)")
    p.add_argument("--no-wait", action="store_true", help="solo lanza, no espera la URL")
    p.add_argument("--sync-config", action="store_true", help="vuelve a subir vault-config antes de encender")
    p.set_defaults(fn=cmd_up)

    sub.add_parser("url", help="muestra la URL actual").set_defaults(fn=cmd_url)
    sub.add_parser("urls", help="lista todos los tuneles y cual esta vivo").set_defaults(fn=cmd_urls)
    p = sub.add_parser("proxy", help="proxy local con cambio automatico de tunel (para opencode)")
    p.add_argument("--port", type=int, default=8787)
    p.set_defaults(fn=cmd_proxy)
    p = sub.add_parser("status", help="estado del notebook")
    p.add_argument("--quota", action="store_true", help="incluye cuota de GPU (experimental)")
    p.set_defaults(fn=cmd_status)
    p = sub.add_parser("logs", help="logs del notebook")
    p.add_argument("-f", "--follow", action="store_true")
    p.set_defaults(fn=cmd_logs)
    sub.add_parser("save-binary", help="guarda llama-server compilado como Dataset (evita recompilar)").set_defaults(fn=cmd_save_binary)
    sub.add_parser("down", help="apaga la IA (deja de gastar GPU)").set_defaults(fn=cmd_down)
    sub.add_parser("quota", help="horas de GPU usadas (experimental)").set_defaults(fn=cmd_quota)
    sub.add_parser("key", help="muestra tu API key").set_defaults(fn=cmd_key)
    sub.add_parser("web", help="abre la web UI en el navegador").set_defaults(fn=cmd_web)
    sub.add_parser("chat", help="chat en la terminal (con /img)").set_defaults(fn=cmd_chat)
    p = sub.add_parser("opencode", help="escribe opencode.json con la URL actual")
    p.add_argument("--out", default="opencode.json")
    p.add_argument("--proxy", action="store_true", help="apunta opencode al proxy local (cambio automatico de tunel)")
    p.add_argument("--port", type=int, default=8787)
    p.set_defaults(fn=cmd_opencode)

    args = ap.parse_args()
    args.fn(args)


if __name__ == "__main__":
    main()
