"""Genera docs/THIRD_PARTY_NOTICES.md del pacchetto completo da catalogo, provenienza e versioni pip."""
import json, sys
from pathlib import Path

root, version = Path(sys.argv[1]), sys.argv[2]
cat = json.load(open(root / "manifest/tools.json", encoding="utf-8"))
prov = json.load(open(root / "manifest/provenance.json"))
freeze = {}
fz = root / "manifest/python-freeze.txt"
if fz.exists():
    for line in fz.read_text().splitlines():
        if "==" in line:
            k, v = line.split("==", 1)
            freeze[k.lower()] = v.strip()
T = {t["id"]: t for t in cat["tools"]}
rows = ["| Strumento | Versione | Licenza | Sorgente alla versione inclusa | SHA-256 del file originale | Verifica |", "|---|---|---|---|---|---|"]
for p in prov:
    t = T.get(p["tool"], {"name": p["tool"], "license": "vedi sorgente"})
    repo = p["source"].replace("github:", "")
    rows.append(f"| {t['name']} | {p['version']} | {t['license']} | https://github.com/{repo}/tree/{p['version']} | `{p['sha256']}` | "
                + ("digest GitHub confermato" if p.get("verified") else "release senza digest pubblicato") + " |")
for t in cat["tools"]:
    for pk in t.get("fetch", {}).get("pip", []):
        rows.append(f"| {t['name']} | {freeze.get(pk, '')} (PyPI) | {t['license']} | {t['homepage']} | vedi manifest\\python-freeze.txt | pip |")
excluded = [t["name"] for t in cat["tools"] if t.get("redistribute", True) is False]
text = f"""# Software di terze parti incluso in AFIP Portable {version}

AFIP Portable non modifica né ricompila questi programmi: sono i file pubblicati dagli autori nelle release ufficiali,
scaricati durante la build. L’SHA-256 di ogni file è confrontato, dove GitHub lo pubblica, con il digest dichiarato
nella release (`manifest\\provenance.json`). Ogni programma resta soggetto alla propria licenza; per quelli sotto GPL,
LGPL e AGPL il sorgente corrispondente è disponibile all’indirizzo indicato, alla versione esatta inclusa.

{chr(10).join(rows)}

## Licenze che richiedono attenzione

- **Volatility Software License** (Volatility 3) e **MVT License 1.1** (MVT): non sono licenze open source standard;
  MVT pone restrizioni d’uso. Leggerle prima dell’impiego professionale.
- **AGPL-3.0** (Velociraptor, MemProcFS, Hayabusa, Takajo): chi ridistribuisce versioni modificate deve pubblicarne il sorgente.

## Non inclusi

{', '.join(excluded) if excluded else 'nessuno'}: i repository non dichiarano una licenza, quindi non vengono ridistribuiti.
Si installano dal programma, menu ≡ → Aggiornamenti software, scaricandoli direttamente dalla fonte ufficiale.
"""
(root / "docs").mkdir(exist_ok=True)
(root / "docs/THIRD_PARTY_NOTICES.md").write_text(text, encoding="utf-8")
