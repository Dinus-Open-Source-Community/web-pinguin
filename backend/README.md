# Backend Pinguin Dinus

Backend menyediakan API portal, listing mirror dari direktori lokal, unduhan
file, dan pencatatan statistik. File tidak disalin ke database. `LISTING_DIR`
menentukan folder root yang boleh dijelajahi dan diunduh.

> **Status:** slice awal sudah diimplementasikan. PostgreSQL 17 berjalan di
> Docker pada `127.0.0.1:5432`, migrasi sudah diterapkan, dan endpoint API telah
> diuji dengan Postman CLI memakai direktori fixture sementara. Survei publik
> dan endpoint admin telah tersedia; admin memakai JWT bertanda tangan.

## Alur listing dan unduhan

1. Frontend meminta `GET /api/v1/listing?path=&page=1&page_size=20`.
2. API membaca isi `LISTING_DIR`, mengembalikan direktori terlebih dahulu,
   kemudian file biasa, dengan pagination.
3. Pengguna membuka folder sampai menemukan file, lalu klik `download_url`.
4. `GET /api/v1/download?path=...` mengalirkan file dari disk ke browser,
   mendukung HTTP Range/resume. `LISTING_DIR` tidak dapat ditembus lewat `..`;
   symlink yang keluar dari root ditolak oleh `os.Root`.
5. API mencatat waktu, relative path, status, byte terkirim, Range, dan estimasi
   selesai ke PostgreSQL. IP dan user-agent tidak disimpan.
6. Worker mengagregasi event per jam dan path prefix katalog. Statistik publik
   membaca event dari endpoint ini dan event mirror yang diimpor dari access log
   Nginx. Importer Nginx memetakan path prefix katalog ke wilayah GeoLite2.

Semua regular file dan folder di bawah `LISTING_DIR` dapat dibaca publik,
termasuk file tersembunyi. Arahkan hanya ke root konten mirror, bukan root
aplikasi atau folder yang berisi credential/private data. Special file seperti
socket tidak ditampilkan atau diunduh.

API publik saat ini:

| Method | Endpoint | Fungsi |
|---|---|---|
| GET | `/health/live` | Proses API hidup |
| GET | `/health/ready` | Memeriksa koneksi PostgreSQL |
| GET | `/api/v1/repositories` | Katalog repository publik; filter `q`, `category`, `distribution`, `sort=name\|updated`, `order`, dan pagination |
| GET | `/api/v1/repositories/{id}` | Detail repository beserta release dan file yang terdaftar |
| GET | `/api/v1/listing` | Jelajah folder/file di `LISTING_DIR` |
| GET | `/api/v1/download` | Unduh file dari root listing |
| GET | `/api/v1/stats/summary?period=24h\|7d\|30d&repository={uuid}&distribution={name}` | Ringkasan request, estimasi selesai, bandwidth, file populer; filter katalog mencocokkan `path_prefix` repository |
| GET | `/api/v1/stats/timeseries?period=24h\|7d\|30d&repository={uuid}&distribution={name}` | Seri waktu request, estimasi selesai, dan byte dengan filter katalog opsional |
| GET | `/api/v1/stats/regions?period=24h\|7d\|30d&repository={uuid}&distribution={name}` | Agregasi wilayah dengan filter katalog opsional; minimum awal 5 event per wilayah, grup kecil digabung ke `Other` per negara atau disembunyikan; tanpa alamat IP |
| GET | `/api/v1/mirrors/status` | Pemeriksaan HEAD untuk target mirror eksternal yang dikonfigurasi |
| GET | `/api/v1/surveys/active` | Form ISO aktif beserta pertanyaan dan opsi publik |
| POST | `/api/v1/surveys/{id}/responses` | Kirim jawaban survei; memerlukan `Idempotency-Key`, jawaban divalidasi dan hanya receipt dikembalikan |
| GET/POST | `/api/v1/admin/surveys` | Daftar survei atau buat draft survei |
| GET/PATCH | `/api/v1/admin/surveys/{id}` | Baca atau ubah draft survei |
| POST | `/api/v1/admin/surveys/{id}/publish`, `/close`, `/archive` | Perubahan status sesuai lifecycle |
| GET | `/api/v1/admin/surveys/{id}/results`, `/responses` | Hasil agregat dan respons dengan kontak disamarkan |
| PATCH | `/api/v1/admin/survey-options/{id}/fulfillment` | Perbarui status pemenuhan permintaan mirror |
| GET | `/api/v1/admin/audit-logs` | Cari jejak audit administrator |
| GET | `/api/v1/admin/workers/analytics` | Status worker dan backlog event |

