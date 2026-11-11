# AFIP Portable · guida

Toolkit forense da pendrive per Windows 10 e 11, per le attività esterne. Doppio clic su `AFIP-Portable.exe`: si apre
un pannello verticale con gli strumenti presenti nella cartella. Un clic e lo strumento parte. Nessuna installazione.

## Il pannello

- **Clic su uno strumento**: lo avvia. I programmi a finestra si aprono; quelli a riga di comando aprono una console con
  lo strumento nel PATH, la cartella `output` come cartella di lavoro e un esempio d’uso.
- **Tasto destro**: Avvia, Apri console, cartella dello strumento, pagina ufficiale, informazioni con SHA-256.
- **Ricerca**: basta scrivere; frecce per scegliere, Invio per avviare, Esc per cancellare, Ctrl+F per tornare alla ricerca.
- **Categorie**: clic sul titolo per chiuderle o aprirle.
- **Pulsanti**: ≡ menu · pin “sempre in primo piano” · ˄ riduce a barra · × chiude. Si trascina dall’intestazione e si
  ridimensiona dai bordi; posizione e preferenze vengono ricordate.
- **Pallino**: verde = pronto, hash verificato · rosso = eseguibile modificato · azzurro vuoto = presente ma non sigillato.

## Aggiornamenti software (menu ≡)

Controlla su GitHub (PyPI per Volatility 3 e MVT) l’ultima versione di ogni strumento e porta in cima, già selezionati,
quelli da installare o aggiornare. “Aggiorna selezionati”:

1. scarica il file dalla release ufficiale;
2. lo confronta con l’SHA-256 pubblicato da GitHub e lo scarta se non corrisponde;
3. prepara la nuova versione a parte e sostituisce la vecchia solo se tutto è andato bene;
4. aggiorna il sigillo e annota nel registro versione precedente, nuova versione e fonte.

Con il **pacchetto leggero** si usa la prima volta così: “Seleziona tutti” → “Aggiorna selezionati”. Volatility 3 e MVT
richiedono il **Python portatile**, che compare nello stesso elenco e va installato per primo (l’ordine è già quello giusto).

Serve internet: usarlo sulla propria postazione, non sul sistema in esame. I programmi da aggiornare devono essere chiusi.
Senza token GitHub concede 60 richieste l’ora; se finiscono, AFIP legge le stesse informazioni dalla pagina pubblica della
release. Da riga di comando: `AFIP-Portable.exe -check all` e `AFIP-Portable.exe -update all`.

## Strumenti personali (menu ≡)

1. “Aggiungi eseguibile…” e scegliere il file (.exe, .bat, .cmd o .html);
2. indicare nome e categoria (esistente o nuova; predefinita “I miei strumenti”);
3. scegliere se copiare solo il file o l’intera cartella (consigliato se il programma usa DLL o file accanto).

AFIP riconosce da solo se è un programma a finestra o da console, lo copia in `tools\custom-…`, lo sigilla e lo mostra
nel pannello. Si rimuove dalla stessa schermata o con il tasto destro; il file originale non viene toccato.

## Cosa fa da solo

- **Controlla ogni eseguibile prima di avviarlo** contro il sigillo (`manifest\sealed.json`). Se un file è stato alterato
  l’avvio si blocca; per avviarlo comunque si scrive una motivazione, che resta nel registro.
- **Registra** avvii, aggiornamenti e strumenti aggiunti in `logs\launch-log.jsonl` (data UTC, postazione, utente,
  strumento, hash), con una catena di hash che rende evidente ogni modifica successiva.
- `AFIP-Portable.exe -verify` controlla toolkit e registro da riga di comando (codice 0 = integro, 2 = differenze).

## Prima dell’uso sul campo

1. **Antivirus**: sulla propria postazione aggiungere un’esclusione per la cartella `tools`. WinPmem, Velociraptor,
   PE-sieve, HollowsHunter, Maigret e le regole YARA/Sigma vengono talvolta segnalati, anche durante la copia.
2. **SmartScreen**: AFIP-Portable.exe non è firmato. Al primo avvio “Ulteriori informazioni → Esegui comunque”, dopo aver
   confrontato l’SHA-256 con `SHA256SUMS.txt` (lo mostra anche menu ≡ → Informazioni).
3. **Volatility 3** per i dump Windows scarica i simboli da Microsoft al primo uso; senza rete servono i pacchetti di
   simboli della Volatility Foundation nella cartella `volatility3\symbols` del Python portatile.
4. **MemProcFS** per montare il dump come unità richiede il driver Dokany installato sul PC.
5. **Pendrive**: exFAT, almeno 4 GB liberi oltre al toolkit per output e aggiornamenti.

## Attenzioni sul sistema in esame

Ogni programma avviato lascia tracce (Prefetch, Amcache, ShimCache, BAM, cronologia USB) e occupa memoria: acquisire per
prima la RAM (WinPmem) e scrivere gli output su un supporto diverso dalla pendrive dei tool. Aprire i database SQLite solo
da copie. Il registro documenta ogni avvio con ora e hash.
