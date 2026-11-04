#!/usr/bin/env python3
"""Instalador del Dev Vault.

Pide solo las credenciales (Kaggle y ngrok/Tailscale), instala lo necesario,
guarda la configuracion local, publica el Dataset de configuracion y deja todo
listo para `python client/vault.py up`.

Uso:
  python scripts/setup.py              instala y configura todo
  python scripts/setup.py --local      solo reconfigura ~/.dev-vault
  python scripts/setup.py --save-bin   sube el llama-server compilado (tras la 1a sesion)
  python scripts/setup.py --check      muestra el estado sin cambiar nada

Nunca imprime claves ni tokens.
"""
import argparse
import getpass
import importlib.util
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
VAULT = ROOT / "client" / "vault.py"
CFG_FILE = Path.home() / ".dev-vault" / "config.json"
KAGGLE_FILE = Path.home() / ".kaggle" / "kaggle.json"
PY = sys.executable


def say(msg=""):
    print(msg, flush=True)


def die(msg):
    say("\nERROR: " + msg)
    sys.exit(1)


def load_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}


def run(cmd, label, **kw):
    say(label)
    return subprocess.run(cmd, text=True, **kw)


def ask(text, current="", secret=False, required=False):
    """Pide un dato. Enter mantiene el valor actual (si existe)."""
    suffix = " [Enter para mantener]" if current else ""
    while True:
        if secret:
            val = getpass.getpass(text + suffix + ": ").strip()
        else:
            val = input(text + suffix + ": ").strip()
        if not val and current:
            return current
        if val or not required:
            return val
        say("  Este dato es obligatorio.")


def check_python():
    if sys.version_info < (3, 9):
        die("Necesitas Python 3.9 o superior (tienes %s)." % sys.version.split()[0])


def check_repo():
    if not VAULT.is_file():
        die("No encuentro client/vault.py. Ejecuta el script desde la carpeta dev-vault.")


def install_deps():
    say("\n== 1/4 Instalando dependencias ==")
    cmd = [PY, "-m", "pip", "install", "--upgrade", "kaggle"]
    req = ROOT / "client" / "requirements.txt"
    if req.is_file() and req.read_text(encoding="utf-8").strip():
        cmd += ["-r", str(req)]
    r = subprocess.run(cmd, text=True)
    if r.returncode != 0:
        die("No pude instalar las dependencias. Revisa tu conexion o ejecuta:\n  " + " ".join(cmd))


def kaggle_oauth_user():
    """Usuario de una sesion OAuth previa (`kaggle auth login`), sin imprimir claves."""
    if not (Path.home() / ".kaggle" / "access_token").is_file():
        return ""
    r = subprocess.run([PY, "-m", "kaggle", "config", "view"], text=True, capture_output=True)
    for line in (r.stdout + r.stderr).splitlines():
        if line.strip().startswith("- username:"):
            return line.split(":", 1)[1].strip()
    return ""


def kaggle_ok():
    r = subprocess.run([PY, "-m", "kaggle", "datasets", "list", "-m", "--page-size", "1"],
                       text=True, capture_output=True)
    return r.returncode == 0


def kaggle_credentials():
    say("\n== 2/4 Credenciales de Kaggle ==")
    env_user = os.environ.get("KAGGLE_USERNAME", "").strip()
    env_key = os.environ.get("KAGGLE_KEY", "").strip()
    saved = load_json(KAGGLE_FILE)
    user = env_user or str(saved.get("username", "")).strip()
    key = env_key or str(saved.get("key", "")).strip()

    if not (user and key):
        oauth = kaggle_oauth_user()
        if oauth and kaggle_ok():
            say("Kaggle ya tiene sesion iniciada como %s." % oauth)
            return oauth

    if user and key:
        say("Ya hay credenciales de Kaggle guardadas.")
    else:
        say("Las consigues en kaggle.com/settings -> API -> Create New Token.")
    if not user:
        user = ask("Usuario de Kaggle", required=True)
    if not key:
        key = ask("API key de Kaggle", secret=True, required=True)

    KAGGLE_FILE.parent.mkdir(parents=True, exist_ok=True)
    KAGGLE_FILE.write_text(json.dumps({"username": user, "key": key}, indent=2), encoding="utf-8")
    try:
        os.chmod(KAGGLE_FILE, 0o600)
    except OSError:
        pass

    say("Comprobando acceso a Kaggle...")
    if not kaggle_ok():
        die("Kaggle rechazo las credenciales. Revisa la API key o ejecuta: python -m kaggle auth login")
    say("Kaggle OK.")
    return user


def tunnel_credentials():
    say("\n== 3/4 Tuneles ==")
    cfg = load_json(CFG_FILE)
    ngrok = cfg.get("ngrok_token", "")
    domain = cfg.get("ngrok_domain", "")

    if ngrok:
        say("ngrok ya esta configurado (Enter mantiene el token actual).")
    else:
        say("ngrok es el tunel recomendado (gratis, dominio fijo y streaming).")
        say("Token: dashboard.ngrok.com/get-started/your-authtoken")
    ngrok = ask("NGROK_AUTHTOKEN", current=ngrok, secret=True)
    domain = ask("Dominio fijo de ngrok (ej. mi-ia.ngrok-free.dev)", current=domain)

    ts_key = cfg.get("ts_authkey", "")
    if ts_key:
        say("Tailscale ya esta configurado (Enter mantiene la clave actual).")
    ts_key = ask("Tailscale auth key (opcional; Enter para omitir)", current=ts_key, secret=True)

    return {"ngrok_token": ngrok, "ngrok_domain": domain, "ts_authkey": ts_key}


