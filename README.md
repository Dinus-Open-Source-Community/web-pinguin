# 🐧 Pinguin Dinus
### Linux Mirror Portal — Universitas Dian Nuswantoro

> **Your Open Source Distribution Hub — Powered by UDINUS.**

## 1. Overview

Pinguin Dinus merupakan portal Linux Mirror milik Universitas Dian Nuswantoro (UDINUS) yang menyediakan akses terhadap berbagai distribusi Linux, file ISO, package repository, dan perangkat lunak open-source.

Pembangunan ulang Pinguin Dinus bertujuan mengembangkan portal katalog mirror
yang menggunakan listing direktori sebagai sumber data. Backend membaca folder
yang dikonfigurasi melalui `LISTING_DIR`, menampilkan file/subfolder, lalu
menyajikan file unduhan melalui endpoint stream. Portal juga dirancang untuk
menampilkan statistik, monitoring, peta aktivitas, dan survei kebutuhan ISO.

### Product Vision

Menjadikan Pinguin Dinus sebagai pusat distribusi dan informasi infrastruktur open-source UDINUS yang informatif, mudah digunakan, transparan, dan dapat diakses oleh komunitas open-source.

---

## 2. Main Features

### 2.1 Home

Halaman utama sebagai pusat informasi dan akses cepat menuju layanan mirror.

- Hero section dan deskripsi layanan Pinguin Dinus.
- Search ISO dan repository.
- Daftar repository populer.
- Statistik total download.
- Informasi total bandwidth.
- Jumlah repository yang tersedia.
- Status dan kesehatan server mirror.
- Visualisasi peta aktivitas download pengguna.

### 2.2 Repository Explorer

Halaman untuk menjelajahi seluruh repository yang tersedia.

- Daftar distribusi Linux.
- Kategori ISO, package repository, dan tools.
- Search repository.
- Filter berdasarkan kategori dan distribusi.
- Sorting berdasarkan nama, popularitas, dan pembaruan.
- Informasi versi dan ukuran file.
- Informasi checksum untuk verifikasi integritas file.
- Link download langsung.
- Informasi lokasi dan status repository.

### 2.3 Statistics & Analytics

Halaman untuk menampilkan statistik penggunaan infrastruktur mirror.

- Total download.
- Total bandwidth yang digunakan.
- Repository paling populer.
- Grafik download harian, mingguan, dan bulanan.
- Statistik berdasarkan distribusi Linux.
- Statistik penggunaan berdasarkan wilayah.
- Peta persebaran aktivitas download.

### 2.4 Download Activity Map

Salah satu fitur unggulan Pinguin Dinus berupa peta interaktif untuk memvisualisasikan persebaran aktivitas download.

**Features:**

- Peta Indonesia dan dunia.
- Visualisasi menggunakan marker dan heatmap.
- Persebaran download berdasarkan negara dan kota.
- Jumlah download berdasarkan wilayah.
- Filter periode: 24 jam, 7 hari, dan 30 hari.
- Filter berdasarkan distribusi Linux atau repository.
- Klik wilayah untuk melihat detail statistik.
- Tren aktivitas download berdasarkan waktu.
- Live activity map dengan pembaruan berkala.
- Historical heatmap untuk melihat aktivitas historis.

**Visualization Modes:**

| Mode | Description |
|---|---|
| Live Activity Map | Menampilkan aktivitas download terbaru dengan pembaruan berkala. |
| Historical Heatmap | Menampilkan akumulasi aktivitas download berdasarkan periode tertentu. |

### 2.5 ISO Request & Survey

Fitur untuk mengumpulkan kebutuhan ISO yang paling banyak diminta dan membantu admin menentukan prioritas mirror.

- Form survei yang dapat dikonfigurasi untuk setiap periode.
- Pilihan ISO yang sudah tersedia dan usulan ISO baru.
- Alasan atau kebutuhan pengguna terhadap ISO tersebut.
- Kontak pengaju yang bersifat opsional.
- Rekap jumlah permintaan dan ranking ISO berdasarkan hasil survei.
- Status tindak lanjut: baru, ditinjau, diprioritaskan, direalisasikan, atau ditolak.

### 2.6 Admin Dashboard

Dashboard terproteksi untuk mengelola survei dan menindaklanjuti hasilnya.

