# Pinguin Dinus Architecture

Dokumen ini mendefinisikan arsitektur awal portal Linux Mirror Pinguin Dinus.

## 1. Keputusan teknologi

| Area | Pilihan | Tanggung jawab |
|---|---|---|
| Web application | TanStack Router + TanStack Query + TypeScript | Routing, server-state, filter, dan visualisasi pada browser |
| Build tooling | Vite | Development server dan production bundle frontend |
| API | Go + Gin | Repository catalog, metrics, health check, dan request mirror |
| Database | PostgreSQL | Catalog repository, agregat analytics, dan request mirror |
| Log processing | Worker Go terjadwal | Parsing Nginx log, deduplikasi, agregasi, dan geolocation |
| Map rendering | Library peta berbasis browser | Marker/heatmap dari data agregat, tanpa mengekspos IP |

TanStack digunakan pada layer aplikasi frontend; backend tetap menjadi sumber data
resmi agar data statistik dan katalog dapat digunakan oleh client lain di masa depan.

## 2. Batas tanggung jawab

```text
Browser
  -> TanStack Router / Query
  -> Go Gin API
       -> PostgreSQL
       -> Mirror metadata

Nginx access log
  -> Analytics worker
       -> IP geolocation provider/database
       -> PostgreSQL (data agregat)
```

Frontend tidak membaca access log, melakukan geolocation, atau menghitung statistik
sendiri. Backend hanya mengembalikan wilayah yang sudah diagregasi.

## 3. Struktur direktori

```text
backend/
  cmd/api/              # Entry point HTTP API
  cmd/worker/           # Entry point log processor
  internal/
    catalog/            # Repository catalog dan service
    analytics/          # Metric, aggregation, dan map queries
    health/             # Health/readiness checks
    requests/           # Request mirror
    geolocation/        # Adapter geolocation
    platform/           # Database, config, dan logging
  migrations/           # SQL migrations
  api/                  # OpenAPI dan API examples

frontend/
  src/
    routes/             # TanStack Router routes
    features/
      home/
      repositories/
      statistics/
      request-mirror/
      documentation/
    components/         # Komponen UI reusable
    lib/                # API client, query keys, formatter
    types/              # Tipe kontrak API

docs/
  ARCHITECTURE.md
```

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
| GET | `/repositories` | Search, filter, sorting, dan pagination |
| GET | `/repositories/:id` | Detail repository, file, checksum, dan mirror URL |
| GET | `/stats/summary` | Download, bandwidth, repository, dan status mirror |
| GET | `/stats/timeseries?period=24h` | Grafik aktivitas per interval waktu |
| GET | `/stats/regions?period=7d&repository=` | Agregat download per negara/kota |
| POST | `/mirror-requests` | Pengajuan repository baru |

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

- `repositories`: identity, slug, distribution, category, description, status.
- `repository_releases`: versi, ukuran, checksum, checksum algorithm, release date.
- `repository_files`: path, size, checksum, download URL.

### Analytics

- `download_events_raw`: akses mentah dengan retensi terbatas dan akses worker saja.
- `download_aggregates`: bucket waktu, repository, region, request count,
  completed count, bytes sent.
- `geo_regions`: kode negara/kota, label publik, dan koordinat yang sudah
  dibulatkan/diwakilkan.

`download_aggregates` menjadi sumber query dashboard. Dashboard tidak melakukan
agregasi dari event mentah.

## 6. Aturan analytics dan privasi

1. HTTP request bukan otomatis completed download. Worker harus membedakan status
   response, `bytes_sent`, range request, dan indikator penyelesaian yang tersedia.
2. Range request/resume perlu dikelompokkan agar tidak menghitung satu file berkali-kali
   secara naif.
3. Koordinat publik dibulatkan atau dipetakan ke centroid wilayah.
4. Wilayah dengan jumlah terlalu kecil digabungkan ke wilayah `Other` untuk
   mencegah identifikasi pengguna.
5. IP hanya dipakai internal untuk geolocation dan tidak disimpan lebih lama dari
   kebijakan retensi.
6. VPN, proxy, NAT, dan jaringan kampus dicatat sebagai keterbatasan akurasi.
7. Endpoint publik memakai agregat minimum 24 jam/7 hari/30 hari dan tidak
   menyediakan pencarian berdasarkan IP.

## 7. Tahapan implementasi

1. Bootstrap frontend TanStack dan backend Gin dengan endpoint health.
2. Tambahkan catalog repository statis dari database dan Repository Explorer.
3. Tambahkan summary dan timeseries analytics.
4. Tambahkan worker Nginx log serta geolocation yang dapat diganti melalui adapter.
5. Tambahkan regions endpoint, peta historical, lalu live activity dengan polling.
6. Tambahkan request mirror, dokumentasi publik, observability, dan retention job.

## 8. Non-functional requirements

- Semua endpoint list wajib memiliki pagination dan batas ukuran halaman.
- Timeout, cancellation, dan structured logging wajib diterapkan pada API/worker.
- Health liveness tidak boleh bergantung pada PostgreSQL; readiness harus memeriksa
  dependency yang dibutuhkan.
- Query statistik harus memakai index pada waktu, repository, dan region.
- API contract diuji sebelum frontend mengandalkannya.