## Penerbitan Token JWT Admin (CLI)

Endpoint `/api/v1/admin/*` memerlukan JWT HS256 bertanda tangan dari `JWT_SECRET` (minimal 32 byte).
Klaim wajib: `sub`, `role` (atau `roles`), `iss`, `aud`, `iat`, `nbf`, dan `exp`; issuer dan audience default adalah `web-pinguin` dan `web-pinguin-admin`. TTL maksimum adalah 24 jam.

### Cara generate token di terminal:

```powershell
# Muat environment variable dari .env
Get-Content ..\.env | ForEach-Object {
  if ($_ -match '^([^=]+)=(.*)$') {
    [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}

# Terbitkan token admin (role penuh)
go run ./cmd/admin-jwt -subject admin-operator -roles admin -ttl 24h
```

### Role yang tersedia:

| Role | Hak Akses | Contoh Perintah |
|---|---|---|
| `admin` | Akses penuh ke seluruh use case admin dan status worker | `go run ./cmd/admin-jwt -subject nama-operator -roles admin -ttl 24h` |
| `survey_manager` | Buat draft, ubah, publish, close, archive survei & baca hasil agregat | `go run ./cmd/admin-jwt -subject nama-operator -roles survey_manager -ttl 12h` |
| `response_reader` | Membaca jawaban survei & detail respons (email disamarkan) | `go run ./cmd/admin-jwt -subject nama-operator -roles response_reader -ttl 12h` |
| `fulfillment_manager` | Memperbarui status realisasi ISO (`prioritized`, `fulfilled`, dll.) | `go run ./cmd/admin-jwt -subject nama-operator -roles fulfillment_manager -ttl 12h` |
| `audit_reader` | Membaca jejak log audit administrator (`/admin/audit-logs`) | `go run ./cmd/admin-jwt -subject nama-operator -roles audit_reader -ttl 12h` |

Gunakan `go run ./cmd/admin-jwt -generate-secret` jika ingin membuat secret acak baru untuk `JWT_SECRET`.
CLI mencetak secret/token ke stdout saja; simpan secret di environment manager dan kirim token melalui kanal aman. Untuk rotasi normal, set `JWT_SECRET` ke key baru dan `JWT_SECRET_PREVIOUS` ke key lama, restart API, lalu terbitkan ulang token operator dengan key baru. API menerima kedua key selama overlap; setelah 24 jam plus toleransi clock 30 detik sejak token lama terakhir diterbitkan, hapus `JWT_SECRET_PREVIOUS` dan restart. Rotasi darurat tanpa overlap mencabut semua token yang ditandatangani key lama. Nilai audit actor berasal dari `sub`. Env lokal lama dengan typo `JWT_SCREET` masih dibaca sebagai fallback; gunakan `JWT_SECRET` untuk konfigurasi baru. Tanpa secret valid endpoint admin membalas `503 admin_disabled`. Email respons selalu disamarkan; jawaban hanya tersedia bagi role `response_reader` atau `admin`. OIDC/SSO belum dihubungkan. Katalog
repository membaca tabel metadata `repositories`; database lokal memakai manifest operator: 89 direktori ISO teramati dan tiga file Debian 13 terverifikasi.
Endpoint listing filesystem tetap terpisah.
Filter statistik repository/distribusi hanya menyertakan event yang path-nya
cocok dengan `path_prefix` repository aktif dan publik. Prefix dapat berbeda dari
slug, misalnya slug `debian` dengan prefix `iso/debian`. Event tanpa metadata
katalog tidak masuk hasil saat filter digunakan.

## PostgreSQL di Docker

