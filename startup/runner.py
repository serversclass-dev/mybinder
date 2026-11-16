import json
import os
import subprocess
import threading
import time

DB_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "startup_commands.json")

def _load_db():
    if not os.path.isfile(DB_PATH):
        return {"commands": []}
    try:
        with open(DB_PATH, "r") as f:
            return json.load(f)
    except Exception:
        return {"commands": []}

def _save_db(data):
    with open(DB_PATH, "w") as f:
        json.dump(data, f, indent=2)

def get_commands():
    return _load_db().get("commands", [])

def add_command(label, cmd, enabled=True):
    db = _load_db()
    import uuid
    entry = {
        "id": str(uuid.uuid4()),
        "label": label,
        "cmd": cmd,
        "enabled": enabled,
        "created": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "last_run": None,
        "last_output": "",
        "last_status": None
    }
    db["commands"].append(entry)
    _save_db(db)
    return entry

def update_command(cid, label=None, cmd=None, enabled=None):
    db = _load_db()
    for c in db["commands"]:
        if c["id"] == cid:
            if label is not None:
                c["label"] = label
            if cmd is not None:
                c["cmd"] = cmd
            if enabled is not None:
                c["enabled"] = enabled
            _save_db(db)
            return c
    return None

def delete_command(cid):
    db = _load_db()
    before = len(db["commands"])
    db["commands"] = [c for c in db["commands"] if c["id"] != cid]
    _save_db(db)
    return len(db["commands"]) < before

def run_command(cid):
    db = _load_db()
    for c in db["commands"]:
        if c["id"] == cid:
            try:
                result = subprocess.run(
                    c["cmd"], shell=True, capture_output=True,
                    text=True, timeout=60
                )
                output = (result.stdout + result.stderr).strip()
                status = result.returncode
            except subprocess.TimeoutExpired:
                output = "ERROR: command timed out after 60s"
                status = -1
            except Exception as e:
                output = f"ERROR: {e}"
                status = -1
            c["last_run"] = time.strftime("%Y-%m-%dT%H:%M:%S")
            c["last_output"] = output[-4000:]
            c["last_status"] = status
            _save_db(db)
            return {"output": output, "status": status}
    return {"output": "Command not found", "status": -1}

def run_all_enabled():
    results = []
    db = _load_db()
    for c in db["commands"]:
        if c.get("enabled", True):
            r = run_command(c["id"])
            results.append({"id": c["id"], "label": c["label"], **r})
    return results

def run_startup_async():
    """Call this at OS boot — runs all enabled commands in background thread."""
    def _worker():
        time.sleep(1)  # small delay so Flask is up first
        run_all_enabled()
    t = threading.Thread(target=_worker, daemon=True)
    t.start()