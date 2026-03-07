import os
import glob

# The directory to process
ROOT_DIR = r"e:\.agent\GOAgent"

# The replacement dictionary
REPLACEMENTS = {
    # Exact phrase replacements for logs/display
    "[GOTorch]": "[iTaK Torch]",
    "[GOAgent]": "[iTaK Agent]",
    "GOTORCH_DEBUG": "ITAK_TORCH_DEBUG",
    "GOTORCH_LIB": "ITAK_TORCH_LIB",
    
    # Generic brand replacements
    "GOTorch": "iTaK Torch",
    "GOAgent": "iTaK Agent",
    "GOSecurity": "iTaK Shield",
    "GOCloud": "iTaK Cloud",
    "GOProxy": "iTaK Proxy",
    
    # Lowercase CLI/binary matches
    "gotorch": "itaktorch",
    "goagent": "itakagent",
    
    # Module path edge cases (e.g. github imports should probably have no spaces)
    "github.com/David2024patton/GOAgent": "github.com/David2024patton/iTaKAgent",
    "github.com/David2024patton/iTaK Agent": "github.com/David2024patton/iTaKAgent", # Fix if it was replaced by generic rule
}

# Add special fix for variables that might get spaces injected by mistake by the generic rule
# e.g., type GOAgent struct -> type iTaK Agent struct (bad in Go)
# We will do a second pass to fix code syntax if needed, or refine our dictionary.
REPLACEMENTS_CODE_SAFE = {
    "iTaK Agent": "iTaKAgent",
    "iTaK Torch": "iTaKTorch",
    "iTaK Shield": "iTaKShield",
    "iTaK Cloud": "iTaKCloud",
    "iTaK Proxy": "iTaKProxy",
}

def process_file(filepath):
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            content = f.read()
            
        original_content = content
        
        # Pass 1: Generic brand replacements
        for old, new in REPLACEMENTS.items():
            content = content.replace(old, new)
            
        # Pass 2: If it's a Go file, we don't want spaces in identifiers.
        if filepath.endswith('.go'):
            for old, new in REPLACEMENTS_CODE_SAFE.items():
                content = content.replace(old, new)
                
            # Bring back the nice spaced versions for console prints inside quotes
            content = content.replace("[iTaKTorch]", "[iTaK Torch]")
            content = content.replace("[iTaKAgent]", "[iTaK Agent]")

        # Pass 3: Fix module path just in case
        content = content.replace("github.com/David2024patton/iTaKAgent", "github.com/David2024patton/iTaKAgent")

        if content != original_content:
            with open(filepath, 'w', encoding='utf-8') as f:
                f.write(content)
            print(f"Updated: {filepath}")
            
    except Exception as e:
        print(f"Error processing {filepath}: {e}")

def main():
    extensions = ['*.go', '*.md', '*.yaml', '*.ps1', 'go.mod']
    for ext in extensions:
        pattern = os.path.join(ROOT_DIR, '**', ext)
        for filepath in glob.glob(pattern, recursive=True):
            # Skip vendor dir if it exists to avoid touching dependencies
            if '\\vendor\\' in filepath or '\\.git\\' in filepath:
                continue
            process_file(filepath)

if __name__ == "__main__":
    main()