def configure(user, creds, publish=True):
    say("\n== 4/4 Configurando el Vault ==")
    cmd = [PY, str(VAULT), "init", "--user", user]
    if creds["ngrok_token"]:
        cmd += ["--ngrok-token", creds["ngrok_token"]]
    if creds["ngrok_domain"]:
        cmd += ["--ngrok-domain", creds["ngrok_domain"]]
    if creds["ts_authkey"]:
        cmd += ["--ts-authkey", creds["ts_authkey"]]
    if not publish:
        cmd.append("--no-publish")

    # stdin cerrado: init no vuelve a preguntar; usa los flags o lo ya guardado.
    r = subprocess.run(cmd, text=True, stdin=subprocess.DEVNULL)
    if r.returncode != 0:
        die("vault.py init fallo (codigo %d)." % r.returncode)


def dataset_exists(user, slug):
    r = subprocess.run([PY, "-m", "kaggle", "datasets", "files", "-d", f"{user}/{slug}"],
                       text=True, capture_output=True)
    return r.returncode == 0


def attach_existing_bin(user):
    """Si el Dataset del binario ya existe, lo adjunta al proximo up."""
    cfg = load_json(CFG_FILE)
    if cfg.get("bin_dataset") or cfg.get("binary_dataset"):
        return True
    if dataset_exists(user, "llama-bin"):
        cfg["bin_dataset"] = True
    elif dataset_exists(user, "llama-bin-t4"):
        cfg["binary_dataset"] = True
    else:
        return False
    CFG_FILE.write_text(json.dumps(cfg, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    spec = importlib.util.spec_from_file_location("vault", VAULT)
    vault = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vault)
    vault.write_push_files(cfg)
    say("Dataset de binario ya existente: se adjuntara sin recompilar.")
    return True


def vault_help():
    r = subprocess.run([PY, str(VAULT), "--help"], text=True, capture_output=True)
    return r.stdout + r.stderr


def save_bin():
    say("Subiendo el llama-server compilado como Dataset...")
    cmd = "bin-save" if "bin-save" in vault_help() else "save-binary"
    r = subprocess.run([PY, str(VAULT), cmd], text=True)
    if r.returncode != 0:
        die("No pude guardar el binario. Deja que la IA compile, apagala con down y repite.")
    say("Binario guardado: las proximas sesiones no compilan.")


def check():
    cfg = load_json(CFG_FILE)
    say("Python:      " + sys.version.split()[0])
    say("Repo:        " + str(ROOT))
    say("vault.py:    " + ("OK" if VAULT.is_file() else "FALTA"))
    say("kaggle.json: " + ("OK" if KAGGLE_FILE.is_file() else "FALTA"))
    say("kaggle oauth:"+ (" OK" if kaggle_oauth_user() else " no"))
    say("config:      " + ("OK" if CFG_FILE.is_file() else "FALTA"))
    say("usuario:     " + (cfg.get("user") or "(sin configurar)"))
    say("ngrok:       " + ("configurado" if cfg.get("ngrok_token") else "falta"))
    say("tailscale:   " + ("configurado" if cfg.get("ts_authkey") else "omitido"))
    say("binario:     " + ("guardado" if cfg.get("bin_dataset") else "aun no"))


def main():
    ap = argparse.ArgumentParser(description="Instala y configura el Dev Vault.")
    ap.add_argument("--local", action="store_true", help="solo reconfigura ~/.dev-vault")
    ap.add_argument("--save-bin", action="store_true", help="sube el llama-server compilado")
    ap.add_argument("--no-publish", action="store_true", help="no sube el Dataset de configuracion")
    ap.add_argument("--check", action="store_true", help="muestra el estado sin cambiar nada")
    args = ap.parse_args()

    check_python()
    check_repo()

    if args.check:
        check()
        return
    if args.save_bin:
        save_bin()
        return

    say("Dev Vault: instalacion y configuracion")
    if not args.local:
        install_deps()
        user = kaggle_credentials()
    else:
        saved = load_json(KAGGLE_FILE)
        user = (os.environ.get("KAGGLE_USERNAME", "").strip()
                or str(saved.get("username", "")).strip()
                or kaggle_oauth_user())
        if not user:
            die("No hay credenciales de Kaggle. Ejecuta el setup completo (sin --local).")

    creds = tunnel_credentials()
    configure(user, creds, publish=not args.no_publish)
    attach_existing_bin(user)

    cfg = load_json(CFG_FILE)
    say("\nListo. Siguiente paso:")
    say("  python client/vault.py up")
    say("  python client/vault.py key     (tu API key para la web)")
    if cfg.get("bin_dataset"):
        say("El binario guardado ya se adjuntara solo: no se recompila.")
    else:
        say("Primera vez: compilara ~25 min. Despues de apagarla con down, ejecuta:")
        say("  python scripts/setup.py --save-bin")


if __name__ == "__main__":
    main()