Database development memakai PostgreSQL di Docker dengan port host
`127.0.0.1:5432` dan volume bernama agar data tetap ada setelah container
dimatikan. Dari root repository, salin `.env.example` menjadi `.env`, isi
`DB_PASSWORD` dan `POSTGRES_PASSWORD` dengan nilai lokal yang sama, lalu start:

```powershell
# Jalankan database saja untuk development backend secara mandiri:
docker compose up -d postgres
docker compose ps

# (Opsional) Jalankan seluruh stack aplikasi sekaligus (Gateway port 7000):
docker compose up -d --build
```

Jangan commit `.env`. Nilai `DB_*` dipakai backend dan `POSTGRES_*` dipakai
container saat pertama kali membuat database. Rancangan schema dan migrasi ada di
[`docs/DATABASE.md`](../docs/DATABASE.md). Dari folder `backend`, muat variabel
database dari file lokal dan atur root mirror sebenarnya:

```powershell
Get-Content ..\.env | ForEach-Object {
  if ($_ -match '^([^=]+)=(.*)$') {
    [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}
$env:LISTING_DIR = 'C:/path/to/mirror-root'
```

Kemudian jalankan migrasi secara eksplisit:

```powershell
go run ./cmd/migrate
```

## Menjalankan API dan worker

Di terminal terpisah, setelah environment tersedia dan migrasi berhasil:

```powershell
go run .
go run ./cmd/worker
```

API bind default hanya ke localhost (`127.0.0.1:8080`). Atur `API_ADDR` hanya
untuk host deployment yang memang perlu menerima koneksi lain. Worker batch
default 5.000 event setiap 60 detik; dapat diatur melalui
`WORKER_BATCH_SIZE` dan `WORKER_INTERVAL_SECONDS`.

## Worker analytics dan retensi

Worker mencatat log JSON saat batch mulai/selesai/gagal. Status terakhir dan
jumlah event pending dapat dibaca lewat endpoint admin di atas. Status `stale`
berarti belum ada sukses atau sukses terakhir lebih dari 5 menit lalu;
`degraded` berarti terdapat kegagalan beruntun. Restart aman: agregasi dan
checkpoint batch berada di transaksi yang sama.

Retensi berjalan sebagai proses terpisah:

```powershell
go run ./cmd/retention
```

Kebijakan retensi yang disetujui pemilik data adalah event mentah teragregasi
90 hari, respons/jawaban survei 365 hari, kontak 90 hari, kunci idempotensi
sampai `expires_at`, dan audit 365 hari. Agregat statistik dipertahankan tanpa
batas waktu. Untuk respons dan kontak, `retention_expires_at` eksplisit mengatur
expiry; bila `NULL`, job memakai masa simpan relatif pada waktu pengiriman atau
pembuatan. Nilai hari event, respons, kontak, dan audit dapat diatur lewat
`RETENTION_RAW_EVENTS_DAYS`, `RETENTION_SURVEY_ANSWERS_DAYS`,
`RETENTION_SURVEY_CONTACTS_DAYS`, dan `RETENTION_AUDIT_LOGS_DAYS`. Jadwal default
24 jam dan ukuran batch 1.000 dapat diubah dengan
`RETENTION_INTERVAL_SECONDS` dan `RETENTION_BATCH_SIZE`.
Worker tidak menghapus event mentah yang belum teragregasi; agregat unduhan
tidak dihapus. Respons yang masih mengikat proposal fulfillment juga ditahan
agar relasi fulfillment tetap valid. Log retensi hanya memuat jumlah baris,
durasi, dan error proses, tanpa email atau isi jawaban.

## Ingest katalog

Sumber metadata dipilih sebagai manifest JSON yang dikelola operator mirror dan
ditinjau sebelum impor. Schema serta cara mengisi ada di
[`../docs/CATALOG-INGEST.md`](../docs/CATALOG-INGEST.md). Jalankan dari folder
`backend` setelah env database tersedia:

```powershell
go run ./cmd/catalog-import C:/path/to/catalog.json
```

