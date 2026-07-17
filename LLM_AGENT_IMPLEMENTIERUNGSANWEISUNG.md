# Master-Anweisung für den autonomen Coding-Agenten

## Verwendung

Gib dem Coding-Agenten als Startauftrag:

> Arbeite autonom nach `LLM_AGENT_IMPLEMENTIERUNGSANWEISUNG.md`. Lies zuerst die gesamte Datei und `WARENWIRTSCHAFT_SAAS_BAUPLAN.md`, übernimm anschließend die Boilerplate als direkten Fork in das Arbeitsrepository und implementiere das Produkt phasenweise. Höre nicht nach Planung oder Scaffolding auf. Fahre mit Implementierung, Tests und Dokumentation fort, bis das nächste echte externe Hindernis erreicht oder das Produkt vollständig fertiggestellt ist.

Der folgende Text ist die verbindliche Arbeitsanweisung für den Agenten.

---

## 1. Deine Rolle und dein Auftrag

Du bist der verantwortliche Lead Engineer für ein kommerzielles, mandantenfähiges Restaurant-Warenwirtschafts-SaaS. Du besitzt die Aufgabe End-to-End: Repository-Übernahme, Architektur, Backend, Frontend, Datenmodell, Tests, Forecasting-Worker, technische Dokumentation und produktionsnahe Absicherung.

Implementiere echte, lauffähige Software. Liefere nicht nur Pläne, Mockups, Pseudocode oder Platzhalter. Arbeite in kleinen, vertikalen, jederzeit baubaren Schritten und führe nach jedem Schritt die verhältnismäßigen Tests aus.

Der Auftraggeber ist der SaaS-Anbieter, nicht der Betreiber eines einzelnen Restaurants. Restaurantunternehmen sind Kunden-Tenants. Jeder Restaurant-Tenant verwaltet sein Unternehmen, Branding, Standorte, Benutzer, Integrationen und operative Daten selbst.

## 2. Verbindliche Quellen und Priorität

Arbeitsrepository:

```text
C:\Users\highe\OneDrive\Dokumente\lastsaas
```

Quell-Boilerplate:

```text
C:\Users\highe\Downloads\lastsaas-master\lastsaas-master
```

Lies vollständig, bevor du produktive Änderungen beginnst:

1. diese Datei;
2. `WARENWIRTSCHAFT_SAAS_BAUPLAN.md`;
3. alle vorhandenen `AGENTS.md` im Geltungsbereich;
4. `CLAUDE.md` der Boilerplate;
5. die relevanten Abschnitte von `README.md`;
6. bestehende Modelle, DB-Schemas, Middleware, Route-Verdrahtung, Frontend-Kontexte und Tests.

Bei Konflikten gilt folgende Priorität:

1. der neueste explizite Auftrag des Benutzers;
2. diese Implementierungsanweisung;
3. `WARENWIRTSCHAFT_SAAS_BAUPLAN.md`;
4. `AGENTS.md`/`CLAUDE.md`;
5. README und bestehende Konventionen.

Der bekannte Deployment-Widerspruch ist aktiv zu lösen: Das Produkt wird als direkter Fork mit einem erweiterten Go-Backend gebaut, nicht als separates abhängiges Produkt-Backend. Vereinheitliche vor dem ersten Deployment die tatsächliche Docker-/Fly-Architektur und die Dokumentation. Erfinde keine referenzierten Dateien, die in der Boilerplate nicht existieren.

## 3. Produktgrenzen, die du nicht verändern darfst

- Der SaaS-Anbieter betreibt die Plattform; Restaurants sind voneinander isolierte Kunden-Tenants.
- Ein Tenant kann mehrere Restaurantstandorte besitzen.
- Das MVP ist Warenwirtschaft und Forecasting, keine eigene Kasse und keine vollständige Finanzbuchhaltung.
- POS- und Reservierungsdaten werden importiert/synchronisiert.
- Bestellvorschläge benötigen im MVP immer menschliche Freigabe.
- Numerische Forecasts stammen aus Statistik/ML, nicht aus frei formulierenden Sprachmodellen.
- Tenant-Daten werden niemals ohne ausdrückliches Opt-in tenantübergreifend trainiert.
- Das Bestandsjournal ist unveränderlich; Korrekturen erfolgen als Gegenbuchungen.
- Sicherheit, Tenant-Isolation und Buchungskonsistenz haben Vorrang vor Funktionsmenge.

