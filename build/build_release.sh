#!/usr/bin/env bash
# Costruisce i pacchetti di AFIP Portable (usato da GitHub Actions, funziona anche in locale su Linux).
#   bash build/build_release.sh v1.3.0
# Requisiti: Go >= 1.22, Python 3 con pip, zip, accesso a github.com e pypi.org.
# Produce in dist/:
#   AFIP-Portable-<versione>-leggero.zip   launcher + catalogo: gli strumenti si installano dagli Aggiornamenti
#   AFIP-Portable-<versione>-completo.zip  con gli strumenti ridistribuibili gia' inclusi, verificati e sigillati
#   SHA256SUMS.txt
set -euo pipefail
VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$DIST/work"
rm -rf "$DIST"; mkdir -p "$WORK"

echo "== test e compilazione"
cd "$ROOT/src"
go test ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$WORK/AFIP-Portable.exe" .
CGO_ENABLED=0 go build -trimpath -o "$WORK/afip-helper" .   # stessa logica, eseguibile su Linux per popolare il pacchetto

base_layout() {   # $1 = cartella AFIP-Portable da creare
  mkdir -p "$1/manifest" "$1/runtime" "$1/docs" "$1/logs" "$1/output" "$1/tools"
  cp "$WORK/AFIP-Portable.exe" "$1/"
  cp "$ROOT/manifest/tools.json" "$1/manifest/"
  cp -r "$ROOT/runtime/bin" "$1/runtime/"
  cp "$ROOT/docs/GUIDA.md" "$1/docs/"
  cp "$ROOT/LICENSE" "$1/LICENSE.txt"
  touch "$1/logs/.keep" "$1/output/.keep" "$1/tools/.keep"
  (cd "$1" && sha256sum AFIP-Portable.exe > SHA256SUMS.txt)
}

finish() {        # $1 = cartella, $2 = nome dello zip
  "$WORK/afip-helper" -base "$1" -seal >/dev/null
  python3 - "$1/manifest/sealed.json" "$VERSION" <<'PY'
import json, sys
p, v = sys.argv[1], sys.argv[2]
d = json.load(open(p)); d["host"] = "build GitHub Actions"; d["user"] = "AFIP Portable " + v
json.dump(d, open(p, "w"), indent=2)
PY
  rm -rf "$1/logs/"*; touch "$1/logs/.keep"
  (cd "$(dirname "$1")" && zip -qr -6 "$DIST/$2" "$(basename "$1")")
  unzip -tq "$DIST/$2" >/dev/null
  echo "   $2: $(du -h "$DIST/$2" | cut -f1)"
}

echo "== pacchetto leggero"
LIGHT="$WORK/leggero/AFIP-Portable"
base_layout "$LIGHT"
cat > "$LIGHT/LEGGIMI.txt" <<TXT
AFIP Portable $VERSION - pacchetto leggero

1. Copiare questa cartella su una pendrive (exFAT).
2. Aprire AFIP-Portable.exe su un PC connesso a internet (la propria postazione).
3. Menu (tre righe) -> Aggiornamenti software -> Seleziona tutti -> Aggiorna selezionati.
   Ogni strumento viene scaricato dalla sua release ufficiale, verificato con l'SHA-256 pubblicato e sigillato.
4. Sulla propria postazione aggiungere all'antivirus un'esclusione per la cartella tools.

Guida: docs\GUIDA.md
TXT
finish "$LIGHT" "AFIP-Portable-$VERSION-leggero.zip"

echo "== pacchetto completo"
FULL="$WORK/completo/AFIP-Portable"
base_layout "$FULL"
# strumenti GitHub ridistribuibili (redistribute != false); i pacchetti pip si installano dopo il Python
IDS=$(python3 - "$ROOT/manifest/tools.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print(",".join(t["id"] for t in d["tools"] if "github" in t.get("fetch", {}) and t.get("redistribute", True) is not False))
PY
)
PIP=$(python3 - "$ROOT/manifest/tools.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print(" ".join(p for t in d["tools"] if t.get("redistribute", True) is not False for p in t.get("fetch", {}).get("pip", [])))
PY
)
"$WORK/afip-helper" -base "$FULL" -update "$IDS"
SP=$(ls -d "$FULL"/tools/python/*/python/Lib/site-packages | head -1)
echo "   pip (pacchetti Windows) in $SP: $PIP"
python3 -m pip install -q --disable-pip-version-check --target "$SP" --platform win_amd64 --python-version 3.12 \
  --implementation cp --only-binary=:all: $PIP
rm -rf "$SP/bin"
python3 -m pip freeze --path "$SP" > "$FULL/manifest/python-freeze.txt"
python3 "$ROOT/build/notices.py" "$FULL" "$VERSION"
cat > "$FULL/LEGGIMI.txt" <<TXT
AFIP Portable $VERSION - pacchetto completo

1. Copiare questa cartella su una pendrive (exFAT).
2. Sulla propria postazione aggiungere all'antivirus un'esclusione per la cartella tools.
3. Doppio clic su AFIP-Portable.exe: un clic su uno strumento lo avvia.

Non inclusi per motivi di licenza (si installano da menu -> Aggiornamenti software): $(python3 -c "import json;print(', '.join(t['name'] for t in json.load(open('$ROOT/manifest/tools.json'))['tools'] if t.get('redistribute',True) is False))")
Guida: docs\GUIDA.md   Licenze e fonti: docs\THIRD_PARTY_NOTICES.md
TXT
finish "$FULL" "AFIP-Portable-$VERSION-completo.zip"

(cd "$DIST" && sha256sum *.zip > SHA256SUMS.txt && cat SHA256SUMS.txt)
rm -rf "$WORK"
