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

## Strumenti

**Acquisizione**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [WinPmem](https://github.com/Velocidex/WinPmem) | Acquisizione della RAM di Windows | Apache-2.0 |
| [Velociraptor](https://github.com/Velocidex/velociraptor) | Raccolta e triage di artefatti (KAPE-like): avvia l'interfaccia locale nel browser | AGPL-3.0 |

**Disco e file system**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [The Sleuth Kit](https://github.com/sleuthkit/sleuthkit) | mmls, fls, icat, istat, tsk_recover | IPL/CPL/GPL |
| [bulk_extractor](https://github.com/simsong/bulk_extractor) | Estrazione di email, URL e altre feature da immagini | MIT |
| [DB Browser for SQLite](https://github.com/sqlitebrowser/sqlitebrowser) | Apertura e interrogazione di database SQLite (app, browser, chat) | GPL-3.0 / MPL-2.0 |
| [Thumbcache Viewer](https://github.com/thumbcacheviewer/thumbcacheviewer) | Miniature dei database thumbcache_*.db di Windows | GPL-3.0 |

**Memoria**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [Volatility 3](https://github.com/volatilityfoundation/volatility3) | Analisi di dump di memoria | Volatility Software License |
| [MemProcFS](https://github.com/ufrisk/MemProcFS) | Analisi di dump di memoria come file system | AGPL-3.0 |
| [PE-sieve](https://github.com/hasherezade/pe-sieve) | Rileva codice iniettato o modificato in un processo in esecuzione | BSD-2-Clause |
| [HollowsHunter](https://github.com/hasherezade/hollows_hunter) | Scansione di tutti i processi in esecuzione alla ricerca di impianti (basato su PE-sieve) | BSD-2-Clause |

**LEAPP (Brignoni)**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [ALEAPP](https://github.com/abrignoni/ALEAPP) | Parser di artefatti Android: senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [iLEAPP](https://github.com/abrignoni/iLEAPP) | Parser di artefatti iOS: senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [RLEAPP](https://github.com/abrignoni/RLEAPP) | Parser dei dati restituiti dai provider (Google, Apple, Meta…): senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [VLEAPP](https://github.com/abrignoni/VLEAPP) | Parser di artefatti dei veicoli: senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [DLEAPP](https://github.com/abrignoni/DLEAPP) | Parser di artefatti di applicazioni desktop (Electron/Chromium): senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [GLEAPP](https://github.com/abrignoni/GLEAPP) | Triage forense di immagini e video: senza argomenti apre la finestra, con argomenti lavora da riga di comando | MIT |
| [GLEAPP MapDownloader](https://github.com/abrignoni/GLEAPP-MapDownloader) | Ritaglia un'area della mappa Protomaps in un file .pmtiles per le mappe offline di GLEAPP | MIT |
| [Batch-LEAPP](https://github.com/abrignoni/batch-leapp) ¹ | Esegue un LEAPP su una cartella di estrazioni, con indice HTML e manifest SHA-256 (finestra; riga di comando dal tasto destro) | Non dichiarata nel repository |

**Mobile**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [MVT](https://github.com/mvt-project/mvt) | Indicatori di spyware su backup iOS e Android | MVT License 1.1 |

**Log e timeline**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [Hayabusa](https://github.com/Yamato-Security/hayabusa) | Timeline e rilevamenti Sigma su registri eventi Windows | AGPL-3.0 |
| [Takajo](https://github.com/Yamato-Security/takajo) | Analisi dei risultati di Hayabusa (riepiloghi, grafi di processi, IOC) | AGPL-3.0 |
| [Chainsaw](https://github.com/WithSecureOpenSource/chainsaw) | Hunting rapido su EVTX con regole Sigma (regole incluse) | GPL-3.0 |
| [evtx_dump](https://github.com/omerbenamram/evtx) | Conversione veloce di file EVTX in JSON o XML | MIT |

**Browser**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [Hindsight](https://github.com/RyanDFIR/hindsight) | Cronologia, download, cookie e cache di Chrome/Chromium | Apache-2.0 |

**Malware e IOC**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [YARA](https://github.com/VirusTotal/yara) | Ricerca di pattern con regole YARA | BSD-3-Clause |
| [YARA-X](https://github.com/VirusTotal/yara-x) | Nuovo motore YARA in Rust | BSD-3-Clause |
| [LOKI 2](https://github.com/Neo23x0/Loki-RS) | Scanner di IOC e regole YARA (signature-base) | GPL-3.0 |
| [capa](https://github.com/mandiant/capa) | Capacità di un eseguibile sospetto (ATT&CK) | Apache-2.0 |
| [FLOSS](https://github.com/mandiant/flare-floss) | Estrazione di stringhe anche offuscate da eseguibili | Apache-2.0 |
| [Detect It Easy](https://github.com/horsicq/DIE-engine) | Tipo di file, compilatore, packer e protezioni | MIT |

**Utilità**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [CyberChef](https://github.com/gchq/CyberChef) | Decodifica, conversione e analisi di dati (offline, nel browser) | Apache-2.0 |

**Immagini e video**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [Forensic Vision Pro](https://github.com/gaiacalamari/ForensicVisionPro) ¹ | Analisi forense di immagini e video: metadati, copy-move, PRNU, steganalisi, report | Non dichiarata nel repository |

**OSINT**

| Strumento | A cosa serve | Licenza |
|---|---|---|
| [Maigret](https://github.com/soxoj/maigret) | Ricerca di un username su centinaia di siti | MIT |

¹ Il repository non dichiara una licenza: non è incluso nel pacchetto completo, si installa dagli Aggiornamenti
scaricandolo direttamente dalla fonte ufficiale.

Lo strumento **Python portatile** ([python-build-standalone](https://github.com/astral-sh/python-build-standalone))
serve a Volatility 3 e MVT ed è gestito dagli Aggiornamenti.

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
