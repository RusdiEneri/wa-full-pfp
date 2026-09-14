import os
import sys
import json
import subprocess
import urllib.request
import tarfile

# Mock @spaces.GPU untuk mencegah error jika Space tidak sengaja di-set ke ZeroGPU
try:
    import spaces
    @spaces.GPU
    def dummy_zero_gpu():
        return True
except Exception:
    pass

def get_latest_go_version():
    try:
        req = urllib.request.Request(
            "https://go.dev/dl/?mode=json",
            headers={"User-Agent": "Mozilla/5.0"}
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read().decode())
            if data and len(data) > 0:
                ver = data[0]["version"]  # e.g. "go1.27.1"
                return ver.lstrip("go")
    except Exception as e:
        print(f"[HF Space] Gagal mendeteksi versi Go otomatis: {e}", flush=True)
    return "1.25.0"

def setup_and_run():
    # Panggil fungsi dummy jika spaces aktif
    try:
        if "spaces" in sys.modules:
            dummy_zero_gpu()
    except Exception:
        pass

    server_bin = "./server"
    
    # 1. Kompilasi binary Go jika belum ada
    if not os.path.exists(server_bin):
        go_ver = get_latest_go_version()
        go_tar_name = f"go{go_ver}.linux-amd64.tar.gz"
        go_url = f"https://go.dev/dl/{go_tar_name}"
        
        print(f"[HF Space] Mengunduh Go v{go_ver} ({go_url})...", flush=True)
        tar_path = "/tmp/go.tar.gz"
        urllib.request.urlretrieve(go_url, tar_path)
        
        print("[HF Space] Mengekstrak Go toolchain ke /tmp/go...", flush=True)
        with tarfile.open(tar_path, "r:gz") as tar:
            tar.extractall(path="/tmp")
            
        go_bin = "/tmp/go/bin/go"
        
        print(f"[HF Space] Mengompilasi server Go dengan Go v{go_ver}...", flush=True)
        env = os.environ.copy()
        env["CGO_ENABLED"] = "0"
        env["GOROOT"] = "/tmp/go"
        env["GOPATH"] = "/tmp/gopath"
        env["PATH"] = f"/tmp/go/bin:{env.get('PATH', '')}"
        
        build_cmd = [go_bin, "build", "-ldflags=-w -s", "-o", "server", "."]
        res = subprocess.run(build_cmd, env=env)
        if res.returncode != 0:
            print("[HF Space] ERROR: Kompilasi Go gagal!", flush=True)
            sys.exit(res.returncode)
            
        print("[HF Space] Kompilasi berhasil!", flush=True)
        
        # Bersihkan file tar installer
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