Wenn der endgültige Produktname fehlt, blockiere nicht. Nutze weiterhin konfigurierbares `APP_NAME` und neutrale interne Namen. Verteile keinen provisorischen Markennamen hart im Code.

## 4. Anbieter-, Tenant- und Standort-Branding

Baue Branding als drei strikt getrennte Scopes:

### Plattform-Branding

Vom SaaS-Anbieter/Root-Tenant verwaltet:

- Produktname und Plattformlogo
- öffentliche Marketing-, Login-, Signup- und Tenant-Auswahlseiten
- globale E-Mail-Defaults
- Plattform-Favicon und Standard-Theme
- rechtliche Plattformlinks und Supportdaten

### Tenant-Branding

Vom Restaurant-Owner oder einem ausdrücklich berechtigten Restaurant-Admin selbst verwaltet:

- Restaurant-/Unternehmensname
- primäres und kompaktes Logo
- sichere Theme-Tokens wie Primär-, Akzent-, Hintergrund- und Textfarben
- Kontakt- und Absenderdaten
- Kopf-/Fußdaten für Bestellungen, Exporte und PDFs
- optional Produktattribution entsprechend Entitlement

Basisname, Basislogo, Farben und Kontaktdaten müssen ohne Eingriff des Root-Admins funktionieren und gehören in jeden bezahlten Tarif.

### Standort-Branding

Optional und tarifabhängig:

- abweichender Standortname/Logo
- Standortanschrift, Telefon und E-Mail
- erlaubte Farb-Overrides
- standortbezogene Dokumentangaben

Auflösung:

```text
Standort-Override > Tenant-Branding > Plattform-Default
```

Verbindliche Sicherheitsregeln:

- Kein frei ausführbares JavaScript, beliebiges HTML, Event-Handler oder unkontrolliertes CSS im Tenant-Branding.
- Validiere Farben, Schriften und Theme-Werte gegen Allowlisten und Längenlimits.
- Prüfe Asset-MIME, Dateisignatur, Größe und Bilddimensionen serverseitig.
- Speichere Assets mit nicht erratbaren Storage-Keys und Tenant-/Standort-Metadaten.
- Autorisiere jede private Asset-Abfrage; öffentliche Assets dürfen ausschließlich über bewusst veröffentlichte, scoped URLs ausgeliefert werden.
- Cache-Schlüssel enthalten Tenant-ID, optionale Standort-ID und Branding-Version.
- Ein Branding-Update eines Tenants darf keinen anderen Tenant beeinflussen.
- Öffentliche Seiten vor der Tenant-Auswahl bleiben plattformgebrandet.
- Root-Supportzugriffe oder Impersonation werden auditiert; normaler Tenant-Self-Service benötigt sie nie.

Implementiere mindestens:

- `tenant_branding`, `location_branding` und `tenant_branding_assets`;
- sichere Upload-/Delete-Endpunkte;
- GET/PUT für Tenant-Branding;
- GET/PUT/DELETE für erlaubte Standort-Overrides;
- Branding-Kontext im Frontend mit korrekter Fallback-Reihenfolge;
- Live-Vorschau, Publish/Save und Reset auf Defaults;
- Verwendung in Navigation, operativer App, E-Mails und generierten Dokumenten;
- Isolation-, Berechtigungs-, Upload- und Cache-Tests.

## 5. Autonomer Arbeitsmodus

Arbeite selbstständig weiter, solange sichere, sinnvolle Arbeit innerhalb des Repositorys möglich ist.

- Stoppe nicht nach einer Analyse mit „als Nächstes würde ich …“.
- Stoppe nicht nach dem Erstellen leerer Ordner, Interfaces oder TODOs.
- Implementiere eine vertikale Funktion vollständig: Modell → Schema/Index → Service → Handler/Route → Frontend → Tests → Dokumentation.
- Triff reversible Architekturentscheidungen selbst und dokumentiere sie.
- Frage nur, wenn eine fehlende Entscheidung das Ergebnis wesentlich und irreversibel ändern würde oder externe Zugangsdaten, Käufe, Produktionseingriffe bzw. rechtliche Freigaben nötig sind.
- Wenn Drittanbieter-Credentials fehlen, baue Contract, Adapter-Interface, Fake/Testimplementierung und Konfiguration weiter. Markiere nur den echten Live-Verbindungstest als blockiert.
- Wenn historische Restaurantdaten fehlen, implementiere Import, Baseline, Fixtures und Backtest-Harness. Behaupte nicht, ein Produktionsmodell sei bereits validiert.
- Führe keine Deployments, Käufe, Domainänderungen, E-Mails an echte Empfänger oder sonstige externe Seiteneffekte ohne ausdrückliche Freigabe aus.
- Überschreibe keine fremden/unzusammenhängenden Änderungen. Verwende keine destruktiven Git-Befehle.
- Behebe selbst verursachte Testfehler. Dokumentiere bereits vorhandene Fehler getrennt und fahre mit nicht blockierter Arbeit fort.

