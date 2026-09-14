import os
import sys
import subprocess
import urllib.request
import tarfile

GO_VERSION = "1.22.6"
GO_URL = f"https://go.dev/dl/go{GO_VERSION}.linux-amd64.tar.gz"

def setup_and_run():
    server_bin = "./server"
    
    # 1. Kompilasi binary Go jika belum ada
    if not os.path.exists(server_bin):
        print(f"[HF Space] Mengunduh Go {GO_VERSION} untuk build...", flush=True)
        tar_path = "/tmp/go.tar.gz"
        urllib.request.urlretrieve(GO_URL, tar_path)
        
        print("[HF Space] Mengekstrak Go toolchain...", flush=True)
        with tarfile.open(tar_path, "r:gz") as tar:
            tar.extractall(path="/tmp")
            
        go_bin = "/tmp/go/bin/go"
        
        print("[HF Space] Mengompilasi server Go...", flush=True)
        env = os.environ.copy()
        env["CGO_ENABLED"] = "0"
        env["HOME"] = "/tmp"
        
        build_cmd = [go_bin, "build", "-ldflags=-w -s", "-o", "server", "."]
        res = subprocess.run(build_cmd, env=env)
        if res.returncode != 0:
            print("[HF Space] ERROR: Kompilasi Go gagal!", flush=True)
            sys.exit(res.returncode)
            
        print("[HF Space] Kompilasi berhasil!", flush=True)
        
        # Bersihkan file installer
        if os.path.exists(tar_path):
            os.remove(tar_path)
            
    # 2. Jalankan server Go di port 7860
    os.chmod(server_bin, 0o755)
    env = os.environ.copy()
    env["PORT"] = os.environ.get("PORT", "7860")
    
    print(f"[HF Space] Menjalankan server Go pada port {env['PORT']}...", flush=True)
    subprocess.run([server_bin], env=env)

if __name__ == "__main__":
    setup_and_run()
