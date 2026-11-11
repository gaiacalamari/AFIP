# AFIP Portable

Toolkit forense **portatile per Windows**: un pannello verticale flottante che avvia con un clic gli strumenti di
digital forensics e incident response tenuti su una pendrive. Pensato per le attività esterne: niente installazione,
niente configurazione.

![Licenza](https://img.shields.io/badge/licenza-MIT-blue)

- **33 strumenti** pubblicati su GitHub, tra cui tutta la famiglia LEAPP di Alexis Brignoni
- **Aggiornamenti integrati**: controlla le release ufficiali, verifica l’SHA-256 pubblicato da GitHub, installa
- **Strumenti personali**: aggiungi i tuoi eseguibili al pannello
- **Integrità**: ogni eseguibile è sigillato e controllato prima dell’avvio; ogni esecuzione finisce in un registro a catena di hash
- Un solo eseguibile Go senza dipendenze (API Win32), interfaccia scura, sempre in primo piano, riducibile a barra

## Download

Dalla pagina [**Releases**](https://github.com/TUO-UTENTE/afip-portable/releases/latest):

| Pacchetto | Contenuto | Per chi |
|---|---|---|
| `AFIP-Portable-<versione>-completo.zip` | launcher + strumenti già installati e sigillati (~1,4 GB) | uso immediato, anche offline |
| `AFIP-Portable-<versione>-leggero.zip` | solo launcher e catalogo (pochi MB) | chi preferisce scaricare ogni strumento dalla sua fonte ufficiale |

Uso: estrarre lo zip su una pendrive (exFAT), aggiungere un’esclusione antivirus per la cartella `tools` sulla propria
postazione e aprire `AFIP-Portable.exe`. Con il pacchetto leggero, la prima volta: menu ≡ → **Aggiornamenti software** →
*Seleziona tutti* → *Aggiorna selezionati*. Guida completa: [docs/GUIDA.md](docs/GUIDA.md).

## Come è fatto

```
src/              launcher in Go (solo libreria standard, API Win32 via syscall), motore di aggiornamento, test
manifest/         tools.json: catalogo degli strumenti (fonte, file da scaricare, eseguibile, tipo)
runtime/bin/      comandi vol, mvt… che usano il Python portatile
build/            build_release.sh: costruisce i pacchetti leggero e completo
docs/             guida
.github/workflows ci.yml (test e compilazione), release.yml (pacchetti allegati a ogni release)
```

Aggiungere uno strumento al catalogo: una voce in `manifest/tools.json` con `id`, `name`, `category`, `kind` (`gui` o
`cli`), `exe` (nome del file da avviare) e `fetch` (`github`: repository, `assets`: espressione regolare del file Windows
nella release, `unpack`: `zip`, `gz`, `tar.gz` o `copy`).

### Compilare

```
cd src
go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H=windowsgui" -o AFIP-Portable.exe .
bash ../build/build_release.sh v0.0.0-test     # pacchetti completi in dist/ (serve Linux con Go, Python 3 e zip)
```

## Avvertenze

- Ogni programma avviato su un sistema acceso lascia tracce (Prefetch, Amcache, BAM, cronologia USB): il registro di AFIP
  Portable serve a documentarle. Acquisire per prima la RAM e scrivere gli output su un supporto diverso dalla pendrive.
- `AFIP-Portable.exe` non è firmato: Windows SmartScreen può avvisare al primo avvio. Verificarne l’SHA-256.
- Gli strumenti di terze parti restano soggetti alle proprie licenze (vedi `docs/THIRD_PARTY_NOTICES.md` nel pacchetto
  completo). In particolare MVT (MVT License) e Volatility 3 (Volatility Software License) hanno condizioni proprie.

## Licenza

Codice di AFIP Portable: [MIT](LICENSE). Sicurezza e segnalazioni: [SECURITY.md](SECURITY.md).