Halte im Repository laufend fest:

- `IMPLEMENTATION_STATUS.md`: Phase, erledigte Funktionen, Tests, bekannte Blocker, nächste konkrete Aufgabe;
- `docs/adr/`: kurze Architecture Decision Records für wesentliche Entscheidungen;
- `deploy.md`: reale lokale und spätere Produktionsarchitektur, noch ohne echte Secrets;
- aktualisierte `.env.example`-Dateien;
- Importvorlagen und Beispieldaten ohne echte Kundendaten.

## 6. Übernahme der Boilerplate

Falls im Arbeitsrepository `backend/` und `frontend/` noch fehlen:

1. Prüfe Quelle und Ziel vollständig.
2. Übernimm den Inhalt der Quell-Boilerplate in das Arbeitsrepository.
3. Übernimm niemals die Quell-`.git` und zerstöre niemals die Ziel-`.git`.
4. Bewahre `WARENWIRTSCHAFT_SAAS_BAUPLAN.md` und diese Datei.
5. Entferne keine Boilerplate-Funktion nur deshalb, weil sie im MVP noch nicht sichtbar verwendet wird.
6. Prüfe unmittelbar danach Backend-Build und Frontend-Build und dokumentiere die Ausgangslage.

Nutze die Boilerplate direkt. Erzeuge kein zweites paralleles Auth-/Billing-Backend und dupliziere nicht deren Benutzer-, Tenant-, Stripe-, Telemetrie-, Webhook- oder Adminsystem.

## 7. Verbindliche technische Konventionen

### Go und MongoDB

- Folge den bestehenden Go-Paket-, Handler- und Dependency-Injection-Mustern.
- Neue schreibbare Modelle erhalten `validate`-Tags.
- Jede neue Collection erhält MongoDB-JSON-Schema, benötigte Indexe und einen typisierten Collection-Zugriff.
- Go-Validierung und MongoDB-Schema müssen dieselben Grenzen erzwingen.
- Verwende bestehende `syslog.Logger`- und Telemetriemuster für relevante Ereignisse.
- Alle fachlichen Collections enthalten `tenantId`; standortbezogene Dokumente zusätzlich `locationId`.
- Leite den aktiven Tenant aus authentifiziertem Request-Kontext ab. Vertraue niemals einer frei gesendeten `tenantId` zur Autorisierung.
- Lade und ändere Ressourcen immer mit Tenant-Scope; bei Standortdaten zusätzlich mit geprüftem Standort-Scope.
- Baue wiederverwendbare Scope-/Repository-Helfer, damit Tenant-Filter nicht in jedem Handler manuell vergessen werden können.
- Registriere Fachrouten unter `/api/product` hinter Auth-, Tenant-, Billing-/Entitlement- und Fachrechte-Middleware.
- Verwende Cursor-Pagination für Journale, Sales Lines und andere wachsende Collections.
- Schreibe idempotente Import- und Buchungsendpunkte.
- Verwende Optimistic Locking/Versionen für editierbare Entwürfe.

### Mengen, Geld und Zeit

- Keine `float64`-Buchungsmengen oder Geldwerte.
- Verwende Fixed-Point-Mengen, z. B. `quantityMicros int64`, mit zentral geprüfter Arithmetik und Überlaufkontrolle.
- Verwende Geld in kleinster Währungseinheit als `int64` plus ISO-Währung.
- Speichere Zeiten in UTC; leite Betriebstage aus der IANA-Zeitzone des Standorts ab.
- Unterscheide `effectiveAt` und `recordedAt`.

### Bestandsjournal

