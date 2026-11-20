"""Menú del Vault portátil. Se ejecuta en una terminal, fuera del chat."""
import argparse
import getpass
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parent
CONF = Path(os.environ["OPENCODE_CONFIG_DIR"])
HOME = CONF / "vault"
TOKEN = HOME / "kaggle-token"
os.environ["VAULT_HOME"] = str(HOME)
sys.path.insert(0, str(ROOT / "client"))
import vault


def save(path, text):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, delete=False) as f:
        f.write(text)
    os.replace(f.name, path)


def deps(action):
    env = ROOT / "venv"
    python = env / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
    if Path(sys.prefix).resolve() == env.resolve():
        return
    if sys.version_info < (3, 11):
        raise SystemExit("Para el Vault necesitas Python 3.11 o posterior.")
    if not python.exists():
        print("Preparando entorno privado de Python…", flush=True)
        subprocess.run([sys.executable, "-m", "venv", str(env)], check=True)
    if subprocess.run([str(python), "-c", "import importlib.util,sys;sys.exit(importlib.util.find_spec('kaggle') is None)"]).returncode:
        print("Instalando el cliente de Kaggle…", flush=True)
        subprocess.run([str(python), "-m", "pip", "install", "kaggle>=2.2.5,<3"], check=True, timeout=600)
    raise SystemExit(subprocess.call([str(python), __file__, action]))


def setup():
    if not sys.stdin.isatty():
        raise SystemExit("Configura el Vault desde una terminal interactiva: opencode_n vault.")
    old = vault.load_cfg() if vault.CFG_FILE.exists() else {}
    print("Kaggle → Settings → API → Generate New Token. Tu cuenta necesita GPU e Internet habilitados.")
    entered = getpass.getpass("Token de Kaggle (Enter: usar credencial existente): ").strip()
    if entered:
        os.environ["KAGGLE_API_TOKEN"] = entered
    from kaggle.api.kaggle_api_extended import KaggleApi
    api = KaggleApi()
    api.authenticate()
    user = api.config_values.get(api.CONFIG_NAME_USER)
    if not user or (entered and api.config_values.get(api.CONFIG_NAME_TOKEN) != entered):
        raise SystemExit("No se pudo verificar el token de Kaggle.")
    if old.get("user", user) != user:
        raise SystemExit("La cuenta no coincide con el Vault guardado. No se cambiaron sus credenciales.")
    if old.get("dataset_created"):
        from kagglesdk.datasets.types.dataset_api_service import ApiGetDatasetRequest
        req = ApiGetDatasetRequest()
        req.owner_slug, req.dataset_slug = user, old["slug"] + "-config"
        with api.build_kaggle_client() as client:
            if not client.datasets.dataset_api_client.get_dataset(req).is_private:
                raise SystemExit("El dataset de configuración debe ser privado. No se subieron credenciales.")
    print("Sesión verificada:", user)
    print("Para el streaming del CLI elige 1) ngrok  2) Tailscale Funnel  3) ambos.")
    choice = input("Túneles [1]: ").strip() or "1"
    if choice not in {"1", "2", "3"}:
        raise SystemExit("Opción no válida.")
    def secret(label, field):
        val = getpass.getpass(label + " (Enter conserva el guardado): ").strip() or old.get(field, "")
        if not val:
            raise SystemExit("Falta " + label)
        return val
    ng = ts = domain = ""
    if choice in {"1", "3"}:
        print("ngrok: dashboard.ngrok.com/get-started/your-authtoken")
        ng = secret("Authtoken de ngrok", "ngrok_token")
        domain = input("Dominio fijo de ngrok (opcional): ").strip() or old.get("ngrok_domain", "")
    if choice in {"2", "3"}:
        print("Tailscale: crea una auth key en tu tailnet y habilita Funnel.")
        ts = secret("Auth key de Tailscale", "ts_authkey")
    if entered:
        save(TOKEN, entered)
        os.environ["KAGGLE_API_TOKEN"] = str(TOKEN)
    # Nombres propios por instalación: no sobrescribir datasets de otro Vault.
    slug = old.get("slug") or "opencode-n-" + secrets.token_hex(5)
    vault.DATASET_SLUG = slug + "-config"
    args = SimpleNamespace(user=user, slug=slug, reset_key=False, idle_minutes=None, max_hours=None,
                           notebook=str(vault.DEFAULT_NB), ngrok_token=ng, ngrok_domain=domain,
                           ts_authkey=ts, ts_hostname=slug, ts_mode="funnel", no_cloudflared=False, no_publish=False)
    # Ya se solicitaron todos los campos. No repetir los prompts de `vault init`.
    stdin = sys.stdin
    try:
        with open(os.devnull) as sys.stdin:
            vault.cmd_init(args)
    finally:
        sys.stdin = stdin


