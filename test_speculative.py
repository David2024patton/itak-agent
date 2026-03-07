import subprocess
import time
import requests
import sys

def run_test(name, cmd, prompt="Write a haiku about the sunrise. Be creative and original.", is_prefix_caching=False):
    print(f"\n======================================")
    print(f"--- Starting {name} ---")
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    
    # Wait for server to load the models
    time.sleep(5)
    
    # Run test
    runs = 2 if is_prefix_caching else 1
    
    for i in range(runs):
        print(f"[{name}] Sending request {i+1}...")
        start = time.time()
        try:
            resp = requests.post("http://localhost:8080/v1/chat/completions", json={
                "model": "test",
                "messages": [{"role": "user", "content": prompt}],
                "max_tokens": 100
            })
            if resp.status_code != 200:
                print(f"Request Error: {resp.text}")
        except Exception as e:
            print(f"Request failed: {e}")
        
        end = time.time()
        print(f"[{name}] Roundtrip {i+1} complete in {end-start:.2f}s")
        
        # Give it a second before the next request
        if is_prefix_caching:
            time.sleep(2)
    
    p.terminate()
    try:
        p.wait(timeout=5)
    except:
        p.kill()
        
    out, _ = p.communicate()
    for line in out.splitlines():
        if "tok/s" in line or "PromptTokens" in line or "[GOTorch]" in line:
            print(line)

print("Compiling gotorch.exe...")
subprocess.run(["go", "build", "-o", "gotorch.exe", "cmd/gotorch/main.go"])

backend = "cpu"
if len(sys.argv) > 1:
    backend = sys.argv[1]
    
print(f"Benchmarking with backend: {backend}")

# 1. Baseline CPU / GPU
run_test("1. Baseline", ["./gotorch.exe", "serve", "--model", "models/qwen2.5-0.5b-instruct-q4_k_m.gguf", "--gpu-layers", "-1", "--backend", backend, "--port", "8080"])

# 2. Prefix Caching Test
# Prompt should be much longer to show the impact of prefix caching
long_prompt = "You are an expert system. " * 50 + "Write a haiku about the sunrise. Be creative and original."
run_test("2. Prefix Caching (Disabled)", ["./gotorch.exe", "serve", "--model", "models/qwen2.5-0.5b-instruct-q4_k_m.gguf", "--gpu-layers", "-1", "--backend", backend, "--port", "8080", "--prefix-cache-size", "0"], prompt=long_prompt, is_prefix_caching=True)
run_test("3. Prefix Caching (Enabled)", ["./gotorch.exe", "serve", "--model", "models/qwen2.5-0.5b-instruct-q4_k_m.gguf", "--gpu-layers", "-1", "--backend", backend, "--port", "8080", "--prefix-cache-size", "16"], prompt=long_prompt, is_prefix_caching=True)

