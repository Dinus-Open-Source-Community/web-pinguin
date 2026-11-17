# 🐧 Pinguin Dinus
### Linux Mirror Portal — Universitas Dian Nuswantoro

> **Your Open Source Distribution Hub — Powered by UDINUS.**

## 1. Overview

Pinguin Dinus merupakan portal Linux Mirror milik Universitas Dian Nuswantoro (UDINUS) yang menyediakan akses terhadap berbagai distribusi Linux, file ISO, package repository, dan perangkat lunak open-source.

Pembangunan ulang Pinguin Dinus bertujuan mengembangkan website katalog mirror menjadi sebuah **Linux Mirror Portal** yang dilengkapi dengan katalog repository, statistik penggunaan, monitoring server, serta visualisasi geografis aktivitas download pengguna.

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

### 2.5 Request Mirror (Optional)

Halaman untuk mengajukan penambahan repository atau distribusi Linux baru.

- Form pengajuan repository.
- Nama distribusi atau repository.
- URL upstream.
- Alasan pengajuan.
- Kontak pengaju.

### 2.6 About & Documentation

Halaman informasi dan dokumentasi layanan Pinguin Dinus.

- Informasi mengenai Pinguin Dinus.
- Informasi infrastruktur server mirror.
- Cara menggunakan layanan mirror.
- Panduan konfigurasi repository Linux.
- Dokumentasi penggunaan mirror.
- Informasi kontak dan kontributor.

---

## 3. Download Analytics System

Sistem analytics memanfaatkan Nginx access log untuk memperoleh data aktivitas download dari server mirror.

### Data Collection Flow

```text
User Downloads File
        |
        v
  Nginx Access Log
        |
        v
   Log Processor
        |
        v
  IP Geolocation
        |
        v
 Analytics Database
        |
        v
    Backend API
        |
        v
  Analytics Dashboard
        |
        v
 Interactive Download Map
```

### Data Collected

| Data | Description |
|---|---|
| IP Address | Alamat IP untuk pemrosesan internal. |
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
| Request Mirror | Pengajuan repository baru. |
| Documentation | Panduan dan dokumentasi penggunaan. |
| About | Informasi layanan, infrastruktur, dan kontributor. |

---

## 6. Product Identity

Pinguin Dinus tidak hanya berfungsi sebagai website untuk mengunduh ISO Linux.

Pinguin Dinus dikembangkan sebagai portal distribusi dan transparansi infrastruktur open-source UDINUS yang menggabungkan:

1. **Repository Distribution** — Distribusi ISO dan package repository.
2. **Download Statistics** — Analisis aktivitas download.
3. **Server Monitoring** — Informasi status infrastruktur mirror.
4. **Geographic Visualization** — Peta persebaran aktivitas pengguna.
5. **Open-source Documentation** — Dokumentasi dan panduan penggunaan.

---

## 7. Development Priorities

| Priority | Feature | Status |
|---|---|---|
| P0 | Home Page | Planned |
| P0 | Repository Explorer | Planned |
| P0 | Direct Download Integration | Planned |
| P1 | Statistics & Analytics | Planned |
| P1 | Download Activity Map | Planned |
| P1 | Historical Heatmap | Planned |
| P1 | Server Monitoring | Planned |
| P2 | Request Mirror | Optional |
| P2 | Documentation | Planned |

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
- Meningkatkan pengalaman pengguna dalam mengakses layanan Pinguin Dinus.

---

**Pinguin Dinus — Your Open Source Distribution Hub.**

Powered by Universitas Dian Nuswantoro (UDINUS).