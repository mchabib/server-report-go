# server-report

Laporan kondisi server terjadwal ke Telegram (bukan alert), dikirim per client
ke chat Telegram masing-masing. Query data dari **Prometheus pusat** (hasil
federasi dari semua Prometheus client)

## Instalasi (sekali saja, di server pusat)

1. Build binary (butuh Go toolchain + akses internet untuk `go mod tidy` sekali,
   tidak perlu di server pusat):
   ```
   go mod tidy
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o server-report .
   ```
2. Copy `server-report`, `.env.example` → `.env`, `clients.yaml.example` →
   `clients.yaml` ke server pusat, misal `/opt/server-report/`
3. Isi `.env`:
   ```
   PROMETHEUS_URL=http://prometheus-pusat:9090
   TELEGRAM_BOT_TOKEN=xxxxx:yyyyy
   ```
4. Isi `clients.yaml` dengan client yang sudah ada (lihat bagian "Menambah
   client baru" di bawah untuk detail tiap field)
5. `chmod +x server-report`
6. Test manual: `./server-report` — cek pesan masuk ke Telegram & exit code
   (`echo $?`, harus `0` kalau semua client sukses)
7. Setup cron (`crontab -e`) — **cukup satu cron di server pusat**, tidak
   perlu apa pun di server client:
   ```
   0 8,12,17,20 * * * cd /opt/server-report && ./server-report >> /var/log/server-report.log 2>&1
   ```

Setelah langkah di atas, instalasi selesai dan tidak perlu diulang lagi.

> **Penting**: baik lewat `go run .` maupun binary, `.env` dan `clients.yaml`
> dibaca dari folder **tempat binary berada** (bukan current directory).
> `go run .` compile ke binary temporary di cache folder, jadi tidak akan
> ketemu `.env`/`clients.yaml` lokal — untuk testing lokal, selalu
> `go build -o server-report .` dulu baru jalankan `./server-report`.

## Menambah client baru (tidak perlu install/restart ulang)

1. Pastikan Prometheus milik client baru sudah punya `external_labels.client`
   yang sesuai dan sudah masuk federasi ke Prometheus pusat:
   ```yaml
   # prometheus.yml di Prometheus milik client baru
   global:
     external_labels:
       client: "Client C"
   ```
2. Tambah entry baru di `/opt/server-report/clients.yaml`:
   ```yaml
   clients:
     - label: "Client C"
       name: "Client C"
       telegram_chat_id: "-1003333333"
   ```
3. Selesai — **tidak perlu restart, redeploy, atau sentuh binary apa pun**.
   Program dieksekusi lewat cron (bukan long-running service), jadi
   `clients.yaml` dibaca ulang setiap run. Client baru otomatis ikut di
   laporan jam berikutnya.

## Field clients.yaml

| Field | Wajib | Keterangan |
|---|---|---|
| `label` | ya | Harus sama persis dengan `external_labels.client` di Prometheus client tsb (termasuk spasi/kapitalisasi). Dipakai untuk filter PromQL `{client="..."}`. Harus unik. |
| `name` | ya | Nama yang tampil di judul laporan Telegram (bebas format). |
| `telegram_chat_id` | ya | Chat/group ID Telegram tujuan laporan client ini. |

Validasi berjalan saat startup — kalau ada field kosong atau `label`
duplikat, program exit dengan error jelas sebelum mengirim apa pun.

## Format laporan

Contoh (nilai ilustratif):

```
📊 Laporan Kondisi Server — Client A
🕒 17 July 2026, 10:52

🟢 Odoo (10.0.0.13:9100) — UP
   CPU: 0.8% | RAM: 37.4% | Disk: 15.0% (42.3 GB tersisa)

🟡 Database (10.0.0.17:9100) — UP
   CPU: 50.4% | RAM: 38.6% | Disk: 92.1% (3.2 GB tersisa)

🔴 Backup-Server (10.0.0.20:9100) — DOWN
```

- Label tiap baris = `job (instance)`, diambil dari label Prometheus.
- 🟢 UP normal, 🟡 UP tapi disk usage > 90% (lihat `diskWarningPercent` di
  `report.go`), 🔴 DOWN (target tidak ter-scrape, node_exporter-nya mati).
- Server DOWN tidak menampilkan CPU/RAM/Disk karena node_exporter-nya juga
  ikut mati, datanya kosong di Prometheus.
- Kalau ada instance yang UP tapi baris CPU/RAM/Disk tidak muncul
  ("metrik tidak tersedia"), biasanya itu bukan target node_exporter (lihat
  catatan job di bawah) — misal self-monitoring Prometheus milik client itu
  sendiri.

## Catatan: query tidak difilter `job`

Query PromQL cuma difilter `{client="..."}`, **sengaja tidak difilter `job`**.
Alasan: tiap Prometheus client menamai job scrape node_exporter-nya
sendiri-sendiri (mis. nama service yang jalan di situ), dan
`honor_labels: true` di federasi mempertahankan nama asli itu — jadi tidak
ada satu nama job global yang bisa cocok untuk semua client. `job` tetap
diambil dari metrik `up` dan ditampilkan di label baris (`job (instance)`),
hanya tidak dipakai sebagai filter.

Konsekuensi: kalau Prometheus milik satu client juga men-scrape exporter lain
selain node_exporter (mis. self-monitoring job `"prometheus"` di
`localhost:9090`), instance itu ikut kefederasi (karena `match[]` federasi
narik semua series `up` tanpa pandang job) dan bisa muncul di laporan sebagai
UP tanpa baris CPU/RAM/Disk. Ini kosmetik, bukan bug — laporan tetap akurat
untuk instance node_exporter yang sebenarnya.

## Perilaku saat error

- Client yang gagal (query Prometheus kosong/error, atau kirim Telegram
  gagal) dicatat sebagai warning ke stderr dan **tidak menghentikan** client
  lain dalam run yang sama.
- Server dengan status `up == 0` (DOWN) ditampilkan tanpa metrik CPU/RAM/Disk
  karena node_exporter-nya juga mati.
- Ringkasan (`N sukses, N gagal`) diprint ke stdout tiap run — cocok untuk
  dipantau lewat log cron.
- Exit code `0` kalau semua client sukses, `1` kalau ada minimal satu yang
  gagal (agar mudah dideteksi monitoring eksternal terhadap cron ini sendiri).

## Struktur

```
server-report-go/
├── main.go        # orkestrasi: load config, loop per client, exit code
├── env.go          # loader .env sederhana (stdlib saja)
├── clients.go      # parsing & validasi clients.yaml
├── prometheus.go   # query PromQL ke Prometheus pusat
├── telegram.go     # kirim sendMessage ke Telegram Bot API
├── report.go       # format teks laporan per client
├── go.mod
├── .env.example
├── clients.yaml.example
└── .gitignore
```

## Sebelum push ke git

`.env`, `clients.yaml`, dan binary `server-report` sudah masuk `.gitignore` —
isinya (bot token asli, IP Prometheus pusat, nama client, chat id) **tidak**
akan ke-commit selama file itu tidak di-`git add -f` manual. Yang boleh
di-commit cuma `.env.example` dan `clients.yaml.example` (isinya placeholder).

Sebelum commit pertama, selalu cek `git status` — pastikan `.env` dan
`clients.yaml` **tidak** muncul di daftar file yang mau di-stage.
