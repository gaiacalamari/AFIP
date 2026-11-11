# Sicurezza

## Strumenti

- Solo release ufficiali pubblicate dagli autori su GitHub (Volatility 3 e MVT da PyPI). Nessun binario è ricompilato o modificato.
- Ogni file scaricato è confrontato con l’SHA-256 che GitHub pubblica per la release; se non corrisponde viene scartato.
  Per le poche release senza digest pubblicato l’hash viene comunque registrato in `manifest/provenance.json`.
- I pacchetti pubblicati qui sono costruiti da GitHub Actions (`.github/workflows/release.yml`) a partire da questo
  repository: il procedimento è leggibile e ripetibile (`build/build_release.sh`).
- Dopo l’installazione ogni file è sigillato: AFIP Portable blocca l’avvio di un eseguibile modificato.

## Verificare un pacchetto scaricato

Confrontare l’SHA-256 dello zip con `SHA256SUMS.txt` allegato alla stessa release:

```powershell
Get-FileHash .\AFIP-Portable-v1.3.0-completo.zip -Algorithm SHA256
```

## Segnalare un problema

Per vulnerabilità del launcher (non degli strumenti di terze parti) usare la funzione
“Report a vulnerability” nella scheda **Security** del repository, non le issue pubbliche.
Per problemi di uno strumento incluso, segnalarlo al progetto originale indicato nel catalogo.