- Membuat, mengubah, mengaktifkan, menutup, dan mengarsipkan form survei.
- Mengelola pertanyaan, pilihan ISO, periode, dan status form.
- Memantau jumlah respons dan hasil agregat tanpa mengekspos data pribadi ke publik.
- Melihat ranking ISO yang paling banyak diminta dan detail respons yang diperlukan.
- Menandai ISO sebagai prioritas mirror dan mencatat status realisasinya.
- Mengelola proses realisasi ISO yang dipilih sampai masuk ke katalog mirror.
- Mengekspor hasil survei untuk kebutuhan analisis dan pelaporan.

### 2.7 About & Documentation

Halaman informasi dan dokumentasi layanan Pinguin Dinus.

- Informasi mengenai Pinguin Dinus.
- Informasi infrastruktur server mirror.
- Cara menggunakan layanan mirror.
- Panduan konfigurasi repository Linux.
- Dokumentasi penggunaan mirror.
- Informasi kontak dan kontributor.

---

## 3. Download Analytics System

Untuk unduhan yang melewati endpoint portal, backend dapat mencatat request
langsung ketika file di-stream dari `LISTING_DIR`. Integrasi Nginx access log
mirror eksternal tetap dapat ditambahkan jika operator memberikan akses log.

### Data Collection Flow

```text
User clicks Download
        |
        v
  Go API reads LISTING_DIR
        |
        v
  API streams file to browser
        |
        v
  Download event in PostgreSQL
        |
        v
 Analytics worker aggregates
        |
        v
  Statistics API / dashboard
```

### Data model and future log fields

Event yang tersimpan oleh runtime awal tidak memuat IP atau user-agent. Field
berikut hanya dapat diproses bila log mirror eksternal tersedia dan kebijakan
privasi/retensinya sudah ditetapkan.

| Data | Description |
|---|---|
| IP Address | Jangan simpan mentah; bila diperlukan untuk geolocation, proses sementara lalu buang. |
| Timestamp | Waktu request download. |
| File Path | File atau repository yang diakses. |
| HTTP Status | Status respons server. |
| Bytes Sent | Jumlah data yang dikirim. |
| Country | Perkiraan negara pengguna. |
| City | Perkiraan kota pengguna. |
| Coordinates | Koordinat hasil IP geolocation. |

### Analytics Metrics

Sistem perlu membedakan beberapa metrik berikut:

- **Download Requests:** Jumlah request yang diterima server.
- **Completed Downloads:** Jumlah proses download yang teridentifikasi selesai.
- **Bandwidth Usage:** Total data yang dikirim oleh server.
- **Repository Popularity:** Repository dengan aktivitas download tertinggi.
- **Geographic Distribution:** Persebaran aktivitas berdasarkan wilayah.

> Catatan: Satu HTTP request tidak selalu merepresentasikan satu download utuh. HTTP Range Request, download parsial, dan resume download harus diperhitungkan dalam perhitungan statistik.

Runtime backend mencatat path relatif, waktu, status HTTP, byte terkirim, Range,
dan estimasi selesai. Importer Nginx dapat mengisi geolokasi dari IP dengan
database GeoLite2 City lokal; IP/user-agent tidak disimpan.

---

## 4. Privacy & Data Accuracy

Penerapan analytics harus memperhatikan privasi pengguna dan akurasi data geografis.

- Lokasi berdasarkan IP merupakan perkiraan, bukan lokasi GPS.
- IP address tidak boleh ditampilkan kepada publik.
- Data peta harus menggunakan agregasi geografis.
- VPN, proxy, NAT, dan jaringan kampus dapat memengaruhi akurasi lokasi.
- Data pribadi harus memiliki kebijakan retensi dan perlindungan yang jelas.
- Jangan menampilkan identitas individu pada visualisasi aktivitas download.
- Hindari menampilkan lokasi geografis yang terlalu spesifik.

---

## 5. Website Navigation

Struktur navigasi utama Pinguin Dinus:

| Navigation | Description |
|---|---|
| Home | Overview, statistik ringkas, dan aktivitas mirror. |
| Repositories | Menjelajahi dan mengunduh ISO serta repository. |
| Statistics | Statistik penggunaan dan download activity map. |
| ISO Survey | Mengisi survei dan mengusulkan ISO yang dibutuhkan. |
| Admin Dashboard | Mengelola form, memantau hasil, dan menindaklanjuti realisasi ISO. |
| Documentation | Panduan dan dokumentasi penggunaan. |
| About | Informasi layanan, infrastruktur, dan kontributor. |

