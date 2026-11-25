# Pinguin Dinus Architecture

Dokumen ini mendefinisikan arsitektur awal **website portal Pinguin Dinus yang dibangun ulang**. Portal mengelola katalog dan pengalaman pengguna; file unduhan tetap disediakan oleh layanan mirror UDINUS yang sudah ada. Dokumen ini adalah rancangan implementasi, bukan pernyataan bahwa semua komponen sudah tersedia.

## 1. Keputusan teknologi

| Area | Pilihan | Tanggung jawab |
|---|---|---|
| Web application | TanStack Router + TanStack Query + TypeScript | Routing, server-state, filter, dan visualisasi pada browser |
| Build tooling | Vite | Development server dan production bundle frontend |
| API | Go + Gin | Repository catalog, metrics, health check, survey, dan admin operations |
| Database | PostgreSQL | Catalog repository, agregat analytics, survey ISO, dan realisasi mirror |
| Log processing | Worker Go terjadwal | Parsing Nginx log, deduplikasi, agregasi, dan geolocation |
| Map rendering | Library peta berbasis browser | Marker/heatmap dari data agregat, tanpa mengekspos IP |

TanStack digunakan pada layer aplikasi frontend; backend tetap menjadi sumber data
resmi agar data statistik dan katalog dapat digunakan oleh client lain di masa depan.

### Database development

PostgreSQL development berjalan di Docker pada `localhost:5432`
(loopback `127.0.0.1:5432`) dengan data pada named volume. Backend mengambil
host, port, database, username, password, dan SSL mode dari environment.
Database produksi/deployment akan ditentukan pada tahap deployment;
`sslmode=disable` hanya untuk koneksi lokal. Rincian skema dan migrasi terdapat
di [DATABASE.md](DATABASE.md).

## 2. Batas tanggung jawab

Baseline situs publik yang ada sebelum rebuild (beranda, listing ISO, sampel
halaman distro, dan catatan hasil pengamatan) disimpan di
[`current-site-scrape/README.md`](current-site-scrape/README.md). Listing mirror
yang terlihat saat scrape berasal dari server eksternal; jumlah dan isinya dapat
berubah dan bukan data seed permanen.

```text
Browser
  -> TanStack Router / Query
  -> Go Gin API
       -> PostgreSQL (event download dan agregat portal)
       -> LISTING_DIR (root direktori mirror yang dikonfigurasi)
  <- listing JSON dan streaming file

Portal download event -> PostgreSQL -> Analytics worker -> agregat statistik

External Nginx access log -> importer/geolocation (tahap berikutnya)
```

Frontend tidak membaca access log, melakukan geolocation, atau menghitung statistik
sendiri. Runtime awal membaca serta menyajikan file dari `LISTING_DIR`; browser
mengunduh melalui endpoint backend. File tidak disalin ke database. Unduhan yang
melewati endpoint itu dicatat tanpa IP/user-agent dan statistik berasal dari event
tersebut. Akses langsung ke mirror eksternal perlu importer log operator yang
belum masuk runtime. Portal bukan host salinan ISO terpisah.

## 3. Struktur direktori

```text
compose.yaml          # Root Docker Compose (Gateway :7000, Frontend, API, Worker, DB)
Caddyfile             # Gateway reverse proxy configuration
backend/
  main.go               # Entry point HTTP API
  cmd/worker/           # Entry point log processor
  internal/
    http/               # Router Gin, middleware, handler, dan response/error mapping
      middleware/       # Request ID, logging, recovery, CORS, auth, rate limit
    catalog/            # Repository catalog dan service
    analytics/          # Metric, aggregation, dan map queries
    health/             # Health/readiness checks
    surveys/            # Form survei ISO, respons, dan hasil agregat
    admin/              # Use case dashboard admin dan fulfillment ISO
    auth/               # Identitas admin, sesi/token, role, dan policy
    audit/              # Catatan tindakan admin
    mirror/             # Adapter metadata, status, dan URL layanan mirror eksternal
    geolocation/        # Adapter geolocation
    platform/
      config/           # Load dan validasi environment/config
      postgres/         # Pool, transaksi, health check, dan SQL repositories
      logging/          # Structured logger
      clock/            # Clock abstraction untuk domain/job
  migrations/           # SQL migrations berurutan
  api/openapi.yaml      # Kontrak API OpenAPI
  configs/              # Contoh konfigurasi non-rahasia
  scripts/              # Utilitas lokal untuk migrasi/operasi
  Dockerfile
  go.mod
  go.sum

frontend/
  Dockerfile
  src/
    routes/             # TanStack Router routes
    features/
      home/
      repositories/
      statistics/
      iso-survey/
      admin/
      documentation/
    components/         # Komponen UI reusable
    lib/                # API client, query keys, formatter
    types/              # Tipe kontrak API

docs/
  ARCHITECTURE.md
  DATABASE.md           # Schema, relasi, index, migrasi, dan koneksi lokal
  api/                  # Contoh request/response atau catatan API tambahan
```

### 3.1 Aturan batas package

- `main.go` dan `cmd/worker` hanya merakit konfigurasi dan dependency, lalu
  menjalankan proses masing-masing. Logika bisnis tidak diletakkan di `cmd/`.