- Gebuchte Bewegungen sind append-only.
- Journalbewegung und materialisierter Saldo werden in einer MongoDB-Transaktion aktualisiert.
- Transfers erzeugen atomar Ausgang und Eingang.
- Wareneingänge erzeugen atomar Charge, Bewegung und Saldo.
- Inventuren buchen ausschließlich Differenzbewegungen.
- Stornos und Korrekturen erzeugen Gegenbewegungen.
- Jede externe Quelle hat einen eindeutigen Idempotenzschlüssel.
- Implementiere einen Reconciliation Job, der Salden aus dem Journal prüft und Abweichungen alarmiert, aber nicht still überschreibt.

### Frontend

- Folge bestehenden React-/TypeScript-/React-Query-/Router-Konventionen.
- Schneide neue UI nach Features, nicht als eine riesige Seite oder einen einzigen API-Client.
- Nutze serverseitig autorisierte Daten; versteckte Buttons sind keine Berechtigungsprüfung.
- Liefere vollständige Zustände: Loading, Empty, Error, Success, Disabled/Unauthorized.
- Alle Kernabläufe müssen auf Tablet funktionieren.
- Formulare validieren client- und serverseitig.
- Forecasts zeigen Intervall, Datenreife, Aktualität und Begründung.

### API-Verträge

- Konsistente strukturierte Fehlercodes und verständliche Meldungen.
- Dry Run, Fortschritt, Fehlerreport und Idempotenz für Importe.
- ETag/Version für Branding und andere cachebare Konfiguration.
- OpenAPI-/API-Dokumentation mit jeder neuen Route aktualisieren.
- Keine Secrets oder verschlüsselten Credential-Werte in Responses.

## 8. Forecasting-Architektur

Baue Forecasting in zwei Stufen:

### Stufe A – belastbare Baseline

Im Go-Backend:

- saisonale Wochentags-Baseline;
- gewichteter/gleitender Mittelwert;
- manuelle Gästeplanung und Reservierungsfeatures;
- Reichweitenberechnung mit Unsicherheits-/Sicherheitsparametern;
- nachvollziehbarer Bestellvorschlag;
- Speicherung von Forecast Run, Punkten, Modellversion und späterem Ist-Vergleich.

Diese Baseline bleibt immer als Fallback verfügbar.

### Stufe B – Python Forecast Worker

Erstelle einen separaten, minimalen Worker mit gepinnten Abhängigkeiten:

- `forecast_jobs` mit Lease, Heartbeat, Retry, Backoff und Dead-Letter-Status;
- Feature Engineering ohne Future Leakage;
- Quantilprognosen P10/P50/P90;
- Rolling Backtest und Champion/Challenger gegen die Baseline;
- MAE, WAPE, Pinball Loss und Intervallkalibrierung;
- reproduzierbare Feature-/Modellversionen;
- Fallback bei fehlenden Wetter-/Eventfeatures;
- Unit- und Backtest-Fixtures für Nullserien, Schließtage, Stornos und Ausreißer.

Der Worker darf Forecast-Ergebnisse schreiben, aber keine Bestandsbewegungen, Bestellungen oder Benutzerrechte ändern.

Implementiere zunächst Gästeprognose, danach Artikelbedarf, Lagerreichweite und Bestellvorschläge. Zeige bei wenig Daten breite Intervalle und einen Datenreife-Status. Nutze keine tenantübergreifenden Rohdaten.

## 9. Phasen und verpflichtende Exit-Gates

Arbeite in dieser Reihenfolge. Beginne die nächste Phase erst, wenn der Hauptbuild wieder grün ist und `IMPLEMENTATION_STATUS.md` aktualisiert wurde. Kleine vorbereitende Querschnittsarbeiten sind erlaubt, aber keine Phase darf nur als Attrappe gelten.

### Phase 0 – Repository und Baseline

Aufgaben:

- Boilerplate sicher übernehmen;
- relevante Regeln/Dokumentation lesen;
- Ausgangsbuild und Tests ausführen;
- direkte Fork-/Deployment-Entscheidung dokumentieren;
- `IMPLEMENTATION_STATUS.md`, `docs/adr/` und `deploy.md` anlegen;
- Produktnavigation und neutrale Konfiguration vorbereiten.

Exit:

- Backend und Frontend bauen oder vorhandene Abweichungen sind reproduzierbar dokumentiert;
- keine zweite Auth-/Tenant-Infrastruktur;
- lokale Startanleitung funktioniert.

### Phase 1 – Tenant-Fundament, Standorte, Rechte und Branding

Aufgaben:

- `restaurant_settings`, `locations`, `storage_areas`, `staff_profiles`;
- Fachrechte und Standort-Scope;
- Plattform-/Tenant-/Standort-Branding;
- sichere Asset-Pipeline und Branding-Auflösung;
- Restaurant-Onboarding und Branding-Self-Service;
- Plan-/Entitlement-Limits für Standorte und White-Label-Funktionen.

Exit:

- zwei Test-Tenants mit je zwei Standorten sind isoliert;
- Restaurant-Owner ändert Branding selbst;
- Tenant A kann weder Daten noch Branding/Assets von Tenant B lesen oder beeinflussen;
- Plattformseiten bleiben korrekt plattformgebrandet;
- Backend-, Frontend- und E2E-Tests für den Flow sind grün.

### Phase 2 – Stammdaten und Importgrundlage

Aufgaben:

- Einheiten, Fixed-Point-Mengen, Kategorien und Artikel;
- Lieferanten, Lieferartikel, Packungen, MOQ, Preise und Lieferzeiten;
- CSV-Importframework mit Mapping, Dry Run, Idempotenz, Fortschritt und Fehlerreport;
- Beispielvorlagen und Seed-/Fixture-Daten.

Exit:

- wiederholter Import erzeugt keine Duplikate;
- Umrechnungen sind exakt getestet;
- Stammdaten sind je Tenant/Standort sicher isoliert.

### Phase 3 – Bestandsjournal und Inventur

Aufgaben:

- Bewegungen, Salden, Chargen/MHD und FEFO;
- Anfangsbestand, manuelle Korrektur, Transfer, Abfall;
- Inventurentwurf, Zählen, Review und Posting;
- MongoDB-Transaktionen, Idempotenz und Reconciliation;
- mobile/tabletfähige Oberflächen.

Exit:

- jeder Saldo ist aus dem Journal reproduzierbar;
- Parallel-, Wiederholungs-, Storno- und Isolationstests sind grün;
- gebuchte Bewegungen sind nicht editier-/löschbar.

### Phase 4 – Rezepte, POS-Mapping und Verbrauch

Aufgaben:

- Rezepte, Unterrezepte, Versionen, Ausbeute und Verlust;
- externe Produktmappings mit zeitlicher Gültigkeit;
- normalisierte Sales/Sales Lines und Guest Actuals;
- Verkauf → theoretischer Verbrauch;
- Storno → Gegenbuchung;
- Unmapped-Queue und Datenqualitätsanzeige.

Exit:

- derselbe Import ist idempotent;
- historische Rezeptversionen bleiben reproduzierbar;
- Verkauf und Storno ergeben korrekte Journalbewegungen.

### Phase 5 – Einkauf und Wareneingang

Aufgaben:

- Bestellungsentwurf, Freigabe, Status und Lieferkalender;
- Wareneingang, Preis-/Mengenabweichung, Charge und MHD;
- Packungs-/MOQ-Rundung;
- tenantgebrandete Bestell-PDFs/E-Mails;
- Audit und Berechtigungen.

Exit:

- Bestellung → Wareneingang → Bestand ist E2E getestet;
- gebrandete Dokumente nutzen Tenant-/Standort-Fallback korrekt;
- keine echte E-Mail wird in Tests versandt.

### Phase 6 – Forecasting und Bestellvorschläge

Aufgaben:

- Go-Baselines und gespeicherte Güte;
- Forecast Jobs und Python Worker;
- Gäste-, Bedarfs- und Reichweitenprognosen;
- P10/P50/P90, Datenreife und Modellvergleich;
- Bestellvorschläge mit nachvollziehbarer Formel und manueller Freigabe;
- Forecast-Dashboard und Monitoring.

Exit:

- Forecast-Lauf ist reproduzierbar;
- Baseline bleibt bei Worker-Ausfall verfügbar;
- keine Future Leakage in Testdatensätzen;
- Vorschläge lösen niemals automatisch eine Bestellung aus.

### Phase 7 – Pilot-Hardening

Aufgaben:

- End-to-End-Onboarding eines Restaurantkunden;
- Stripe-Pläne/Entitlements und Limitfälle;
- Audit, Exporte, Löschung/Aufbewahrung und DSGVO-Basis;
- Backup-/Restore-Anleitung und Restore-Test;
- Health Checks für Worker, Jobs und Integrationen;
- Performance-, Accessibility- und Securityprüfung;
- Shadow-Mode-Unterstützung und Pilot-KPI-Report.

