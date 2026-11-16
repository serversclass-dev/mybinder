
import os
import shutil
import tarfile
import urllib.request
import subprocess
from pathlib import Path

HOME = Path.home()
VERSION = "1.25.4"

GO_DIR = HOME / "go"
GOPATH = HOME / "go-workspace"
ARCHIVE = HOME / f"go{VERSION}.linux-amd64.tar.gz"
BASHRC = HOME / ".bashrc"

URL = f"https://go.dev/dl/go{VERSION}.linux-amd64.tar.gz"

try:
    # 1. Download Go
    print("[1/6] Downloading Go...")
    urllib.request.urlretrieve(URL, ARCHIVE)

    # 2. Extract Go
    print("[2/6] Extracting Go...")

    if GO_DIR.exists():
        shutil.rmtree(GO_DIR)

    with tarfile.open(ARCHIVE, "r:gz") as tar:
        tar.extractall(HOME)

    # 3. Configure environment
    print("[3/6] Configuring environment...")

    config = """
# Go environment
export GOROOT="$HOME/go"
export GOPATH="$HOME/go-workspace"
export PATH="$GOROOT/bin:$GOPATH/bin:$PATH"
"""

    existing = BASHRC.read_text() if BASHRC.exists() else ""

    if "# Go environment" not in existing:
        with BASHRC.open("a") as f:
            f.write("\n" + config + "\n")

    # 4. Create workspace
    print("[4/6] Creating Go workspace...")
    GOPATH.mkdir(exist_ok=True)

    # 5. Activate environment in Python process
    print("[5/6] Activating Go environment...")

    os.environ["GOROOT"] = str(GO_DIR)
    os.environ["GOPATH"] = str(GOPATH)
    os.environ["PATH"] = (
        f"{GO_DIR}/bin:{GOPATH}/bin:{os.environ['PATH']}"
    )

    # 6. Verify
    print("[6/6] Verifying Go...")

    subprocess.run(
        [str(GO_DIR / "bin/go"), "version"],
        check=True
    )

    ARCHIVE.unlink(missing_ok=True)

    print("\nGo installed successfully!")
    print("GOROOT:", GO_DIR)
    print("GOPATH:", GOPATH)
    print("\nConfiguration saved to ~/.bashrc")

    # IMPORTANT:
    # A Python process cannot modify its parent shell's PATH.
    # This prints the command needed to update the current shell.
    print("\nRun this once in your terminal:")
    print("source ~/.bashrc")

except Exception as e:
    print(f"\nInstallation failed: {e}")