Import memakai upsert berdasarkan slug repository, pasangan repository/version,
dan pasangan release/path. Record yang tidak ada di manifest tidak dihapus;
ubah status menjadi `hidden` atau `retired` secara eksplisit. Tetapkan
`path_prefix` sesuai folder repository pada `$request_uri`; prefix tersebut
menjadi kunci pemetaan event dan dapat terdiri dari beberapa segmen.

## Komponen dan batas yang tersisa

- `main.go` di root `backend`: komposisi config, listing root, pool PostgreSQL, router Gin, dan
  graceful shutdown.
- `cmd/worker`: agregasi event idempoten dalam transaksi.
- `cmd/retention`: penghapusan data lawas bertahap sesuai env retention policy.
- `cmd/catalog-import`: validasi dan upsert manifest katalog secara transaksional.
- `cmd/migrate`: menerapkan migrasi bernomor dan menolak checksum migrasi yang
  berubah setelah diterapkan.
- `internal/listing`: listing serta pembacaan file yang dibatasi ke root.
- `internal/analytics`: penyimpanan event, query statistik, dan agregator.
- `internal/platform`: konfigurasi dan PostgreSQL pool.

Untuk geolokasi otomatis, sediakan database GeoLite2 City lokal dan atur
`GEOIP_DB_PATH`.
`go test ./internal/geolocation` menjalankan lookup terhadap database contoh
MaxMind yang berisi data fiktif; pengujian ini memeriksa parser dan pemetaan,
bukan akurasi GeoLite2 produksi. Fixture dan lisensinya ada di
`internal/geolocation/testdata`.
Format Nginx yang didukung dapat dibuat dengan konfigurasi berikut:

```nginx
log_format pinguin_analytics '$time_iso8601 $status $body_bytes_sent $request_uri $remote_addr';
access_log /var/log/nginx/pinguin-access.log pinguin_analytics;
```

Contoh fixture sintetis untuk parser adalah
`2026-11-24T10:00:00+07:00 200 8192 /repo-slug/file.iso 10.0.0.8`;
alamat private akan dipetakan menjadi wilayah unknown tanpa dikirim ke GeoLite2.
Ambil contoh log produksi yang sudah disanitasi dari operator sebelum
mengaktifkan impor untuk mirror sebenarnya.
Importer menerima log berformat
`$time_iso8601 $status $body_bytes_sent $request_uri $remote_addr`; jalankan
`NGINX_ACCESS_LOG=/path/to/access.log GEOIP_DB_PATH=/path/to/GeoLite2-City.mmdb go run ./cmd/nginx-import`.
IP dipakai hanya di memori untuk lookup dan tidak ditulis ke database. Importer
menyimpan offset pada file `.pinguin-offset.json` (atau `NGINX_STATE_PATH`)
dan melewati baris parsial sampai eksekusi berikutnya. Checkpoint mengenali
identitas file di filesystem yang mendukungnya; truncate atau rotasi ke file
baru memulai dari byte nol dengan source key generasi baru. Baris malformed
menghentikan impor dengan byte offset dan mempertahankan checkpoint sebelum
baris itu agar dapat diperbaiki lalu dicoba ulang. Kode negara ISO yang sudah disediakan di kolom
terakhir juga diterima sebagai fallback tanpa GeoIP DB. Definisikan
`MIRROR_TARGETS` sebagai daftar pasangan `base64url(nama)|base64url(URL)` yang
dipisah koma; URL harus HTTPS tanpa userinfo/fragment. Host yang mengarah ke IP
privat, loopback, link-local, shared, reserved, atau dokumentasi ditolak saat
probe; redirect tidak diikuti agar target tak dapat berpindah ke alamat lain.
Log error importer berformat JSON dan memuat `error_class`:
`permanent_input_error` berarti
perbaiki baris pada byte offset yang dilaporkan sebelum menjalankan ulang;
`transient_database_error` aman dicoba ulang setelah database pulih karena
insert dan checkpoint idempoten. `configuration_error` dan
`geolocation_database_error` memerlukan perbaikan konfigurasi/database geodata;
`checkpoint_state_error` memerlukan pemeriksaan file offset sebelum dipulihkan.
Worker mengagregasi event unduhan lokal; retention job menghapus event mentah
lawas hanya setelah berhasil diagregasi.