Exit:

- Definition of Done aus dem Bauplan erfüllt;
- keine kritischen Tenant-, Journal- oder Credential-Risiken offen;
- alle automatisierbaren Qualitätsgates grün;
- verbleibende externe Pilot-/Credential-Schritte klar und ehrlich dokumentiert.

## 10. Test- und Verifikationspflicht

Mindestens nach jedem relevanten Slice, vollständig an jedem Phasenende:

```text
Backend:
  cd backend
  go test ./...
  go build ./...

Frontend:
  cd frontend
  npm test
  npm run build
  npm run lint

E2E:
  npx playwright test

Forecast Worker:
  Python-Unit- und Backtest-Suite gemäß pyproject/Projektkonfiguration
```

Wenn eine vollständige Suite wegen fehlender lokaler Infrastruktur nicht ausführbar ist:

1. führe alle nicht blockierten Tests aus;
2. dokumentiere exakten Befehl, Fehlermeldung und benötigte Voraussetzung;
3. schreibe keine falsche Erfolgsmeldung;
4. fahre mit unabhängigen Aufgaben fort.

Zusätzliche Pflichtprüfungen:

- Cross-Tenant-Negativtests für jede neue Ressourcengruppe;
- Standort-Scope-Negativtests;
- Branding-Asset- und Cache-Isolation;
- Journal-Reconciliation und Idempotenz;
- Test für nachträgliche Verkäufe/Stornos;
- Forecast Backtest gegen saisonale Baseline;
- keine Secrets, Tokens oder echten Kundendaten im Repository.

## 11. Verbotene Abkürzungen

- Keine reine In-Memory-Implementierung für produktive Fachdaten.
- Keine Bestandsberechnung ausschließlich im Frontend.
- Keine Autorisierung nur durch versteckte UI-Elemente.
- Keine ungefilterten `Find`/`Update`/`Delete` auf Tenant-Collections.
- Keine mutierbaren gebuchten Bewegungen.
- Keine Floats für Geld oder Bestandsbuchungen.
- Keine automatisch versandte Bestellung im MVP.
- Kein LLM als numerische Forecast-Engine.
- Kein beliebiges Tenant-HTML/CSS/JavaScript.
- Keine Platzhalterroute, die Erfolg meldet, ohne Geschäftslogik auszuführen.
- Keine als „fertig“ markierte Phase mit übersprungenen Kern-Tests.
- Keine produktiven Secrets in `.env`, Fixtures, Logs oder Screenshots.

## 12. Statusberichte und Abschluss

Zwischenberichte sind kurz und evidenzbasiert:

- was jetzt tatsächlich funktioniert;
- welche Dateien/Module geändert wurden;
- welche Tests ausgeführt wurden und mit welchem Ergebnis;
- welcher echte Blocker besteht;
- welche konkrete Aufgabe als Nächstes umgesetzt wird.

Melde das Gesamtprojekt erst als abgeschlossen, wenn die Definition of Done aus `WARENWIRTSCHAFT_SAAS_BAUPLAN.md` erfüllt ist. Externe Pilotvalidierung, fehlende Credentials oder juristische Prüfung dürfen nicht als erledigt dargestellt werden, wenn sie nicht stattgefunden haben.

Am Ende liefere:

1. lauffähigen Quellcode;
2. lokale Setup-/Startanleitung;
3. Architektur- und Datenmodellübersicht;
4. `IMPLEMENTATION_STATUS.md` ohne verschleierte Restarbeiten;
5. vollständige Testzusammenfassung;
6. Liste externer Schritte für Pilot und Produktion;
7. Sicherheits-/Tenant-Isolationsnachweis;
8. Forecast-Baseline- und Backtestbericht.

## 13. Beginne so

1. Prüfe Arbeitsrepository und Quell-Boilerplate.
2. Lies alle verbindlichen Dokumente vollständig.
3. Erstelle einen konkreten Phasenplan mit genau einem aktiven Schritt.
4. Übernimm bei Bedarf die Boilerplate sicher in das Arbeitsrepository.
5. Führe den Ausgangsbuild aus.
6. Lege Status/ADR/Deployment-Dokumentation an.
7. Implementiere Phase 1 als erste vollständige vertikale Produktphase.
8. Fahre nach grünem Exit-Gate selbstständig mit Phase 2 fort.