---

## 6. Product Identity

Pinguin Dinus tidak hanya berfungsi sebagai website untuk mengunduh ISO Linux.

Pinguin Dinus dikembangkan sebagai portal distribusi dan transparansi infrastruktur open-source UDINUS yang menggabungkan:

1. **Repository Distribution** — Distribusi ISO dan package repository.
2. **ISO Demand Survey** — Pengumpulan kebutuhan ISO berdasarkan suara pengguna.
3. **Download Statistics** — Analisis aktivitas download.
4. **Server Monitoring** — Informasi status infrastruktur mirror.
5. **Geographic Visualization** — Peta persebaran aktivitas pengguna.
6. **Open-source Documentation** — Dokumentasi dan panduan penggunaan.

---

## 7. Development Priorities

| Priority | Feature | Status |
|---|---|---|
| P0 | Home Page | Planned (Frontend pending) |
| P0 | Repository Explorer | Backend implemented (`/api/v1/repositories`); frontend pending |
| P0 | LISTING_DIR file listing and download API | Backend implemented (`/api/v1/listing`, `/api/v1/download`); frontend pending |
| P1 | Statistics & Analytics | Backend implemented (API summary, timeseries, hourly worker); frontend dashboard pending |
| P1 | Download Activity Map | Backend regions API implemented (`/api/v1/stats/regions`); frontend map pending |
| P1 | Historical Heatmap | Backend regions API implemented; frontend visualization pending |
| P1 | Server Monitoring | Backend implemented (`/health/live`, `/health/ready`, `/api/v1/mirrors/status`); frontend status UI pending |
| P1 | ISO Request & Survey | Backend implemented (`/api/v1/surveys/active`, `POST /responses`); frontend pending |
| P1 | Admin Dashboard & ISO Fulfillment | Backend implemented (Admin JWT, survey CRUD, fulfillment, audit logs); frontend dashboard pending |
| P2 | Documentation | Backend & architecture docs complete; frontend guides pending |

## 7.1 Architecture

Arsitektur implementasi menggunakan **TanStack Router + TanStack Query +
TypeScript + Vite** pada frontend dan **Go + Gin** pada backend. Backend
menyediakan API versioned, sedangkan worker terpisah memproses Nginx access log
menjadi agregat analytics yang aman untuk ditampilkan pada dashboard.

Detail struktur direktori, kontrak API, model data, dan aturan privasi tersedia di
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## 8. Project Goals

- Mempermudah pengguna menemukan dan mengunduh repository Linux.
- Menyediakan informasi repository yang terstruktur.
- Memberikan transparansi penggunaan layanan mirror.
- Memvisualisasikan persebaran aktivitas download secara geografis.
- Menyediakan informasi kesehatan infrastruktur server.
- Menentukan prioritas ISO yang di-mirror berdasarkan kebutuhan pengguna.
- Membantu admin memantau dan merealisasikan permintaan ISO secara terukur.
- Meningkatkan pengalaman pengguna dalam mengakses layanan Pinguin Dinus.

---

**Pinguin Dinus — Your Open Source Distribution Hub.**

Powered by Universitas Dian Nuswantoro (UDINUS).

---

### 9.1 Mirror Ubuntu dan `LISTING_DIR`