- `internal/http` menerjemahkan HTTP menjadi pemanggilan use case. Handler tidak
  berisi query SQL atau aturan bisnis.
- Package domain (`catalog`, `surveys`, `analytics`, `admin`) memiliki service/use
  case dan interface penyimpanan yang diperlukan. Implementasi PostgreSQL dapat
  disusun sebagai subpackage `postgres` di domain terkait.
- `mirror` adalah adapter ke layanan mirror yang sudah ada. Kegagalan mirror
  tidak boleh membuat katalog lokal kehilangan metadata yang sudah disimpan;
  sinkronisasi dan cache harus memiliki kebijakan kedaluwarsa.
- `auth` memeriksa identitas dan role; `audit` menyimpan tindakan admin. Jangan
  menganggap URL `/admin` saja sebagai perlindungan akses.
- Hindari package `common`/`utils` serba guna. Letakkan tipe dan fungsi dekat
  pemilik domainnya.

### 3.2 Proses runtime

| Proses | Tanggung jawab | Dependency wajib |
|---|---|---|
| Gateway (Caddy) | Reverse proxy publik di port 7000, routing /api/* ke Go API dan /* ke Frontend | API & Frontend |
| Frontend | TanStack Router/Vite SPA & SSR runner di port 3000 internal | Gateway & API |
| API | Endpoint publik/admin, validasi, katalog, survei, statistik, health | PostgreSQL; adapter mirror sesuai endpoint |
| Worker | Membaca log, memproses event, geolokasi, agregasi, retensi | Sumber log, PostgreSQL, geolocation provider/database |
| Migrator | Menjalankan perubahan skema sebelum versi aplikasi diluncurkan | PostgreSQL |

API dan worker dibangun dari modul backend yang sama tetapi dijalankan terpisah.
Worker tidak menjadi dependency liveness API.

### 3.3 Kontrak package Go

```text
main.go
  -> config, postgres pool, logger, repositories, services, HTTP router

cmd/worker/main.go
  -> config, postgres pool, log source, parser, geolocation, analytics processor

internal/http/handlers
  -> domain service interfaces (tidak menjalankan SQL langsung)

internal/{catalog,surveys,analytics,admin}
  -> model, validasi domain, use case, repository interfaces

internal/{catalog,surveys,analytics,audit}/postgres
  -> implementasi repository interface dengan query PostgreSQL
```

- `main.go` dan `cmd/worker` adalah composition root. Masing-masing membaca
  config yang tervalidasi, membuat dependency, menangani signal shutdown, lalu
  menutup HTTP server/pool dengan graceful shutdown.
- `internal/http` berisi router, handler per fitur, binding/validasi transport,
  middleware umum, dan pemetaan error domain menjadi HTTP. Middleware admin
  memvalidasi JWT HS256 dari `JWT_SECRET`, klaim identitas/role, dan expiry.
- Setiap domain memiliki model dan interface repository yang diperlukan oleh
  service/use case. Implementasi SQL berada dekat domain (`catalog/postgres`,
  `surveys/postgres`, dan seterusnya), sedangkan `platform/postgres` hanya
  menyediakan pool, transaksi, dan health check.
- `admin` mengorkestrasi use case admin dan menulis audit; hindari import siklik
  dengan menerima interface service catalog/survey yang diperlukan.
- `analytics` menyediakan query agregat untuk API dan processor agregasi untuk
  worker. Parser sumber log serta geolocation adalah adapter yang dapat diganti.
- Worker tidak mengakses HTTP handler. API tidak membaca log mentah.
- Test unit diletakkan dekat package; integration test memakai database/volume
  development terpisah dan tidak menghapus data pengguna.

### 3.4 Konfigurasi koneksi lokal

API, worker, dan migrator membaca konfigurasi `DB_HOST`, `DB_PORT`, `DB_NAME`,
`DB_USER`, `DB_PASSWORD`, `DB_SSLMODE`, serta timeout/pool dari environment.
Nilai development default yang terdokumentasi adalah `127.0.0.1:5432` pada
PostgreSQL Docker; kredensial tidak memiliki default di source code. Konfigurasi
production harus diberikan oleh environment deployment/secret store.

## 4. Kontrak API minimum

Semua endpoint diawali `/api/v1` dan mengembalikan JSON. Error memakai bentuk
yang konsisten:

```json
{
  "error": {
    "code": "invalid_query",
    "message": "period must be one of 24h, 7d, or 30d"
  }
}
```

| Method | Endpoint | Kegunaan |
|---|---|---|
| GET | `/health/live` | Proses API hidup |
| GET | `/health/ready` | API siap melayani setelah dependency dicek |
| GET | `/listing?path=&page=1&page_size=20` | Listing direktori/file dalam `LISTING_DIR` (runtime aktif) |
| GET | `/download?path=...` | Streaming file dengan dukungan Range dan event analytics (runtime aktif) |
| GET | `/stats/summary?period=30d` | Request, estimasi selesai, bandwidth, dan file populer (runtime aktif) |
| GET | `/stats/timeseries?period=24h` | Seri waktu aktivitas unduhan portal (runtime aktif) |
| GET | `/repositories` | Search/filter katalog metadata (runtime aktif) |
| GET | `/repositories/:id` | Detail repository (runtime aktif) |
| GET | `/stats/regions?period=7d&repository=&distribution=` | Agregasi wilayah (runtime aktif; lokasi tersedia bila event punya geodata) |
| GET | `/surveys/active` | Form survei ISO yang sedang aktif untuk publik (runtime aktif) |
| POST | `/surveys/:id/responses` | Mengirim respons survei ISO dengan validasi dan idempotency (runtime aktif) |
| GET | `/admin/surveys` | Mengelola daftar form survei (admin) |
| POST | `/admin/surveys` | Membuat form survei (admin) |
| PATCH | `/admin/surveys/:id` | Mengubah status dan konfigurasi form (admin) |
| GET | `/admin/surveys/:id/results` | Melihat hasil agregat dan ranking ISO (admin) |
| PATCH | `/admin/survey-options/:id/fulfillment` | Memperbarui status realisasi ISO (admin) |
| POST | `/admin/surveys/:id/publish` | Menerbitkan survei yang sudah divalidasi (admin) |
| POST | `/admin/surveys/:id/close` | Menutup penerimaan respons (admin) |
| POST | `/admin/surveys/:id/archive` | Mengarsipkan survei tertutup (admin) |
| GET | `/admin/surveys/:id` | Membaca konfigurasi survei untuk editor (admin) |
| GET | `/admin/surveys/:id/responses` | Melihat respons operasional dengan izin khusus (admin) |
| GET | `/admin/audit-logs` | Menelusuri tindakan admin dengan filter dan pagination (admin) |

Tabel ini mencampur endpoint runtime dan rencana pengembangan lain. Endpoint
admin yang tercantum di sini aktif dan memerlukan `Authorization: Bearer <JWT>`;
JWT HS256 harus membawa `sub`, `role`/`roles`, `iss`, `aud`, `iat`, `nbf`, dan
`exp`. Signing key minimal 32 byte. Tanpa konfigurasi valid, endpoint admin
mengembalikan `503 admin_disabled`. Role runtime: `admin`, `survey_manager`,
`response_reader`, `fulfillment_manager`, dan `audit_reader`; role yang tidak
diizinkan untuk operasi mengembalikan `403`.

### 4.1 Konvensi request dan response

- Base path adalah `/api/v1`. JSON memakai `Content-Type: application/json`;
  timestamp menggunakan ISO 8601 UTC. ID resource adalah UUID string.
- List memakai bentuk `{ "data": [...], "pagination": { "page": 1,
  "page_size": 20, "total": 0, "total_pages": 0 } }`. Default `page=1`,
  `page_size=20`, maksimum `page_size=100`.
- Detail/mutasi sukses memakai `{ "data": ... }`. Pengiriman survei dapat
  mengembalikan HTTP `201` dan `{ "data": { "receipt_id": "...",
  "submitted_at": "..." } }` tanpa memantulkan kontak/jawaban sensitif.
- Error memakai bentuk di atas dengan HTTP status yang sesuai: `400` query/
  payload invalid, `401` belum terautentikasi, `403` tidak berizin, `404`
  resource tidak ada/tidak terlihat, `409` konflik status/idempotency, `422`
  aturan domain gagal, `429` rate limit, `500` kesalahan tak terduga.
- Semua response error membawa `request_id`; detail internal hanya ada di log.
  Respons admin juga membawa `X-Request-ID` untuk korelasi audit/log.
- Gunakan `Idempotency-Key` pada POST respons survei. Server menyimpan hasil
  untuk key + cakupan survei dalam masa retensi yang ditetapkan; pemakaian key
  yang sama dengan payload berbeda menghasilkan `409`.

### 4.1.1 Keputusan pemilik dan verifikasi eksternal

Pemilik sistem telah menyetujui kebijakan retensi dan model autentikasi saat ini.
Keputusan berikut menjadi kontrak implementasi; integrasi produksi tetap perlu
diverifikasi dengan operator mirror:

| Topik | Rekomendasi awal | Konsekuensi kontrak |
|---|---|---|
| Login admin | Operator menerbitkan token JWT HS256 individual melalui CLI dari `JWT_SECRET`; klaim `sub`, issuer/audience, role, dan masa berlaku maksimum 24 jam. Role: admin, pengelola survei, pembaca respons, pengelola fulfillment, pembaca audit. | Audit mencatat `sub`. SSO/OIDC tidak digunakan saat ini; pencabutan darurat memakai rotasi signing key. |
| Respons berulang | Izinkan survei anonim; jamin idempotensi untuk retry menggunakan `Idempotency-Key`, rate limit dan proteksi spam. Jangan mengklaim satu respons per orang karena identitas anonim tidak bisa ditegakkan dengan andal. | Perubahan payload/endpoint hanya perlu jika kelak mensyaratkan login atau undangan unik. |
| Data kontak survei | Kontak opsional, jangan diwajibkan; pisahkan/batasi akses, samarkan pada dashboard, dan jangan masukkan ke agregat/audit. Sediakan teks tujuan penggunaan pada form. | Field `contact.email` opsional; hapus atau perluas hanya jika kebutuhan pemilik produk sudah jelas. |
| Retensi | Disetujui: 90 hari event mentah yang sudah diagregasi, 365 hari respons/jawaban, 90 hari kontak, 365 hari audit, dan agregat tanpa batas waktu. Kunci idempotensi disimpan sampai `expires_at`. | `retention_expires_at` mengatur respons/kontak jika berisi nilai; data lama memakai cutoff konfigurasi. Batch terbatas; event pending dan respons terkait fulfillment ditahan. |
| Sumber metadata mirror | Ingest memakai manifest JSON yang dikelola operator; belum ada sumber upstream/manifest produksi terverifikasi. | Upsert idempoten; hilangnya record dari manifest tidak otomatis menghapusnya. |
| Format access log | Importer mendukung `$time_iso8601 $status $body_bytes_sent $request_uri $remote_addr`, state offset, dan identitas file bila platform mendukungnya. | Contoh log operator dan uji rotasi real masih dibutuhkan. |
| Lokasi dan ambang agregasi | Publikasikan centroid wilayah; minimum awal 5 event per wilayah. Wilayah kecil digabung ke `Other` per negara jika gabungannya mencapai 5, selebihnya disembunyikan. | Koordinat `Other` bernilai null. Threshold menghitung event, bukan orang unik; owner tetap perlu meninjau ambang sebelum produksi. |

Sebelum produksi, operator masih perlu memberikan sampel access log tersanitasi,
manifest katalog dari sumber mirror yang disetujui, dan database GeoLite2 City
berlisensi beserta prosedur pembaruannya. SSO/OIDC dapat ditambahkan kemudian
jika proses penerbitan CLI tidak lagi sesuai kebutuhan.

### 4.2 Endpoint katalog dan statistik

| Method/path | Query/body | Hasil/aturan |
|---|---|---|
| `GET /repositories` | `q`, `category`, `distribution`, `sort`, `order`, `page`, `page_size` | Ringkasan repository terfilter dan paginated. Nilai sort/order allowlist; `sort` default `name`, `order` default `asc`. |
| `GET /repositories/{id}` | ID UUID | Detail repository, releases/files, checksum, status mirror, dan URL unduhan. Tidak mengembalikan isi file. |
| `GET /stats/summary` | `period=24h\|7d\|30d`, opsional `repository` dan `distribution` | Jendela waktu/`generated_at`, request, estimasi selesai, bandwidth, dan 10 file populer. Filter repository mencocokkan `path_prefix` (atau slug jika prefix kosong). |
| `GET /stats/timeseries` | `period=24h\|7d\|30d`, opsional `repository` dan `distribution` | Bucket waktu dengan request/completed sesuai definisi metrik dan bytes sent; filter memakai aturan prefix summary. |
| `GET /stats/regions` | `period=24h\|7d\|30d`, opsional `repository` | Agregat publik wilayah; minimum 5 event per wilayah. Grup kecil digabung menjadi `Other` per negara jika gabungannya mencapai 5, selain itu disembunyikan. Koordinat `Other` null. |

Periode adalah batas trailing dari waktu sekarang dalam UTC. Interval bucket
ditentukan server (1 jam untuk 24 jam, 1 hari untuk 7/30 hari). Format UUID filter
repository yang tidak valid menghasilkan `400`; UUID valid tanpa event cocok
menghasilkan agregat kosong dengan `200`.

### 4.3 Endpoint survei publik

**`GET /surveys/active`** mengembalikan `data: null` bila tidak ada survei aktif,
atau konfigurasi publik berikut:

```json
{
  "data": {
    "id": "uuid",
    "title": "Kebutuhan ISO",
    "description": "Pilih kebutuhan Anda",
    "starts_at": "2026-11-01T00:00:00Z",
    "ends_at": "2026-12-01T00:00:00Z",
    "questions": [
      {
        "id": "uuid",
        "label": "ISO yang dibutuhkan",
        "type": "single_choice",
        "required": true,
        "options": [{ "id": "uuid", "label": "Debian" }]
      }
    ]
  }
}
```

Jenis pertanyaan awal yang didukung: `single_choice`, `multiple_choice`,
`short_text`, `long_text`, dan `iso_proposal` untuk satu usulan ISO baru. Publik
hanya menerima label dan ID pilihan yang diizinkan; upstream URL, catatan admin,
jumlah respons privat, dan data kontak tidak disertakan.

**`POST /surveys/{id}/responses`** menerima jawaban yang terikat pada question
dan option ID. Contoh payload:

```json
{
  "answers": [
    { "question_id": "uuid", "option_ids": ["uuid"] },
    { "question_id": "uuid", "text": "Untuk kebutuhan praktikum" }
  ],
  "contact": { "email": "opsional@example.org" }
}
```

Tolak survei tutup/kedaluwarsa, pertanyaan/opsi asing, tipe/ukuran tidak sesuai,
pertanyaan wajib kosong, atau frekuensi berlebih. Kontak opsional; aturan
validasi dan penggunaan kontak harus diberitahukan di UI. Respons tidak dapat
dibaca ulang oleh pengguna anonim. Karena identitas anonim tidak pasti,
perlindungan satu respons memerlukan keputusan tentang akun, token undangan,
atau deduplikasi berisiko privasi (lihat bagian keputusan terbuka).

Untuk pertanyaan `iso_proposal`, field `text` menjadi nama ISO usulan dan
disimpan sebagai `survey_proposals` terkait dengan jawaban itu. Pertanyaan alasan
atau kebutuhan tetap memakai `long_text`, sehingga usulan ISO dapat diranking
dan ditindaklanjuti terpisah dari alasan pengaju.

### 4.4 Endpoint admin

Semua `/admin/*` mensyaratkan autentikasi dan permission. Draf permission awal:
`survey:read`, `survey:write`, `survey:publish`, `response:read_sensitive`,
`fulfillment:write`, `audit:read`, `export:write`. Role dipetakan ke permission
di konfigurasi/identity provider; setiap aksi diperiksa di server.

| Method/path | Payload/query | Hasil/aturan |
|---|---|---|
| `GET /admin/surveys` | `status`, `page`, `page_size` | Daftar survei dan jumlah respons, tanpa jawaban/kontak. |
| `POST /admin/surveys` | Judul, deskripsi, periode, questions/options | Membuat `draft`; validasi field dan nested resource. |
| `GET /admin/surveys/{id}` | — | Detail konfigurasi form untuk editor. |
| `PATCH /admin/surveys/{id}` | Field yang dapat diedit | Hanya draft yang dapat mengubah struktur; status tidak diubah di endpoint ini. |
| `POST /admin/surveys/{id}/publish` | — | Validasi kelengkapan dan jadwal, ubah draft ke active, audit. |
| `POST /admin/surveys/{id}/close` | Opsional `reason` | Tutup survei aktif, audit. |
| `POST /admin/surveys/{id}/archive` | — | Arsipkan survei tertutup, audit. |
| `GET /admin/surveys/{id}/results` | `page`, `page_size`, filter opsi | Hasil agregat/ranking; detail individu hanya dengan `response:read_sensitive`. |
| `GET /admin/surveys/{id}/responses` | `page`, `page_size` | Data operasional minimum, kontak disamarkan; wajib permission sensitif untuk membuka data kontak. |
| `PATCH /admin/survey-options/{id}/fulfillment` | `status`, `priority`, `note`, `repository_id` opsional | Validasi transisi fulfillment dan repository, simpan audit perubahan. |
| `GET /admin/audit-logs` | `actor_id`, `action`, `resource_id`, rentang waktu, pagination | Audit paginated; permission `audit:read`. |

Tambah export sebagai `POST /admin/surveys/{id}/exports` hanya jika format,
batas ukuran, retensi file ekspor, dan permission telah ditetapkan. Hindari
response sinkron besar; gunakan job dan tautan kedaluwarsa untuk dataset besar.

### 4.5 Health dan keamanan endpoint

- `GET /health/live`: `200` jika proses API hidup; tidak memeriksa PostgreSQL.
- `GET /health/ready`: `200` bila dependency wajib API siap, selain itu `503`;
  response tidak membeberkan alamat host, credential, atau detail infrastruktur.
- `429` menyertakan `Retry-After`. Rate limit diterapkan setidaknya pada kirim
  respons survei dan login/identity callback bila dikelola sendiri.
- Endpoint admin tidak boleh cache di shared proxy (`Cache-Control: no-store`).
  Response publik survei/katalog boleh cache sesuai `ETag`/`Cache-Control` yang
  ditentukan implementasi.

Semua endpoint di tabel berada di bawah prefix `/api/v1`; prefix tidak diulang
pada kolom endpoint. Untuk perubahan status survei, aksi publish/close/archive
harus divalidasi terhadap transisi status yang sah. `PATCH` generik tidak boleh
melewati validasi transisi tersebut.

Parameter list yang disepakati:

- `q`: pencarian nama/deskripsi.
- `category`: `iso`, `package`, atau `tool`.
- `distribution`: slug distribusi.
- `sort`: `name`, `downloads`, atau `updated`.
- `order`: `asc` atau `desc`.
- `page` dan `page_size`: pagination dengan batas server.

Respons statistik wilayah wajib hanya berisi `region`, `country`, `latitude`,
`longitude`, `downloads`, dan `period`. IP, user-agent, dan identifier request
tidak boleh keluar dari API publik.

## 5. Model data inti

### Catalog

- `repositories`: identity, API slug, mirror `path_prefix`, distribution,
  category, description, status.
- `repository_releases`: versi, ukuran, checksum, checksum algorithm, release date.
- `repository_files`: mirror-relative path, size, checksum, download URL.

`repositories` adalah katalog portal, bukan tempat menyimpan isi ISO. URL unduh
mengarah ke layanan mirror. Jika mirror menyediakan metadata yang dapat disinkronkan,
adapter `mirror` memperbarui status dan metadata katalog berdasarkan kebijakan
refresh yang ditetapkan.
`path_prefix` memisahkan identitas API dari layout fisik mirror; contoh live
`/iso/debian/...` dipetakan ke slug `debian` dengan prefix `iso/debian`.

### ISO survey dan realisasi mirror

- `surveys`: judul, deskripsi, periode, status, dan metadata form.
- `survey_questions`: pertanyaan, tipe jawaban, urutan, dan status wajib.
- `survey_options`: pilihan ISO, slug, upstream URL, dan status ketersediaan.
- `survey_proposals`: usulan ISO teks bebas yang dikirim pengguna untuk ditinjau admin.
- `survey_responses`: respons yang sudah divalidasi dengan retensi sesuai kebijakan.
- `survey_response_answers`: jawaban per pertanyaan atau pilihan ISO.
- `survey_response_selections`: relasi jawaban ke pilihan opsi untuk mendukung
  pilihan tunggal maupun jamak.
- `mirror_fulfillments`: prioritas, status tindak lanjut, catatan admin, dan
  referensi ke opsi/proposal serta repository ketika ISO berhasil direalisasikan.

Admin dashboard wajib menggunakan autentikasi dan otorisasi berbasis peran.
Endpoint publik hanya dapat membaca form aktif dan mengirim respons; endpoint
admin dapat mengelola form, memantau hasil, serta memperbarui status realisasi
ISO tanpa mengekspos data pribadi kepada publik.

### Analytics

- `download_events_raw`: event download portal tanpa IP/user-agent dan, kelak,
  event Nginx yang telah disanitasi; retensi terbatas dan akses worker saja.
- `download_aggregates`: bucket waktu, repository, region, request count,
  completed count, bytes sent.
- `geo_regions`: kode negara/kota, label publik, dan koordinat yang sudah
  dibulatkan/diwakilkan.

`download_aggregates` menjadi sumber query dashboard. Dashboard tidak melakukan
agregasi dari event mentah.

### 5.1 Relasi utama dan batas data

```text
repositories 1 --- * repository_releases 1 --- * repository_files
surveys 1 --- * survey_questions
surveys 1 --- * survey_options
surveys 1 --- * survey_responses 1 --- * survey_response_answers
survey_response_answers 1 --- 0..1 survey_proposals
survey_options 1 --- * survey_response_selections
survey_response_selections * --- 1 survey_response_answers
survey_options 1 --- 0..1 mirror_fulfillments
survey_proposals 1 --- 0..1 mirror_fulfillments
mirror_fulfillments * --- 0..1 repositories
repositories 1 --- * download_aggregates * --- 1 geo_regions
```

- Setiap tabel memiliki primary key stabil, `created_at`, dan bila perlu
  `updated_at`; foreign key dan unique constraint menjaga integritas.
- Slug repository unik. Path file unik dalam release/repository sesuai model
  katalog yang dipilih. Checksum menyimpan algoritma secara eksplisit.
- `survey_responses` mengacu pada survei dan menyimpan metadata pengiriman
  seminimal mungkin. Kontak opsional dipisahkan/dibatasi aksesnya jika perlu
  untuk menerapkan retensi berbeda; jangan mengembalikan data itu ke endpoint
  hasil publik.
- Jawaban pilihan mengacu pada option milik survei yang sama. Validasi server
  menolak option/pertanyaan yang tidak aktif atau tidak termasuk survei.
- Pertanyaan bertipe `iso_proposal` menyimpan usulan teks pada
  `survey_proposals`; proposal dapat dikelompokkan dan ditindaklanjuti tanpa
  mengubah survei yang sudah dipublikasikan.
- Ranking opsi dihitung dari pilihan jawaban; ranking proposal bebas dihitung
  dari `survey_proposals.normalized_name`. Hasil dihitung saat query pada volume
  awal; jangan menyimpan salinan agregat yang dapat basi tanpa strategi refresh.
- `download_events_raw` mempunyai retensi terbatas dan akses worker saja.
  Untuk IP, pilih kebijakan eksplisit: geolokasi lalu buang, atau simpan bentuk
  terlindungi untuk deduplikasi sementara. Jangan menyimpan IP mentah tanpa
  batas waktu dan kontrol akses yang terdokumentasi.
- `download_aggregates` memiliki bucket waktu dan dimensi repository/wilayah;
  tetapkan unique key atas kombinasi dimensi agar worker dapat melakukan
  upsert idempoten. Index minimal mengikuti query waktu, repository, dan region.
- `audit_logs` menyimpan admin, aksi, resource, waktu, hasil, request ID, serta
  perubahan yang aman dicatat. Jangan memasukkan password, token, IP pengguna,
  atau isi kontak/respons pribadi secara penuh ke audit log.

### 5.2 Tabel tambahan untuk operasi

Rancangan tabel domain di atas perlu dilengkapi tabel/konsep berikut saat
implementasi:

| Tabel/konsep | Tujuan | Catatan |
|---|---|---|
| `admin_users` (belum ada) | Identitas admin individual | Runtime menerima `sub` dari token penerbitan operator/CLI; SSO atau identity provider belum dihubungkan. Jangan simpan password atau token di DB. |
| Role JWT | Hak akses API | `admin`, `survey_manager`, `response_reader`, `fulfillment_manager`, dan `audit_reader`; pemeriksaan izin dilakukan middleware per route. |
| `audit_logs` | Jejak operasi admin | Append-only untuk aplikasi; akses terbatas |
| `idempotency_keys` (opsional) | Mencegah respons survei ganda akibat retry | Terapkan bila mekanisme deduplikasi request membutuhkannya |
| `worker_checkpoints` (opsional) | Posisi pemrosesan log | Gunakan jika sumber log tidak menjamin pengiriman ulang aman |

JWT individual per operator dipilih tanpa SSO/OIDC. Token membawa `sub` dan role
untuk audit/otorisasi; pencabutan token stateless dilakukan dengan rotasi signing
key dan token dibatasi maksimum 24 jam.
Pembacaan hasil/respons sensitif diaudit sebelum payload dikirim; jika penulisan
audit gagal, API menolak mengirim payload tersebut.

## 6. Alur fitur survei ISO dan dashboard admin

### 6.1 Siklus survei

```text
Admin membuat draft
  -> menyusun pertanyaan dan opsi ISO
  -> validasi form (judul, periode, opsi, aturan respons)
  -> publish
  -> publik membaca survei aktif
  -> publik mengirim respons
  -> server validasi dan simpan respons
  -> hasil agregat/ranking tersedia bagi admin
  -> admin meninjau dan menetapkan prioritas
  -> fulfillment: baru -> ditinjau -> diprioritaskan
  -> mirror disiapkan dan katalog portal diperbarui
  -> fulfillment direalisasikan atau ditolak, dengan catatan
  -> survei ditutup/diarsipkan dan data mengikuti retensi
```

Status survei yang disarankan: `draft`, `active`, `closed`, `archived`.
Hanya survei `active` dalam rentang waktu yang berlaku menerima respons.
Maksimal satu survei aktif per cakupan/periode perlu ditegakkan bila aturan
produk menghendakinya. Status fulfillment: `new`, `reviewing`, `prioritized`,
`in_progress`, `fulfilled`, `rejected`. Transisi direkam di audit log.

### 6.2 Form request ISO publik

1. `GET /surveys/active` hanya mengembalikan konfigurasi form aktif dan opsi
   publik yang boleh dipilih.
2. UI mengirim jawaban ke `POST /surveys/:id/responses`; server memvalidasi
   periode, jenis/ukuran jawaban, pertanyaan wajib, opsi milik survei, serta
   batas frekuensi pengiriman.
3. Server menyimpan respons dan jawaban secara konsisten dalam transaksi.
   Retry jaringan tidak boleh menggandakan respons tanpa sengaja; gunakan
   idempotency key atau aturan deduplikasi yang telah diputuskan.
4. Kontak bersifat opsional dan hanya digunakan untuk tujuan yang dijelaskan
   kepada pengaju. Jangan tampilkan kontak dalam ranking atau statistik publik.
5. Response API mengembalikan tanda terima generik, bukan detail respons orang
   lain atau informasi internal.

### 6.3 Dashboard admin

Dashboard memakai API `/admin/*` yang memerlukan autentikasi dan izin per aksi.
Layar/kemampuan minimum:

- daftar survei dengan status, periode, jumlah respons, dan aksi yang diizinkan;
- editor draft untuk judul, deskripsi, periode, pertanyaan, dan opsi ISO;
- preview dan validasi sebelum publish;
- ringkasan hasil, ranking opsi, filter, dan ekspor yang tercatat;
- tampilan detail respons hanya untuk role yang memang memerlukannya, dengan
  penyamaran kontak secara default;
- pengelolaan prioritas dan status fulfillment sampai referensi repository
  portal dihubungkan;
- audit trail perubahan status dan konfigurasi.

Publik hanya mendapat form aktif dan hasil agregat yang secara eksplisit
ditetapkan untuk dibuka. Tidak ada endpoint publik untuk membaca respons
individual.

## 7. Keamanan, privasi, dan operasi

### Autentikasi dan otorisasi

- Endpoint publik tidak memerlukan login, tetapi pengiriman survei memiliki
  validasi, rate limit, dan perlindungan spam yang sesuai.
- Endpoint admin memvalidasi JWT HS256 pada setiap request dan memeriksa izin
  route berdasarkan role. Tolak secara default jika signature, issuer, audience,
  expiry, atau permission tidak valid. Kontak respons selalu disamarkan.
- Simpan secret di environment/secret manager; file konfigurasi contoh hanya
  berisi placeholder. Terapkan TLS pada deployment.
- CORS menggunakan allowlist origin frontend. Jangan memakai wildcard bersama
  credential.
- Cookie sesi, bila kelak dipilih, harus menggunakan `HttpOnly`, `Secure`, dan
  `SameSite` sesuai deployment serta perlindungan CSRF. JWT bearer saat ini
  memiliki expiry wajib dengan TTL maksimum 24 jam, tetapi belum memiliki
  pencabutan per pengguna. Rotasi normal memakai `JWT_SECRET` baru dan
  `JWT_SECRET_PREVIOUS` selama overlap; setelah token lama kedaluwarsa, hapus
  key lama dan restart layanan. Untuk kebocoran key, hilangkan key lama segera
  dan terbitkan ulang token operator.

### Validasi dan perlindungan API

- Validasi payload, panjang teks, MIME/ukuran ekspor, enum, tanggal, pagination,
  dan ownership resource di server.
- Batasi ukuran body, timeout request, concurrency yang relevan, dan rate limit
  per endpoint. Jangan mengungkap stack trace atau query database ke client.
- Gunakan parameterized SQL. Jalankan migrasi dengan role database khusus;
  aplikasi runtime hanya mendapat izin yang dibutuhkan.
- Ekspor hasil admin harus menerapkan otorisasi, batas ukuran, dan audit.

### Logging dan observability

- Structured log memuat timestamp, level, service, request ID, route template,
  status, durasi, dan error code. Hindari body request dan data pribadi.
- Pisahkan liveness (`/health/live`) dari readiness (`/health/ready`). Readiness
  memeriksa dependency yang dibutuhkan API; worker menyediakan status proses
  atau metrik terpisah.
- Catat metrik API (latensi/error), worker (event diproses/gagal/tertinggal),
  serta koneksi database. Alert threshold ditentukan saat deployment.
- Audit log adalah rekam jejak bisnis/admin; jangan menganggap log aplikasi
  biasa sebagai penggantinya.

### Retensi dan pemulihan

- Jalankan retention job berkala dan pantau jumlah
  record terhapus/ditahan serta durasi.
- Tentukan backup PostgreSQL, jadwal, enkripsi, akses, dan uji pemulihan.
- Worker harus aman terhadap restart dan pengolahan ulang: gunakan checkpoint
  atau deduplikasi serta upsert agregat transaksional.

## 8. Aturan analytics dan privasi

1. HTTP request bukan otomatis completed download. Worker harus membedakan status
   response, `bytes_sent`, range request, dan indikator penyelesaian yang tersedia.
2. Range request/resume perlu dikelompokkan agar tidak menghitung satu file berkali-kali
   secara naif.
3. Koordinat publik dibulatkan atau dipetakan ke centroid wilayah.
4. Endpoint regions menggunakan minimum awal 5 event: wilayah kecil digabung ke
   `Other` per negara jika totalnya mencapai ambang; jika tidak, disembunyikan.
   Koordinat `Other` null. Ini mengurangi paparan agregat jarang, tetapi bukan
   jaminan anonimitas per orang karena threshold menghitung event, bukan actor unik.
5. IP hanya dipakai internal untuk geolocation dan tidak disimpan lebih lama dari
   kebijakan retensi.
6. VPN, proxy, NAT, dan jaringan kampus dicatat sebagai keterbatasan akurasi.
7. Respons survei dan kontak pengaju memiliki retensi serta akses terbatas sesuai
   kebijakan privasi; hasil publik hanya berupa agregat.
8. Endpoint publik memakai agregat minimum 24 jam/7 hari/30 hari dan tidak
   menyediakan pencarian berdasarkan IP.

## 9. Tahapan implementasi dan status

Daftar ini berasal dari roadmap awal. Status runtime mencakup API katalog,
survei publik/admin, analytics, importer Nginx, GeoLite2 lookup, agregasi
wilayah, monitoring mirror, retensi terjadwal, status worker, dan scaffold
deployment. Dashboard frontend dan verifikasi data/operator produksi serta
restore drill di infrastruktur target masih lanjutan.

1. Disetujui pemilik: masa retensi dan JWT per-operator tanpa SSO saat ini.
   Verifikasi sumber manifest, sampel log Nginx, dan GeoLite2 produksi menunggu
   data operator mirror.
2. Selesai: modul Go, config tervalidasi, PostgreSQL, migrator, structured
   logging, router Gin, dan endpoint health tersedia.
3. Selesai untuk runtime saat ini: katalog metadata, listing filesystem, unduhan,
   dan monitoring target mirror tersedia.
4. Selesai: survei ISO publik, validasi, idempotency, dan hasil agregat.
5. Selesai untuk admin JWT multi-role: signing key environment/rotasi, actor
   audit dari `sub`, CRUD/status, dan fulfillment ISO. Hubungkan dashboard
   frontend sebagai lanjutan.
6. Selesai: summary/timeseries tersedia; monitoring mirror aktif untuk target
   yang dikonfigurasi.
7. Sebagian selesai: importer Nginx menangani checkpoint/rotasi dan event
   idempoten; log operator dan database GeoLite2 produksi belum diverifikasi.
   Retensi otomatis dan observability worker tersedia.
8. Selesai: endpoint regions tersedia; peta historis/live polling di frontend
   masih pekerjaan lanjutan.
9. Scaffold deployment, backup/restore scripts, dokumentasi rollback, dan CI
   contract checks tersedia; deploy target, offsite backup, restore drill, dan
   hasil pipeline pada repository remote belum ada.

## 10. Non-functional requirements

- Semua endpoint list wajib memiliki pagination dan batas ukuran halaman.
- Timeout, cancellation, dan structured logging wajib diterapkan pada API/worker.
- Health liveness tidak boleh bergantung pada PostgreSQL; readiness harus memeriksa
  dependency yang dibutuhkan.
- Query statistik harus memakai index pada waktu, repository, dan region.
- API contract didokumentasikan dalam OpenAPI dan diverifikasi sebelum frontend
  mengandalkannya.
- Operasi admin harus diaudit dan hanya dapat diakses oleh peran yang berwenang.
- Migrasi harus dapat dilacak, berjalan berurutan, dan memiliki prosedur pemulihan.
- Worker harus dapat dimulai ulang tanpa menggandakan agregat.
- Deployment menyediakan backup database dan prosedur pemulihan yang terdokumentasi.