def link():
    state = vault.load_state()
    name, url = vault.pick_alive({k: v for k, v in vault.state_urls(state).items() if k != "cloudflared"}, tries=2)
    if not name or urlsplit(url).scheme != "https":
        raise SystemExit("No hay túnel HTTPS con streaming disponible. Revisa ngrok o Tailscale en el menú Vault.")
    model = state["model"]
    key = vault.load_cfg()["api_key"]
    # Confirmar que responde el Vault de este usuario, no solo una página /health.
    with vault.http_get(url.rstrip("/") + "/v1/models", {"Authorization": "Bearer " + key}, timeout=15) as r:
        models = json.load(r)["data"]
    if not any(m.get("id") == model for m in models):
        raise SystemExit("El modelo anunciado no aparece en el endpoint. No se cambió el perfil.")
    path = CONF / "opencode.json"
    cfg = json.loads(path.read_text(encoding="utf-8-sig"))
    provider = json.loads((ROOT / "config/opencode.json").read_text())["providers"]["kaggle"]
    entry = provider["models"].pop("__MODEL__")
    entry["name"] = model
    if model == "Qwen3.8-27B":
        entry["variants"] = json.loads((ROOT / "config/qwen-variants.json").read_text())
        entry["capabilities"]["reasoning"] = {"supported": True}
        entry["compatibility"]["reasoningField"] = "reasoning_content"
    p = cfg.setdefault("providers", {}).setdefault("kaggle", provider)
    p.setdefault("settings", {}).update(baseURL=url.rstrip("/") + "/v1", apiKey="{file:" + (HOME / "endpoint-key").as_posix() + "}")
    p.setdefault("models", {}).setdefault(model, entry)
    cfg["model"] = "kaggle/" + model
    save(HOME / "endpoint-key", key)
    save(path, json.dumps(cfg, ensure_ascii=False, indent=2) + "\n")
    print("Vault conectado. Modelo:", cfg["model"], "(en una sesión existente, selecciónalo en /models).")


def main():
    ap = argparse.ArgumentParser(description="Gestiona tu propio Vault de Kaggle.")
    ap.add_argument("action", nargs="?", default="menu", choices=["menu", "setup", "up", "down", "status", "urls", "logs"])
    action = ap.parse_args().action
    if action == "menu":
        print("Vault de Kaggle\n1. Encender y conectar\n2. Configurar cuenta y túneles\n3. Estado y cuota\n4. Apagar\n5. Ver túneles\n6. Ver logs\n0. Volver")
        action = {"1":"up", "2":"setup", "3":"status", "4":"down", "5":"urls", "6":"logs", "0":None, "":None}.get(input("Opción: ").strip(), "invalid")
        if action is None: return
        if action == "invalid": raise SystemExit("Opción no válida.")
    configured = vault.CFG_FILE.exists() and vault.load_cfg().get("dataset_created")
    if action not in ("up", "setup") and not configured:
        print("Todavía no hay un Vault configurado."); return
    if TOKEN.exists():
        os.environ["KAGGLE_API_TOKEN"] = str(TOKEN)
    deps(action)
    if action == "setup" or not configured:
        setup()
    if action == "setup": return
    vault.DATASET_SLUG = vault.load_cfg()["slug"] + "-config"
    if action == "up":
        print("Esperando a que Kaggle prepare el dataset privado…", flush=True)
        dataset = vault.load_cfg()["user"] + "/" + vault.DATASET_SLUG
        for _ in range(30):
            r = subprocess.run([sys.executable, "-m", "kaggle", "datasets", "status", dataset],
                               capture_output=True, text=True, timeout=30)
            if r.returncode == 0 and r.stdout.strip().lower().endswith("ready"): break
            time.sleep(4)
        else:
            raise SystemExit("El dataset todavía no está listo. Vuelve a usar /up; no se inició la GPU.")
        vault.cmd_up(SimpleNamespace(test_dflash=False, no_wait=False, sync_config=False, timeout=100))
        link()
    elif action == "down":
        vault.cmd_down(None)
    elif action == "urls":
        vault.cmd_urls(None)
    elif action == "logs":
        vault.cmd_logs(SimpleNamespace(follow=False))
    else:
        vault.cmd_status(SimpleNamespace(quota=True))


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit("\nCancelado.")
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        sys.exit("Vault: no se completó la operación (" + type(error).__name__ + "). Revisa la conexión y el paso anterior.")