Mirror Nevacloud menyediakan direktori Ubuntu pada
[`https://mirror.nevacloud.com/ubuntu/`](https://mirror.nevacloud.com/ubuntu/).
Di dalamnya tersedia `iso-release/` dan `ubuntu-archive/`. Halaman status
Citrahost (`https://mirror.citrahost.com/status/`) adalah informasi status
layanan, bukan direktori file untuk dijadikan sumber listing.

Backend Pinguin membaca **direktori lokal** dari `LISTING_DIR`; variabel ini
tidak menerima URL HTTPS. Agar file Ubuntu dari Nevacloud muncul pada API,
siapkan/sinkronkan isi direktori mirror tersebut ke disk server/komputer, lalu
arahkan `.env` ke folder lokal yang ingin diekspos. Contoh jika mirror sudah
disinkronkan ke `C:\mirrors\ubuntu`:

```dotenv
LISTING_DIR=C:/mirrors/ubuntu
```

Setelah API dijalankan ulang, cek isinya melalui:

```text
http://127.0.0.1:8080/api/v1/listing
```

Jika memilih folder lokal `C:\mirrors\ubuntu`, isinya akan terlihat langsung
di root listing API. Untuk membuka ISO, telusuri folder yang muncul (misalnya
`iso-release`) dan gunakan nilai `path` file dari respons listing pada endpoint
`/api/v1/download?path=...`. Jangan isi `LISTING_DIR` dengan URL
`https://mirror.nevacloud.com/ubuntu/`, karena backend akan menganggapnya
sebagai nama path lokal.

### Supaya pengguna bisa mengunduh lewat API Pinguin

Ada dua hal yang harus tersedia: file Ubuntu dapat dibaca backend, dan alamat
API dapat dijangkau pengguna. Konfigurasi default hanya melayani request dari
komputer itu sendiri (`127.0.0.1:8080`), jadi link localhost hanya bisa dipakai
di mesin yang menjalankan API.

1. **Sediakan file di disk server.** Sinkronkan mirror yang diizinkan operator
   dari Nevacloud ke direktori lokal, misalnya `C:/mirrors/ubuntu`. Path pada
   `.env` harus menunjuk direktori itu. Jangan memakai URL mirror sebagai nilai
   `LISTING_DIR`.
2. **Jalankan API dengan konfigurasi database dan direktori tersebut.** Pastikan
   PostgreSQL sehat, migrasi sudah diterapkan, dan API berjalan. API perlu
   dijalankan ulang setelah perubahan environment.
3. **Publikasikan API dengan domain HTTPS.** Di server, pasang reverse proxy
   seperti Nginx/Caddy ke backend dan gunakan TLS. Atur `API_ADDR` ke interface
   yang sesuai jaringan deployment; bind ke `0.0.0.0` hanya jika firewall dan
   reverse proxy server sudah membatasi akses yang diizinkan. Jangan membuka
   port database ke internet.
4. **Uji dari jaringan pengguna.** Buka `https://api-domain-milikmu/health/ready`
   dan `https://api-domain-milikmu/api/v1/listing`. Pastikan health ready
   merespons `200` dan listing menampilkan file mirror.
5. **Bagikan URL download dari listing.** Ambil nilai `path` file ISO dari
   respons listing. Contoh jika file berada di `iso-release/ubuntu.iso`:

   ```text
   https://api-domain-milikmu/api/v1/download?path=iso-release%2Fubuntu.iso
   ```

   Lebih baik klien memakai field `download_url` dari listing dan
   menggabungkannya dengan base URL API. Endpoint mengalirkan file langsung,
   mendukung Range/resume, dan mencatat event ke PostgreSQL.

Perlu ruang disk dan bandwidth untuk cara ini: data ditransfer dari Nevacloud
ke disk lokal, kemudian dikirim lagi oleh server API kepada pengguna. Pastikan
kebijakan mirror mengizinkan sinkronisasi dan distribusi ulang, pilih direktori
yang memang boleh diakses publik, serta sediakan kapasitas disk, bandwidth, dan
monitoring yang cukup. Jika tidak ingin menyimpan salinan lokal, implementasi
backend perlu diubah menjadi proxy/redirect ke upstream; runtime saat ini belum
melakukan itu.

---

## 9. Pengujian API dengan Postman

Endpoint API telah diuji menggunakan Postman CLI dengan direktori fixture
sementara. Sembilan skenario yang dicatat lulus, mencakup health check, listing,
validasi pagination dan path traversal, unduh penuh dan sebagian, serta
statistik. Rincian respons dan temuan perbaikan tersedia di
[`docs/LAPORAN-PENGUJIAN-API.md`](docs/LAPORAN-PENGUJIAN-API.md).

Panduan di bawah ini dapat digunakan untuk mengulang pengujian secara manual.
Kontrak OpenAPI juga memuat beberapa endpoint rencana. API katalog repository
publik kini tersedia; endpoint survei publik/admin dan regions sudah tersedia di
runtime. Endpoint admin memakai JWT HS256; lihat bagian [Penerbitan Token JWT Admin](backend/README.md#penerbitan-token-jwt-admin-cli) pada panduan backend untuk perintah penerbitan token, klaim, role, dan rotasi signing key.

### Prasyarat & Cara Menjalankan

Tersedia dua opsi untuk menjalankan aplikasi:

#### Opsi 1: Full-Stack Docker Compose (Port 7000 - Direkomendasikan)
Menjalankan seluruh ekosistem aplikasi sekaligus: Frontend (TanStack/Vite), Backend API (Go Gin), Worker Analitik, Database PostgreSQL, migrasi database & seed katalog otomatis, serta Gateway Caddy:

```powershell
docker compose up -d --build
docker compose ps
```

Setelah container berjalan:
- **Web Frontend:** `http://localhost:7000`
- **Health Check:** `http://localhost:7000/health/ready`
- **API Endpoints:** `http://localhost:7000/api/v1/...` (bebas kendala CORS karena berada di bawah gateway yang sama).

#### Opsi 2: Standalone Development (Port 8080 & 3000)
Jika ingin menjalankan backend atau frontend secara native untuk proses coding cepat:
1. Jalankan PostgreSQL saja dari Docker:
   ```powershell
   docker compose up -d postgres
   ```
2. Jalankan Backend Go di terminal terpisah (bind ke `127.0.0.1:8080`):
   ```powershell
   cd backend
   go run .
   ```
3. Jalankan Frontend di terminal terpisah (bind ke `127.0.0.1:3000`):
   ```powershell
   cd frontend
   npm run dev
   ```

### Siapkan Postman

Buat Environment bernama **Web Pinguin Local** dengan variabel berikut:

| Variable | Initial value | Keterangan |
|---|---|---|
| `baseUrl` | `http://localhost:7000` (atau `http://127.0.0.1:8080` mode standalone) | URL API lokal |
| `listingPath` | `ubuntu-24.04.3-desktop-amd64.iso.torrent` | Path file sample yang disimpan di `mirror-sample/` |

File sample `.torrent` (bukan ISO) dapat dipakai untuk mengetes transfer dengan
ukuran kecil. Untuk memakai sample ini, pastikan `.env` lokal berisi:

```dotenv
LISTING_DIR=E:/Mahasiswa/SEM 3/doscom_project/web-pinguin/mirror-sample
```

File berada di `mirror-sample/ubuntu-24.04.3-desktop-amd64.iso.torrent`. Setelah
mengubah `.env`, mulai ulang API. Catatan: ketika dokumentasi ini disiapkan,
API belum berjalan di port `8080`; request Postman baru akan berhasil setelah
API dijalankan dan `/health/live` merespons `200`.

### Request download sample langsung

Buat request baru dengan pengaturan berikut:

- **Method:** `GET`
- **URL:** `{{baseUrl}}/api/v1/download?path={{listingPath}}`
- **Authorization:** `No Auth`
- **Headers:** tidak perlu header tambahan
- **Send:** klik **Send**, lalu pilih **Save Response to File** bila ingin
  menyimpan hasilnya sebagai file di komputer.

Respons yang diharapkan adalah HTTP `200` dengan body biner sekitar 473 KiB dan
header `Content-Disposition` yang berisi nama file sample. Untuk cek Range,
duplikasi request tersebut lalu tambahkan header `Range: bytes=0-99`; respons
yang diharapkan `206` dengan 100 byte.

URL langsung tanpa variable, untuk disalin ke Postman:

```text
http://127.0.0.1:8080/api/v1/download?path=ubuntu-24.04.3-desktop-amd64.iso.torrent
```

Buat request berikut. Semua request tidak memerlukan autentikasi. Di tab **Tests**
Postman, assertion opsional berikut dapat dipakai untuk memeriksa status:

```javascript
pm.test("status sesuai", function () {
  pm.expect(pm.response.code).to.be.oneOf([200, 206]);
});
```

Sesuaikan assertion download menjadi `pm.response.to.have.status(200)` atau
`206` untuk request Range. Assertion health/statistik/listing dapat memakai
`pm.response.to.have.status(200)`.

### Daftar request

| Nama request | Method dan URL | Pemeriksaan yang diharapkan |
|---|---|---|
| Health live | `GET {{baseUrl}}/health/live` | `200`, JSON `{"status":"ok"}` |
| Health ready | `GET {{baseUrl}}/health/ready` | `200` jika database dan `LISTING_DIR` siap; `503` jika dependency tidak siap |
| Katalog repository | `GET {{baseUrl}}/api/v1/repositories` | `200`, daftar repository publik dan pagination; saat ini kosong sampai metadata katalog diisi |
| Filter katalog | `GET {{baseUrl}}/api/v1/repositories?category=iso&sort=updated&order=desc` | `200`, filter kategori dan urutan; validasi kategori/sort/order yang tidak dikenal memberi `400` |
| Listing root | `GET {{baseUrl}}/api/v1/listing?page=1&page_size=20` | `200`, `data.entries` berisi direktori/file di root listing dan objek `pagination` |
| Listing subfolder | `GET {{baseUrl}}/api/v1/listing?path=contoh&page=1&page_size=20` | `200` dengan isi folder `contoh`; gunakan path folder yang tersedia |
| Listing path invalid | `GET {{baseUrl}}/api/v1/listing?path=..` | `400` dengan `error.code` bernilai `invalid_path` |
| Listing tidak ditemukan | `GET {{baseUrl}}/api/v1/listing?path=folder-tidak-ada` | `404` dengan `error.code` bernilai `not_found` |
| Download file | `GET {{baseUrl}}/api/v1/download?path={{listingPath}}` | `200`, body biner file; `Content-Disposition`/`Content-Length` mengikuti file |
| Download sebagian | URL sama dengan download, header `Range: bytes=0-99` | `206` dan `Content-Range` jika file berukuran cukup; body berisi byte yang diminta |
| Download tanpa path | `GET {{baseUrl}}/api/v1/download` | `400` dengan error JSON |
| Ringkasan statistik | `GET {{baseUrl}}/api/v1/stats/summary?period=24h` | `200`, `data` berisi metrik download dan bandwidth; nilai bisa nol sebelum ada download |
| Filter statistik repository | `GET {{baseUrl}}/api/v1/stats/summary?period=7d&repository={{repositoryId}}` | `200`, hanya event yang path-nya cocok dengan `path_prefix` repository terpilih |
| Filter statistik distro | `GET {{baseUrl}}/api/v1/stats/timeseries?period=30d&distribution=Ubuntu` | `200`, bucket waktu untuk event yang terpetakan ke repository Ubuntu |
| Statistik periode default | `GET {{baseUrl}}/api/v1/stats/summary` | `200`, memakai periode default `30d` |
| Statistik periode invalid | `GET {{baseUrl}}/api/v1/stats/summary?period=all` | `400` dengan `error.code` bernilai `invalid_query` |
| Timeseries | `GET {{baseUrl}}/api/v1/stats/timeseries?period=7d` | `200`, `data` berupa array bucket waktu; dapat kosong jika belum ada data |

### Urutan pengujian yang disarankan

1. Jalankan **Health live** dan **Health ready**. Jika ready tidak `200`, cek
   koneksi DB, migrasi, dan keberadaan `LISTING_DIR`.
2. Jalankan **Listing root**, lalu ambil path file dari hasilnya untuk mengisi
   `listingPath`. Path harus relatif terhadap root listing.
3. Jalankan **Download file**. Untuk memeriksa resume/range, aktifkan header
   `Range: bytes=0-99` lalu jalankan **Download sebagian**.
4. Jalankan summary dan timeseries sebelum dan sesudah download. API menyimpan
   event unduhan ke PostgreSQL; worker terpisah mengagregasikannya untuk statistik.
   Hasil uji fixture tercatat di laporan pengujian tertaut di atas.
5. Jalankan request path/periode invalid untuk memastikan error dikembalikan
   dalam bentuk JSON `error` yang menyertakan `code`, `message`, dan `request_id`.

Download mencatat event statistik di PostgreSQL. Gunakan file fixture atau file
yang aman untuk diunduh; jangan mengarahkan `LISTING_DIR` ke direktori aplikasi
atau direktori yang berisi data privat. Endpoint aktif dan konfigurasi rinci
tersedia di [`backend/README.md`](backend/README.md), sedangkan kontrak OpenAPI
berada di [`backend/api/openapi.yaml`](backend/api/openapi.yaml).
